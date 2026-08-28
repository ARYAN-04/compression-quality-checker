// Package report renders benchmark outputs: raw JSON traces, markdown
// safety matrices, terminal tables, and standalone responsive HTML.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// SaveJSON writes a RunResult as an execution trace file.
func SaveJSON(path string, run domain.RunResult) error {
	clean, err := cleanPath(path)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode results: %w", err)
	}
	return os.WriteFile(clean, data, 0o644)
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

// LoadRuns loads one or more execution traces. The spec may be a single
// path or a glob pattern (e.g. "results/*.json") for multi-model runs.
func LoadRuns(spec string) ([]domain.RunResult, error) {
	paths := []string{spec}
	if hasGlobMeta(spec) {
		matched, err := filepath.Glob(spec)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", spec, err)
		}
		if len(matched) == 0 {
			return nil, fmt.Errorf("no files match %s", spec)
		}
		sort.Strings(matched)
		paths = matched
	}
	runs := make([]domain.RunResult, 0, len(paths))
	for _, p := range paths {
		run, err := LoadJSON(p)
		if err != nil {
			return nil, err
		}
		run.SuitePath = p // remember provenance of each trace
		runs = append(runs, run)
	}
	return runs, nil
}

func hasGlobMeta(spec string) bool {
	return strings.ContainsAny(spec, "*?[")
}

// MergeRuns combines traces from multiple subject models into one
// aggregate: results are concatenated and levels form an ordered union.
func MergeRuns(runs []domain.RunResult) domain.RunResult {
	merged := domain.RunResult{}
	levelSeen := map[domain.CompressionLevel]bool{}
	models := map[string]bool{}
	for _, run := range runs {
		if merged.RunID != "" {
			merged.RunID += "+"
		}
		merged.RunID += run.RunID
		merged.Tasks += run.Tasks
		merged.Results = append(merged.Results, run.Results...)
		for _, l := range run.Levels {
			if !levelSeen[l] {
				levelSeen[l] = true
				merged.Levels = append(merged.Levels, l)
			}
		}
		if run.Model != "" {
			models[run.Model] = true
		}
	}
	sort.Slice(merged.Levels, func(i, j int) bool {
		return levelOrder(merged.Levels[i]) < levelOrder(merged.Levels[j])
	})
	names := make([]string, 0, len(models))
	for m := range models {
		names = append(names, m)
	}
	sort.Strings(names)
	merged.Model = strings.Join(names, "+")
	return merged
}

func levelOrder(l domain.CompressionLevel) int {
	for i, v := range domain.AllLevels {
		if v == l {
			return i
		}
	}
	return len(domain.AllLevels)
}
