package handlers

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	attachmentapp "github.com/sunweilin/forgify/backend/internal/app/attachment"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// AttachmentHandler serves the 4 /api/v1/attachments/* endpoints: multipart upload, metadata
// fetch, raw-bytes download, and soft-delete. Bytes are stored content-addressed (CAS) and
// reach the LLM later via chat (M5.2) resolving attachment ids into provider content parts.
//
// AttachmentHandler 提供 /api/v1/attachments/* 的 4 端点：multipart 上传、元数据取、原始字节下载、
// 软删。字节内容寻址（CAS）存储，稍后经 chat（M5.2）把 id 解析成 provider content part 进 LLM。
type AttachmentHandler struct {
	svc *attachmentapp.Service
	log *zap.Logger
}

// NewAttachmentHandler constructs the handler.
//
// NewAttachmentHandler 构造 handler。
func NewAttachmentHandler(svc *attachmentapp.Service, log *zap.Logger) *AttachmentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &AttachmentHandler{svc: svc, log: log.Named("handlers.attachment")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *AttachmentHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/attachments", h.Upload)
	mux.HandleFunc("GET /api/v1/attachments/{id}", h.Get)
	mux.HandleFunc("GET /api/v1/attachments/{id}/content", h.Content)
	mux.HandleFunc("DELETE /api/v1/attachments/{id}", h.Delete)
}

// uploadHeadroom is the slack above MaxBytes the request body may use (multipart framing
// overhead); the file itself is re-checked against MaxBytes in the Service.
//
// uploadHeadroom 是请求体在 MaxBytes 之上的余量（multipart 封装开销）；文件本身在 Service 再按
// MaxBytes 复检。
const uploadHeadroom = 1 << 20

// Upload handles POST /api/v1/attachments — a multipart form with a single "file" field.
//
// Upload 处理 POST /api/v1/attachments —— 单 "file" 字段的 multipart 表单。
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(limitspkg.Current().Guards.AttachmentMaxMB)<<20+uploadHeadroom)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		responsehttpapi.Error(w, http.StatusRequestEntityTooLarge, "ATTACHMENT_BAD_UPLOAD",
			"could not read multipart upload (too large or malformed)", nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "ATTACHMENT_BAD_UPLOAD",
			"missing 'file' form field", nil)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		responsehttpapi.Error(w, http.StatusBadRequest, "ATTACHMENT_BAD_UPLOAD",
			"could not read uploaded file", nil)
		return
	}

	// Trust the declared part type; sniff when absent or generic so kind classification works.
	// 信任声明的 part 类型；缺失或泛型时嗅探，使 kind 分类生效。
	mime := header.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}

	a, err := h.svc.Upload(r.Context(), header.Filename, mime, data)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, a)
}

func (h *AttachmentHandler) Get(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, a)
}

// Content streams the raw blob bytes with the stored mime type — for the frontend to preview /
// download the file.
//
// Content 以存储的 mime 类型流出原始 blob 字节——供前端预览/下载。
func (h *AttachmentHandler) Content(w http.ResponseWriter, r *http.Request) {
	a, data, err := h.svc.Download(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	mime := a.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// inline preview; strip quotes from the filename so the header can't be broken.
	// 内联预览；从文件名剥引号，避免破坏 header。
	w.Header().Set("Content-Disposition", `inline; filename="`+strings.ReplaceAll(a.Filename, `"`, "")+`"`)
	_, _ = w.Write(data)
}

func (h *AttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}
