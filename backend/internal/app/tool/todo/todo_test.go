package todo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newTestService(t *testing.T) *todoapp.Service {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := dbinfra.Migrate(db, &tododomain.Todo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return todoapp.NewService(todostore.New(db), nil, zap.NewNop())
}

func ctxWithConv(id string) context.Context {
	return reqctxpkg.WithConversationID(context.Background(), id)
}


func TestTodoTools_ReturnsFourTools(t *testing.T) {
	tools := TodoTools(newTestService(t))
	if len(tools) != 4 {
		t.Fatalf("len = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name()] = true
	}
	for _, want := range []string{"TodoCreate", "TodoList", "TodoGet", "TodoUpdate"} {
		if !names[want] {
			t.Errorf("missing tool %q (got: %v)", want, names)
		}
	}
}


func TestTodoCreate_Identity(t *testing.T) {
	tool := &TodoCreate{svc: newTestService(t)}
	if tool.Name() != "TodoCreate" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.IsReadOnly() {
		t.Error("TodoCreate should not be read-only")
	}
}

func TestTodoCreate_ValidateInput_RequiresSubject(t *testing.T) {
	tool := &TodoCreate{svc: newTestService(t)}
	if err := tool.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, tododomain.ErrSubjectRequired) {
		t.Errorf("want ErrSubjectRequired, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"subject":"  "}`)); !errors.Is(err, tododomain.ErrSubjectRequired) {
		t.Errorf("whitespace subject should fail, got %v", err)
	}
}

func TestTodoCreate_Execute_PersistsAndReturnsJSON(t *testing.T) {
	svc := newTestService(t)
	tool := &TodoCreate{svc: svc}
	ctx := ctxWithConv("cv_x")
	out, err := tool.Execute(ctx, `{"subject":"Run tests","active_form":"Running tests"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got tododomain.Todo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal (raw=%q): %v", out, err)
	}
	if got.Subject != "Run tests" || got.ID == "" {
		t.Errorf("got %+v", got)
	}
	// And it's actually persisted — ListByConversation must see it.
	// 实际落库——ListByConversation 必须看到。
	todos, _ := svc.List(ctx)
	if len(todos) != 1 {
		t.Errorf("expected 1 persisted todo, got %d", len(todos))
	}
}


func TestTodoList_Identity(t *testing.T) {
	tool := &TodoList{svc: newTestService(t)}
	if tool.Name() != "TodoList" {
		t.Errorf("Name = %q", tool.Name())
	}
	if !tool.IsReadOnly() {
		t.Error("TodoList should be read-only")
	}
}

func TestTodoList_Execute_ReturnsAllTodos(t *testing.T) {
	svc := newTestService(t)
	ctx := ctxWithConv("cv_x")
	for i, subj := range []string{"a", "b", "c"} {
		if _, err := svc.Create(ctx, todoapp.CreateInput{Subject: subj}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	out, err := (&TodoList{svc: svc}).Execute(ctx, `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Total int               `json:"total"`
		Todos []tododomain.Todo `json:"todos"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if resp.Total != 3 || len(resp.Todos) != 3 {
		t.Errorf("total=%d todos=%d, want 3 each", resp.Total, len(resp.Todos))
	}
}

func TestTodoList_Execute_NoConvID_ReportsFriendly(t *testing.T) {
	tool := &TodoList{svc: newTestService(t)}
	out, err := tool.Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Todo list failed") && !strings.Contains(out, "missing conversation") {
		t.Errorf("expected friendly missing-conv message, got: %q", out)
	}
}


func TestTodoGet_ValidateInput_RequiresTodoID(t *testing.T) {
	tool := &TodoGet{svc: newTestService(t)}
	err := tool.ValidateInput(json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "todo_id") {
		t.Errorf("want todo_id error, got %v", err)
	}
}

func TestTodoGet_Execute_RetrievesByID(t *testing.T) {
	svc := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, todoapp.CreateInput{Subject: "fetch me"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	out, err := (&TodoGet{svc: svc}).Execute(ctx, `{"todo_id":"`+created.ID+`"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "fetch me") {
		t.Errorf("expected subject in result, got: %q", out)
	}
}

func TestTodoGet_Execute_UnknownID_FriendlyMessage(t *testing.T) {
	tool := &TodoGet{svc: newTestService(t)}
	out, err := tool.Execute(ctxWithConv("cv_x"), `{"todo_id":"td_doesnotexist"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message, got: %q", out)
	}
}


func TestTodoUpdate_ValidateInput_RequiresTodoID(t *testing.T) {
	tool := &TodoUpdate{svc: newTestService(t)}
	err := tool.ValidateInput(json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "todo_id") {
		t.Errorf("want todo_id error, got %v", err)
	}
}

func TestTodoUpdate_ValidateInput_RejectsBadStatus(t *testing.T) {
	tool := &TodoUpdate{svc: newTestService(t)}
	err := tool.ValidateInput(json.RawMessage(`{"todo_id":"td_x","status":"bogus"}`))
	if !errors.Is(err, tododomain.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestTodoUpdate_Execute_StatusToInProgressApplied(t *testing.T) {
	svc := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, todoapp.CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	out, err := (&TodoUpdate{svc: svc}).Execute(ctx,
		`{"todo_id":"`+created.ID+`","status":"in_progress"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got tododomain.Todo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if got.Status != tododomain.StatusInProgress {
		t.Errorf("Status = %q, want in_progress", got.Status)
	}
}

func TestTodoUpdate_Execute_StatusDeletedRoutesToDelete(t *testing.T) {
	svc := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, todoapp.CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	out, err := (&TodoUpdate{svc: svc}).Execute(ctx,
		`{"todo_id":"`+created.ID+`","status":"deleted"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, `"deleted":true`) {
		t.Errorf("expected deletion confirmation, got: %q", out)
	}
	// After delete, the todo should disappear from the list.
	// 删除后列表里应消失。
	todos, _ := svc.List(ctx)
	if len(todos) != 0 {
		t.Errorf("expected 0 todos after delete, got %d", len(todos))
	}
}


func TestClassifyTodoErr_KnownSentinels(t *testing.T) {
	cases := map[error]string{
		tododomain.ErrNotFound:        "not found",
		tododomain.ErrSubjectRequired: "subject is required",
		tododomain.ErrInvalidStatus:   "Invalid status",
	}
	for sentinel, fragment := range cases {
		got := classifyTodoErr(sentinel, "op")
		if !strings.Contains(got, fragment) {
			t.Errorf("classifyTodoErr(%v) = %q, want fragment %q", sentinel, got, fragment)
		}
	}
}

func TestClassifyTodoErr_UnknownErrFallsBack(t *testing.T) {
	got := classifyTodoErr(errors.New("strange"), "op")
	if !strings.Contains(got, "Todo op failed") || !strings.Contains(got, "strange") {
		t.Errorf("unexpected: %q", got)
	}
}
