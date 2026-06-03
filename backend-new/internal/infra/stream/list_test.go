package stream

import (
	"context"
	"testing"
)

func TestListReturnsDurable(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	b.Publish(ctx, durableEvent())
	b.Publish(ctx, durableEvent())
	got, more, err := b.List(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || more {
		t.Errorf("List = %d items, more=%v; want 2, false", len(got), more)
	}
}

func TestListPagination(t *testing.T) {
	b := New(16)
	ctx := wsCtx("ws1")
	for i := 0; i < 5; i++ {
		b.Publish(ctx, durableEvent())
	}
	got, more, _ := b.List(ctx, 0, 2)
	if len(got) != 2 || !more {
		t.Fatalf("page1 = %d items, more=%v; want 2, true", len(got), more)
	}
	last := got[len(got)-1].Seq
	got2, more2, _ := b.List(ctx, last, 10)
	if len(got2) != 3 || more2 {
		t.Errorf("page2 = %d items, more=%v; want 3, false", len(got2), more2)
	}
}

func TestListEmptyWorkspace(t *testing.T) {
	b := New(16)
	got, more, err := b.List(wsCtx("never-published"), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || more {
		t.Errorf("empty ws List = %d items, more=%v; want 0, false", len(got), more)
	}
}

func TestListRequiresWorkspace(t *testing.T) {
	b := New(16)
	if _, _, err := b.List(context.Background(), 0, 10); err == nil {
		t.Error("List with no workspace: want error, got nil")
	}
}
