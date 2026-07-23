package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// --- Schema generation ---

func TestGenerateSchemaPrimitives(t *testing.T) {
	type input struct {
		Name   string `json:"name" desc:"the name"`
		Age    int    `json:"age" desc:"the age"`
		Active bool   `json:"active"`
		Score  float64
	}
	s := GenerateSchema[input]()

	if s["type"] != "object" {
		t.Errorf("type = %v, want object", s["type"])
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties not a map: %T", s["properties"])
	}
	name, ok := props["name"].(map[string]any)
	if !ok || name["type"] != "string" {
		t.Errorf("name field wrong: %+v", props["name"])
	}
	if name["description"] != "the name" {
		t.Errorf("name description = %v, want 'the name'", name["description"])
	}
	if age := props["age"].(map[string]any); age["type"] != "integer" {
		t.Errorf("age type = %v, want integer", age["type"])
	}
	if active := props["active"].(map[string]any); active["type"] != "boolean" {
		t.Errorf("active type = %v, want boolean", active["type"])
	}
	if score := props["Score"].(map[string]any); score["type"] != "number" {
		t.Errorf("Score type = %v, want number", score["type"])
	}

	required, _ := s["required"].([]string)
	if len(required) != 4 {
		t.Errorf("required = %v, want 4 entries", required)
	}
}

func TestGenerateSchemaPointersOptional(t *testing.T) {
	type input struct {
		Required  string  `json:"required"`
		Optional  *string `json:"optional,omitempty"`
		Optional2 *int    `json:"optional2,omitempty"`
	}
	s := GenerateSchema[input]()
	required, _ := s["required"].([]string)
	if len(required) != 1 || required[0] != "required" {
		t.Errorf("required = %v, want only [required]", required)
	}
	props := s["properties"].(map[string]any)
	if opt, ok := props["optional"].(map[string]any); !ok || opt["type"] != "string" {
		t.Errorf("optional field type = %v, want string", props["optional"])
	}
}

func TestGenerateSchemaSlicesAndMaps(t *testing.T) {
	type input struct {
		Tags    []string          `json:"tags"`
		Numbers []int             `json:"numbers,omitempty"`
		Labels  map[string]string `json:"labels"`
		Meta    map[string]any    `json:"meta,omitempty"`
	}
	s := GenerateSchema[input]()
	props := s["properties"].(map[string]any)

	tags := props["tags"].(map[string]any)
	if tags["type"] != "array" {
		t.Errorf("tags type = %v, want array", tags["type"])
	}
	if items := tags["items"].(map[string]any); items["type"] != "string" {
		t.Errorf("tags items type = %v, want string", items["type"])
	}

	numbers := props["numbers"].(map[string]any)
	if items := numbers["items"].(map[string]any); items["type"] != "integer" {
		t.Errorf("numbers items type = %v, want integer", items["type"])
	}

	labels := props["labels"].(map[string]any)
	if ap, _ := labels["additionalProperties"].(map[string]any); ap == nil || ap["type"] != "string" {
		t.Errorf("labels additionalProperties wrong: %+v", labels["additionalProperties"])
	}

	meta := props["meta"].(map[string]any)
	if meta["type"] != "object" {
		t.Errorf("meta should be object: %+v", meta)
	}
}

func TestGenerateSchemaNested(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	type input struct {
		Inner inner `json:"inner"`
	}
	s := GenerateSchema[input]()
	props := s["properties"].(map[string]any)
	innerSchema := props["inner"].(map[string]any)
	if innerSchema["type"] != "object" {
		t.Errorf("inner type = %v, want object", innerSchema["type"])
	}
	innerProps := innerSchema["properties"].(map[string]any)
	if a := innerProps["a"].(map[string]any); a["type"] != "string" {
		t.Errorf("inner.a type = %v, want string", a["type"])
	}
}

func TestGenerateSchemaSkipsUnexported(t *testing.T) {
	type input struct {
		Public  string `json:"public"`
		private string
	}
	s := GenerateSchema[input]()
	props := s["properties"].(map[string]any)
	if _, exists := props["private"]; exists {
		t.Error("unexported field should be skipped")
	}
	if _, exists := props["Public"]; exists {
		t.Error("field without json tag should fall back to Go name; want 'public'")
	}
	if _, exists := props["public"]; !exists {
		t.Error("public field missing")
	}
}

