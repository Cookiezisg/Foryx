// skills.go — HTTP transport for the Skill subsystem (skill.md §11).
// 9 endpoints covering full CRUD + body fetch + drag-import + manual
// rescan + manual invoke. Stdlib mux quirk handling matches the
// sandbox + mcp pattern: standalone :action paths register literally,
// {name}:action paths use the {nameAction} wildcard + splitAction.
//
// 9 endpoints:
//   GET    /api/v1/skills                         list metadata (no body)
//   GET    /api/v1/skills/{name}                  one skill detail
//   GET    /api/v1/skills/{name}/body             raw SKILL.md bytes
//   POST   /api/v1/skills                         create new skill
//   PUT    /api/v1/skills/{name}                  replace skill content
//   DELETE /api/v1/skills/{name}                  delete skill dir → 204
//   POST   /api/v1/skills:import                  drag-import (multipart or JSON)
//   POST   /api/v1/skills:refresh                 manual rescan (debug)
//   POST   /api/v1/skills/{name}:invoke           manual activate via UI / slash
//
// skills.go ——Skill 子系统 HTTP transport（skill.md §11）。9 端点全 CRUD
// + body 取 + 拖入 + 手动重扫 + 手动 invoke。stdlib mux 古怪同 sandbox /
// mcp：standalone :action 字面注册，{name}:action 走 {nameAction} 通配 +
// splitAction。
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// skillImportMaxBytes caps the upload size for :import. 1 MB is generous
// — a single SKILL.md is typically a few KB, and the cap is per-request
// (multiple skills go in separate parts).
//
// skillImportMaxBytes 限定 :import 上传大小。1 MB 富余——单 SKILL.md 几
// KB；上限 per-request（多 skill 走分 part）。
const skillImportMaxBytes int64 = 1 << 20

// SkillsHandler hosts the 9 skill endpoints. log is for handler-side
// instrumentation (rare; most observability comes from the Service +
// Watcher layers).
//
// SkillsHandler 持 9 个 skill 端点。log 给 handler 侧 instrumentation
// （罕见；observability 多在 Service + Watcher）。
type SkillsHandler struct {
	svc *skillapp.Service
	log *zap.Logger
}

// NewSkillsHandler constructs a SkillsHandler.
//
// NewSkillsHandler 构造 SkillsHandler。
func NewSkillsHandler(svc *skillapp.Service, log *zap.Logger) *SkillsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SkillsHandler{svc: svc, log: log.Named("handlers.skills")}
}

// Register attaches the 9 routes to mux.
//
// Register 把 9 路由挂到 mux。
func (h *SkillsHandler) Register(mux *http.ServeMux) {
	// Standalone :action paths (literal registration).
	// 独立 :action 路径（字面注册）。
	mux.HandleFunc("POST /api/v1/skills:import", h.Import)
	mux.HandleFunc("POST /api/v1/skills:refresh", h.Refresh)

	// CRUD root.
	// CRUD 根。
	mux.HandleFunc("GET /api/v1/skills", h.List)
	mux.HandleFunc("POST /api/v1/skills", h.Create)

	// {name} variants.
	// {name} 变体。
	mux.HandleFunc("GET /api/v1/skills/{name}", h.Get)
	mux.HandleFunc("GET /api/v1/skills/{name}/body", h.GetBody)
	mux.HandleFunc("PUT /api/v1/skills/{name}", h.Replace)
	mux.HandleFunc("DELETE /api/v1/skills/{name}", h.Delete)

	// {name}:invoke uses the {nameAction} wildcard (only one action under
	// {name}, but consistent with sandbox/mcp pattern leaves room to
	// add more later).
	// {name}:invoke 用 {nameAction} 通配（{name} 下当前仅 1 action，但
	// 与 sandbox/mcp 一致让后续加更多有空间）。
	mux.HandleFunc("POST /api/v1/skills/{nameAction}", h.NameAction)
}

// ── Read ─────────────────────────────────────────────────────────────

// List returns every loaded skill (frontmatter included; body excluded
// to keep the L1 catalog cost predictable).
//
// List 返每个已加载 skill（含 frontmatter；不含 body 让 L1 catalog 成
// 本可预测）。
func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	skills := h.svc.List(r.Context())
	responsehttpapi.Success(w, http.StatusOK, skills)
}

// Get returns one skill's full metadata.
//
// Get 返单个 skill 的完整元数据。
func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := h.svc.Get(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, sk)
}

