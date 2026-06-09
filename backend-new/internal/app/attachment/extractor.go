package attachment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// Extractor pulls plain text out of a binary document (PDF / Office). It is the pluggable port the
// attachment Service calls when a model can't read a document natively — the text is inlined into
// the turn instead. An extractor that can't handle a mime returns ErrExtractionUnsupported so the
// caller degrades to a placeholder. Future audio/video/OCR extractors implement this same port.
//
// Extractor 从二进制文档（PDF / Office）抽纯文本。它是可插端口：当模型不能原生读文档时，attachment
// Service 调它把文本内联进回合。不认某 mime 的 extractor 返回 ErrExtractionUnsupported，调用方降级
// 为占位。未来音频/视频/OCR extractor 实现同一端口。
type Extractor interface {
	Extract(ctx context.Context, mime string, data []byte) (string, error)
}

// ErrExtractionUnsupported signals that this extractor has no handler for the given mime; the
// caller degrades the attachment to a text placeholder rather than failing the turn.
//
// ErrExtractionUnsupported 表本 extractor 没有该 mime 的处理器；调用方把附件降级为文字占位，而非
// 让回合失败。
var ErrExtractionUnsupported = fmt.Errorf("attachment: extraction unsupported for this mime")

// SandboxRunner is the slice of the sandbox app Service the document extractor needs: ensure a
// shared python env with the extraction toolchain, then run a one-shot script. sandboxapp.Service
// satisfies this structurally (DIP — attachment never imports app/sandbox).
//
// SandboxRunner 是文档 extractor 需要的 sandbox app Service 切片：确保一个装了抽取工具链的共享
// python env，再跑一次性脚本。sandboxapp.Service 结构化满足它（DIP——attachment 不 import
// app/sandbox）。
type SandboxRunner interface {
	EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error)
	Spawn(ctx context.Context, owner sandboxdomain.Owner, opts sandboxdomain.SpawnOpts) (*sandboxdomain.ExecutionResult, error)
}

// extractorOwner is the fixed, machine-global owner of the one shared extraction env: a single venv
// holding pdfplumber / python-docx / openpyxl / python-pptx, reused across every workspace (it
// holds no user data — bytes flow through stdin/stdout, never into the env).
//
// extractorOwner 是那个唯一共享抽取 env 的固定、全机 owner：单个 venv 装 pdfplumber / python-docx /
// openpyxl / python-pptx，跨所有 workspace 复用（它不存用户数据——字节经 stdin/stdout 流过，绝不进 env）。
var extractorOwner = sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindAttachment, ID: "extractor"}

// extractorDeps is the pip toolchain for the document main line (PDF + MS Office). Audio (whisper),
// video, and scanned-image OCR (tesseract) are deliberately absent — they slot in as separate
// extractors later, not by bloating this env.
//
// extractorDeps 是文档主线（PDF + MS Office）的 pip 工具链。音频（whisper）/视频/扫描件 OCR
// （tesseract）刻意不在此——它们以独立 extractor 后补，不靠膨胀本 env。
var extractorDeps = []string{"pdfplumber", "python-docx", "openpyxl", "python-pptx"}

// Office Open XML mime types the extractor can read (PDF handled separately).
//
// extractor 能读的 Office Open XML mime（PDF 单独处理）。
const (
	mimePDF  = "application/pdf"
	mimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePPTX = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
)

// SandboxExtractor extracts document text by running a python script in the sandbox. The script is
// passed via `python -c` with the mime as argv[1] and the file bytes on stdin; it prints a JSON
// {text} or {error}. The shared env is ensured on every call (idempotent + fast once Ready; the
// first call ever pays the pip install).
//
// SandboxExtractor 通过在 sandbox 跑 python 脚本抽文档文本。脚本经 `python -c` 传入、mime 作 argv[1]、
// 文件字节走 stdin；它打印 JSON {text} 或 {error}。每次调用都 ensure 共享 env（幂等 + Ready 后很快；
// 史上第一次付 pip install 代价）。
type SandboxExtractor struct {
	sandbox SandboxRunner
}

// NewSandboxExtractor builds the document extractor over the sandbox runner.
//
// NewSandboxExtractor 在 sandbox runner 上构造文档 extractor。
func NewSandboxExtractor(sandbox SandboxRunner) *SandboxExtractor {
	return &SandboxExtractor{sandbox: sandbox}
}

