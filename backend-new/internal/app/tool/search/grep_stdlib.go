package search

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	maxStdlibFileBytes   = 32 * 1024 * 1024
	maxStdlibScannerLine = 8 * 1024 * 1024
)

// noiseDirs are skipped during WalkDir so we don't spend time crawling
// node_modules / .git / virtualenvs. (These would also be skipped by rg
// when .gitignore is honoured.)
//
// noiseDirs 在 WalkDir 时直接跳过，避免在 node_modules / .git / venv 里浪费
// 时间。rg 走 .gitignore 时也会跳。
var noiseDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	".venv":        {},
	"venv":         {},
	"__pycache__":  {},
	".forgify":     {},
}

var typeExtensions = map[string][]string{
	"go":     {".go"},
	"py":     {".py"},
	"js":     {".js", ".mjs", ".cjs"},
	"ts":     {".ts"},
	"tsx":    {".tsx"},
	"jsx":    {".jsx"},
	"rust":   {".rs"},
	"rs":     {".rs"},
	"c":      {".c", ".h"},
	"cpp":    {".cpp", ".cxx", ".cc", ".hpp", ".hxx"},
	"java":   {".java"},
	"rb":     {".rb"},
	"php":    {".php"},
	"swift":  {".swift"},
	"kotlin": {".kt", ".kts"},
	"yaml":   {".yml", ".yaml"},
	"yml":    {".yml", ".yaml"},
	"json":   {".json"},
	"xml":    {".xml"},
	"html":   {".html", ".htm"},
	"css":    {".css", ".scss", ".sass"},
	"md":     {".md", ".markdown"},
	"sh":     {".sh", ".bash"},
	"toml":   {".toml"},
	"sql":    {".sql"},
}

// execStdlib runs the search using stdlib regexp; isDir = directory walk vs single-file scan.
//
// execStdlib 用 stdlib regexp 跑搜索；isDir 决定走目录还是单文件。
func (t *Grep) execStdlib(ctx context.Context, args grepArgs, isDir bool) (string, error) {
	re, err := compileGrepRegex(args)
	if err != nil {
		return fmt.Sprintf("Invalid regex pattern: %v", err), nil
	}

	candidates, err := collectCandidates(args, isDir)
	if err != nil {
		return "", fmt.Errorf("Grep.execStdlib: %w", err)
	}
	sort.Strings(candidates)

	switch args.OutputMode {
	case OutputModeContent:
		return searchContent(ctx, re, candidates, args, isDir), nil
	case OutputModeCount:
		return searchCount(ctx, re, candidates, args), nil
	default:
		return searchFilesWithMatches(ctx, re, candidates, args), nil
	}
}

// compileGrepRegex prepends `(?i)` for IgnoreCase and `(?s)` for Multiline.
//
// compileGrepRegex 给正则前置 `(?i)` 大小写不敏感，`(?s)` multiline 模式。
func compileGrepRegex(args grepArgs) (*regexp.Regexp, error) {
	var prefix strings.Builder
	if args.IgnoreCase {
		prefix.WriteString("(?i)")
	}
	if args.Multiline {
		prefix.WriteString("(?s)")
	}
	return regexp.Compile(prefix.String() + args.Pattern)
}

// collectCandidates returns absolute paths to scan; walks dir or scans single file, skipping noiseDirs.
//
// collectCandidates 返回扫描路径列表；目录走 WalkDir 单文件直接扫，跳过 noiseDirs。
func collectCandidates(args grepArgs, isDir bool) ([]string, error) {
	if !isDir {
		// Single-file search: type/glob filter still applies — empty result
		// is a legitimate "no files matched filter" outcome.
		// 单文件搜索：type/glob 过滤仍生效——空结果是合法的“无文件匹配过滤”。
		if !fileMatchesFilters(args.Path, args) {
			return nil, nil
		}
		return []string{args.Path}, nil
	}

	var out []string
	walkErr := filepath.WalkDir(args.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if _, skip := noiseDirs[d.Name()]; skip && path != args.Path {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if fileMatchesFilters(path, args) {
			out = append(out, path)
		}
		return nil
	})
	return out, walkErr
}

