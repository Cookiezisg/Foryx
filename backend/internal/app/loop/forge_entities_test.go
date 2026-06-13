package loop

import (
	"context"
	"strings"
	"testing"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// TestStreamLLM_ForgeDoubleWritesToEntities: a forge tool_call's streaming args are mirrored onto
// the entities stream as a forge node (scope = a forge session keyed by the tool_call id), so the
// entity panel fills in live. The entities bridge is the only one seeded here, isolating the
// double-write from the messages emitter.
//
// TestStreamLLM_ForgeDoubleWritesToEntities：forge tool_call 的流式 args 被镜像到 entities 流，成 forge
// 节点（scope=以 tool_call id 为键的 forge 会话），使实体面板实时填充。此处只种 entities bridge，把双写与
// messages emitter 隔离。
func TestStreamLLM_ForgeDoubleWritesToEntities(t *testing.T) {
	ent := &captureBridge{}
	ctx := entitystreamapp.WithBridge(context.Background(), ent)
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{{
		toolStartEv(0, "tc1", "create_function"),
		toolDeltaEv(0, `{"ops":[{"op":"set_code",`),
		toolDeltaEv(0, `"code":"def f(): pass"}]}`),
		finishEv(),
	}}}
	forgeOf := func(name string) (toolapp.ForgeSpec, bool) {
		if name == "create_function" {
			return toolapp.ForgeSpec{Kind: "function", Op: "create"}, true
		}
		return toolapp.ForgeSpec{}, false
	}

	streamLLM(ctx, client, llminfra.Request{}, forgeOf, nil)

	if len(ent.events) != 4 {
		t.Fatalf("want 4 entities frames (open + 2 delta + close), got %d: %+v", len(ent.events), ent.events)
	}
	open, ok := ent.events[0].Frame.(streamdomain.Open)
	if !ok || open.Node.Type != entitystreamapp.NodeForge {
		t.Fatalf("frame[0] not a forge Open: %+v", ent.events[0])
	}
	// The forge session is keyed by the SERVER-minted tool_call block id (blk_), never the
	// provider's wire id — providers recycle wire ids, which cannot key anything durable.
	// forge 会话以服务端铸造的 tool_call 块 id（blk_）为键、绝非 provider 线缆 id——provider 会
	// 复用线缆 id，不能作任何持久键。
	if ent.events[0].Scope.Kind != streamdomain.KindFunction || !strings.HasPrefix(ent.events[0].Scope.ID, "blk_") {
		t.Fatalf("forge not scoped to a blk_-keyed function forge session: %+v", ent.events[0].Scope)
	}
	for i := 1; i < 4; i++ {
		if ent.events[i].Scope.ID != ent.events[0].Scope.ID {
			t.Fatalf("frame[%d] scope drifted from the forge session: %+v", i, ent.events[i].Scope)
		}
	}
	if !strings.Contains(string(open.Node.Content), `"create"`) {
		t.Fatalf("open content missing op=create: %s", open.Node.Content)
	}
	cl, ok := ent.events[3].Frame.(streamdomain.Close)
	if !ok || cl.Result == nil || !strings.Contains(string(cl.Result.Content), "def f()") {
		t.Fatalf("close result missing the final args snapshot: %+v", ent.events[3])
	}
}

// TestStreamLLM_NonForgeToolNoEntities: a non-forge tool_call emits nothing on the entities stream.
//
// TestStreamLLM_NonForgeToolNoEntities：非 forge tool_call 不在 entities 流发任何帧。
func TestStreamLLM_NonForgeToolNoEntities(t *testing.T) {
	ent := &captureBridge{}
	ctx := entitystreamapp.WithBridge(context.Background(), ent)
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{{
		toolStartEv(0, "tc1", "Read"),
		toolDeltaEv(0, `{"path":"/x"}`),
		finishEv(),
	}}}
	noForge := func(string) (toolapp.ForgeSpec, bool) { return toolapp.ForgeSpec{}, false }

	streamLLM(ctx, client, llminfra.Request{}, noForge, nil)

	if len(ent.events) != 0 {
		t.Fatalf("non-forge tool must not emit entities frames, got %d", len(ent.events))
	}
}