func TestGenerateSchemaJSONSchemaTag(t *testing.T) {
	type input struct {
		Field string `json:"field" jsonschema:"description=from jsonschema tag"`
	}
	s := GenerateSchema[input]()
	props := s["properties"].(map[string]any)
	field := props["field"].(map[string]any)
	if field["description"] != "from jsonschema tag" {
		t.Errorf("description = %v, want 'from jsonschema tag'", field["description"])
	}
}

// --- TypedTool ---

type echoInput struct {
	Msg string `json:"msg" desc:"the message to echo"`
}

func TestTypedToolSchema(t *testing.T) {
	tool := NewTypedTool("echo", "echoes the input", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	})
	schema := tool.Schema()
	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}
	props := schema["properties"].(map[string]any)
	if msg, _ := props["msg"].(map[string]any); msg == nil || msg["description"] != "the message to echo" {
		t.Errorf("msg field description missing: %+v", props["msg"])
	}
}

func TestTypedToolExecuteSuccess(t *testing.T) {
	tool := NewTypedTool("echo", "echoes", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	})
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	var res Result[echoInput]
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Data.Msg != "hi" {
		t.Errorf("result = %+v", res)
	}
}

func TestTypedToolExecuteInvalidArgs(t *testing.T) {
	tool := NewTypedTool("echo", "echoes", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	})
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{"msg":123}`))
	var res Result[echoInput]
	_ = json.Unmarshal([]byte(out), &res)
	if res.OK {
		t.Errorf("expected failure on invalid args")
	}
	if !strings.Contains(res.Error, "invalid arguments") {
		t.Errorf("error = %q, want 'invalid arguments'", res.Error)
	}
}

func TestTypedToolExecutePanicRecovered(t *testing.T) {
	tool := NewTypedTool("panic", "panics", func(ctx context.Context, in echoInput) Result[echoInput] {
		panic("boom")
	})
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	var res Result[echoInput]
	_ = json.Unmarshal([]byte(out), &res)
	if res.OK {
		t.Errorf("expected failure on panic")
	}
	if !strings.Contains(res.Error, "panic") {
		t.Errorf("error = %q, want 'panic'", res.Error)
	}
}

func TestTypedToolExecuteHandlerError(t *testing.T) {
	tool := NewTypedTool("fail", "fails", func(ctx context.Context, in echoInput) Result[echoInput] {
		return Err[echoInput](errors.New("handler failure"))
	})
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	var res Result[echoInput]
	_ = json.Unmarshal([]byte(out), &res)
	if res.OK || res.Error != "handler failure" {
		t.Errorf("result = %+v", res)
	}
}

func TestTypedToolCapabilityMetadata(t *testing.T) {
	tool := NewTypedTool("secure", "needs command", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	}, WithCapability[echoInput, echoInput]("command"))

	if tool.Capability() != "command" {
		t.Errorf("Capability = %q", tool.Capability())
	}
}

func TestRegistryCapabilityFiltering(t *testing.T) {
	r := NewRegistry()
	r.Register(NewTypedTool("open", "anyone", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	}))
	r.Register(NewTypedTool("secure", "command only", func(ctx context.Context, in echoInput) Result[echoInput] {
		return OK(in)
	}, WithCapability[echoInput, echoInput]("command")))

	// Investigate-only agent should get the open tool but not the secure one.
	got := r.ListForCapabilities([]string{"investigate"})
	if len(got) != 1 || got[0].Name() != "open" {
		t.Errorf("investigate-only = %+v", got)
	}
	// Command agent gets both.
	got = r.ListForCapabilities([]string{"investigate", "command"})
	if len(got) != 2 {
		t.Errorf("command agent = %+v", got)
	}
	// Unrestricted (empty caps) gets everything.
	got = r.ListForCapabilities(nil)
	if len(got) != 2 {
		t.Errorf("unrestricted = %+v", got)
	}
}

func TestResultEnvelopeSerialization(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		r := OK("hello")
		out := r.String()
		var parsed Result[string]
		_ = json.Unmarshal([]byte(out), &parsed)
		if !parsed.OK || parsed.Data != "hello" {
			t.Errorf("round-trip failed: %+v", parsed)
		}
	})
	t.Run("err", func(t *testing.T) {
		r := Err[string](errors.New("oops"))
		out := r.String()
		if !strings.Contains(out, "oops") || strings.Contains(out, `"data"`) {
			t.Errorf("err envelope wrong: %s", out)
		}
	})
	t.Run("err nil is generic", func(t *testing.T) {
		r := Err[string](nil)
		if r.OK || r.Error == "" {
			t.Errorf("nil err should produce generic failure: %+v", r)
		}
	})
}