// GetBody returns the raw SKILL.md bytes (UI body editor pulls on demand).
//
// GetBody 返 SKILL.md 原始字节（UI body 编辑器按需拉）。
func (h *SkillsHandler) GetBody(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := h.svc.Body(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"body": string(body)})
}

// ── Mutate ───────────────────────────────────────────────────────────

// skillCreateRequest is the shared shape for POST /skills + PUT
// /skills/{name}. body field carries the markdown content; frontmatter
// arrives as a structured object so the client doesn't have to assemble
// the YAML themselves (the UI form has discrete fields).
//
// skillCreateRequest 是 POST /skills + PUT /skills/{name} 共享形状。
// body 字段是 markdown 内容；frontmatter 结构化（UI 表单是离散字段，
// 不需要客户端自己拼 YAML）。
type skillCreateRequest struct {
	Name        string                  `json:"name"`
	Frontmatter skilldomain.Frontmatter `json:"frontmatter"`
	Body        string                  `json:"body"`
}

// Create writes a brand-new SKILL.md. 201 on success; 409 on conflict.
//
// Create 写全新 SKILL.md。成功 201；冲突 409。
func (h *SkillsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req skillCreateRequest
	if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"name is required", nil)
		return
	}
	sk, err := h.svc.Create(r.Context(), req.Name, req.Frontmatter, req.Body)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, sk)
}

// Replace overwrites an existing SKILL.md. 200 on success; 404 if name
// doesn't exist (caller should POST /skills instead).
//
// Replace 覆盖已存在 SKILL.md。成功 200；不存在 404（调用方改 POST）。
func (h *SkillsHandler) Replace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req skillCreateRequest
	if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	sk, err := h.svc.Replace(r.Context(), name, req.Frontmatter, req.Body)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, sk)
}

// Delete removes the skill directory. 204 on success.
//
// Delete 移除 skill 目录。成功 204。
func (h *SkillsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.svc.Delete(r.Context(), name); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ── Standalone :actions ──────────────────────────────────────────────

// Refresh forces a full Scan (debug + UI "refresh" button — the 1s
// polling loop is the typical update path). Returns the resulting list.
//
// Refresh 强制全 Scan（debug + UI "refresh" 按钮——1s 轮询是常规更新
// 路径）。返结果列表。
func (h *SkillsHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Scan(r.Context()); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, h.svc.List(r.Context()))
}

// importRequest is the JSON-body shape (alternative to multipart).
// Each entry carries one skill the user is dropping. Same envelope
// shape as ImportFile so the handler is a thin pass-through.
//
// importRequest 是 JSON body 形状（multipart 之外的另一条路径）。
// 每条一个 skill；与 ImportFile 同形让 handler 是薄通道。
type importRequest struct {
	Files []struct {
		Name string `json:"name"`
		Body string `json:"body"` // raw SKILL.md content
	} `json:"files"`
}

