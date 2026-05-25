package chat

import (
	"fmt"
	"strings"
	"time"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// renderMentionsXML turns resolved references into one <mentions> block for the
// user LLM message. Code types carry a snapshot marker so the LLM refetches
// (get_function/...) before editing; documents are static reference content.
//
// renderMentionsXML 把已解析引用拼成一个 <mentions> 块；代码类带 snapshot 标记
// 提示 LLM 改前先 get 最新，document 是静态参考内容。
func renderMentionsXML(refs []mentiondomain.Reference, sentAt time.Time) string {
	var b strings.Builder
	b.WriteString("<mentions>\n")
	for _, r := range refs {
		fmt.Fprintf(&b, "<mention type=%q id=%q name=%q>\n", r.Type, r.ID, r.Name)
		if r.Type != mentiondomain.MentionDocument && r.Content != "" {
			fmt.Fprintf(&b, "(snapshot at %s)\n", sentAt.UTC().Format(time.RFC3339))
		}
		if r.Content == "" {
			b.WriteString("[引用的实体无法加载]")
		} else {
			b.WriteString(r.Content)
		}
		b.WriteString("\n</mention>\n")
	}
	b.WriteString("</mentions>\n")
	return b.String()
}
