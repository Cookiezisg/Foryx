package pagination

import (
	"errors"
	"net/http/httptest"
	"testing"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

func TestParse_Defaults(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/tools", nil)
	p, err := Parse(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Cursor != "" {
		t.Errorf("cursor: got %q, want empty", p.Cursor)
	}
	if p.Limit != DefaultLimit {
		t.Errorf("limit: got %d, want %d", p.Limit, DefaultLimit)
	}
}

func TestParse_ExplicitValues(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/tools?cursor=abc&limit=10", nil)
	p, err := Parse(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Cursor != "abc" || p.Limit != 10 {
		t.Errorf("got (%q, %d), want (abc, 10)", p.Cursor, p.Limit)
	}
}

func TestParse_LimitClamp(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/tools?limit=999999", nil)
	p, err := Parse(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Limit != MaxLimit {
		t.Errorf("limit not clamped: got %d, want %d", p.Limit, MaxLimit)
	}
}

func TestParse_InvalidLimit(t *testing.T) {
	cases := []string{"0", "-1", "abc", "1.5"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/tools?limit="+raw, nil)
			_, err := Parse(r)
			if !errors.Is(err, errorsdomain.ErrInvalidRequest) {
				t.Errorf("limit=%q: want ErrInvalidRequest, got %v", raw, err)
			}
		})
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	type cursor struct {
		ID        string `json:"id"`
		UpdatedAt int64  `json:"updatedAt"`
	}
	in := cursor{ID: "abc-123", UpdatedAt: 1711000000}

	encoded, err := EncodeCursor(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded cursor is empty")
	}

	var out cursor
	if err := DecodeCursor(encoded, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestEncode_NilReturnsEmpty(t *testing.T) {
	encoded, err := EncodeCursor(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if encoded != "" {
		t.Errorf("nil should encode to empty, got %q", encoded)
	}
}

func TestDecode_EmptyIsNoop(t *testing.T) {
	var v struct{ ID string }
	if err := DecodeCursor("", &v); err != nil {
		t.Errorf("empty cursor should decode as no-op, got %v", err)
	}
	if v.ID != "" {
		t.Errorf("target mutated on empty cursor: %+v", v)
	}
}

func TestDecode_MalformedReturnsInvalidRequest(t *testing.T) {
	var v struct{ ID string }
	err := DecodeCursor("this-is-not-base64!!!", &v)
	if !errors.Is(err, errorsdomain.ErrInvalidRequest) {
		t.Errorf("want ErrInvalidRequest, got %v", err)
	}
}

func TestDecode_ValidBase64InvalidJSON(t *testing.T) {
	var v struct{ ID string }
	err := DecodeCursor("aGVsbG8", &v)
	if !errors.Is(err, errorsdomain.ErrInvalidRequest) {
		t.Errorf("want ErrInvalidRequest, got %v", err)
	}
}