// Extract returns the document's plain text. A mime with no python handler short-circuits to
// ErrExtractionUnsupported before any env work. A python-side parse error (corrupt file, etc.)
// surfaces as a wrapped error; the caller degrades to a placeholder either way.
//
// Extract 返回文档纯文本。无 python 处理器的 mime 在任何 env 工作前短路返回 ErrExtractionUnsupported。
// python 侧解析错（损坏文件等）以包装错误冒出；两种情况调用方都降级为占位。
func (e *SandboxExtractor) Extract(ctx context.Context, mime string, data []byte) (string, error) {
	if !isExtractableDoc(mime) {
		return "", ErrExtractionUnsupported
	}
	env, err := e.sandbox.EnsureEnv(ctx, extractorOwner,
		sandboxdomain.EnvSpec{Runtime: sandboxdomain.RuntimeSpec{Kind: "python"}, Deps: extractorDeps}, nil)
	if err != nil {
		return "", fmt.Errorf("attachment.extract: ensure env: %w", err)
	}
	if env.Status != sandboxdomain.EnvStatusReady {
		return "", fmt.Errorf("attachment.extract: env not ready (%s): %s", env.Status, env.ErrorMsg)
	}
	res, err := e.sandbox.Spawn(ctx, extractorOwner, sandboxdomain.SpawnOpts{
		Cmd:   "python",
		Args:  []string{"-c", extractScript, mime},
		Stdin: data,
	})
	if err != nil {
		return "", fmt.Errorf("attachment.extract: spawn: %w", err)
	}
	if !res.Ok {
		return "", fmt.Errorf("attachment.extract: python exit %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(res.Stdout, &out); err != nil {
		return "", fmt.Errorf("attachment.extract: decode output: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("attachment.extract: %s", out.Error)
	}
	return out.Text, nil
}

// isExtractableDoc reports whether the document main-line python toolchain handles this mime.
//
// isExtractableDoc 报告文档主线 python 工具链是否处理该 mime。
func isExtractableDoc(mime string) bool {
	switch mime {
	case mimePDF, mimeDOCX, mimeXLSX, mimePPTX:
		return true
	}
	return false
}

// extractScript dispatches by argv[1] (mime), reads the file from stdin, and prints {"text":...}
// or {"error":...}. It never exits non-zero on a parse failure — that is reported in JSON so a
// corrupt file degrades gracefully; only a broken interpreter / missing package fails the spawn.
//
// extractScript 按 argv[1]（mime）分派，从 stdin 读文件，打印 {"text":...} 或 {"error":...}。解析
// 失败绝不非零退出——以 JSON 报告使损坏文件优雅降级；只有解释器坏 / 缺包才让 spawn 失败。
const extractScript = `
import sys, io, json

def extract(mime, buf):
    if mime == "application/pdf":
        import pdfplumber
        with pdfplumber.open(buf) as pdf:
            return "\n\n".join((pg.extract_text() or "") for pg in pdf.pages)
    if mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
        import docx
        return "\n".join(p.text for p in docx.Document(buf).paragraphs)
    if mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
        import openpyxl
        wb = openpyxl.load_workbook(buf, read_only=True, data_only=True)
        lines = []
        for ws in wb.worksheets:
            lines.append("# " + ws.title)
            for row in ws.iter_rows(values_only=True):
                cells = ["" if c is None else str(c) for c in row]
                if any(cells):
                    lines.append("\t".join(cells))
        return "\n".join(lines)
    if mime == "application/vnd.openxmlformats-officedocument.presentationml.presentation":
        from pptx import Presentation
        chunks = []
        for i, slide in enumerate(Presentation(buf).slides, 1):
            chunks.append("# Slide %d" % i)
            for shape in slide.shapes:
                if shape.has_text_frame and shape.text_frame.text:
                    chunks.append(shape.text_frame.text)
        return "\n".join(chunks)
    raise ValueError("unsupported mime: " + mime)

def main():
    mime = sys.argv[1] if len(sys.argv) > 1 else ""
    buf = io.BytesIO(sys.stdin.buffer.read())
    try:
        print(json.dumps({"text": extract(mime, buf)}))
    except Exception as e:
        print(json.dumps({"error": "%s: %s" % (type(e).__name__, e)}))

main()
`
