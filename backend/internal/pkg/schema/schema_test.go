package schema

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestFromJSONSchema covers the MCP inputSchema → flat []Field projection: a normal object
// schema with two typed properties (sorted by name, descriptions carried), and the empty /
// invalid inputs that must collapse to nil. An unrecognized property type coarsens to object.
//
// TestFromJSONSchema 覆盖 MCP inputSchema → 扁平 []Field 投影：含两个有类型属性的正常 object
// schema（按 name 排序、带描述），以及必须坍缩为 nil 的空 / 非法输入。未识别的属性类型粗化为 object。
func TestFromJSONSchema(t *testing.T) {
	t.Run("normal object with two typed properties", func(t *testing.T) {
		raw := json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "search text"},
				"limit": {"type": "number"}
			}
		}`)
		got := FromJSONSchema(raw)
		want := []Field{
			{Name: "limit", Type: TypeNumber},
			{Name: "query", Type: TypeString, Description: "search text"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("FromJSONSchema mismatch:\n got = %#v\nwant = %#v", got, want)
		}
	})

	t.Run("unrecognized type coarsens to object", func(t *testing.T) {
		// "integer" is now a recognized alias (→ number, F5); use a genuinely-unknown type here.
		raw := json.RawMessage(`{"properties": {"blob": {"type": "frobnicate"}}}`)
		got := FromJSONSchema(raw)
		want := []Field{{Name: "blob", Type: TypeObject}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("FromJSONSchema mismatch:\n got = %#v\nwant = %#v", got, want)
		}
	})

	t.Run("empty and invalid inputs return nil", func(t *testing.T) {
		cases := map[string]json.RawMessage{
			"no properties": json.RawMessage(`{"type": "object"}`),
			"empty props":   json.RawMessage(`{"properties": {}}`),
			"not json":      json.RawMessage(`not json`),
			"empty bytes":   json.RawMessage(``),
		}
		for name, raw := range cases {
			if got := FromJSONSchema(raw); got != nil {
				t.Errorf("%s: expected nil, got %#v", name, got)
			}
		}
	})
}
