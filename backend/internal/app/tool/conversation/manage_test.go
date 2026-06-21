package conversation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	conversationapp "github.com/sunweilin/anselm/backend/internal/app/conversation"
	conversationdomain "github.com/sunweilin/anselm/backend/internal/domain/conversation"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// fakeManager records the last Update call so a test can assert the archive/pin field the tool set.
type fakeManager struct {
	gotID string
	gotIn conversationapp.UpdateInput
	calls int
}

func (f *fakeManager) Update(_ context.Context, id string, in conversationapp.UpdateInput) (*conversationdomain.Conversation, error) {
	f.gotID, f.gotIn, f.calls = id, in, f.calls+1
	return &conversationdomain.Conversation{ID: id, Archived: in.Archived != nil && *in.Archived, Pinned: in.Pinned != nil && *in.Pinned}, nil
}

// Test_manageConversation_Schema pins the contract: the action enum + the anti-fabrication
// description (compaction is automatic, no button) so a future edit can't silently drop the line
// that stops the agent inventing a UI compact button (F38).
func Test_manageConversation_Schema(t *testing.T) {
	tool := &ManageConversation{mgr: &fakeManager{}}
	if tool.Name() != "manage_conversation" {
		t.Fatalf("name = %q", tool.Name())
	}
	var schema struct {
		Properties struct {
			Action struct {
				Enum []string `json:"enum"`
			} `json:"action"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("parameters not valid JSON: %v", err)
	}
	got := strings.Join(schema.Properties.Action.Enum, ",")
	if got != "archive,unarchive,pin,unpin,rename" {
		t.Fatalf("action enum = %q", got)
	}
	desc := tool.Description()
	if !strings.Contains(desc, "AUTOMATICALLY") || !strings.Contains(strings.ToLower(desc), "button") {
		t.Fatalf("description must state compaction is automatic + no button (F38): %q", desc)
	}
	// F106: the hidden auto-unarchive contract is disclosed so the agent can warn rather than let the
	// next message silently undo an archive of the active thread.
	if !strings.Contains(strings.ToLower(desc), "unarchive") {
		t.Fatalf("description must disclose that messaging an archived thread auto-unarchives it (F106): %q", desc)
	}
}

// Test_manageConversation_NoConversationInCtx: outside a conversation, degrade to a tool-result
// string (nil error) so the LLM can adjust — mirrors get_subagent_trace.
func Test_manageConversation_NoConversationInCtx(t *testing.T) {
	tool := &ManageConversation{mgr: &fakeManager{}}
	out, err := tool.Execute(context.Background(), `{"action":"archive"}`)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(out, "only available inside a conversation") {
		t.Fatalf("expected graceful no-ctx string, got %q", out)
	}
}

// Test_manageConversation_Actions: each action maps to the right archive/pin PATCH field.
func Test_manageConversation_Actions(t *testing.T) {
	cases := []struct {
		action      string
		wantArchive *bool
		wantPinned  *bool
	}{
		{"archive", ptr(true), nil},
		{"unarchive", ptr(false), nil},
		{"pin", nil, ptr(true)},
		{"unpin", nil, ptr(false)},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			fm := &fakeManager{}
			tool := &ManageConversation{mgr: fm}
			ctx := reqctxpkg.SetConversationID(context.Background(), "cv_1")
			if _, err := tool.Execute(ctx, `{"action":"`+c.action+`"}`); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if fm.calls != 1 || fm.gotID != "cv_1" {
				t.Fatalf("Update not called for cv_1 (calls=%d id=%q)", fm.calls, fm.gotID)
			}
			if !ptrEq(fm.gotIn.Archived, c.wantArchive) {
				t.Fatalf("Archived = %v, want %v", deref(fm.gotIn.Archived), deref(c.wantArchive))
			}
			if !ptrEq(fm.gotIn.Pinned, c.wantPinned) {
				t.Fatalf("Pinned = %v, want %v", deref(fm.gotIn.Pinned), deref(c.wantPinned))
			}
		})
	}
}

// Test_manageConversation_Rename — F107: rename sets the Title PATCH (UpdateInput.Title already exists +
// HTTP PATCH renames). The tool was archive/pin-only, so the agent fabricated a UI gesture for rename.
func Test_manageConversation_Rename(t *testing.T) {
	fm := &fakeManager{}
	tool := &ManageConversation{mgr: fm}
	ctx := reqctxpkg.SetConversationID(context.Background(), "cv_1")
	if _, err := tool.Execute(ctx, `{"action":"rename","title":"Deploy planning"}`); err != nil {
		t.Fatalf("rename execute: %v", err)
	}
	if fm.gotIn.Title == nil || *fm.gotIn.Title != "Deploy planning" {
		t.Fatalf("rename should set Title to 'Deploy planning', got %v", fm.gotIn.Title)
	}
	if err := tool.ValidateInput([]byte(`{"action":"rename"}`)); err == nil {
		t.Fatal("rename without a title must reject")
	}
	if err := tool.ValidateInput([]byte(`{"action":"rename","title":"x"}`)); err != nil {
		t.Fatalf("rename with a title must pass: %v", err)
	}
	// A whitespace-only title is a blank rename in disguise — must reject (trim-aware), at both the
	// ValidateInput guard and the Execute write point.
	if err := tool.ValidateInput([]byte(`{"action":"rename","title":"   "}`)); err == nil {
		t.Fatal("rename with a whitespace-only title must reject")
	}
	if _, err := tool.Execute(ctx, `{"action":"rename","title":"   "}`); err == nil {
		t.Fatal("Execute rename with a whitespace-only title must reject")
	}
	// A padded title is stored trimmed.
	if _, err := tool.Execute(ctx, `{"action":"rename","title":"  Padded  "}`); err != nil {
		t.Fatalf("padded rename: %v", err)
	}
	if fm.gotIn.Title == nil || *fm.gotIn.Title != "Padded" {
		t.Fatalf("padded title must be stored trimmed as 'Padded', got %v", fm.gotIn.Title)
	}
}

// Test_manageConversation_ValidateInput rejects an action outside the enum + bad JSON.
func Test_manageConversation_ValidateInput(t *testing.T) {
	tool := &ManageConversation{mgr: &fakeManager{}}
	if err := tool.ValidateInput([]byte(`{"action":"delete"}`)); err == nil {
		t.Fatal("action=delete must reject")
	}
	if err := tool.ValidateInput([]byte(`{`)); err == nil {
		t.Fatal("bad JSON must reject")
	}
	if err := tool.ValidateInput([]byte(`{"action":"pin"}`)); err != nil {
		t.Fatalf("action=pin must pass: %v", err)
	}
}

// Test_ConversationTools_IncludesManage guards the group so manage_conversation can't be dropped.
func Test_ConversationTools_IncludesManage(t *testing.T) {
	tools := ConversationTools(nil, &fakeManager{})
	var found bool
	for _, tl := range tools {
		if tl.Name() == "manage_conversation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("manage_conversation missing from ConversationTools: %v", tools)
	}
}

func ptr(b bool) *bool { return &b }
func deref(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}
func ptrEq(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