func fileMatchesFilters(path string, args grepArgs) bool {
	if args.Type != "" {
		exts, known := typeExtensions[args.Type]
		if !known {
			return false
		}
		ext := strings.ToLower(filepath.Ext(path))
		matched := false
		for _, e := range exts {
			if e == ext {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if args.Glob != "" {
		if ok, _ := doublestar.Match(args.Glob, filepath.Base(path)); ok {
			return true
		}
		rel := path
		if absRoot := args.Path; absRoot != "" {
			if r, err := filepath.Rel(absRoot, path); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		if ok, _ := doublestar.Match(args.Glob, rel); ok {
			return true
		}
		return false
	}
	return true
}

func searchFilesWithMatches(ctx context.Context, re *regexp.Regexp, files []string, args grepArgs) string {
	var sb strings.Builder
	emitted := 0
	for _, p := range files {
		if ctx.Err() != nil {
			break
		}
		hit, _ := fileHasMatch(p, re, args.Multiline)
		if !hit {
			continue
		}
		sb.WriteString(p)
		sb.WriteByte('\n')
		emitted++
		if args.HeadLimit > 0 && emitted >= args.HeadLimit {
			fmt.Fprintf(&sb, "... [truncated at %d files; raise head_limit to see more]\n", args.HeadLimit)
			break
		}
	}
	if emitted == 0 {
		return noMatchesMessage(args)
	}
	return sb.String()
}

func searchCount(ctx context.Context, re *regexp.Regexp, files []string, args grepArgs) string {
	var sb strings.Builder
	emitted := 0
	for _, p := range files {
		if ctx.Err() != nil {
			break
		}
		_, count := fileHasMatch(p, re, args.Multiline)
		if count == 0 {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d\n", p, count)
		emitted++
		if args.HeadLimit > 0 && emitted >= args.HeadLimit {
			fmt.Fprintf(&sb, "... [truncated at %d files; raise head_limit to see more]\n", args.HeadLimit)
			break
		}
	}
	if emitted == 0 {
		return noMatchesMessage(args)
	}
	return sb.String()
}

// searchContent emits matching lines with optional line numbers and
// before/after context. Path prefix is omitted when the search root is a
// single file (matches rg's behaviour).
//
// searchContent 输出匹配行；可选行号 + 前后上下文。单文件 root 时省略 path
// 前缀（与 rg 行为一致）。
func searchContent(ctx context.Context, re *regexp.Regexp, files []string, args grepArgs, isDir bool) string {
	var sb strings.Builder
	emitted := 0
	for _, p := range files {
		if ctx.Err() != nil {
			break
		}
		matches := scanFileContent(p, re, args)
		if len(matches) == 0 {
			continue
		}
		for _, m := range matches {
			writeContentLine(&sb, p, m, args, isDir)
			emitted++
			if args.HeadLimit > 0 && emitted >= args.HeadLimit {
				fmt.Fprintf(&sb, "... [truncated at %d matches; raise head_limit to see more]\n", args.HeadLimit)
				return sb.String()
			}
		}
	}
	if emitted == 0 {
		return noMatchesMessage(args)
	}
	return sb.String()
}

func fileHasMatch(path string, re *regexp.Regexp, multiline bool) (bool, int) {
	if multiline {
		data, err := readFileBounded(path, maxStdlibFileBytes)
		if err != nil {
			return false, 0
		}
		all := re.FindAllIndex(data, -1)
		return len(all) > 0, len(all)
	}
	f, err := os.Open(path) //nolint:gosec // path comes from filepath.WalkDir under the validated root.
	if err != nil {
		return false, 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxStdlibScannerLine)
	count := 0
	for scanner.Scan() {
		count += len(re.FindAllIndex(scanner.Bytes(), -1))
	}
	return count > 0, count
}

type matchedLine struct {
	lineNum int
	text    string
	context bool
}

func scanFileContent(path string, re *regexp.Regexp, args grepArgs) []matchedLine {
	if args.Multiline {
		return scanFileContentMultiline(path, re, args)
	}
	return scanFileContentLineMode(path, re, args)
}

func scanFileContentLineMode(path string, re *regexp.Regexp, args grepArgs) []matchedLine {
	f, err := os.Open(path) //nolint:gosec // path is under the validated walk root.
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxStdlibScannerLine)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil
	}

	// Pre-compute match lines so context loops never relabel a match as context.
	// 预算 match 行；context 循环跳过它们，防 match 被错标为 context。
	matchLines := make(map[int]bool)
	for i, ln := range lines {
		if re.MatchString(ln) {
			matchLines[i] = true
		}
	}

	emitted := make(map[int]bool)
	var out []matchedLine
	for i := range lines {
		if !matchLines[i] {
			continue
		}
		for off := args.Before; off > 0; off-- {
			j := i - off
			if j < 0 || emitted[j] || matchLines[j] {
				continue
			}
			out = append(out, matchedLine{lineNum: j + 1, text: lines[j], context: true})
			emitted[j] = true
		}
		if !emitted[i] {
			out = append(out, matchedLine{lineNum: i + 1, text: lines[i], context: false})
			emitted[i] = true
		}
		// After context — skip lines that are themselves matches.
		// 后置上下文——跳过本身就是 match 的行。
		for off := 1; off <= args.After; off++ {
			j := i + off
			if j >= len(lines) || emitted[j] || matchLines[j] {
				continue
			}
			out = append(out, matchedLine{lineNum: j + 1, text: lines[j], context: true})
			emitted[j] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lineNum < out[j].lineNum })
	return out
}

func scanFileContentMultiline(path string, re *regexp.Regexp, args grepArgs) []matchedLine {
	data, err := readFileBounded(path, maxStdlibFileBytes)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	hits := re.FindAllIndex(data, -1)
	if len(hits) == 0 {
		return nil
	}

	matchLines := make(map[int]bool)
	for _, h := range hits {
		startLine := byteOffsetToLine(data, h[0])
		endLine := byteOffsetToLine(data, h[1]-1)
		for ln := startLine; ln <= endLine; ln++ {
			matchLines[ln] = true
		}
	}

	emitted := make(map[int]bool)
	var out []matchedLine
	maxLine := len(lines)
	for ln := 1; ln <= maxLine; ln++ {
		if !matchLines[ln] {
			continue
		}
		for off := args.Before; off > 0; off-- {
			j := ln - off
			if j < 1 || emitted[j] {
				continue
			}
			if matchLines[j] {
				continue
			}
			out = append(out, matchedLine{lineNum: j, text: lines[j-1], context: true})
			emitted[j] = true
		}
		if !emitted[ln] {
			out = append(out, matchedLine{lineNum: ln, text: lines[ln-1], context: false})
			emitted[ln] = true
		}
		for off := 1; off <= args.After; off++ {
			j := ln + off
			if j > maxLine || emitted[j] {
				continue
			}
			if matchLines[j] {
				continue
			}
			out = append(out, matchedLine{lineNum: j, text: lines[j-1], context: true})
			emitted[j] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lineNum < out[j].lineNum })
	return out
}

// writeContentLine writes one matchedLine in rg --no-heading format; path omitted on single-file searches.
//
// writeContentLine 按 rg --no-heading 格式输出一行；单文件搜索省 path。
func writeContentLine(sb *strings.Builder, path string, m matchedLine, args grepArgs, isDir bool) {
	sep := byte(':')
	if m.context {
		sep = '-'
	}
	if isDir {
		sb.WriteString(path)
		sb.WriteByte(sep)
	}
	if args.ShowLines {
		fmt.Fprintf(sb, "%d", m.lineNum)
		sb.WriteByte(sep)
	}
	sb.WriteString(m.text)
	sb.WriteByte('\n')
}

func readFileBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d-byte multiline scan cap", limit)
	}
	return os.ReadFile(path) //nolint:gosec // path is under validated walk root.
}

func byteOffsetToLine(data []byte, b int) int {
	if b < 0 {
		b = 0
	}
	if b > len(data) {
		b = len(data)
	}
	line := 1
	for i := range b {
		if data[i] == '\n' {
			line++
		}
	}
	return line
}
