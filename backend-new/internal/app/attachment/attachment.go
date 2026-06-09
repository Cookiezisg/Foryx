// Package attachment owns the Service for uploaded files: hash → content-addressed blob store +
// metadata row, download, soft-delete, orphan-blob GC, and LLM injection (ToContentParts turns
// attachments into provider-agnostic llm.ContentPart for a chat turn). The bytes live in a
// BlobStore (a port, implemented by infra/fs/blob); the metadata lives in attachmentdomain.
// Repository. Workspace isolation is automatic at both layers (orm + blob both key off ctx).
//
// Package attachment 持有上传文件的 Service：哈希 → 内容寻址 blob 存储 + 元数据行、下载、软删、
// 孤儿 blob GC，以及 LLM 注入（ToContentParts 把附件变成与 provider 无关的 llm.ContentPart 供聊天
// 回合）。字节在 BlobStore（端口，infra/fs/blob 实现）；元数据在 attachmentdomain.Repository。
// workspace 隔离两层都自动（orm + blob 都据 ctx）。
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"go.uber.org/zap"

	attachmentdomain "github.com/sunweilin/forgify/backend/internal/domain/attachment"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// BlobStore is the content-addressed byte store (port; infra/fs/blob implements it). Put is a
// no-op when the sha already exists (dedup); Sweep is orphan GC against a keep-set.
//
// BlobStore 是内容寻址字节存储（端口；infra/fs/blob 实现）。sha 已存在时 Put 为 no-op（dedup）；
// Sweep 按保留集做孤儿 GC。
type BlobStore interface {
	Put(ctx context.Context, sha string, data []byte) error
	Get(ctx context.Context, sha string) ([]byte, error)
	Exists(ctx context.Context, sha string) (bool, error)
	Sweep(ctx context.Context, keep map[string]bool) (int, error)
}

// Service is the attachment application façade.
//
// Service 是附件应用 façade。
type Service struct {
	repo  attachmentdomain.Repository
	blobs BlobStore
	log   *zap.Logger
}

// New constructs a Service; panics on nil logger, repo, or blobs (all required).
//
// New 构造 Service；nil logger/repo/blobs panic（皆必需）。
func New(repo attachmentdomain.Repository, blobs BlobStore, log *zap.Logger) *Service {
	if log == nil {
		panic("attachmentapp.New: nil logger")
	}
	if repo == nil || blobs == nil {
		panic("attachmentapp.New: repo and blobs are required")
	}
	return &Service{repo: repo, blobs: blobs, log: log}
}

