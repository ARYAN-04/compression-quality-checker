package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/report"
	"can-i-compress-it/internal/runner"
	"can-i-compress-it/internal/suite"
)

// CellCompletedMsg notifies the UI that a single task cell finished.
type CellCompletedMsg struct {
	Cell       domain.CellResult
	CellIndex  int
	TotalCells int
}

// RunFinishedMsg delivers the completed run result and table.
type RunFinishedMsg struct {
	RunResult domain.RunResult
	Table     report.SafetyTable
	Err       error
}

// ExecutionController coordinates background scheduler execution with Bubble Tea.
type ExecutionController struct {
	cellChan   chan domain.CellResult
	finishChan chan RunFinishedMsg
	cancelFn   context.CancelFunc
	totalCells int
}

// NextCmd returns a tea.Cmd waiting for the next cell or completion event.
func (c *ExecutionController) NextCmd() tea.Cmd {
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		for {
			if c.cellChan != nil {
				select {
				case cell, ok := <-c.cellChan:
					if ok {
						return CellCompletedMsg{Cell: cell, TotalCells: c.totalCells}
					}
					c.cellChan = nil
				default:
				}
			}

			select {
			case cell, ok := <-c.cellChan:
				if ok {
					return CellCompletedMsg{Cell: cell, TotalCells: c.totalCells}
				}
				c.cellChan = nil
			case finish, ok := <-c.finishChan:
				if ok {
					return finish
				}
				return nil
			}
		}
	}
}

// Cancel terminates ongoing background execution.
func (c *ExecutionController) Cancel() {
	if c != nil && c.cancelFn != nil {
		c.cancelFn()
	}
}

// RunningViewModel manages Step 4: live execution monitor.
type RunningViewModel struct {
	spinner        spinner.Model
	progress       progress.Model
	totalCells     int
	completedCells int
	currentLevel   domain.CompressionLevel
	currentTaskID  string
	recentCells    []domain.CellResult
	correctTools   int
	correctParams  int
	multiHopPass   int
	totalTokens    int
	totalLatencyMs int64
	startTime      time.Time
	err            error
	width          int
	height         int
}

// NewRunningViewModel initializes the live execution view.
func NewRunningViewModel() RunningViewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorElectricCyan)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(60),
		progress.WithoutPercentage(),
	)

	return RunningViewModel{
		spinner:     s,
		progress:    p,
		recentCells: make([]domain.CellResult, 0, 8),
		startTime:   time.Now(),
		width:       80,
	}
}

// SetSize adapts progress bar and views to terminal dimensions.
func (m *RunningViewModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.progress.Width = max(20, min(80, width-25))
}

// Init starts the spinner.
func (m RunningViewModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles live progress messages.
func (m RunningViewModel) Update(msg tea.Msg) (RunningViewModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmds = append(cmds, cmd)

	case CellCompletedMsg:
		if m.totalCells == 0 && msg.TotalCells > 0 {
			m.totalCells = msg.TotalCells
		}
		m.completedCells++
		m.currentLevel = msg.Cell.CompressionLevel
		m.currentTaskID = msg.Cell.TaskID
		m.totalTokens += msg.Cell.TokensConsumed
		m.totalLatencyMs += msg.Cell.LatencyMs

		if msg.Cell.SelectionCorrect {
			m.correctTools++
		}
		if msg.Cell.ParamFillCorrect {
			m.correctParams++
		}
		if msg.Cell.MultiHopComplete {
			m.multiHopPass++
		}

		m.recentCells = append(m.recentCells, msg.Cell)
		if len(m.recentCells) > 7 {
			m.recentCells = m.recentCells[len(m.recentCells)-7:]
		}

		if m.totalCells > 0 {
			pct := float64(m.completedCells) / float64(m.totalCells)
			cmd := m.progress.SetPercent(pct)
			cmds = append(cmds, cmd)
		}

	case RunFinishedMsg:
		if msg.Err != nil {
			m.err = msg.Err
		}
	}

	return m, tea.Batch(cmds...)
}

