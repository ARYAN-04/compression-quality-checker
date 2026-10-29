// Package tui implements the interactive Bubble Tea wizard and safety matrix explorer.
package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/suite"
)

// WizardStep represents the active wizard state.
type WizardStep int

const (
	StepProvider WizardStep = iota
	StepSuite
	StepCompression
	StepRunning
	StepResults
)

// AppModel is the top-level Bubble Tea state machine.
type AppModel struct {
	step            WizardStep
	providerStep    ProviderStepModel
	suiteStep       SuiteStepModel
	compressionStep CompressionStepModel
	runningView     RunningViewModel
	resultsView     ResultsViewModel
	resolvedTasks   []domain.Task
	selectedLevels  []domain.CompressionLevel
	execCtrl        *ExecutionController
	width           int
	height          int
	quitting        bool
}

// NewAppModel initializes the full TUI wizard application.
func NewAppModel() AppModel {
	return AppModel{
		step:            StepProvider,
		providerStep:    NewProviderStepModel(),
		suiteStep:       NewSuiteStepModel(),
		compressionStep: NewCompressionStepModel(),
		runningView:     NewRunningViewModel(),
		resultsView:     NewResultsViewModel(),
	}
}

type autoRunMsg struct{}

// Init sets up initial tea commands.
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.EnterAltScreen,
		m.runningView.Init(),
	}
	if os.Getenv("CAN_I_COMPRESS_IT_AUTORUN") == "1" || os.Getenv("CAN_I_COMPRESS_IT_AUTORUN") == "true" {
		cmds = append(cmds, func() tea.Msg { return autoRunMsg{} })
	}
	return tea.Batch(cmds...)
}

// Update handles message dispatch across all wizard steps.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case autoRunMsg:
		if m.step != StepRunning {
			return m.runFullFactorial()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.providerStep.SetSize(msg.Width, msg.Height)
		m.suiteStep.SetSize(msg.Width, msg.Height)
		m.compressionStep.SetSize(msg.Width, msg.Height)
		m.runningView.SetSize(msg.Width, msg.Height)
		m.resultsView.SetSize(msg.Width, msg.Height)
		return m.updateSubView(msg)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.execCtrl != nil {
				m.execCtrl.Cancel()
			}
			m.quitting = true
			return m, tea.Quit

		case "ctrl+f":
			if m.step != StepRunning {
				return m.runFullFactorial()
			}

		case "f", "F":
			if m.step == StepResults || (m.step == StepProvider && m.providerStep.focused == focusPreset) {
				return m.runFullFactorial()
			}

		case "q", "Q":
			if m.step == StepResults {
				m.quitting = true
				return m, tea.Quit
			}

		case "r", "R":
			if m.step == StepResults {
				m.step = StepProvider
				return m, nil
			}

		case "esc":
			return m.handleBack()

		case "enter":
			return m.handleAdvance()
		}

	case CellCompletedMsg:
		var cmd tea.Cmd
		m.runningView, cmd = m.runningView.Update(msg)
		if m.execCtrl != nil {
			return m, tea.Batch(cmd, m.execCtrl.NextCmd())
		}
		return m, cmd

	case RunFinishedMsg:
		m.execCtrl = nil
		if msg.Err != nil {
			m.runningView.err = msg.Err
			return m, nil
		}
		m.resultsView.SetResult(
			msg.RunResult,
			msg.Table,
			m.compressionStep.jsonPathInput.Value(),
			m.compressionStep.htmlPathInput.Value(),
			m.compressionStep.mdPathInput.Value(),
		)
		m.step = StepResults
		return m, nil
	}

	return m.updateSubView(msg)
}

func (m AppModel) handleBack() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepSuite:
		m.step = StepProvider
		return m, nil
	case StepCompression:
		m.step = StepSuite
		return m, nil
	case StepRunning:
		if m.execCtrl != nil {
			m.execCtrl.Cancel()
		}
		m.step = StepCompression
		return m, nil
	case StepResults:
		m.quitting = true
		return m, tea.Quit
	default:
		m.quitting = true
		return m, tea.Quit
	}
}

