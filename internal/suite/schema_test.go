package suite

import (
	"path/filepath"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestEmbeddedSchemasLoadAndValidate(t *testing.T) {
	names := EmbeddedSchemaNames()
	if len(names) != 11 {
		t.Fatalf("expected 11 embedded schemas, got %d", len(names))
	}

	expectedCounts := map[string]int{
		"github":       7,
		"jira":         5,
		"slack":        5,
		"sqlite":       4,
		"filesystem":   7,
		"postgres":     5,
		"memory":       5,
		"fetch":        3,
		"brave-search": 2,
		"puppeteer":    5,
		"git":          5,
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			raw, ok := EmbeddedSchema(name)
			if !ok || len(raw) == 0 {
				t.Fatalf("failed to load embedded schema %s", name)
			}

			tools, ok := EmbeddedSchemaTools(name)
			if !ok {
				t.Fatalf("failed to parse tools for schema %s", name)
			}

			wantCount := expectedCounts[name]
			if len(tools) != wantCount {
				t.Errorf("schema %s tool count = %d, want %d", name, len(tools), wantCount)
			}

			tasks, err := FromSchema(raw)
			if err != nil {
				t.Fatalf("FromSchema failed on %s: %v", name, err)
			}
			if len(tasks) != len(tools) {
				t.Errorf("FromSchema generated %d tasks, want %d", len(tasks), len(tools))
			}

			for _, task := range tasks {
				if task.ID == "" {
					t.Errorf("task ID is empty")
				}
				if task.Category != domain.CategoryImported {
					t.Errorf("task %s category = %s, want %s", task.ID, task.Category, domain.CategoryImported)
				}
				if task.Prompt == "" {
					t.Errorf("task %s has empty prompt", task.ID)
				}
				if task.Expected.ToolName == "" {
					t.Errorf("task %s has empty expected tool name", task.ID)
				}
				if len(task.Tools) != len(tools) {
					t.Errorf("task %s tool catalogue size = %d, want %d", task.ID, len(task.Tools), len(tools))
				}
			}
		})
	}
}

func TestFromSchemaFile(t *testing.T) {
	for _, name := range EmbeddedSchemaNames() {
		// Test via relative path to suites/schemas/
		path := filepath.Join("..", "..", "suites", "schemas", name+".json")
		tasks, err := FromSchemaFile(path)
		if err != nil {
			t.Fatalf("FromSchemaFile(%s) failed: %v", path, err)
		}
		if len(tasks) == 0 {
			t.Fatalf("FromSchemaFile(%s) returned 0 tasks", path)
		}
	}
}

func TestFromSchemaFileEmbeddedFallback(t *testing.T) {
	// Should resolve via embedded schema name when file doesn't exist on disk
	tasks, err := FromSchemaFile("postgres")
	if err != nil {
		t.Fatalf("FromSchemaFile(postgres) embedded fallback failed: %v", err)
	}
	if len(tasks) != 5 {
		t.Fatalf("expected 5 tasks for postgres, got %d", len(tasks))
	}
}

func TestParseSchemaFormats(t *testing.T) {
	t.Run("Standard MCP Wrapper", func(t *testing.T) {
		jsonStr := `{"tools":[{"name":"custom_tool","description":"Does something.","inputSchema":{"type":"object","properties":{"arg1":{"type":"string"}},"required":["arg1"]}}]}`
		tools, err := ParseSchemaTools([]byte(jsonStr))
		if err != nil {
			t.Fatalf("ParseSchemaTools failed: %v", err)
		}
		if len(tools) != 1 || tools[0].Name != "custom_tool" {
			t.Fatalf("unexpected tools parsed: %+v", tools)
		}
	})

	t.Run("JSON-RPC Result Wrapper", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"rpc_tool","description":"Remote tool.","inputSchema":{"type":"object","properties":{}}}]}}`
		tools, err := ParseSchemaTools([]byte(jsonStr))
		if err != nil {
			t.Fatalf("ParseSchemaTools failed: %v", err)
		}
		if len(tools) != 1 || tools[0].Name != "rpc_tool" {
			t.Fatalf("unexpected tools parsed: %+v", tools)
		}
	})

	t.Run("Direct Array", func(t *testing.T) {
		jsonStr := `[{"name":"array_tool","description":"Array tool.","parameters":{"type":"object","properties":{"num":{"type":"integer"}}}}]`
		tools, err := ParseSchemaTools([]byte(jsonStr))
		if err != nil {
			t.Fatalf("ParseSchemaTools failed: %v", err)
		}
		if len(tools) != 1 || tools[0].Name != "array_tool" {
			t.Fatalf("unexpected tools parsed: %+v", tools)
		}
	})

	t.Run("Empty or Invalid", func(t *testing.T) {
		_, err := ParseSchemaTools([]byte(`{}`))
		if err == nil {
			t.Fatal("expected error on empty object")
		}
		_, err = ParseSchemaTools([]byte(`invalid json`))
		if err == nil {
			t.Fatal("expected error on invalid json")
		}
	})
}
