// Package chat extracts text from chat attachment files by MIME type.
//
// Package chat 按 MIME 类型从聊天附件中提取文本内容。
package chat

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pdflib "github.com/dslipak/pdf"
	catlib "github.com/lu4p/cat"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// IsImage reports whether the MIME type should route via LLM Vision instead of text extraction.
//
// IsImage 判断该 MIME 是否应走 LLM Vision 而非文本提取。
func IsImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// Extract returns the file's text content; callers must IsImage-gate first.
//
// Extract 返回文件文本内容；调用方需先调 IsImage 判图。
func Extract(storagePath, mimeType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(storagePath))

	switch {
	case isPlainText(mimeType, ext):
		return extractPlainText(storagePath)
	case mimeType == "application/pdf" || ext == ".pdf":
		return extractPDF(storagePath)
	case isOfficeDoc(mimeType, ext):
		return extractOfficeDoc(storagePath)
	case isExcel(mimeType, ext):
		return extractExcel(storagePath)
	case ext == ".pptx":
		return extractPPTX(storagePath)
	case mimeType == "text/html" || ext == ".html" || ext == ".htm":
		return extractHTML(storagePath)
	default:
		return "", chatdomain.ErrAttachmentTypeUnsupported
	}
}

func isPlainText(mimeType, ext string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch ext {
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs",
		".rb", ".php", ".sh", ".yaml", ".yml", ".toml", ".ini", ".env",
		".json", ".csv", ".tsv", ".md", ".markdown", ".rst", ".xml", ".sql":
		return true
	}
	return false
}

func extractPlainText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return string(b), nil
}

func extractPDF(path string) (string, error) {
	r, err := pdflib.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, reader); err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return buf.String(), nil
}

func isOfficeDoc(mimeType, ext string) bool {
	switch ext {
	case ".docx", ".odt", ".rtf":
		return true
	}
	switch mimeType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.oasis.opendocument.text",
		"application/rtf", "text/rtf":
		return true
	}
	return false
}

func extractOfficeDoc(path string) (string, error) {
	text, err := catlib.File(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return text, nil
}

func isExcel(mimeType, ext string) bool {
	switch ext {
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return true
	}
	return mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

func extractExcel(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		sb.WriteString("[Sheet: ")
		sb.WriteString(sheet)
		sb.WriteString("]\n")
		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func extractPPTX(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "ppt/slides/slide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		texts, _ := pptxSlideText(rc)
		rc.Close()
		sb.WriteString(texts)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func pptxSlideText(r io.Reader) (string, error) {
	var sb strings.Builder
	dec := xml.NewDecoder(r)
	inText := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inText = t.Name.Local == "t"
		case xml.CharData:
			if inText {
				sb.Write(t)
				sb.WriteByte(' ')
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		}
	}
	return sb.String(), nil
}

func extractHTML(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer f.Close()

	doc, err := html.Parse(f)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}

	var sb strings.Builder
	htmlText(doc, &sb)
	return sb.String(), nil
}

func htmlText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style":
			return
		}
	}
	if n.Type == html.TextNode {
		trimmed := strings.TrimSpace(n.Data)
		if trimmed != "" {
			sb.WriteString(trimmed)
			sb.WriteByte('\n')
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		htmlText(c, sb)
	}
}
