package tui

import (
	"math"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/report"
	"can-i-compress-it/internal/suite"
)

func TestDetectParamsB(t *testing.T) {
	tests := []struct {
		model    string
		expected float64
	}{
		{"llama-3-8b-instruct", 8.0},
		{"qwen2.5-7b", 7.0},
		{"deepseek-coder-1.3b", 1.3},
		{"mistral-7b-v0.2", 7.0},
		{"llama-3.1-70b", 70.0},
		{"phi-3-mini-3.8b", 3.8},
		{"qwen2.5-32b-coder", 32.0},
		{"smollm-135m", 0.135},
		{"smollm-360m", 0.36},
		{"gpt-4o-mini", 0.0},
	}

	for _, tt := range tests {
		got := DetectParamsB(tt.model)
		if math.Abs(got-tt.expected) > 0.001 {
			t.Errorf("DetectParamsB(%q) = %v; want %v", tt.model, got, tt.expected)
		}
	}
}

func TestProviderStepModel(t *testing.T) {
	m := NewProviderStepModel()
	if err := m.Validate(); err != nil {
		t.Fatalf("expected default preset to be valid, got %v", err)
	}

	// Invalidate model
	m.modelInput.SetValue("")
	if err := m.Validate(); err == nil {
		t.Fatalf("expected validation error on empty model")
	}

	// Switch presets
	m.applyPreset(4) // Offline Mock
	if !m.IsMock() {
		t.Fatalf("expected preset 4 to be mock")
	}

	// Key navigation
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestSuiteStepModel(t *testing.T) {
	m := NewSuiteStepModel()

	// Default template mode
	tasks, err := m.ResolveTasks()
	if err != nil {
		t.Fatalf("ResolveTasks templates failed: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected tasks from templates")
	}

	// Switch to generator mode
	m.mode = SuiteModeGenerator
	tasks, err = m.ResolveTasks()
	if err != nil {
		t.Fatalf("ResolveTasks generator failed: %v", err)
	}
	if len(tasks) != 12 {
		t.Fatalf("expected 12 synthetic tasks, got %d", len(tasks))
	}

	// Key update
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestCompressionStepModel(t *testing.T) {
	m := NewCompressionStepModel()
	levels, err := m.SelectedLevels()
	if err != nil {
		t.Fatalf("SelectedLevels failed: %v", err)
	}
	if len(levels) != 5 {
		t.Fatalf("expected 5 levels selected by default, got %d", len(levels))
	}

	// Toggle level off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	levels, err = m.SelectedLevels()
	if err != nil {
		t.Fatalf("SelectedLevels failed: %v", err)
	}
	if len(levels) != 4 {
		t.Fatalf("expected 4 levels selected after toggle, got %d", len(levels))
	}

	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestRunningViewModel(t *testing.T) {
	m := NewRunningViewModel()
	m.totalCells = 10
	cell := domain.CellResult{
		Model:            "mock-7b",
		CompressionLevel: domain.LevelNone,
		TaskID:           "test_task_001",
		ToolSelected:     "test_tool",
		SelectionCorrect: true,
		ParamFillCorrect: true,
		LatencyMs:        45,
		TokensConsumed:   120,
	}

	m, _ = m.Update(CellCompletedMsg{Cell: cell, TotalCells: 10})
	if m.completedCells != 1 {
		t.Fatalf("expected completedCells=1, got %d", m.completedCells)
	}
	if m.correctTools != 1 || m.correctParams != 1 {
		t.Fatalf("expected 1 correct tool and param, got %d %d", m.correctTools, m.correctParams)
	}

	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestResultsViewModel(t *testing.T) {
	m := NewResultsViewModel()
	run := domain.RunResult{
		Model:           "mock-7b",
		ParameterCountB: 7.0,
		Levels:          domain.AllLevels,
		Results: []domain.CellResult{
			{Model: "mock-7b", CompressionLevel: domain.LevelNone, TaskID: "t1", ToolSelected: "t1", SelectionCorrect: true, ParamFillCorrect: true},
			{Model: "mock-7b", CompressionLevel: domain.LevelLow, TaskID: "t1", ToolSelected: "t1", SelectionCorrect: true, ParamFillCorrect: true},
			{Model: "mock-7b", CompressionLevel: domain.LevelMedium, TaskID: "t1", ToolSelected: "t1", SelectionCorrect: true, ParamFillCorrect: true},
			{Model: "mock-7b", CompressionLevel: domain.LevelHigh, TaskID: "t1", ToolSelected: "t1", SelectionCorrect: true, ParamFillCorrect: true},
			{Model: "mock-7b", CompressionLevel: domain.LevelMax, TaskID: "t1", ToolSelected: "t1", SelectionCorrect: true, ParamFillCorrect: true},
		},
	}
	tbl := report.BuildTable([]domain.RunResult{run})
	m.SetResult(run, tbl, "./results/results.json", "./results/report.html", "./results/table.md")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !m.showTaskLog {
		t.Fatalf("expected showTaskLog=true")
	}

	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestAppModelStateMachine(t *testing.T) {
	app := NewAppModel()
	if app.step != StepProvider {
		t.Fatalf("expected initial step to be StepProvider")
	}

	// Advance Step 1 -> Step 2
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = m.(AppModel)
	if app.step != StepSuite {
		t.Fatalf("expected step to advance to StepSuite, got %v", app.step)
	}

	// Advance Step 2 -> Step 3
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = m.(AppModel)
	if app.step != StepCompression {
		t.Fatalf("expected step to advance to StepCompression, got %v", app.step)
	}

	// Back Step 3 -> Step 2
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(AppModel)
	if app.step != StepSuite {
		t.Fatalf("expected step to go back to StepSuite, got %v", app.step)
	}

	// Window resize test
	m, _ = app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app = m.(AppModel)
	if app.width != 100 || app.height != 40 {
		t.Fatalf("expected width=100 height=40, got %d %d", app.width, app.height)
	}

	view := app.View()
	if view == "" {
		t.Fatalf("expected non-empty view")
	}
}

func TestStartBenchmarkMock(t *testing.T) {
	tasks := suite.Builtin()[:2]
	levels := []domain.CompressionLevel{domain.LevelNone}
	model := inference.NewMock("test-mock")

	ctrl, cmd := StartBenchmark(
		model,
		tasks,
		levels,
		"",
		7.0,
		"test-suite",
		"mock",
		"",
		"",
		"",
	)
	if ctrl == nil || cmd == nil {
		t.Fatalf("expected non-nil ctrl and cmd")
	}

	// Read messages until finished
	var receivedCells int
	var finished bool
	for !finished {
		msg := ctrl.NextCmd()()
		switch msg := msg.(type) {
		case CellCompletedMsg:
			receivedCells++
		case RunFinishedMsg:
			if msg.Err != nil {
				t.Fatalf("run failed with error: %v", msg.Err)
			}
			finished = true
		default:
			t.Fatalf("unexpected msg: %T", msg)
		}
	}

	if receivedCells != 2 {
		t.Fatalf("expected 2 completed cells, got %d", receivedCells)
	}
}
