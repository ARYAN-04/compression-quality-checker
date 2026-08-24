// Package suite synthesizes and loads evaluation task suites.
package suite

import (
	"encoding/json"
	"fmt"
	"os"

	"can-i-compress-it/internal/domain"
)

// Load reads a task suite from a JSON file.
func Load(path string) ([]domain.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	var tasks []domain.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parse suite %s: %w", path, err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("suite %s contains no tasks", path)
	}
	return tasks, nil
}

// Save writes tasks to path as pretty-printed JSON.
func Save(path string, tasks []domain.Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("encode suite: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
