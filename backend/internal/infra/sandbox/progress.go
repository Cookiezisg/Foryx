// progress.go parses uv stderr line-by-line into structured (stage, detail)
// progress updates. uv prints three summary lines per `uv sync` run:
//
//	"Resolved 12 packages in 1.5s"
//	"Prepared 12 packages in 800ms"
//	"Installed 12 packages in 200ms"
//
// (Downloading individual packages happens inside the Prepared phase — uv may
// emit per-package "Downloading numpy" sub-progress lines, but the only
// summary lines we treat as stage transitions are the three above.)
//
// scanProgress is a dual-purpose stream pump: lines that match a known stage
// are dispatched to the OnProgress callback; everything else is buffered into
// errBuf. On Sync success the errBuf content is discarded; on failure it is
// surfaced through SyncError.Stderr → ForgeVersion.EnvError → LLM tool_result,
// so the LLM sees the actual resolver / network / build error and can call
// edit_forge to self-correct (MVP "punt to AI" philosophy, sandbox iteration
// doc §11.1).
//
// progress.go 把 uv stderr 行解析成结构化 (stage, detail) 进度更新。
// uv 每次 `uv sync` 输出三个总结行（见上方英文示例），下载发生在 Prepared
// 阶段内部，单包 sub-progress 行不当 stage 转换。
//
// scanProgress 是双路流泵：识别成阶段的行调 OnProgress callback；其他行
// 缓存到 errBuf。Sync 成功时 errBuf 丢弃；失败时透传 SyncError.Stderr →
// ForgeVersion.EnvError → LLM tool_result——LLM 看到真实 resolver / 网络
// / 构建错误，调 edit_forge 自救（MVP "punt 给 AI" 哲学，沙箱迭代 §11.1）。

package sandbox

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// progressUpdate is the parsed shape of one matched uv stderr line.
//
// progressUpdate 是已识别的 uv stderr 行解析形状。
type progressUpdate struct {
	Stage  string // "resolving" | "preparing" | "installing"
	Detail string // raw line, trimmed of leading/trailing whitespace
}

// parseUVLine matches one uv stderr line against known stage prefixes and
// returns the parsed update, or nil if the line is not a stage summary.
//
// parseUVLine 匹配一条 uv stderr 行的已知 stage 前缀，返回解析后的更新；
// 非 stage 总结行返 nil。
func parseUVLine(line string) *progressUpdate {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(line, "Resolved "):
		return &progressUpdate{Stage: "resolving", Detail: line}
	case strings.HasPrefix(line, "Prepared "):
		return &progressUpdate{Stage: "preparing", Detail: line}
	case strings.HasPrefix(line, "Installed "):
		return &progressUpdate{Stage: "installing", Detail: line}
	}
	return nil
}

// scanProgress reads from r line by line. Recognized stage lines are
// dispatched to onProgress (if non-nil); every other line is appended to
// errBuf for later inspection on failure. Returns when r is exhausted or
// errors — scanner errors are deliberately swallowed because we always have
// an outer cmd.Run() error to surface failure.
//
// onProgress may be nil — useful for tests or when the caller doesn't want
// progress updates.
//
// errBuf must be non-nil — caller owns it and inspects it on Sync failure.
//
// scanProgress 按行读 r。识别成 stage 的行调 onProgress（非 nil 时）；
// 其他行追加到 errBuf 供失败时检查。r 读完或出错时返回——scanner 错误
// 故意吞掉，因为我们总有外层 cmd.Run() 错误来反映失败。
//
// onProgress 可为 nil（测试 / 不需要进度的场景）。
// errBuf 必须非 nil——调用方持有，失败时读它。
func scanProgress(r io.Reader, onProgress func(stage, detail string), errBuf *bytes.Buffer) {
	scanner := bufio.NewScanner(r)
	// Default scanner buffer is 64KB per line — uv error chains can be long
	// (transitive dep conflicts), so bump the limit to 1MB. Lines longer than
	// that are extremely unlikely and we'd rather truncate than panic.
	//
	// 默认 scanner 行缓冲 64KB——uv 错误链可能很长（传递依赖冲突），
	// 提升到 1MB；超过截断不 panic。
	const maxLine = 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)

	for scanner.Scan() {
		line := scanner.Text()
		if u := parseUVLine(line); u != nil {
			if onProgress != nil {
				onProgress(u.Stage, u.Detail)
			}
			continue
		}
		// Unrecognized line: buffer it. errBuf is consulted on failure.
		// 无法识别的行：缓存。失败时调用方读 errBuf。
		errBuf.WriteString(line)
		errBuf.WriteByte('\n')
	}
}
