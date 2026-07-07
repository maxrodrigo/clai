// Package schema handles JSON Schema parsing, validation, and shorthand expansion.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Schema holds a compiled JSON Schema used to validate model output.
type Schema struct {
	raw      string
	compiled *jsonschema.Schema
	expanded map[string]interface{} // for JSONSchemaString()
}

// Parse parses a schema string. Accepts:
//   - JSON Schema object (any valid JSON Schema)
//   - Shorthand: {"field": "type"} where types are
//     str/int/float/bool/date/array/object, or a nested {"field": "type"} map
//     for object fields.
func Parse(input string) (*Schema, error) {
	if input == "" {
		return nil, nil
	}

	input = strings.TrimSpace(input)

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	expanded, err := expandSchema(raw)
	if err != nil {
		return nil, err
	}

	// Compile for full validation.
	schemaBytes, err := json.Marshal(expanded)
	if err != nil {
		// Should never happen: expanded contains only primitives.
		return nil, fmt.Errorf("marshaling expanded schema: %w", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	// Enforce "format" keywords (e.g. date) as assertions rather than
	// annotations, so the "date" shorthand actually validates output.
	compiler.AssertFormat()
	if err := compiler.AddResource("schema.json", doc); err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}

	return &Schema{raw: input, compiled: compiled, expanded: expanded}, nil
}

// expandSchema converts shorthand type names to a proper JSON Schema object.
//
// Shorthand format: every value is either a type name string or a nested
// shorthand map describing an object field.
//
//	{"name": "str", "count": "int", "author": {"name": "str"}}
//
// Full JSON Schema: passed through unchanged. Detected when structural
// keywords ($schema, properties, anyOf, ...) are present or the top-level
// "type" value is a real JSON Schema type. This avoids the ambiguity of a
// shorthand field named "type" being mistaken for the JSON Schema keyword.
func expandSchema(raw map[string]interface{}) (map[string]interface{}, error) {
	if isFullSchema(raw) {
		return raw, nil
	}

	properties := make(map[string]interface{}, len(raw))
	required := make([]string, 0, len(raw))

	for field, val := range raw {
		prop, err := expandField(val)
		if err != nil {
			return nil, fmt.Errorf("schema field %q: %w", field, err)
		}
		properties[field] = prop
		required = append(required, field)
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}, nil
}

// expandField expands a single shorthand value into a JSON Schema fragment.
// A string is a type alias; a map is a nested object schema.
func expandField(val interface{}) (map[string]interface{}, error) {
	switch v := val.(type) {
	case string:
		return resolveType(v)
	case map[string]interface{}:
		return expandSchema(v)
	default:
		return nil, fmt.Errorf("expected a type string or nested object, got %T", val)
	}
}

// isFullSchema reports whether raw looks like a proper JSON Schema object
// rather than a shorthand field→type map. We detect structural keywords and
// also check if the "type" value (when present) is a valid JSON Schema type
// string rather than a user-defined field name.
func isFullSchema(raw map[string]interface{}) bool {
	// These keywords only appear in JSON Schema, not as shorthand field names.
	structuralKeywords := []string{
		"$schema", "properties", "anyOf", "allOf", "oneOf",
		"$ref", "if", "then", "else", "not", "items", "enum", "const",
	}
	for _, kw := range structuralKeywords {
		if _, ok := raw[kw]; ok {
			return true
		}
	}
	// If "type" is present and its value is a valid JSON Schema type string
	// (not a shorthand alias like "str"), treat the whole thing as a full schema.
	if t, ok := raw["type"]; ok {
		if s, isStr := t.(string); isStr {
			switch s {
			case "object", "array", "string", "number", "integer", "boolean", "null":
				return true
			}
		}
	}
	return false
}

// resolveType maps a shorthand type name to a JSON Schema fragment.
func resolveType(t string) (map[string]interface{}, error) {
	switch strings.ToLower(t) {
	case "str", "string":
		return map[string]interface{}{"type": "string"}, nil
	case "int", "integer":
		return map[string]interface{}{"type": "integer"}, nil
	case "float", "number":
		return map[string]interface{}{"type": "number"}, nil
	case "bool", "boolean":
		return map[string]interface{}{"type": "boolean"}, nil
	case "date":
		return map[string]interface{}{"type": "string", "format": "date"}, nil
	case "array", "list":
		return map[string]interface{}{"type": "array"}, nil
	case "object", "map":
		return map[string]interface{}{"type": "object"}, nil
	default:
		return nil, fmt.Errorf("unknown type %q (use: str, int, float, bool, date, array, object)", t)
	}
}

// ValidationError indicates model output failed schema validation.
// Callers can use errors.As to detect this and map it to exit code 3.
type ValidationError struct {
	Detail string
}

func (e *ValidationError) Error() string { return e.Detail }

// Validate checks that output is valid JSON conforming to the schema using
// the full JSON Schema validator.
func (s *Schema) Validate(output []byte) error {
	if s == nil {
		return nil
	}

	var v interface{}
	d := json.NewDecoder(bytes.NewReader(output))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return &ValidationError{Detail: fmt.Sprintf("output is not valid JSON: %v", err)}
	}

	if err := s.compiled.Validate(v); err != nil {
		var vErr *jsonschema.ValidationError
		if errors.As(err, &vErr) && vErr != nil {
			return &ValidationError{Detail: formatValidationError(vErr)}
		}
		return &ValidationError{Detail: err.Error()}
	}
	return nil
}

// formatValidationError formats a validation error with field paths.
func formatValidationError(vErr *jsonschema.ValidationError) string {
	var msgs []string
	collectErrors(vErr, &msgs)
	if len(msgs) == 0 {
		return vErr.Error()
	}
	if len(msgs) == 1 {
		return msgs[0]
	}
	return strings.Join(msgs, "; ")
}

// collectErrors recursively collects error messages with field paths.
func collectErrors(vErr *jsonschema.ValidationError, msgs *[]string) {
	if vErr.ErrorKind != nil {
		path := "/"
		if len(vErr.InstanceLocation) > 0 {
			path = "/" + strings.Join(vErr.InstanceLocation, "/")
		}
		*msgs = append(*msgs, fmt.Sprintf("%s: %v", path, vErr.ErrorKind))
	}
	for _, cause := range vErr.Causes {
		collectErrors(cause, msgs)
	}
}

// SystemInstruction returns a system prompt fragment instructing the model to
// produce JSON matching this schema. Returns "" if the schema is nil.
func (s *Schema) SystemInstruction() string {
	if s == nil {
		return ""
	}
	return "Respond with valid JSON matching this schema: " + s.JSONSchemaString()
}

// JSONSchemaString returns the expanded JSON Schema as a compact JSON string
// for inclusion in the system prompt. Returns "" if the schema is nil.
func (s *Schema) JSONSchemaString() string {
	if s == nil {
		return ""
	}
	data, err := json.Marshal(s.expanded)
	if err != nil {
		// expanded contains only primitives from expandSchema; this should never happen.
		panic(fmt.Sprintf("schema: marshal expanded schema: %v", err))
	}
	return string(data)
}
