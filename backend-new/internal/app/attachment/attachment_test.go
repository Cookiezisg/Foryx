package attachment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	attachmentdomain "github.com/sunweilin/forgify/backend/internal/domain/attachment"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	blobfs "github.com/sunweilin/forgify/backend/internal/infra/fs/blob"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	attachmentstore "github.com/sunweilin/forgify/backend/internal/infra/store/attachment"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// newSvc wires the Service over a real in-memory metadata store + a real temp-dir CAS blob
// store, exercising the full upload→hash→store→download pipeline offline.
//
// newSvc 把 Service 接在真 in-memory 元数据 store + 真 temp 目录 CAS blob 上，离线走完整
// 上传→哈希→存储→下载链。
func newSvc(t *testing.T) (*Service, *blobfs.Store, context.Context) {
	return newSvcWith(t, nil)
}

func newSvcWith(t *testing.T, ext Extractor) (*Service, *blobfs.Store, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range attachmentstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	blobs := blobfs.New(t.TempDir())
	svc := New(attachmentstore.New(ormpkg.Open(sqlDB)), blobs, ext, zap.NewNop())
	return svc, blobs, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestUpload_RoundTrip_AndKind(t *testing.T) {
	svc, _, ctx := newSvc(t)
	data := []byte("\x89PNG fake image bytes")
	a, err := svc.Upload(ctx, "photo.png", "image/png", data)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if a.Kind != attachmentdomain.KindImage {
		t.Errorf("kind = %q, want image", a.Kind)
	}
	if a.SHA256 != sha(data) || a.SizeBytes != int64(len(data)) {
		t.Errorf("meta: sha=%s size=%d", a.SHA256, a.SizeBytes)
	}
	if len(a.ID) < 4 || a.ID[:4] != "att_" {
		t.Errorf("id prefix: %s", a.ID)
	}
	gotA, gotData, err := svc.Download(ctx, a.ID)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if gotA.ID != a.ID || !bytes.Equal(gotData, data) {
		t.Errorf("download mismatch")
	}
}

func TestUpload_KindClassification(t *testing.T) {
	svc, _, ctx := newSvc(t)
	cases := []struct{ name, mime, want string }{
		{"a.pdf", "application/pdf", attachmentdomain.KindDocument},
		{"a.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", attachmentdomain.KindDocument},
		{"a.txt", "text/plain", attachmentdomain.KindText},
		{"a.json", "application/json", attachmentdomain.KindText},
		{"a.mp3", "audio/mpeg", attachmentdomain.KindAudio},
		{"weird.bin", "application/octet-stream", attachmentdomain.KindOther},
		{"code.go", "application/octet-stream", attachmentdomain.KindText}, // ext fallback
	}
	for _, c := range cases {
		a, err := svc.Upload(ctx, c.name, c.mime, []byte("data-"+c.name))
		if err != nil {
			t.Fatalf("upload %s: %v", c.name, err)
		}
		if a.Kind != c.want {
			t.Errorf("%s (%s): kind = %q, want %q", c.name, c.mime, a.Kind, c.want)
		}
	}
}

func TestUpload_Empty(t *testing.T) {
	svc, _, ctx := newSvc(t)
	if _, err := svc.Upload(ctx, "e.txt", "text/plain", nil); !errors.Is(err, attachmentdomain.ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestUpload_TooLarge(t *testing.T) {
	svc, _, ctx := newSvc(t)
	// Size is checked before hashing, so the oversized buffer is never read.
	big := make([]byte, attachmentdomain.MaxBytes+1)
	if _, err := svc.Upload(ctx, "big.bin", "application/octet-stream", big); !errors.Is(err, attachmentdomain.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestUpload_DedupSameBytes(t *testing.T) {
	svc, blobs, ctx := newSvc(t)
	data := []byte("identical content")
	a1, _ := svc.Upload(ctx, "first.txt", "text/plain", data)
	a2, _ := svc.Upload(ctx, "second.txt", "text/plain", data)
	if a1.ID == a2.ID {
		t.Error("two uploads should yield distinct attachment ids")
	}
	if a1.SHA256 != a2.SHA256 {
		t.Error("identical bytes should share one sha (dedup)")
	}
	if ok, _ := blobs.Exists(ctx, a1.SHA256); !ok {
		t.Error("blob missing")
	}
}

func TestDelete_KeepsBlobUntilGC(t *testing.T) {
	svc, blobs, ctx := newSvc(t)
	a, _ := svc.Upload(ctx, "x.txt", "text/plain", []byte("bye"))
	if err := svc.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, a.ID); !errors.Is(err, attachmentdomain.ErrNotFound) {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
	if ok, _ := blobs.Exists(ctx, a.SHA256); !ok {
		t.Error("blob removed before GC")
	}
}

func TestGC_RefcountBySHA(t *testing.T) {
	svc, blobs, ctx := newSvc(t)
	shared := []byte("shared bytes")
	a1, _ := svc.Upload(ctx, "one.txt", "text/plain", shared) // both reference one blob (dedup)
	a2, _ := svc.Upload(ctx, "two.txt", "text/plain", shared)
	lone, _ := svc.Upload(ctx, "lone.bin", "application/octet-stream", []byte("unique"))

	// Delete one of the two shared-blob rows + the lone row.
	_ = svc.Delete(ctx, a1.ID)
	_ = svc.Delete(ctx, lone.ID)

	removed, err := svc.GC(ctx)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if removed != 1 { // only the lone blob is orphaned; the shared blob is still referenced by a2
		t.Errorf("removed = %d, want 1", removed)
	}
	if ok, _ := blobs.Exists(ctx, a2.SHA256); !ok {
		t.Error("shared blob GC'd while still referenced by a live row")
	}
	if ok, _ := blobs.Exists(ctx, lone.SHA256); ok {
		t.Error("orphan blob survived GC")
	}
}

func TestToContentParts_ByKind(t *testing.T) {
	svc, _, ctx := newSvc(t)
	imgBytes := []byte("\x89PNG pixels")
	pdfBytes := []byte("%PDF-1.7 body")
	img, _ := svc.Upload(ctx, "pic.png", "image/png", imgBytes)
	txt, _ := svc.Upload(ctx, "notes.txt", "text/plain", []byte("hello world"))
	pdf, _ := svc.Upload(ctx, "doc.pdf", "application/pdf", pdfBytes)

	// Vision + native-docs: image → image_url (data-URL), text → inlined text, PDF → file part
	// (handed over raw). Order follows the id slice, not the DB's IN-clause order.
	parts, err := svc.ToContentParts(ctx, []string{img.ID, txt.ID, pdf.ID}, Capabilities{Vision: true, NativeDocs: true})
	if err != nil {
		t.Fatalf("ToContentParts: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want 3", len(parts))
	}
	if parts[0].Type != llminfra.PartImageURL || !strings.HasPrefix(parts[0].ImageURL, "data:image/png;base64,") {
		t.Errorf("part[0] = %+v, want image_url data-URL", parts[0])
	}
	if parts[1].Type != llminfra.PartText || !strings.Contains(parts[1].Text, "hello world") || !strings.Contains(parts[1].Text, "notes.txt") {
		t.Errorf("part[1] = %+v, want inlined text with filename", parts[1])
	}
	if parts[2].Type != llminfra.PartFile || parts[2].MediaType != "application/pdf" ||
		parts[2].Filename != "doc.pdf" || parts[2].Data != base64.StdEncoding.EncodeToString(pdfBytes) {
		t.Errorf("part[2] = %+v, want file part with base64 PDF", parts[2])
	}
}

func TestToContentParts_NonVisionDegradesImage(t *testing.T) {
	svc, _, ctx := newSvc(t)
	img, _ := svc.Upload(ctx, "pic.png", "image/png", []byte("\x89PNG"))

	parts, err := svc.ToContentParts(ctx, []string{img.ID}, Capabilities{})
	if err != nil {
		t.Fatalf("ToContentParts: %v", err)
	}
	if len(parts) != 1 || parts[0].Type != llminfra.PartText {
		t.Fatalf("parts = %+v, want one text note", parts)
	}
	if !strings.Contains(parts[0].Text, "pic.png") || !strings.Contains(parts[0].Text, "vision") {
		t.Errorf("degraded note = %q, want mention of file + vision", parts[0].Text)
	}
}

func TestToContentParts_SkipsMissingPreservingOrder(t *testing.T) {
	svc, _, ctx := newSvc(t)
	txt, _ := svc.Upload(ctx, "a.txt", "text/plain", []byte("A"))

	// A stale id between two real ones is skipped; the rest keep their order and the turn survives.
	parts, err := svc.ToContentParts(ctx, []string{"att_deadbeefdeadbeef", txt.ID}, Capabilities{Vision: true})
	if err != nil {
		t.Fatalf("ToContentParts: %v", err)
	}
	if len(parts) != 1 || parts[0].Type != llminfra.PartText || !strings.Contains(parts[0].Text, "A") {
		t.Errorf("parts = %+v, want only the live text part", parts)
	}
}

func TestToContentParts_EmptyIDs(t *testing.T) {
	svc, _, ctx := newSvc(t)
	parts, err := svc.ToContentParts(ctx, nil, Capabilities{Vision: true})
	if err != nil || parts != nil {
		t.Errorf("empty ids = (%v, %v), want (nil, nil)", parts, err)
	}
}

// fakeExtractor is an Extractor stub: it records the mime it saw and returns a canned text/err.
//
// fakeExtractor 是 Extractor 桩：记录看到的 mime，返回预设 text/err。
type fakeExtractor struct {
	text string
	err  error
	mime string
}

func (f *fakeExtractor) Extract(_ context.Context, mime string, _ []byte) (string, error) {
	f.mime = mime
	return f.text, f.err
}

func TestToContentParts_NonNativeDocExtracts(t *testing.T) {
	ext := &fakeExtractor{text: "extracted body text"}
	svc, _, ctx := newSvcWith(t, ext)
	pdf, _ := svc.Upload(ctx, "report.pdf", "application/pdf", []byte("%PDF bytes"))

	// NativeDocs=false → the document is text-extracted and inlined instead of handed over raw.
	parts, err := svc.ToContentParts(ctx, []string{pdf.ID}, Capabilities{})
	if err != nil {
		t.Fatalf("ToContentParts: %v", err)
	}
	if len(parts) != 1 || parts[0].Type != llminfra.PartText {
		t.Fatalf("parts = %+v, want one text part", parts)
	}
	if ext.mime != "application/pdf" {
		t.Errorf("extractor saw mime %q, want application/pdf", ext.mime)
	}
	if !strings.Contains(parts[0].Text, "extracted body text") ||
		!strings.Contains(parts[0].Text, "report.pdf") || !strings.Contains(parts[0].Text, "text-extracted") {
		t.Errorf("text part = %q, want extracted body + filename + label", parts[0].Text)
	}
}

func TestToContentParts_DocDegradesWhenNoExtractor(t *testing.T) {
	svc, _, ctx := newSvc(t) // nil extractor
	pdf, _ := svc.Upload(ctx, "report.pdf", "application/pdf", []byte("%PDF"))

	parts, err := svc.ToContentParts(ctx, []string{pdf.ID}, Capabilities{})
	if err != nil {
		t.Fatalf("ToContentParts: %v", err)
	}
	if len(parts) != 1 || parts[0].Type != llminfra.PartText || !strings.Contains(parts[0].Text, "unavailable") {
		t.Errorf("parts = %+v, want placeholder note (extraction unavailable)", parts)
	}
}

func TestToContentParts_ExtractionFailureDegrades(t *testing.T) {
	ext := &fakeExtractor{err: errors.New("boom")}
	svc, _, ctx := newSvcWith(t, ext)
	pdf, _ := svc.Upload(ctx, "report.pdf", "application/pdf", []byte("%PDF"))

	parts, _ := svc.ToContentParts(ctx, []string{pdf.ID}, Capabilities{})
	if len(parts) != 1 || parts[0].Type != llminfra.PartText || !strings.Contains(parts[0].Text, "could not be extracted") {
		t.Errorf("parts = %+v, want degraded note on extraction failure", parts)
	}
}

// fakeSandbox is a SandboxRunner stub recording the EnsureEnv spec + Spawn opts it received.
//
// fakeSandbox 是 SandboxRunner 桩，记录收到的 EnsureEnv spec + Spawn opts。
type fakeSandbox struct {
	ensured  sandboxdomain.EnvSpec
	spawned  sandboxdomain.SpawnOpts
	status   string
	stdout   string
	ok       bool
	spawnErr error
}

func (f *fakeSandbox) EnsureEnv(_ context.Context, _ sandboxdomain.Owner, spec sandboxdomain.EnvSpec, _ sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	f.ensured = spec
	st := f.status
	if st == "" {
		st = sandboxdomain.EnvStatusReady
	}
	return &sandboxdomain.Env{Status: st}, nil
}

func (f *fakeSandbox) Spawn(_ context.Context, _ sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (*sandboxdomain.ExecutionResult, error) {
	f.spawned = opts
	if f.spawnErr != nil {
		return nil, f.spawnErr
	}
	return &sandboxdomain.ExecutionResult{Ok: f.ok, Stdout: []byte(f.stdout)}, nil
}

func TestSandboxExtractor_Success(t *testing.T) {
	sb := &fakeSandbox{ok: true, stdout: `{"text":"hello from pdf"}`}
	text, err := NewSandboxExtractor(sb).Extract(context.Background(), "application/pdf", []byte("rawpdf"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if text != "hello from pdf" {
		t.Errorf("text = %q, want hello from pdf", text)
	}
	if sb.ensured.Runtime.Kind != "python" || len(sb.ensured.Deps) == 0 {
		t.Errorf("EnsureEnv spec = %+v, want python + extraction deps", sb.ensured)
	}
	// Spawn: python -c <script> <mime>; raw bytes on stdin.
	if sb.spawned.Cmd != "python" || len(sb.spawned.Args) != 3 ||
		sb.spawned.Args[0] != "-c" || sb.spawned.Args[2] != "application/pdf" {
		t.Errorf("spawn args = %+v, want [-c <script> application/pdf]", sb.spawned.Args)
	}
	if string(sb.spawned.Stdin) != "rawpdf" {
		t.Errorf("stdin = %q, want rawpdf", sb.spawned.Stdin)
	}
}

func TestSandboxExtractor_UnsupportedMimeShortCircuits(t *testing.T) {
	sb := &fakeSandbox{ok: true, stdout: `{"text":"x"}`}
	_, err := NewSandboxExtractor(sb).Extract(context.Background(), "audio/mpeg", []byte("data"))
	if !errors.Is(err, ErrExtractionUnsupported) {
		t.Errorf("err = %v, want ErrExtractionUnsupported", err)
	}
	if sb.ensured.Runtime.Kind != "" { // must short-circuit before any env work
		t.Error("EnsureEnv called for unsupported mime; should short-circuit")
	}
}

func TestSandboxExtractor_PythonErrorWrapped(t *testing.T) {
	sb := &fakeSandbox{ok: true, stdout: `{"error":"PdfError: corrupt"}`}
	_, err := NewSandboxExtractor(sb).Extract(context.Background(), "application/pdf", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("err = %v, want wrapped python-side error", err)
	}
}

func TestSandboxExtractor_NonZeroExitErrors(t *testing.T) {
	sb := &fakeSandbox{ok: false} // interpreter crashed (e.g. missing package)
	_, err := NewSandboxExtractor(sb).Extract(context.Background(), "application/pdf", []byte("x"))
	if err == nil {
		t.Error("want error on non-zero python exit")
	}
}
