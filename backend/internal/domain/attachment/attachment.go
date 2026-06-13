// Package attachment is the domain layer for user-uploaded files attached to chat turns: a
// content-addressed blob (the bytes, stored on disk by SHA-256) plus a metadata row (att_).
// The bytes NEVER enter SQLite — the blob lives under the workspace, the row carries only
// identity (sha / filename / mime / size / kind). Multiple rows may reference one blob
// (content-addressed dedup). Kind classifies how the file reaches the LLM (chat M5.2):
// image → vision block, document/text → inline-or-extract, audio/video → extraction (deferred,
// pluggable). Pure structs + the storage contract; upload / download / GC orchestration is in app.
//
// Package attachment 是聊天回合上传文件的 domain 层：一个内容寻址的 blob（字节按 SHA-256 存盘）
// + 一条元数据行（att_）。字节**绝不进 SQLite**——blob 在 workspace 下，行只承载身份
// （sha / 文件名 / mime / 大小 / 类别）。多行可指同一 blob（内容寻址 dedup）。Kind 决定文件如何
// 进 LLM（chat M5.2）：image→vision 块、document/text→内联或抽取、audio/video→抽取（延后、可插）。
// 纯 struct + 存储契约；上传/下载/GC 编排在 app。
package attachment

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
)

// Attachment is one uploaded file's metadata row. SHA256 is the content-addressed key into
// the blob store (identical uploads dedup to one blob, many rows). A business/Log table with
// soft-delete (D1): a deleted row is a tombstone; its blob is reclaimed by GC once no live row
// references the sha.
//
// Attachment 是一个上传文件的元数据行。SHA256 是 blob 存储的内容寻址键（相同上传 dedup 成一个
// blob、多行）。业务表软删（D1）：删行留墓碑；当无活跃行引用该 sha 时 blob 由 GC 回收。
type Attachment struct {
	ID          string     `db:"id,pk"              json:"id"` // att_<16hex>
	WorkspaceID string     `db:"workspace_id,ws"    json:"-"`
	SHA256      string     `db:"sha256"             json:"sha256"`
	Filename    string     `db:"filename"           json:"filename"`
	MimeType    string     `db:"mime_type"          json:"mimeType"`
	SizeBytes   int64      `db:"size_bytes"         json:"sizeBytes"`
	Kind        string     `db:"kind"               json:"kind"`
	CreatedAt   time.Time  `db:"created_at,created" json:"createdAt"`
	DeletedAt   *time.Time `db:"deleted_at,deleted" json:"-"`
}

// Kind buckets an upload by how it reaches the LLM. image → vision; document / text → text
// (inline or extracted); audio / video → extraction (pluggable, deferred); other → opaque.
//
// Kind 按文件如何进 LLM 分桶。image→vision；document/text→文本（内联或抽取）；audio/video→抽取
// （可插、延后）；other→不透明。
const (
	KindImage    = "image"
	KindDocument = "document"
	KindText     = "text"
	KindAudio    = "audio"
	KindVideo    = "video"
	KindOther    = "other"
)

var (
	ErrNotFound = errorspkg.New(errorspkg.KindNotFound, "ATTACHMENT_NOT_FOUND", "attachment not found")
	ErrTooLarge = errorspkg.New(errorspkg.KindTooLarge, "ATTACHMENT_TOO_LARGE", "file exceeds the 50 MB limit")
	ErrEmpty    = errorspkg.New(errorspkg.KindInvalid, "ATTACHMENT_EMPTY", "empty file")
)

// KindFromMIME classifies an upload by mime type (a "; charset=…" suffix is stripped), with a
// filename-extension fallback for the generic application/octet-stream case.
//
// KindFromMIME 按 mime 类型分类（剥 "; charset=…" 后缀），对 application/octet-stream 等泛型用
// 文件扩展名兜底。
func KindFromMIME(mime, filename string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch {
	case strings.HasPrefix(m, "image/"):
		return KindImage
	case strings.HasPrefix(m, "audio/"):
		return KindAudio
	case strings.HasPrefix(m, "video/"):
		return KindVideo
	case m == "application/pdf", isOfficeMIME(m):
		return KindDocument
	case strings.HasPrefix(m, "text/"), isTextualMIME(m):
		return KindText
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf", ".docx", ".xlsx", ".pptx", ".odt", ".epub":
		return KindDocument
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".heic", ".heif":
		return KindImage
	case ".mp3", ".wav", ".m4a", ".flac", ".ogg":
		return KindAudio
	case ".mp4", ".mov", ".avi", ".webm", ".mkv":
		return KindVideo
	case ".txt", ".md", ".markdown", ".json", ".csv", ".tsv", ".xml", ".yaml", ".yml", ".html", ".htm",
		".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs", ".rb", ".php", ".sh", ".sql":
		return KindText
	}
	return KindOther
}

func isOfficeMIME(m string) bool {
	switch m {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document", // docx
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",         // xlsx
		"application/vnd.openxmlformats-officedocument.presentationml.presentation", // pptx
		"application/vnd.oasis.opendocument.text",                                   // odt
		"application/epub+zip": // epub
		return true
	}
	return false
}

func isTextualMIME(m string) bool {
	switch m {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml",
		"application/javascript", "application/csv", "application/x-sh", "application/sql":
		return true
	}
	return false
}

// Repository is the metadata storage contract; workspace isolation + soft-delete are applied by
// the orm layer from ctx. The blob bytes live in a separate content-addressed store (app port).
//
// Repository 是元数据存储契约；workspace 隔离 + 软删由 orm 层据 ctx 施加。blob 字节在另一个内容
// 寻址存储（app 端口）。
type Repository interface {
	Insert(ctx context.Context, a *Attachment) error
	Get(ctx context.Context, id string) (*Attachment, error)
	GetBatch(ctx context.Context, ids []string) ([]*Attachment, error)
	SoftDelete(ctx context.Context, id string) error

	// ListLiveSHAs returns the distinct sha256 of every live (non-deleted) attachment in the
	// ctx workspace — the keep-set for blob GC.
	//
	// ListLiveSHAs 返 ctx workspace 内每个活跃（未删）附件的去重 sha256——blob GC 的保留集。
	ListLiveSHAs(ctx context.Context) ([]string, error)
}
