package forge

import (
	"testing"
)

func TestParseToolCode_BasicFunction(t *testing.T) {
	code := `
def parse_csv(csv_text: str, delimiter: str = ',') -> list:
    """Parse CSV text into rows.

    Args:
        csv_text: The CSV content to parse.
        delimiter: Field separator character.

    Returns:
        List of rows, each row is a list of strings.
    """
    import csv, io
    return list(csv.reader(io.StringIO(csv_text), delimiter=delimiter))
`
	got, err := parseForgeCode("", code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FuncName != "parse_csv" {
		t.Errorf("func name: want parse_csv, got %s", got.FuncName)
	}
	if len(got.Parameters) != 2 {
		t.Fatalf("want 2 params, got %d", len(got.Parameters))
	}

	p0 := got.Parameters[0]
	if p0.Name != "csv_text" || p0.Type != "str" || !p0.Required || p0.Default != nil {
		t.Errorf("param[0]: %+v", p0)
	}
	if p0.Description != "The CSV content to parse." {
		t.Errorf("param[0] description: %q", p0.Description)
	}

	p1 := got.Parameters[1]
	if p1.Name != "delimiter" || p1.Required {
		t.Errorf("param[1] required should be false: %+v", p1)
	}
	if p1.Default == nil || *p1.Default != "','" {
		t.Errorf("param[1] default: %v", p1.Default)
	}

	if got.Return.Type != "list" {
		t.Errorf("return type: want list, got %s", got.Return.Type)
	}
	if got.Return.Description == "" {
		t.Error("return description should not be empty")
	}
}

func TestParseToolCode_NoDocstring(t *testing.T) {
	code := `
def add(a: int, b: int) -> int:
    return a + b
`
	got, err := parseForgeCode("", code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FuncName != "add" {
		t.Errorf("func name: want add, got %s", got.FuncName)
	}
	// No docstring — descriptions are empty, not an error.
	for _, p := range got.Parameters {
		if p.Description != "" {
			t.Errorf("expected empty description without docstring, got %q", p.Description)
		}
	}
	if got.Return.Type != "int" {
		t.Errorf("return type: want int, got %s", got.Return.Type)
	}
}

func TestParseToolCode_NoReturnAnnotation(t *testing.T) {
	code := `
def greet(name: str):
    return "hello " + name
`
	got, err := parseForgeCode("", code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Return.Type != "" {
		t.Errorf("expected empty return type, got %q", got.Return.Type)
	}
}

func TestParseToolCode_SyntaxError(t *testing.T) {
	_, err := parseForgeCode("", "def broken(: this is not valid python")
	if err == nil {
		t.Error("expected error for invalid Python, got nil")
	}
}

func TestParseToolCode_NoFunction(t *testing.T) {
	_, err := parseForgeCode("", "x = 1 + 2")
	if err == nil {
		t.Error("expected error when no function defined")
	}
}

func TestParseToolCode_ComplexDefaultValues(t *testing.T) {
	code := `
def process(data: list, options: dict = None, count: int = 10) -> dict:
    pass
`
	got, err := parseForgeCode("", code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Parameters) != 3 {
		t.Fatalf("want 3 params, got %d", len(got.Parameters))
	}
	if got.Parameters[0].Required != true {
		t.Error("data should be required")
	}
	if got.Parameters[1].Required != false {
		t.Error("options should not be required")
	}
}
