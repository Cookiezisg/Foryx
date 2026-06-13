package mcp

import (
	"context"
	"errors"
	"time"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
)

// SetSearchNotifier wires the optional write-side search hook (bootstrap).
//
// SetSearchNotifier 接上可选的写侧搜索钩子（bootstrap）。
func (s *Service) SetSearchNotifier(n searchdomain.Notifier) { s.search = n }

func (s *Service) notifySearch(ctx context.Context, name string) {
	searchdomain.Notify(ctx, s.search, searchdomain.TypeMCP, name, "")
}

// SearchSource projects an mcp server: server card + ONE ROW PER CACHED TOOL
// (anchor = tool name) — the palette's hit unit is the callable tool. The
// encrypted config (env/headers) NEVER enters the projection: a plaintext
// index row would undo the at-rest encryption.
//
// Projection identity is the server NAME (the entity's public, workspace-unique
// key: HTTP addresses /mcp-servers/{name}, mounts resolve mcp:<name>/<tool>).
// Keying by row id once made every emitted refHint ("mcp:msv_…/tool")
// unresolvable by agent mounts — a dead wireable ref (AC-27).
//
// SearchSource 投影 mcp server：server 卡 + **每个缓存工具一行**（anchor=工具名）——
// 面板命中单元是可调用工具。加密 config（env/headers）**永不入投影**：索引明文
// 落盘等于废掉落盘加密。
//
// 投影身份用 server **NAME**（实体的公开 workspace 唯一键：HTTP 以 /mcp-servers/{name}
// 寻址、挂载解析 mcp:<name>/<tool>）。曾按行 id 键控，导致发出的 refHint
// （"mcp:msv_…/tool"）对 agent 挂载永不可解析——可接线 ref 物理死亡（AC-27）。
func (s *Service) SearchSource() *SearchSource { return &SearchSource{svc: s} }

type SearchSource struct{ svc *Service }

func (ss *SearchSource) Type() searchdomain.EntityType { return searchdomain.TypeMCP }

func (ss *SearchSource) Stamps(ctx context.Context) (map[string]time.Time, error) {
	srvs, err := ss.svc.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(srvs))
	for _, srv := range srvs {
		out[srv.Name] = srv.UpdatedAt
	}
	return out, nil
}

func (ss *SearchSource) Docs(ctx context.Context, name string) ([]searchdomain.SourceDoc, error) {
	srv, err := ss.svc.repo.GetByName(ctx, name)
	if errors.Is(err, mcpdomain.ErrServerNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	docs := []searchdomain.SourceDoc{{
		ChunkNo:   0,
		Title:     srv.Name,
		Body:      searchdomain.CapRunes(srv.Description + "\n" + srv.Transport + " " + srv.Command),
		UpdatedAt: srv.UpdatedAt,
	}}
	// Tools come from the live connection cache: a disconnected server keeps
	// its card searchable, its tools reappear on reconnect (connectOne notifies).
	// 工具来自活连接缓存：断开的 server 卡片仍可搜，工具在重连时回来（connectOne 通知）。
	statuses, err := ss.svc.ListServers(ctx)
	if err == nil {
		for _, st := range statuses {
			if st.ID != srv.ID {
				continue
			}
			for i, tool := range st.Tools {
				docs = append(docs, searchdomain.SourceDoc{
					ChunkNo:   i + 1,
					Anchor:    tool.Name,
					Title:     srv.Name + "/" + tool.Name,
					Body:      searchdomain.CapRunes(tool.Description),
					UpdatedAt: srv.UpdatedAt,
				})
			}
		}
	}
	return docs, nil
}
