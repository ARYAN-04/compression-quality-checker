package mcp

import (
	"context"
	"slices"
	"testing"

	"can-i-compress-it/internal/domain"
)

func sampleTools() []domain.ToolDefinition {
	full := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "description": "The identifier of the record."},
			"verbose_opt": map[string]any{"type": "boolean", "description": "Optional verbose output toggle."},
		},
		"required": []any{"id"},
	}
	return []domain.ToolDefinition{{
		Name:        "get_record",
		Description: "Fetches a record. Returns the full payload for the given id.",
		Parameters:  full,
	}}
}

func TestCompressNoneKeepsEverything(t *testing.T) {
	out := compressTools(domain.LevelNone, sampleTools())
	if len(out) != 1 || out[0].Description == "" {
		t.Fatal("none level must preserve descriptions")
	}
}

func TestCompressHighStripsDescriptions(t *testing.T) {
	out := compressTools(domain.LevelHigh, sampleTools())
	if out[0].Description != "" {
		t.Fatal("high must strip tool description")
	}
	props := out[0].Parameters["properties"].(map[string]any)
	id := props["id"].(map[string]any)
	if _, has := id["description"]; has {
		t.Fatal("high must strip parameter descriptions")
	}
}

func TestCompressMaxReplacesWithMetaTools(t *testing.T) {
	out := compressTools(domain.LevelMax, sampleTools())
	names := []string{}
	for _, tool := range out {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, MetaGetSchema) || !slices.Contains(names, MetaInvoke) {
		t.Fatalf("max should expose meta-tools only, got %v", names)
	}
	if slices.Contains(names, "get_record") {
		t.Fatal("max must hide original tools")
	}
}

func TestBuiltinMaxMetaFlow(t *testing.T) {
	p := newBuiltinProxy(domain.LevelMax, sampleTools()).(*builtinProxy)
	ctx := context.Background()
	schema, err := p.Call(ctx, domain.ToolCall{
		Name:      MetaGetSchema,
		Arguments: map[string]any{"tool_name": "get_record"},
	})
	if err != nil {
		t.Fatalf("get_tool_schema: %v", err)
	}
	if _, ok := schema.(map[string]any); !ok {
		t.Fatal("schema result must be a map")
	}
	if _, err := p.Call(ctx, domain.ToolCall{
		Name:      MetaInvoke,
		Arguments: map[string]any{"tool_name": "get_record", "arguments": map[string]any{"id": "r1"}},
	}); err != nil {
		t.Fatalf("invoke_tool: %v", err)
	}
	if _, err := p.Call(ctx, domain.ToolCall{Name: "get_record"}); err == nil {
		t.Fatal("direct call at max must be rejected")
	}
	if len(p.calls) != 1 || p.calls[0].Name != "get_record" {
		t.Fatalf("expected one unwrapped underlying call, got %+v", p.calls)
	}
}
