package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

func TestRenderMentionsXML_DocumentNoSnapshotMarker(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionDocument, ID: "doc_1", Name: "Spec", Content: "# Body"},
	}
	out := renderMentionsXML(refs, time.Now())
	if !strings.Contains(out, `<mention type="document" id="doc_1" name="Spec">`) {
		t.Errorf("missing document tag: %q", out)
	}
	if strings.Contains(out, "snapshot at") {
		t.Errorf("document should not carry snapshot marker: %q", out)
	}
	if !strings.Contains(out, "# Body") {
		t.Errorf("missing doc content: %q", out)
	}
}

func TestRenderMentionsXML_CodeCarriesSnapshotMarker(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionFunction, ID: "f_1", Name: "csv", Content: "def csv(): pass"},
	}
	out := renderMentionsXML(refs, time.Date(2026, 5, 25, 8, 0, 0, 0, time.UTC))
	if !strings.Contains(out, "(snapshot at 2026-05-25T08:00:00Z)") {
		t.Errorf("function should carry snapshot marker: %q", out)
	}
}

func TestRenderMentionsXML_StubRendersPlaceholder(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionDocument, ID: "doc_x", Name: "(无法加载)", Content: ""},
	}
	out := renderMentionsXML(refs, time.Now())
	if !strings.Contains(out, "[引用的实体无法加载]") {
		t.Errorf("stub should render placeholder: %q", out)
	}
}

type fakeMentionResolver struct {
	typ mentiondomain.MentionType
	ref *mentiondomain.Reference
	err error
}

func (f fakeMentionResolver) Type() mentiondomain.MentionType { return f.typ }
func (f fakeMentionResolver) Resolve(_ context.Context, _ string) (*mentiondomain.Reference, error) {
	return f.ref, f.err
}

func TestRegisterMentionResolver_KeysByType(t *testing.T) {
	s := &Service{}
	s.RegisterMentionResolver(fakeMentionResolver{typ: mentiondomain.MentionDocument})
	if _, ok := s.mentionResolvers[mentiondomain.MentionDocument]; !ok {
		t.Error("resolver not registered under its Type()")
	}
}
