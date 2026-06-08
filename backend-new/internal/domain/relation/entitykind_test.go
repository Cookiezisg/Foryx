package relation

import "testing"

func TestKindForID(t *testing.T) {
	cases := []struct {
		id       string
		wantKind string
		wantOK   bool
	}{
		{"fn_a1b2c3d4e5f6a7b8", EntityKindFunction, true},
		{"hd_a1b2c3d4e5f6a7b8", EntityKindHandler, true},
		{"wf_a1b2c3d4e5f6a7b8", EntityKindWorkflow, true},
		{"ag_a1b2c3d4e5f6a7b8", EntityKindAgent, true},
		{"doc_aabbccdd11223344", EntityKindDocument, true},
		{"cv_aabbccdd11223344", EntityKindConversation, true},
		// sk_/mcp_ rule is fixed now; tables come in 波次 3, but resolution already works.
		{"sk_aabbccdd11223344", EntityKindSkill, true},
		{"mcp_aabbccdd11223344", EntityKindMCP, true},
		{"trg_aabbccdd11223344", EntityKindTrigger, true},
		{"ctl_aabbccdd11223344", EntityKindControl, true},
		{"apf_aabbccdd11223344", EntityKindApproval, true},
		// unknown prefix, an execution-record prefix (not an entity), and name/blank forms.
		{"xyz_aabbccdd11223344", "", false},
		{"fne_aabbccdd11223344", "", false},
		{"web-search", "", false},
		{"_leading", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		gotKind, gotOK := KindForID(c.id)
		if gotKind != c.wantKind || gotOK != c.wantOK {
			t.Errorf("KindForID(%q) = (%q, %v), want (%q, %v)", c.id, gotKind, gotOK, c.wantKind, c.wantOK)
		}
	}
}

func TestIsValidKind(t *testing.T) {
	for _, k := range []string{KindCreate, KindEdit, KindEquip, KindLink} {
		if !IsValidKind(k) {
			t.Errorf("IsValidKind(%q) = false, want true", k)
		}
	}
	// old vocabulary must not validate.
	for _, k := range []string{"uses", "forged", "links_to", "equip_function", ""} {
		if IsValidKind(k) {
			t.Errorf("IsValidKind(%q) = true, want false", k)
		}
	}
}

func TestIsValidEntityKind(t *testing.T) {
	all := []string{
		EntityKindFunction, EntityKindHandler, EntityKindWorkflow, EntityKindAgent,
		EntityKindDocument, EntityKindConversation, EntityKindSkill, EntityKindMCP,
		EntityKindTrigger, EntityKindControl, EntityKindApproval,
	}
	for _, k := range all {
		if !IsValidEntityKind(k) {
			t.Errorf("IsValidEntityKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"memory", "todo", "flowrun", ""} {
		if IsValidEntityKind(k) {
			t.Errorf("IsValidEntityKind(%q) = true, want false", k)
		}
	}
}
