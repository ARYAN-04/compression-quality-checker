// Package domain defines the shared types and interfaces of the
// can-i-compress-it benchmarking harness.
package domain

import (
	"context"
	"fmt"
)

// CompressionLevel identifies one mcp-compressor intensity tier.
type CompressionLevel string

// The five compression levels under evaluation.
const (
	LevelNone   CompressionLevel = "none"
	LevelLow    CompressionLevel = "low"
	LevelMedium CompressionLevel = "medium"
	LevelHigh   CompressionLevel = "high"
	LevelMax    CompressionLevel = "max"
)

// AllLevels lists the compression levels in ascending intensity order.
var AllLevels = []CompressionLevel{LevelNone, LevelLow, LevelMedium, LevelHigh, LevelMax}

// ParseLevel validates s as a compression level.
func ParseLevel(s string) (CompressionLevel, error) {
	for _, l := range AllLevels {
		if CompressionLevel(s) == l {
			return l, nil
		}
	}
	return "", fmt.Errorf("unknown compression level %q", s)
}

// Task categories engineered to trigger specific small-model failure modes.
const (
	CategoryAmbiguousName    = "ambiguous_name"
	CategoryOverlappingSchem = "overlapping_schema"
	CategoryMultiHop         = "multihop"
)

// Task is a single evaluation unit: a prompt plus the tool catalogue visible
// to the model and the ground-truth call(s) expected from it.
type Task struct {
	ID       string           `json:"id"`
	Category string           `json:"category"`
	Prompt   string           `json:"prompt"`
	Tools    []ToolDefinition `json:"tools"`
	Expected ExpectedCall     `json:"expected"`
	MultiHop []ExpectedCall   `json:"multi_hop,omitempty"`
}

// ToolDefinition is an MCP-style JSON schema for one tool.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ExpectedCall is the ground truth for a single tool invocation.
type ExpectedCall struct {
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments"`
	StrictType bool           `json:"strict_type"`
}

// ToolCall is a tool invocation produced by a model (or meta-tool unwrap).
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// CellResult records one scored model x level x task observation.
type CellResult struct {
	Model            string           `json:"model"`
	CompressionLevel CompressionLevel `json:"compression_level"`
	TaskID           string           `json:"task_id"`
	Category         string           `json:"category"`
	ToolSelected     string           `json:"tool_selected"`
	SelectionCorrect bool             `json:"selection_correct"`
	ParamFillCorrect bool             `json:"param_fill_correct"`
	MultiHopComplete bool             `json:"multihop_complete"`
	TokensConsumed   int              `json:"tokens_consumed"`
	LatencyMs        int64            `json:"latency_ms"`

	// RunID ties a cell to the matrix execution that produced it.
	RunID string `json:"run_id"`
}

// Correct reports whether the cell passed every metric that applies to
// its category: multi-hop tasks additionally require chain completion.
func (r CellResult) Correct() bool {
	if !r.SelectionCorrect || !r.ParamFillCorrect {
		return false
	}
	if r.Category == CategoryMultiHop {
		return r.MultiHopComplete
	}
	return true
}

// RunResult is the serializable output of one full matrix execution.
type RunResult struct {
	RunID     string             `json:"run_id"`
	Model     string             `json:"model"`
	Endpoint  string             `json:"endpoint"`
	SuitePath string             `json:"suite_path"`
	Levels    []CompressionLevel `json:"levels"`
	Tasks     int                `json:"tasks"`
	Results   []CellResult       `json:"results"`

	// ParameterCountB is the subject model's size in billions of
	// parameters; zero means unknown (pre-v0.2 traces).
	ParameterCountB float64 `json:"parameter_count_b,omitempty"`
}

// FailureCliffReport is the per-model safety verdict.
type FailureCliffReport struct {
	Model            string           `json:"model"`
	ParameterCountB  float64          `json:"parameter_count_b"`
	BaselineAccuracy float64          `json:"baseline_accuracy"`
	CliffLevel       CompressionLevel `json:"cliff_level"`
	CliffAccuracy    float64          `json:"cliff_accuracy"`
	AccuracyDelta    float64          `json:"accuracy_delta"`
	CI95Low          float64          `json:"ci_95_low"`
	CI95High         float64          `json:"ci_95_high"`
}

// Runner executes a full factorial matrix over tasks and levels.
type Runner interface {
	ExecuteMatrix(ctx context.Context, suite []Task, levels []CompressionLevel) ([]CellResult, error)
}

// Evaluator scores a produced call against ground truth BFCL-style.
type Evaluator interface {
	ScoreCall(expected ExpectedCall, actual ToolCall) (selectionOK, paramsOK bool)
}
