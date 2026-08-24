package suite

import (
	"can-i-compress-it/internal/domain"
)

// Builtin returns the embedded synthetic_v1 suite used when no suite
// file exists, so the harness runs out of the box.
func Builtin() []domain.Task {
	return NewGenerator(Options{NumTasks: 30, Ambiguity: "high", Seed: 42}).Generate()
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
