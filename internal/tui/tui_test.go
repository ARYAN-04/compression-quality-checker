package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

	// Switch presets to Offline Mock
	m.applyPreset(4)
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

	// Default standard full factorial synthetic suite mode
	tasks, err := m.ResolveTasks()
	if err != nil {
		t.Fatalf("ResolveTasks standard failed: %v", err)
	}
	if len(tasks) != len(suite.Builtin()) {
		t.Fatalf("expected %d canonical tasks in standard suite, got %d", len(suite.Builtin()), len(tasks))
	}

	// Switch to templates mode
	m.mode = SuiteModeTemplates
	tasks, err = m.ResolveTasks()
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

	// Switch to templates mode (where no text input is focused)
	m.mode = SuiteModeTemplates
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if m.mode != SuiteModeStandard {
		t.Fatalf("expected mode to switch back to standard on '1'")
	}
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

	// Select all levels with 'a'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	levels, err = m.SelectedLevels()
	if err != nil {
		t.Fatalf("SelectedLevels failed: %v", err)
	}
	if len(levels) != 5 {
		t.Fatalf("expected 5 levels selected after 'a', got %d", len(levels))
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

func TestAppModelQuickFullFactorial(t *testing.T) {
	app := NewAppModel()
	app.providerStep.applyPreset(4) // Offline Mock
	app.providerStep.focused = focusPreset

	// Press 'f' on Step 1 to trigger Full Factorial Run (all 5 levels x 12 tasks = 60 cells)
	m, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(AppModel)
	if app.step != StepRunning {
		t.Fatalf("expected step to advance immediately to StepRunning, got %v", app.step)
	}
	expectedTasks := len(suite.Builtin())
	if len(app.resolvedTasks) != expectedTasks {
		t.Fatalf("expected %d resolved tasks, got %d", expectedTasks, len(app.resolvedTasks))
	}
	if len(app.selectedLevels) != 5 {
		t.Fatalf("expected 5 selected levels, got %d", len(app.selectedLevels))
	}
	if app.runningView.totalCells != expectedTasks*5 {
		t.Fatalf("expected totalCells=%d, got %d", expectedTasks*5, app.runningView.totalCells)
	}
	if cmd == nil || app.execCtrl == nil {
		t.Fatalf("expected active execution controller and command")
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

type failChatModel struct {
	inference.Model
	failOnTask string
}

func (m failChatModel) Chat(ctx context.Context, msgs []inference.Message, tools []domain.ToolDefinition) (inference.Response, error) {
	for _, msg := range msgs {
		if msg.Role == "user" && msg.Content == m.failOnTask {
			return inference.Response{}, fmt.Errorf("simulated endpoint channel 400 error")
		}
	}
	return m.Model.Chat(ctx, msgs, tools)
}

func TestStartBenchmarkToleratesEndpointFailure(t *testing.T) {
	tasks := suite.Builtin()[:2]
	levels := []domain.CompressionLevel{domain.LevelNone}
	baseModel := inference.NewMock("test-mock")
	model := failChatModel{Model: baseModel, failOnTask: tasks[0].Prompt}

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

	var cells []domain.CellResult
	var finished bool
	for !finished {
		msg := ctrl.NextCmd()()
		switch msg := msg.(type) {
		case CellCompletedMsg:
			cells = append(cells, msg.Cell)
		case RunFinishedMsg:
			if msg.Err != nil {
				t.Fatalf("run failed with unexpected error: %v", msg.Err)
			}
			finished = true
		default:
			t.Fatalf("unexpected msg: %T", msg)
		}
	}

	if len(cells) != 2 {
		t.Fatalf("expected 2 completed cells, got %d", len(cells))
	}
	if cells[0].SelectionCorrect {
		t.Errorf("expected failed cell 0 to have SelectionCorrect=false")
	}
	if !cells[1].SelectionCorrect {
		t.Errorf("expected passing cell 1 to have SelectionCorrect=true")
	}
}

func TestCompressionStepTextInputAndTilde(t *testing.T) {
	m := NewCompressionStepModel()
	// Uncheck first level
	m.levels[0].Selected = false

	// Focus on compressorBin
	m.focusedField = focusCompressorBin
	m.compressorBin.Focus()
	m.compressorBin.SetValue("")

	// Type 'a', 'A', ' ', '1' into compressorBin
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// Level 0 should still be unselected (typing 'a' or ' ' did not trigger level shortcuts)
	if m.levels[0].Selected {
		t.Fatalf("expected level 0 to stay unselected when typing 'a' into text input")
	}
	if m.compressorBin.Value() != "aA 1" {
		t.Fatalf("expected compressorBin to contain 'aA 1', got %q", m.compressorBin.Value())
	}

	// Test tilde expansion in CompressorBinary
	m.backend = BackendSubprocess
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}

	m.compressorBin.SetValue("~")
	if got := m.CompressorBinary(); got != home {
		t.Fatalf("CompressorBinary('~') = %q, want %q", got, home)
	}

	m.compressorBin.SetValue("~/bin/mcp-compressor")
	expectedPath := filepath.Join(home, "bin/mcp-compressor")
	if got := m.CompressorBinary(); got != expectedPath {
		t.Fatalf("CompressorBinary('~/bin/mcp-compressor') = %q, want %q", got, expectedPath)
	}

	m.compressorBin.SetValue("/usr/local/bin/mcp-compressor")
	if got := m.CompressorBinary(); got != "/usr/local/bin/mcp-compressor" {
		t.Fatalf("CompressorBinary('/usr/local/bin/mcp-compressor') = %q, want %q", got, "/usr/local/bin/mcp-compressor")
	}
}

func TestSuiteStepTextInputKeyRouting(t *testing.T) {
	// 1. SuiteModeGenerator with genTasksInput focused (genFieldIdx = 0)
	m := NewSuiteStepModel()
	m.mode = SuiteModeGenerator
	m.genFieldIdx = 0
	m.updateGenFocus()
	m.genTasksInput.SetValue("")

	// Type numbers '1', '2', ' ', 'a', '5' into genTasksInput
	for _, r := range []rune{'1', '2', ' ', 'a', '5'} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.mode != SuiteModeGenerator {
		t.Fatalf("expected mode to remain SuiteModeGenerator, got %v", m.mode)
	}
	if m.genTasksInput.Value() != "12 a5" {
		t.Fatalf("expected genTasksInput to be '12 a5', got %q", m.genTasksInput.Value())
	}

	// 2. SuiteModeLive with liveCmdInput focused
	m.mode = SuiteModeLive
	m.liveCmdInput.SetValue("")
	for _, r := range []rune{'1', ' ', 'n', 'p', 'x', ' ', '4'} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.mode != SuiteModeLive {
		t.Fatalf("expected mode to remain SuiteModeLive, got %v", m.mode)
	}
	if m.liveCmdInput.Value() != "1 npx 4" {
		t.Fatalf("expected liveCmdInput to be '1 npx 4', got %q", m.liveCmdInput.Value())
	}

	// 3. SuiteModeFile with filePathInput focused
	m.mode = SuiteModeFile
	m.filePathInput.SetValue("")
	for _, r := range []rune{'2', ' ', 't', 'e', 's', 't', ' ', '3', '.', 'j', 's', 'o', 'n'} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.mode != SuiteModeFile {
		t.Fatalf("expected mode to remain SuiteModeFile, got %v", m.mode)
	}
	if m.filePathInput.Value() != "2 test 3.json" {
		t.Fatalf("expected filePathInput to be '2 test 3.json', got %q", m.filePathInput.Value())
	}

	// 4. SuiteModeTemplates (no text input focused) -> should switch tabs and toggle
	m.mode = SuiteModeTemplates
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.mode != SuiteModeGenerator {
		t.Fatalf("expected mode to switch to SuiteModeGenerator on '3', got %v", m.mode)
	}

	m.mode = SuiteModeTemplates
	m.templateIdx = 0
	origSelected := m.templates[0].Selected
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.templates[0].Selected == origSelected {
		t.Fatalf("expected template[0].Selected to toggle on space in SuiteModeTemplates")
	}
}

func TestTUI_EnvironmentOverrides(t *testing.T) {
	t.Setenv("CAN_I_COMPRESS_IT_MODEL", "custom-model")
	t.Setenv("CAN_I_COMPRESS_IT_ENDPOINT", "http://127.0.0.1:9999/v1")
	t.Setenv("CAN_I_COMPRESS_IT_PARAMS_B", "12.5")
	t.Setenv("CAN_I_COMPRESS_IT_AUTORUN", "1")

	app := NewAppModel()
	if got := app.providerStep.modelInput.Value(); got != "custom-model" {
		t.Fatalf("expected modelInput 'custom-model', got %q", got)
	}
	if got := app.providerStep.endpointInput.Value(); got != "http://127.0.0.1:9999/v1" {
		t.Fatalf("expected endpointInput 'http://127.0.0.1:9999/v1', got %q", got)
	}
	if got := app.providerStep.paramsInput.Value(); got != "12.5" {
		t.Fatalf("expected paramsInput '12.5', got %q", got)
	}

	updated, _ := app.Update(autoRunMsg{})
	updatedApp, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("expected AppModel, got %T", updated)
	}
	if updatedApp.step != StepRunning {
		t.Fatalf("expected step StepRunning after autoRunMsg, got %v", updatedApp.step)
	}
	if updatedApp.execCtrl != nil {
		updatedApp.execCtrl.Cancel()
	}
}
