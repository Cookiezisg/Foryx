package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWithProgress_RoundTrip(t *testing.T) {
	if progressFrom(context.Background()) != nil {
		t.Fatal("no sink set → progressFrom must be nil")
	}
	got := ""
	ctx := WithProgress(context.Background(), func(s string) { got = s })
	if sink := progressFrom(ctx); sink == nil {
		t.Fatal("WithProgress sink not retrievable")
	} else {
		sink("x")
	}
	if got != "x" {
		t.Fatalf("sink not the one set: %q", got)
	}
	// nil sink is a no-op wrap.
	if progressFrom(WithProgress(context.Background(), nil)) != nil {
		t.Fatal("WithProgress(nil) must not set a sink")
	}
}

// TestOnProgress_RoutesByToken: a server progress notification reaches the CallTool that registered
// its token, and an unknown token is dropped (no panic).
//
// TestOnProgress_RoutesByToken：server 进度通知到达登记了该 token 的 CallTool，未知 token 丢弃（不 panic）。
func TestOnProgress_RoutesByToken(t *testing.T) {
	c := &client{}
	var got []string
	c.progress.Store("7", func(s string) { got = append(got, s) })

	c.onProgress(context.Background(), &mcpsdk.ProgressNotificationClientRequest{
		Params: &mcpsdk.ProgressNotificationParams{ProgressToken: "7", Message: "indexing", Progress: 3, Total: 10},
	})
	// unknown token → dropped, no panic.
	c.onProgress(context.Background(), &mcpsdk.ProgressNotificationClientRequest{
		Params: &mcpsdk.ProgressNotificationParams{ProgressToken: "999", Message: "stray"},
	})

	if len(got) != 1 || !strings.Contains(got[0], "indexing") || !strings.Contains(got[0], "3/10") {
		t.Fatalf("progress not routed/formatted to the registered sink: %v", got)
	}
}
