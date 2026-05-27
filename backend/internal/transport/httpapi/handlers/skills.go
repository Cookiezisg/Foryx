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

const skillImportMaxBytes int64 = 1 << 20

// SkillsHandler hosts the 9 skill endpoints.
//
// SkillsHandler 持 9 个 skill 端点。
type SkillsHandler struct {
	svc *skillapp.Service
	log *zap.Logger
}

func NewSkillsHandler(svc *skillapp.Service, log *zap.Logger) *SkillsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SkillsHandler{svc: svc, log: log.Named("handlers.skills")}
}

func (h *SkillsHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/skills:import", h.Import)
	mux.HandleFunc("POST /api/v1/skills:refresh", h.Refresh)
	mux.HandleFunc("GET /api/v1/skills", h.List)
	mux.HandleFunc("POST /api/v1/skills", h.Create)
	mux.HandleFunc("GET /api/v1/skills/{name}", h.Get)
	mux.HandleFunc("GET /api/v1/skills/{name}/body", h.GetBody)
	mux.HandleFunc("PUT /api/v1/skills/{name}", h.Replace)
	mux.HandleFunc("DELETE /api/v1/skills/{name}", h.Delete)
	mux.HandleFunc("POST /api/v1/skills/{nameAction}", h.NameAction)
}

// List returns every loaded skill's frontmatter; body excluded.
//
// List 返每个已加载 skill 的 frontmatter,不含 body。
func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	skills := h.svc.List(r.Context())
	responsehttpapi.Success(w, http.StatusOK, skills)
}

func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := h.svc.Get(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, sk)
}

// GetBody returns the raw SKILL.md bytes.
//
// GetBody 返 SKILL.md 原始字节。
func (h *SkillsHandler) GetBody(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := h.svc.Body(r.Context(), name)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{"body": string(body)})
}

type skillCreateRequest struct {
	Name        string                  `json:"name"`
	Frontmatter skilldomain.Frontmatter `json:"frontmatter"`
	Body        string                  `json:"body"`

	// FlatDescription is the convenience field for clients that send a flat
	// `{name, description, body}` shape (mirroring testend's UI fields). When
	// present and Frontmatter.Description is empty, it auto-populates the
	// frontmatter's description + name.
	//
	// FlatDescription 是兼容扁平 `{name, description, body}` 请求形的便利字段
	// （对应 testend UI 的三字段）；当它非空且 Frontmatter.Description 空时,
	// 自动补 frontmatter 的 description + name。
	FlatDescription string `json:"description,omitempty"`
}

// normalizeFrontmatter back-fills Frontmatter.Name + Description from top-level
// fields when the caller used the flat `{name, description, body}` shape.
//
// normalizeFrontmatter 调用方用扁平形时把顶层字段回填到 Frontmatter。
func (req *skillCreateRequest) normalizeFrontmatter() {
	if strings.TrimSpace(req.Frontmatter.Name) == "" {
		req.Frontmatter.Name = req.Name
	}
	if strings.TrimSpace(req.Frontmatter.Description) == "" {
		req.Frontmatter.Description = req.FlatDescription
	}
}

func (h *SkillsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req skillCreateRequest
	if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	req.normalizeFrontmatter()
	sk, err := h.svc.Create(r.Context(), req.Name, req.Frontmatter, req.Body)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, sk)
}

func (h *SkillsHandler) Replace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req skillCreateRequest
	if err := decodeJSONLimit(w, r, skillImportMaxBytes, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	req.normalizeFrontmatter()
	sk, err := h.svc.Replace(r.Context(), name, req.Frontmatter, req.Body)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, sk)
}

func (h *SkillsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.svc.Delete(r.Context(), name); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// Refresh forces a full rescan.
//
// Refresh 强制全量重扫。
func (h *SkillsHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Scan(r.Context()); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, h.svc.List(r.Context()))
}

type importRequest struct {
	Files []struct {
		Name string `json:"name"`
		Body string `json:"body"`
	} `json:"files"`
}

// Import accepts multipart "file" parts or JSON {files:[{name,body}]}.
//
// Import 接 multipart "file" parts 或 JSON {files:[{name,body}]}。
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
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, res)
}

type skillInvokeRequest struct {
	Arguments []string `json:"arguments"`
}

// NameAction dispatches POST /api/v1/skills/{name}:action; only :invoke today.
//
// NameAction 派发 POST /api/v1/skills/{name}:action;当前仅 :invoke。
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

// decodeJSONLimit reads up to maxBytes via MaxBytesReader and wraps errors.
//
// decodeJSONLimit 读至多 maxBytes 并把错误包成 INVALID_REQUEST。
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
