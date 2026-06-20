package mcp

import (
	"testing"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

// TestRemoteTransportOrder pins the fallback order: the declared transport first, then the other —
// and a blank/unknown label tries streamable-http (the modern default) before SSE. This is what lets
// a server with a stale "sse" registry label still connect over streamable-http (F85).
//
// TestRemoteTransportOrder 钉死 fallback 顺序：先声明的、再另一个；空/未知标签先试 streamable-http（现代默认）
// 再 SSE——这正是让 registry 标签陈旧（标 sse 实为 streamable）的 server 仍连得上的关键（F85）。
func TestRemoteTransportOrder(t *testing.T) {
	cases := []struct {
		declared string
		want     []string
	}{
		{mcpdomain.TransportSSE, []string{mcpdomain.TransportSSE, mcpdomain.TransportStreamableHTTP}},
		{mcpdomain.TransportStreamableHTTP, []string{mcpdomain.TransportStreamableHTTP, mcpdomain.TransportSSE}},
		{"", []string{mcpdomain.TransportStreamableHTTP, mcpdomain.TransportSSE}},
	}
	for _, c := range cases {
		got := remoteTransportOrder(c.declared)
		if len(got) != 2 || got[0] != c.want[0] || got[1] != c.want[1] {
			t.Errorf("remoteTransportOrder(%q) = %v, want %v", c.declared, got, c.want)
		}
	}
}
