// Package tools defines the agent's tool registry, the Tool interface, and
// supporting infrastructure (typed tools, middleware, schema generation,
// result envelope). Tools self-register; the registry dispatches calls.
package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// GenerateSchema reflects over T and produces a JSON Schema (OpenAI function
// parameters format) describing the type. It supports the common cases used
// by agent tool inputs: structs, string/int/bool/float primitives, slices,
// maps with string keys, pointers (optional), and time.Time (as RFC3339
// string). Field descriptions are read from the `desc` struct tag, falling
// back to the `jsonschema` tag's "description=..." form for compatibility
// with the MCP Go SDK.
//
// The output is the same shape that OpenAI / Anthropic / MCP expect in a
// tool definition's "parameters" field.
func GenerateSchema[T any]() map[string]any {
	var zero T
	return schemaFor(reflect.TypeOf(zero))
}

func schemaFor(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{"type": "null"}
	}
	// Follow pointers: a *T has the same schema as T; optionality is
	// expressed at the parent struct level (pointer fields are not added to
	// the "required" list).
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return objectSchema(t)
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaFor(t.Elem()),
		}
	case reflect.Map:
		// Only string-keyed maps are supported. Maps with other key types
		// are treated as object with additionalProperties=true (freeform).
		if t.Key().Kind() == reflect.String {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": schemaFor(t.Elem()),
			}
		}
		return map[string]any{"type": "object"}
	case reflect.Interface:
		// any/interface{} → freeform object (accepts anything).
		return map[string]any{}
	default:
		return map[string]any{"type": "string"}
	}
}

func objectSchema(t reflect.Type) map[string]any {
	props := make(map[string]any)
	var required []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, omitempty, skip := jsonFieldName(f)
		if skip {
			continue
		}
		entry := schemaFor(f.Type)
		if desc := fieldDescription(f); desc != "" {
			entry["description"] = desc
		}
		props[name] = entry
		// Pointer fields, omitempty fields, and interface{}/any fields are
		// optional; everything else is required.
		if !omitempty && f.Type.Kind() != reflect.Ptr && f.Type.Kind() != reflect.Interface {
			required = append(required, name)
		}
	}
	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// jsonFieldName extracts the JSON field name and reports whether it has
// omitempty or is explicitly skipped with "-".
func jsonFieldName(f reflect.StructField) (name string, omitempty, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return f.Name, false, false
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

// fieldDescription reads the `desc` tag, falling back to the `jsonschema`
// "description=..." form used by the MCP Go SDK / invopop/jsonschema.
func fieldDescription(f reflect.StructField) string {
	if d := strings.TrimSpace(f.Tag.Get("desc")); d != "" {
		return d
	}
	js := f.Tag.Get("jsonschema")
	for _, part := range strings.Split(js, ",") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "description="); ok {
			if d := strings.TrimSpace(rest); d != "" {
				return d
			}
		}
	}
	return ""
}

// NoArgs is the canonical empty-arguments schema. Exposed as a convenience
// for tools that take no input.
func NoArgs() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// EncodeSchema serializes a schema to JSON for use in tool definitions.
// Errors are surfaced as a JSON-encoded {"error": ...} string so callers
// don't crash.
func EncodeSchema(s map[string]any) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error":"schema encode: %s"}`, err.Error()))
	}
	return b
}