func (m AppModel) handleAdvance() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepProvider:
		if err := m.providerStep.Validate(); err != nil {
			m.providerStep.errMessage = err.Error()
			return m, nil
		}
		m.step = StepSuite
		return m, nil

	case StepSuite:
		tasks, err := m.suiteStep.ResolveTasks()
		if err != nil {
			m.suiteStep.errMessage = err.Error()
			return m, nil
		}
		m.resolvedTasks = tasks
		m.step = StepCompression
		return m, nil

	case StepCompression:
		levels, err := m.compressionStep.SelectedLevels()
		if err != nil {
			m.compressionStep.errMessage = err.Error()
			return m, nil
		}
		m.selectedLevels = levels
		m.step = StepRunning
		m.runningView = NewRunningViewModel()
		if m.width > 0 {
			m.runningView.SetSize(m.width, m.height)
		}
		m.runningView.totalCells = len(m.resolvedTasks) * len(m.selectedLevels)

		return m.startExecution()
	}
	return m, nil
}

// runFullFactorial directly executes the canonical 60-cell full factorial matrix (synthetic_v1 across all 5 levels).
func (m AppModel) runFullFactorial() (tea.Model, tea.Cmd) {
	if err := m.providerStep.Validate(); err != nil {
		m.providerStep.errMessage = err.Error()
		m.step = StepProvider
		return m, nil
	}
	m.resolvedTasks = suite.Builtin()
	m.selectedLevels = domain.AllLevels
	m.step = StepRunning
	m.runningView = NewRunningViewModel()
	if m.width > 0 {
		m.runningView.SetSize(m.width, m.height)
	}
	m.runningView.totalCells = len(m.resolvedTasks) * len(m.selectedLevels)

	return m.startExecution()
}

func (m AppModel) startExecution() (tea.Model, tea.Cmd) {
	var model inference.Model
	modelName := strings.TrimSpace(m.providerStep.modelInput.Value())
	if m.providerStep.IsMock() {
		model = inference.NewMock(modelName)
	} else {
		endpoint := strings.TrimSpace(m.providerStep.endpointInput.Value())
		apiKey := strings.TrimSpace(m.providerStep.apiKeyInput.Value())
		model = inference.NewClient(modelName, endpoint, apiKey)
	}

	compressorBin := m.compressionStep.CompressorBinary()
	paramsB := m.providerStep.ParamsB()
	jsonPath := strings.TrimSpace(m.compressionStep.jsonPathInput.Value())
	htmlPath := strings.TrimSpace(m.compressionStep.htmlPathInput.Value())
	mdPath := strings.TrimSpace(m.compressionStep.mdPathInput.Value())
	endpoint := strings.TrimSpace(m.providerStep.endpointInput.Value())

	ctrl, startCmd := StartBenchmark(
		model,
		m.resolvedTasks,
		m.selectedLevels,
		compressorBin,
		paramsB,
		"synthetic_v1",
		endpoint,
		jsonPath,
		htmlPath,
		mdPath,
	)
	m.execCtrl = ctrl
	return m, tea.Batch(m.runningView.Init(), startCmd)
}

func (m AppModel) updateSubView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.step {
	case StepProvider:
		m.providerStep, cmd = m.providerStep.Update(msg)
	case StepSuite:
		m.suiteStep, cmd = m.suiteStep.Update(msg)
	case StepCompression:
		m.compressionStep, cmd = m.compressionStep.Update(msg)
	case StepRunning:
		m.runningView, cmd = m.runningView.Update(msg)
	case StepResults:
		m.resultsView, cmd = m.resultsView.Update(msg)
	}
	return m, cmd
}

// View renders the current step view.
func (m AppModel) View() string {
	if m.quitting {
		return "Exiting can-i-compress-it wizard.\n"
	}

	var content string
	switch m.step {
	case StepProvider:
		content = m.providerStep.View()
	case StepSuite:
		content = m.suiteStep.View()
	case StepCompression:
		content = m.compressionStep.View()
	case StepRunning:
		content = m.runningView.View()
	case StepResults:
		content = m.resultsView.View()
	}

	return content + "\n"
}

// Run launches the interactive Bubble Tea application.
func Run() error {
	p := tea.NewProgram(NewAppModel(), tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui execution: %w", err)
	}
	return nil
}
