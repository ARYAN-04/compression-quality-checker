// Package report renders benchmark outputs: raw JSON traces, markdown
// safety matrices, terminal tables, and standalone responsive HTML.
package report

import (
	"encoding/json"
	"fmt"
	"os"

	"can-i-compress-it/internal/domain"
)

// SaveJSON writes a RunResult as an execution trace file.
func SaveJSON(path string, run domain.RunResult) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode results: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadJSON reads a previously saved execution trace.
func LoadJSON(path string) (domain.RunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.RunResult{}, fmt.Errorf("read results %s: %w", path, err)
	}
	var run domain.RunResult
	if err := json.Unmarshal(data, &run); err != nil {
		return domain.RunResult{}, fmt.Errorf("parse results %s: %w", path, err)
	}
	return run, nil
}