// Import handles drag-import. Multipart payload accepts multiple "file"
// parts, each filename without extension becoming the skill name. JSON
// body alternative is for testend / curl-from-CLI users (each entry
// carries explicit name + raw SKILL.md content).
//
// ?overwrite=true bypasses conflict detection and force-replaces any
// existing skill with the imported one.
//
// V1 limitation: ZIP / tar.gz / folder uploads are NOT supported on
// the server side — the front-end can unpack on the client and POST
// each SKILL.md as a separate "file" part. Server-side archive support
// is V2.
//
// Import 处理拖入。multipart 多 "file" part，每个 filename 去扩展为 skill
// 名。JSON body 替代给 testend / curl-CLI 用户（含 name + 原 SKILL.md）。
// ?overwrite=true 绕过冲突检测强替换。V1 限：服务端不解 ZIP / tar / 文件夹
// ——前端 client-side 拆完逐个 POST。
func (h *SkillsHandler) Import(w http.ResponseWriter, r *http.Request) {
	overwrite := r.URL.Query().Get("overwrite") == "true"
	contentType := r.Header.Get("Content-Type")

	var files []skillapp.ImportFile
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(skillImportMaxBytes); err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"failed to parse multipart form: "+err.Error(), nil)
			return
		}
		// Iterate every "file" part. webkitdirectory / multi-file uploads
		// land here as repeated "file" entries with their original
		// filenames preserved.
		// 走每个 "file" part。webkitdirectory / 多文件上传走重复 "file"
		// 条目，原 filename 保留。
		fhs := r.MultipartForm.File["file"]
		if len(fhs) == 0 {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"no 'file' parts found in multipart payload", nil)
			return
		}
		for _, fh := range fhs {
			name := strings.TrimSuffix(fh.Filename, ".md")
			name = strings.TrimSuffix(name, ".SKILL")
			f, err := fh.Open()
			if err != nil {
				responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("handlers.Import: open part %q: %v", fh.Filename, err), nil)
				return
			}
			// Use io.ReadAll instead of a hand-rolled Read loop so non-EOF
			// io errors (disk fail / connection drop) surface as a 400
			// rather than getting silently swallowed via `if rerr != nil
			// { break }`. The previous loop would treat truncated buffers
			// as legitimate SKILL.md bytes and let the service-layer
			// frontmatter parser report a misleading "invalid frontmatter"
			// error instead of the real I/O cause.
			//
			// 用 io.ReadAll 而非手卷 Read loop——non-EOF io error（disk
			// fail / 连接断）会显式 400 而非被 silent break 吞。原 loop
			// 会把截断 buf 当合法 SKILL.md，service parse fail 报"invalid
			// frontmatter"误导调试。
			buf, rerr := io.ReadAll(f)
			f.Close()
			if rerr != nil {
				responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("handlers.Import: read part %q: %v", fh.Filename, rerr), nil)
				return
			}
			files = append(files, skillapp.ImportFile{Name: name, RawSkillMD: buf})
		}
	} else {
		var req importRequest
		if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		if len(req.Files) == 0 {
			responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
				"no files in payload (expect {files:[{name,body}]})", nil)
			return
		}
		for _, f := range req.Files {
			files = append(files, skillapp.ImportFile{Name: f.Name, RawSkillMD: []byte(f.Body)})
		}
	}

	res, err := h.svc.Import(r.Context(), files, overwrite)
	if err != nil {
		// Per-file errors live in res.Errors; this top-level err is
		// only for batch-level failures (e.g. rescan after write).
		// per-file 错误在 res.Errors；top-level err 仅批次级失败（如写后
		// rescan 失败）。
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

// ── {name}:action dispatch ───────────────────────────────────────────

// skillInvokeRequest is the body for POST /skills/{name}:invoke. Arguments
// are positional; UI sends `["1234", "verbose"]`-style lists.
//
// skillInvokeRequest 是 POST /skills/{name}:invoke 的 body。Arguments 位置
// 参数；UI 发 `["1234", "verbose"]` 列表。
type skillInvokeRequest struct {
	Arguments []string `json:"arguments"`
}

// NameAction dispatches POST /api/v1/skills/{name}:action. Currently
// only :invoke is defined — kept extensible to mirror sandbox/mcp.
//
// NameAction 派发 POST /api/v1/skills/{name}:action。当前仅 :invoke
// ——可扩展同 sandbox/mcp 模式。
func (h *SkillsHandler) NameAction(w http.ResponseWriter, r *http.Request) {
	name, action := splitAction(r.PathValue("nameAction"))
	if name == "" || action == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"path must be {name}:{action}", nil)
		return
	}
	switch action {
	case "invoke":
		var req skillInvokeRequest
		// Empty body OK (skill with no positional args).
		// 空 body OK（无位置参数的 skill）。
		if r.ContentLength > 0 {
			if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
				responsehttpapi.FromDomainError(w, h.log, err)
				return
			}
		}
		out, err := h.svc.Activate(r.Context(), name, req.Arguments)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, map[string]any{"result": out})
	default:
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"unknown action: "+action, nil)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

// decodeJSONLimit reads up to maxBytes from r.Body and decodes into out.
// MaxBytesReader prevents giant uploads from exhausting memory.
// Errors are wrapped via joinInvalidRequest so handler can route them
// through responsehttpapi.FromDomainError → 400 INVALID_REQUEST (same
// pattern as decodeJSON, B2 §S16 fix).
//
// decodeJSONLimit 从 r.Body 读至多 maxBytes 解到 out。MaxBytesReader
// 防巨型上传爆内存。错误经 joinInvalidRequest 包装让 handler 走
// FromDomainError → 400 INVALID_REQUEST（同 decodeJSON pattern）。
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, maxBytes int64, out any) error {
	body := http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(body)
	if err := dec.Decode(out); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return fmt.Errorf("handlers.decodeJSONLimit: request body exceeds %d bytes: %w",
				maxBytes, joinInvalidRequest(err))
		}
		return fmt.Errorf("handlers.decodeJSONLimit: %w", joinInvalidRequest(err))
	}
	return nil
}