// Upload validates size, hashes the bytes, stores the blob (dedup), and inserts the metadata row.
// The blob is written before the row so a row never points at a missing blob.
//
// Upload 校验大小、哈希字节、存 blob（dedup）、插元数据行。blob 先于行写入，故行绝不指向缺失 blob。
func (s *Service) Upload(ctx context.Context, filename, mime string, data []byte) (*attachmentdomain.Attachment, error) {
	if len(data) == 0 {
		return nil, attachmentdomain.ErrEmpty
	}
	if int64(len(data)) > attachmentdomain.MaxBytes {
		return nil, attachmentdomain.ErrTooLarge
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if err := s.blobs.Put(ctx, sha, data); err != nil {
		return nil, fmt.Errorf("attachmentapp.Upload: blob: %w", err)
	}
	a := &attachmentdomain.Attachment{
		ID:        idgenpkg.New("att"),
		SHA256:    sha,
		Filename:  filepath.Base(filename), // display only; blob is keyed by sha, not name
		MimeType:  mime,
		SizeBytes: int64(len(data)),
		Kind:      attachmentdomain.KindFromMIME(mime, filename),
	}
	if err := s.repo.Insert(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Get fetches one attachment's metadata.
//
// Get 取一个附件的元数据。
func (s *Service) Get(ctx context.Context, id string) (*attachmentdomain.Attachment, error) {
	return s.repo.Get(ctx, id)
}

// Download returns an attachment's metadata + its blob bytes.
//
// Download 返回附件元数据 + 其 blob 字节。
func (s *Service) Download(ctx context.Context, id string) (*attachmentdomain.Attachment, []byte, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	data, err := s.blobs.Get(ctx, a.SHA256)
	if err != nil {
		return nil, nil, fmt.Errorf("attachmentapp.Download: %w", err)
	}
	return a, data, nil
}

// Delete soft-deletes the metadata row; the blob is reclaimed later by GC if no live row
// references its sha (another attachment may share it).
//
// Delete 软删元数据行；若无活跃行引用其 sha（另一附件可能共享），blob 稍后由 GC 回收。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.SoftDelete(ctx, id)
}

// GC sweeps orphan blobs in the ctx workspace: blobs whose sha is referenced by no live row.
//
// GC 清 ctx workspace 的孤儿 blob：sha 无活跃行引用的 blob。
func (s *Service) GC(ctx context.Context) (int, error) {
	shas, err := s.repo.ListLiveSHAs(ctx)
	if err != nil {
		return 0, err
	}
	keep := make(map[string]bool, len(shas))
	for _, sha := range shas {
		keep[sha] = true
	}
	return s.blobs.Sweep(ctx, keep)
}

// ToContentParts resolves attachment ids into provider-agnostic LLM content parts for one user
// turn (chat M5.2 prepends the user's own text part, then sends the message; each provider renders
// the parts into its own wire). The mapping by Kind:
//   - image    → an image_url part (data-URL) when visionCapable; otherwise a text note (the model
//     can't see images, so degrade rather than send a part it would reject).
//   - text     → the file's content inlined as a text part (cheap, universal — every model reads it).
//   - document → a file part (MediaType + base64 + filename); providers with native PDF input render
//     it, the rest skip it and rely on sandbox text-extraction (R0053).
//   - audio/video/other → a text placeholder; real extraction (STT / OCR) is R0053.
//
// Order follows ids. A missing/unreadable blob is skipped with a warning — a stale id must never
// fail the turn (best-effort, like a dangling mention). visionCapable is supplied by the caller
// from the resolved model's capability, keeping this layer free of model-catalog knowledge.
//
// ToContentParts 把附件 id 解析成与 provider 无关的 LLM 内容块，供一个 user 回合（chat M5.2 在前面
// 拼上用户自己的 text part 再发；各家 provider 把这些块渲成自家 wire）。按 Kind 映射：image→image_url
// （data-URL，仅 visionCapable 时；否则降级为文字提示）；text→文件内容内联成 text part（廉价通用）；
// document→file part（MediaType+base64+文件名，原生支持 PDF 的家渲、其余跳过靠 R0053 沙箱抽取）；
// audio/video/other→文字占位（真抽取 STT/OCR 是 R0053）。顺序随 ids；缺失/不可读 blob 告警跳过——
// 陈旧 id 绝不能让回合失败（尽力而为，如悬空 mention）。visionCapable 由调用方按解析后的模型能力传入，
// 使本层不持模型目录知识。
func (s *Service) ToContentParts(ctx context.Context, ids []string, visionCapable bool) ([]llminfra.ContentPart, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	metas, err := s.repo.GetBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	// GetBatch (WHERE id IN) does not preserve order; index by id and walk ids so parts keep the
	// caller's order (part order is semantically meaningful to the model).
	//
	// GetBatch（WHERE id IN）不保序；按 id 建索引、按 ids 遍历，使 parts 保持调用方顺序（part 顺序
	// 对模型有语义）。
	byID := make(map[string]*attachmentdomain.Attachment, len(metas))
	for _, a := range metas {
		byID[a.ID] = a
	}
	out := make([]llminfra.ContentPart, 0, len(ids))
	for _, id := range ids {
		a := byID[id]
		if a == nil {
			s.log.Warn("attachmentapp.ToContentParts: attachment not found, skipping", zap.String("attachment_id", id))
			continue
		}
		data, err := s.blobs.Get(ctx, a.SHA256)
		if err != nil {
			s.log.Warn("attachmentapp.ToContentParts: blob unreadable, skipping",
				zap.String("attachment_id", a.ID), zap.String("sha256", a.SHA256), zap.Error(err))
			continue
		}
		switch a.Kind {
		case attachmentdomain.KindImage:
			if visionCapable {
				out = append(out, llminfra.ContentPart{Type: llminfra.PartImageURL, ImageURL: dataURL(a.MimeType, data)})
			} else {
				out = append(out, textNote("image %q attached, but the current model has no vision", a.Filename))
			}
		case attachmentdomain.KindText:
			out = append(out, llminfra.ContentPart{Type: llminfra.PartText, Text: inlineText(a.Filename, data)})
		case attachmentdomain.KindDocument:
			out = append(out, llminfra.ContentPart{
				Type:      llminfra.PartFile,
				MediaType: a.MimeType,
				Data:      base64.StdEncoding.EncodeToString(data),
				Filename:  a.Filename,
			})
		default: // audio / video / other — extraction (STT / OCR / parse) is R0053
			out = append(out, textNote("file %q (%s) attached; content extraction is not yet available", a.Filename, a.Kind))
		}
	}
	return out, nil
}

// dataURL builds a base64 data-URL ("data:<mime>;base64,<data>") for an inline image.
//
// dataURL 为内联图构造 base64 data-URL。
func dataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// inlineText wraps a text file's content as a labelled text part so the model knows the filename.
//
// inlineText 把文本文件内容包成带文件名标注的 text part，让模型知道文件名。
func inlineText(filename string, data []byte) string {
	if filename != "" {
		return fmt.Sprintf("Attached file %q:\n%s", filename, data)
	}
	return string(data)
}

// textNote renders a degraded-attachment placeholder as a text part.
//
// textNote 把降级附件占位渲成 text part。
func textNote(format string, args ...any) llminfra.ContentPart {
	return llminfra.ContentPart{Type: llminfra.PartText, Text: "[" + fmt.Sprintf(format, args...) + "]"}
}
