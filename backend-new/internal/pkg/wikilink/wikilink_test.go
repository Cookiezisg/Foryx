package wikilink

import (
	"sort"
	"testing"
)

func TestParse_EmptyBody(t *testing.T) {
	if got := Parse(""); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParse_NoMatch(t *testing.T) {
	body := "Just plain markdown — no wikilinks here."
	if got := Parse(body); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParse_SingleWikilink(t *testing.T) {
	body := "See [[fn_a1b2c3d4e5f6a7b8]] for details."
	got := Parse(body)
	if len(got) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(got))
	}
	if got[0].ID != "fn_a1b2c3d4e5f6a7b8" || got[0].Count != 1 {
		t.Errorf("got %+v", got[0])
	}
}

func TestParse_DedupAndCount(t *testing.T) {
	body := `First mention [[fn_a1b2c3d4e5f6a7b8]].
Second mention [[fn_a1b2c3d4e5f6a7b8]] again.
And once: [[wf_11223344aabbccdd]].`
	got := Parse(body)
	if len(got) != 2 {
		t.Fatalf("expected 2 dedup'd refs, got %d", len(got))
	}
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	if got[0].ID != "fn_a1b2c3d4e5f6a7b8" || got[0].Count != 2 {
		t.Errorf("expected fn count=2, got %+v", got[0])
	}
	if got[1].ID != "wf_11223344aabbccdd" || got[1].Count != 1 {
		t.Errorf("expected wf count=1, got %+v", got[1])
	}
}

func TestParse_ReturnsAllIdShapedTokens(t *testing.T) {
	// wikilink does NOT filter by known prefix — that's relation's job. Any
	// [[<prefix>_<16hex>]] token is returned, even an unknown prefix.
	//
	// wikilink 不按已知前缀过滤——那是 relation 的活。任何 [[<prefix>_<16hex>]]
	// token 都返回，即使前缀未知。
	body := "Unknown [[xyz_a1b2c3d4e5f6a7b8]] and known [[doc_aabbccdd11223344]]"
	got := Parse(body)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs (no prefix filtering), got %d: %+v", len(got), got)
	}
}

func TestParse_MalformedSkipped(t *testing.T) {
	// Not 16-hex chars / wrong format / nested brackets — all should not match
	body := "[[fn_short]] [[fn_a1b2c3d4e5f6a7b8z]] [fn_aabbccdd11223344] [[FN_aabbccdd11223344]]"
	got := Parse(body)
	if got != nil {
		t.Errorf("expected no matches for malformed/wrong-case, got %+v", got)
	}
}

func TestParse_NonIDShapedNotMatched(t *testing.T) {
	// Only <prefix>_<16hex> tokens match. A human-readable name (a skill's display
	// name, an mcp server name) is not ID-shaped, so it is not a wikilink — to tag
	// such an entity you reference its id; see TestParse_SkillMcpIDsMatched.
	//
	// 只有 <前缀>_<16hex> 形态匹配。人类可读名字（skill 显示名、mcp server 名）非 ID 形态，
	// 故不是 wikilink——要 tag 这种实体得引用其 id；见 TestParse_SkillMcpIDsMatched。
	body := "[[csv_parse]] is a skill; [[postgres]] is an mcp server."
	if got := Parse(body); got != nil {
		t.Errorf("expected no matches for name-shaped refs, got %+v", got)
	}
}

func TestParse_SkillMcpIDsMatched(t *testing.T) {
	// skill/mcp ids use the sk_/mcp_ prefixes (fixed as the rule for the coming
	// unified id system); they are ID-shaped and ARE extracted, so a document can
	// [[tag]] a skill or mcp by id.
	//
	// skill/mcp 的 id 用 sk_/mcp_ 前缀（为即将统一的 id 体系定下的规矩）；它们是 ID 形态、
	// 会被抽取，故 document 可按 id [[tag]] 一个 skill 或 mcp。
	body := "[[sk_a1b2c3d4e5f6a7b8]] and [[mcp_aabbccdd11223344]]"
	got := Parse(body)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs, got %d: %+v", len(got), got)
	}
}
