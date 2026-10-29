package suite

import (
	"can-i-compress-it/internal/domain"
)

// Builtin returns the canonical 30-task benchmark suite built from embedded
// MCP server schemas across ambiguous_name, overlapping_schema, and multihop
// categories, replacing synthetic tasks with realistic natural-language goals.
func Builtin() []domain.Task {
	return BuiltinTasks()
}

// UniqueTools merges the catalogues of all tasks into one deduplicated
// list, serving as the target server's full tool surface.
func UniqueTools(tasks []domain.Task) []domain.ToolDefinition {
	seen := map[string]bool{}
	out := make([]domain.ToolDefinition, 0, len(tasks))
	for _, t := range tasks {
		for _, tool := range t.Tools {
			if !seen[tool.Name] {
				seen[tool.Name] = true
				out = append(out, tool)
			}
		}
	}
	return out
}