// StartBenchmark spawns execution in a background goroutine and returns the controller + initial command.
func StartBenchmark(
	model inference.Model,
	tasks []domain.Task,
	levels []domain.CompressionLevel,
	compressorBin string,
	paramsB float64,
	suitePath string,
	endpoint string,
	jsonPath string,
	htmlPath string,
	mdPath string,
) (*ExecutionController, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	cellChan := make(chan domain.CellResult, 200)
	finishChan := make(chan RunFinishedMsg, 1)
	totalCells := len(tasks) * len(levels)

	ctrl := &ExecutionController{
		cellChan:   cellChan,
		finishChan: finishChan,
		cancelFn:   cancel,
		totalCells: totalCells,
	}

	go func() {
		var closeOnce sync.Once
		closeCellChan := func() {
			closeOnce.Do(func() { close(cellChan) })
		}
		defer closeCellChan()
		defer close(finishChan)

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		if logPath := os.Getenv("CAN_I_COMPRESS_IT_LOG"); logPath != "" {
			if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				defer func() { _ = f.Close() }()
				logger = slog.New(slog.NewTextHandler(f, nil))
			}
		}

		sched := runner.NewScheduler(model, func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{
				Level:        level,
				Binary:       compressorBin,
				TargetServer: "suite",
				ThreadFields: suite.ThreadedFields(tasks),
			}, suite.UniqueTools(tasks))
		}, logger)

		sched.SetOnCellResult(func(c domain.CellResult) {
			select {
			case cellChan <- c:
			case <-ctx.Done():
			}
		})

		cells, err := sched.ExecuteMatrix(ctx, tasks, levels)
		closeCellChan()
		if err != nil {
			finishChan <- RunFinishedMsg{Err: err}
			return
		}

		run := domain.RunResult{
			RunID:           fmt.Sprintf("run_%d", time.Now().Unix()),
			Model:           model.Name(),
			Endpoint:        endpoint,
			SuitePath:       suitePath,
			Levels:          levels,
			Tasks:           len(tasks),
			Results:         cells,
			ParameterCountB: paramsB,
		}

		if jsonPath != "" {
			_ = report.SaveJSON(jsonPath, run)
		}
		tbl := report.BuildTable([]domain.RunResult{run})
		if htmlPath != "" {
			title := "Compression Safety Matrix — " + model.Name()
			_ = report.SaveHTML(htmlPath, tbl.HTML(title))
		}
		if mdPath != "" {
			_ = report.SaveMarkdown(mdPath, tbl.Markdown())
		}

		finishChan <- RunFinishedMsg{RunResult: run, Table: tbl}
	}()

	return ctrl, ctrl.NextCmd()
}

// View renders Step 4.
func (m RunningViewModel) View() string {
	var b strings.Builder
	b.WriteString(RenderHeader(3, "Evaluating tool-use accuracy across compression levels in real time."))
	b.WriteString("\n")

	// Status line
	statusText := fmt.Sprintf("%s Benchmarking matrix cell: level %s &middot; task %s",
		m.spinner.View(),
		StyleSelected.Render(string(m.currentLevel)),
		StyleLabel.Render(m.currentTaskID))
	b.WriteString(statusText + "\n\n")

	// Progress bar
	progressPct := 0.0
	if m.totalCells > 0 {
		progressPct = float64(m.completedCells) / float64(m.totalCells) * 100
	}
	progressInfo := fmt.Sprintf("%s  %.0f%% (%d/%d cells)", m.progress.View(), progressPct, m.completedCells, m.totalCells)
	b.WriteString(progressInfo + "\n\n")

	// Live Metrics Stats Box
	avgLatency := int64(0)
	if m.completedCells > 0 {
		avgLatency = m.totalLatencyMs / int64(m.completedCells)
	}
	toolPct := 0.0
	paramPct := 0.0
	if m.completedCells > 0 {
		toolPct = float64(m.correctTools) / float64(m.completedCells) * 100
		paramPct = float64(m.correctParams) / float64(m.completedCells) * 100
	}

	statsLine := fmt.Sprintf(
		"Tool Match: %s | Param Match: %s | Avg Latency: %s | Tokens: %s",
		StyleSuccess.Render(fmt.Sprintf("%.1f%% (%d/%d)", toolPct, m.correctTools, m.completedCells)),
		StyleSuccess.Render(fmt.Sprintf("%.1f%% (%d/%d)", paramPct, m.correctParams, m.completedCells)),
		StyleLabel.Render(fmt.Sprintf("%dms", avgLatency)),
		StyleLabel.Render(fmt.Sprintf("%d", m.totalTokens)),
	)
	b.WriteString(statsLine + "\n\n")

	// Streaming Live Results Table
	b.WriteString(StyleSectionTitle.Render("Live Evaluation Stream (Recent Cells):") + "\n")
	var streamRows []string
	streamRows = append(streamRows, fmt.Sprintf("%-6s  %-8s  %-24s  %-20s  %-8s  %-7s  %-8s",
		"Status", "Level", "Task ID", "Tool Selected", "Multi-Hop", "Latency", "Tokens"))
	streamRows = append(streamRows, strings.Repeat("─", max(40, min(88, m.width-10))))

	for _, c := range m.recentCells {
		status := StyleSuccess.Render("✓ PASS")
		if !c.Correct() {
			status = StyleError.Render("✗ FAIL")
		}
		hop := "-"
		if c.MultiHopComplete {
			hop = "✓"
		}
		streamRows = append(streamRows, fmt.Sprintf("%s  %-8s  %-24s  %-20s  %-8s  %5dms  %6d",
			status,
			c.CompressionLevel,
			truncate(c.TaskID, 24),
			truncate(c.ToolSelected, 20),
			hop,
			c.LatencyMs,
			c.TokensConsumed,
		))
	}

	b.WriteString(strings.Join(streamRows, "\n") + "\n")

	if m.err != nil {
		b.WriteString("\n" + StyleError.Render("Execution Error: "+m.err.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("Controls: [Ctrl+C / Esc] Abort execution"))
	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
