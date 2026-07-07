package schema

import (
	"strings"
	"testing"
)

func TestShorthandExpansion(t *testing.T) {
	s, err := Parse(`{"name": "str", "count": "int", "active": "bool"}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	t.Run("valid output", func(t *testing.T) {
		if err := s.Validate([]byte(`{"name": "Alice", "count": 42, "active": true}`)); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		if err := s.Validate([]byte(`{"name": "Alice"}`)); err == nil {
			t.Error("Validate() = nil, want error for missing field")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		if err := s.Validate([]byte(`{"name": 123, "count": 42, "active": true}`)); err == nil {
			t.Error("Validate() = nil, want error for wrong type")
		}
	})
}

func TestShorthandDate(t *testing.T) {
	s, err := Parse(`{"published": "date"}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if err := s.Validate([]byte(`{"published": "2024-03-05"}`)); err != nil {
		t.Errorf("Validate(valid date) = %v, want nil", err)
	}
	if err := s.Validate([]byte(`{"published": "not-a-date"}`)); err == nil {
		t.Error("Validate(invalid date) = nil, want error")
	}
}

func TestShorthandNestedObject(t *testing.T) {
	s, err := Parse(`{"title": "str", "author": {"name": "str", "age": "int"}}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if err := s.Validate([]byte(`{"title": "Go", "author": {"name": "Alice", "age": 30}}`)); err != nil {
		t.Errorf("Validate(valid nested) = %v, want nil", err)
	}
	if err := s.Validate([]byte(`{"title": "Go", "author": {"name": "Alice"}}`)); err == nil {
		t.Error("Validate(nested missing required) = nil, want error")
	}
	if err := s.Validate([]byte(`{"title": "Go", "author": {"name": 1, "age": 30}}`)); err == nil {
		t.Error("Validate(nested wrong type) = nil, want error")
	}
}

func TestNestedObject(t *testing.T) {
	// Full JSON Schema with a nested object passes through unchanged.
	s, err := Parse(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"address": {
				"type": "object",
				"properties": {"city": {"type": "string"}},
				"required": ["city"]
			}
		},
		"required": ["name", "address"]
	}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	valid := []byte(`{"name": "Alice", "address": {"city": "Paris"}}`)
	if err := s.Validate(valid); err != nil {
		t.Errorf("Validate(nested) = %v, want nil", err)
	}

	invalid := []byte(`{"name": "Alice", "address": {"city": 42}}`)
	if err := s.Validate(invalid); err == nil {
		t.Error("Validate(nested wrong type) = nil, want error")
	}
}

func TestNilValidate(t *testing.T) {
	var s *Schema
	if err := s.Validate([]byte(`{}`)); err != nil {
		t.Errorf("nil Schema.Validate() = %v, want nil", err)
	}
}

func TestInvalidInput(t *testing.T) {
	if _, err := Parse("not json"); err == nil {
		t.Error("Parse(\"not json\") = nil, want error")
	}
}

func TestResolveType(t *testing.T) {
	// All shorthand aliases must map to correct JSON Schema types.
	cases := []struct{ in, wantType, wantFormat string }{
		{"str", "string", ""}, {"string", "string", ""},
		{"int", "integer", ""}, {"integer", "integer", ""},
		{"float", "number", ""}, {"number", "number", ""},
		{"bool", "boolean", ""}, {"boolean", "boolean", ""},
		{"date", "string", "date"},
		{"array", "array", ""}, {"list", "array", ""},
		{"object", "object", ""}, {"map", "object", ""},
	}
	for _, tc := range cases {
		got, err := resolveType(tc.in)
		if err != nil {
			t.Errorf("resolveType(%q) error: %v", tc.in, err)
			continue
		}
		if got["type"] != tc.wantType {
			t.Errorf("resolveType(%q) type = %v, want %q", tc.in, got["type"], tc.wantType)
		}
		if tc.wantFormat != "" && got["format"] != tc.wantFormat {
			t.Errorf("resolveType(%q) format = %v, want %q", tc.in, got["format"], tc.wantFormat)
		}
	}
}

func TestResolveType_unknown(t *testing.T) {
	_, err := resolveType("uuid")
	if err == nil {
		t.Error("resolveType(unknown) = nil, want error")
	}
}

func TestIsFullSchema(t *testing.T) {
	// A map with structural keywords is detected as full JSON Schema.
	full := map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	if !isFullSchema(full) {
		t.Error("isFullSchema({type:object,properties:...}) = false, want true")
	}
	// A map with only string values is shorthand.
	shorthand := map[string]interface{}{"name": "str", "count": "int"}
	if isFullSchema(shorthand) {
		t.Error("isFullSchema({name:str,count:int}) = true, want false")
	}
}

func TestJSONSchemaString(t *testing.T) {
	s, err := Parse(`{"name": "str"}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	got := s.JSONSchemaString()
	if got == "" {
		t.Error("JSONSchemaString() = empty, want non-empty JSON")
	}
	// Must be valid JSON containing the property.
	if !strings.Contains(got, "name") {
		t.Errorf("JSONSchemaString() missing property: %q", got)
	}
}

func TestValidate_invalidJSON(t *testing.T) {
	s, err := Parse(`{"name": "str"}`)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if err := s.Validate([]byte("not json")); err == nil {
		t.Error("Validate(non-JSON) = nil, want error")
	}
}
