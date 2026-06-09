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
	repo      attachmentdomain.Repository
	blobs     BlobStore
	extractor Extractor // optional (nil → documents degrade to a placeholder for non-native models)
	log       *zap.Logger
}

// New constructs a Service; panics on nil logger, repo, or blobs (all required). extractor is
// optional — nil means a document sent to a model without native document input degrades to a
// placeholder instead of being text-extracted.
//
// New 构造 Service；nil logger/repo/blobs panic（皆必需）。extractor 可选——nil 时，发给无原生文档
// 输入模型的文档降级为占位，而非抽文本。
func New(repo attachmentdomain.Repository, blobs BlobStore, extractor Extractor, log *zap.Logger) *Service {
	if log == nil {
		panic("attachmentapp.New: nil logger")
	}
	if repo == nil || blobs == nil {
		panic("attachmentapp.New: repo and blobs are required")
	}
	return &Service{repo: repo, blobs: blobs, extractor: extractor, log: log}
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

// Capabilities tells ToContentParts what the resolved target model can natively accept, so it can
// decide whether to hand a file over raw or degrade it. Both flags come from the caller (chat M5.2)
// via the model catalog — this layer holds no model knowledge.
//
// Capabilities 告诉 ToContentParts 解析后的目标模型能原生接受什么，据此决定原样递交还是降级。两个
// flag 都由调用方（chat M5.2）按模型目录传入——本层不持模型知识。
type Capabilities struct {
	Vision     bool // model can see images natively
	NativeDocs bool // model can read an inline document (PDF) natively (anthropic / openai / gemini)
}

// ToContentParts resolves attachment ids into provider-agnostic LLM content parts for one user turn
// (chat M5.2 prepends the user's own text part, then sends; each provider renders the parts into its
// own wire). Mapping by Kind:
//   - image    → image_url (data-URL) when caps.Vision; else a text note (degrade — don't send a
//     part the model would reject).
//   - text     → the file's content inlined as a text part (cheap, universal).
//   - document → caps.NativeDocs ? a file part (PDF handed over raw, read natively) : sandbox
//     text-extracted, token-capped text — with a placeholder note if no extractor / extraction fails.
//   - audio/video/other → a text placeholder (those extractors are future Extractor plug-ins).
//
// Order follows ids. A missing/unreadable blob is skipped with a warning — a stale id must never
// fail the turn (best-effort, like a dangling mention).
//
// ToContentParts 把附件 id 解析成与 provider 无关的 LLM 内容块，供一个 user 回合（chat M5.2 前面拼上
// 用户文本 part 再发；各家渲成自家 wire）。按 Kind 映射：image→image_url（data-URL，仅 caps.Vision；
// 否则文字提示降级）；text→文件内容内联 text part；document→caps.NativeDocs ? file part（PDF 原样递交、
// 原生读）: sandbox 抽取文本（token 截断），无 extractor / 抽取失败则占位；audio/video/other→文字占位
// （那些 extractor 是未来插件）。顺序随 ids；缺失/不可读 blob 告警跳过——陈旧 id 绝不让回合失败。
func (s *Service) ToContentParts(ctx context.Context, ids []string, caps Capabilities) ([]llminfra.ContentPart, error) {
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
			if caps.Vision {
				out = append(out, llminfra.ContentPart{Type: llminfra.PartImageURL, ImageURL: dataURL(a.MimeType, data)})
			} else {
				out = append(out, textNote("image %q attached, but the current model has no vision", a.Filename))
			}
		case attachmentdomain.KindText:
			out = append(out, llminfra.ContentPart{Type: llminfra.PartText, Text: inlineText(a.Filename, data)})
		case attachmentdomain.KindDocument:
			if caps.NativeDocs {
				out = append(out, llminfra.ContentPart{
					Type:      llminfra.PartFile,
					MediaType: a.MimeType,
					Data:      base64.StdEncoding.EncodeToString(data),
					Filename:  a.Filename,
				})
			} else {
				out = append(out, s.extractDocPart(ctx, a, data))
			}
		default: // audio / video / other — those extractors are future Extractor plug-ins
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

// extractDocPart text-extracts a document for a model that can't read it natively, capping the
// result to maxExtractedChars. With no extractor configured, or on an unsupported mime / extraction
// failure, it degrades to a placeholder note — never failing the turn.
//
// extractDocPart 为不能原生读文档的模型抽取文本，截断到 maxExtractedChars。无 extractor、或 mime
// 不支持 / 抽取失败时，降级为占位——绝不让回合失败。
func (s *Service) extractDocPart(ctx context.Context, a *attachmentdomain.Attachment, data []byte) llminfra.ContentPart {
	if s.extractor == nil {
		return textNote("document %q attached, but text extraction is unavailable for this model", a.Filename)
	}
	text, err := s.extractor.Extract(ctx, a.MimeType, data)
	if err != nil {
		s.log.Warn("attachmentapp.ToContentParts: document extraction failed, degrading",
			zap.String("attachment_id", a.ID), zap.String("mime", a.MimeType), zap.Error(err))
		return textNote("document %q attached, but its text could not be extracted", a.Filename)
	}
	body, truncated := truncateForLLM(text)
	suffix := ""
	if truncated {
		suffix = ", truncated"
	}
	return llminfra.ContentPart{
		Type: llminfra.PartText,
		Text: fmt.Sprintf("Attached document %q (text-extracted%s):\n%s", a.Filename, suffix, body),
	}
}

// maxExtractedChars caps inlined extracted text (~100K tokens at ~4 chars/token, aligning with
// LibreChat's default fileTokenLimit). The head is kept — a document leads with its substance.
//
// maxExtractedChars 截断内联抽取文本（~4 字符/token 下约 100K token，对齐 LibreChat 默认
// fileTokenLimit）。保头部——文档开头即正文。
const maxExtractedChars = 400_000

// truncateForLLM caps s to maxExtractedChars runes, returning the (possibly trimmed) text and
// whether it was trimmed. A byte-length check short-circuits the common small-file case.
//
// truncateForLLM 把 s 截到 maxExtractedChars 个 rune，返回（可能裁过的）文本 + 是否裁过。字节长度
// 预检短路常见小文件。
func truncateForLLM(s string) (string, bool) {
	if len(s) <= maxExtractedChars { // bytes ≥ runes, so within cap by bytes ⇒ within cap by runes
		return s, false
	}
	r := []rune(s)
	if len(r) <= maxExtractedChars {
		return s, false
	}
	return string(r[:maxExtractedChars]), true
}
