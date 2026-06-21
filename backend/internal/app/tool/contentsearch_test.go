package tool

import "testing"

// TestSlimPageResult_DisclosesTruncation pins F175-M4: a vertical search tool's result must carry the
// full `total` (not just the returned `count`) and signal nextCursor/hasMore when more results exist,
// so the LLM can't read a 20-item list as "exactly 20 exist". A non-truncated result omits hasMore.
//
// TestSlimPageResult_DisclosesTruncation 锁 F175-M4：垂搜工具结果须带全量 `total`（非仅返回 `count`）
// 并在有更多时示意 nextCursor/hasMore，使 LLM 不把 20 条读成「恰有 20 条」。未截断则不报 hasMore。
func TestSlimPageResult_DisclosesTruncation(t *testing.T) {
	// truncated: 20 returned of 47 total, more pages exist.
	r := SlimPageResult(20, 47, "cursor_xyz", "functions", []string{})
	if r["count"] != 20 || r["total"] != 47 {
		t.Fatalf("count/total wrong: %+v", r)
	}
	if r["nextCursor"] != "cursor_xyz" || r["hasMore"] != true {
		t.Fatalf("truncated result must carry nextCursor+hasMore: %+v", r)
	}
	if _, ok := r["functions"]; !ok {
		t.Fatalf("list must live under its key: %+v", r)
	}

	// not truncated: no cursor → no hasMore / nextCursor keys, total still present.
	r2 := SlimPageResult(3, 3, "", "functions", []string{})
	if r2["total"] != 3 {
		t.Fatalf("total wrong: %+v", r2)
	}
	if _, ok := r2["hasMore"]; ok {
		t.Fatalf("non-truncated result must not claim hasMore: %+v", r2)
	}
	if _, ok := r2["nextCursor"]; ok {
		t.Fatalf("non-truncated result must omit nextCursor: %+v", r2)
	}
}
