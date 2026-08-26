package tui

import (
	"fmt"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/report"
	"can-i-compress-it/internal/stats"
)

// ResultsViewModel manages Step 5: results explorer and safety matrix viewer.
type ResultsViewModel struct {
	runResult    domain.RunResult
	table        report.SafetyTable
	jsonPath     string
	htmlPath     string
	mdPath       string
	showTaskLog  bool
	actionNotice string
	errNotice    string
	width        int
	height       int
}

// NewResultsViewModel initializes the results explorer view.
func NewResultsViewModel() ResultsViewModel {
	return ResultsViewModel{
		width: 80,
	}
}

// SetSize stores terminal window dimensions for results rendering.
func (m *ResultsViewModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetResult updates the model with the latest benchmark run.
func (m *ResultsViewModel) SetResult(run domain.RunResult, tbl report.SafetyTable, jsonPath, htmlPath, mdPath string) {
	m.runResult = run
	m.table = tbl
	m.jsonPath = jsonPath
	m.htmlPath = htmlPath
	m.mdPath = mdPath
	m.actionNotice = ""
	m.errNotice = ""
}

// Update handles key bindings in the results explorer.
func (m ResultsViewModel) Update(msg tea.Msg) (ResultsViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "o", "O":
			if m.htmlPath != "" {
				if err := openHTMLInBrowser(m.htmlPath); err != nil {
					m.errNotice = "Failed to open browser: " + err.Error()
				} else {
					m.actionNotice = "Opened HTML report in default browser: " + m.htmlPath
					m.errNotice = ""
				}
			}
			return m, nil

		case "m", "M":
			if m.mdPath != "" {
				if err := report.SaveMarkdown(m.mdPath, m.table.Markdown()); err != nil {
					m.errNotice = "Failed to save Markdown: " + err.Error()
				} else {
					m.actionNotice = "Markdown table exported to: " + m.mdPath
					m.errNotice = ""
				}
			}
			return m, nil

		case "t", "T":
			m.showTaskLog = !m.showTaskLog
			return m, nil
		}
	}
	return m, nil
}

func openHTMLInBrowser(htmlPath string) error {
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		absPath = htmlPath
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", absPath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", absPath)
	default:
		cmd = exec.Command("xdg-open", absPath)
	}
	return cmd.Start()
}

// View renders Step 5 results explorer.
func (m ResultsViewModel) View() string {
	var b strings.Builder
	b.WriteString(RenderHeader(4, "Evaluation complete. Safety matrix, failure cliffs, and statistical intervals."))
	b.WriteString("\n\n")

	// Model Header Info
	paramsStr := "-"
	if m.runResult.ParameterCountB > 0 {
		paramsStr = fmt.Sprintf("%.1fB", m.runResult.ParameterCountB)
	}
	headerInfo := fmt.Sprintf("Model: %s  |  Params: %s  |  Tasks: %d  |  Levels: %d",
		StyleSelected.Render(m.runResult.Model),
		StyleLabel.Render(paramsStr),
		len(m.runResult.Results)/max(1, len(m.runResult.Levels)),
		len(m.runResult.Levels),
	)
	b.WriteString(headerInfo + "\n\n")

	if m.showTaskLog {
		b.WriteString(m.renderTaskLogView())
	} else {
		b.WriteString(m.renderMatrixView())
		b.WriteString("\n\n")
		b.WriteString(m.renderStatsView())
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderArtifactsView())

	if m.actionNotice != "" {
		b.WriteString("\n" + StyleSuccess.Render("✓ "+m.actionNotice))
	}
	if m.errNotice != "" {
		b.WriteString("\n" + StyleError.Render("⚠ "+m.errNotice))
	}

	b.WriteString("\n\n" + StyleHelp.Render("Hotkeys: [O] Open HTML Report • [M] Export Markdown • [T] Toggle Task Details • [R] Rerun Sweep • [Q] Exit"))
	return b.String()
}

func (m ResultsViewModel) renderMatrixView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Safety Matrix (Tool-Use Accuracy):")+"\n")

	header := fmt.Sprintf("%-24s  %-10s", "Model", "Params (B)")
	for _, l := range m.table.Levels {
		header += fmt.Sprintf("  %-8s", string(l))
	}
	header += "  Failure Cliff"
	rows = append(rows, header)
	rows = append(rows, strings.Repeat("─", len(header)+4))

	for _, row := range m.table.Rows {
		bStr := "-"
		if row.B > 0 {
			bStr = fmt.Sprintf("%.1fB", row.B)
		}
		line := fmt.Sprintf("%-24s  %-10s", truncate(row.Model, 24), bStr)
		for _, l := range m.table.Levels {
			acc := row.Cells[l].Acc * 100
			styledAcc := StyleSuccess.Render(fmt.Sprintf("%5.1f%%", acc))
			if acc < 70 {
				styledAcc = StyleError.Render(fmt.Sprintf("%5.1f%%", acc))
			} else if acc < 90 {
				styledAcc = StyleWarning.Render(fmt.Sprintf("%5.1f%%", acc))
			}
			line += fmt.Sprintf("  %s  ", styledAcc)
		}
		if row.HasCliff {
			line += "  " + StyleBadgeWarn.Render(fmt.Sprintf(">= %s", row.Cliff))
		} else {
			line += "  " + StyleBadgeGood.Render("NONE OBSERVED")
		}
		rows = append(rows, line)
	}

	return strings.Join(rows, "\n")
}

func (m ResultsViewModel) renderStatsView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Statistical Analysis & Bootstrap Confidence Intervals (Drop vs None):")+"\n")

	// Extract baseline accuracy vector
	baselineScores := make([]float64, 0)
	for _, r := range m.runResult.Results {
		if r.CompressionLevel == domain.LevelNone {
			v := 0.0
			if r.Correct() {
				v = 1.0
			}
			baselineScores = append(baselineScores, v)
		}
	}

	rng := rand.New(rand.NewSource(42))
	for _, l := range m.runResult.Levels {
		if l == domain.LevelNone {
			continue
		}
		levelScores := make([]float64, 0)
		for _, r := range m.runResult.Results {
			if r.CompressionLevel == l {
				v := 0.0
				if r.Correct() {
					v = 1.0
				}
				levelScores = append(levelScores, v)
			}
		}

		ciLow, ciHigh := stats.BootstrapDropCI(baselineScores, levelScores, 1000, rng)
		baseAcc := stats.Accuracy(filterLevel(m.runResult.Results, domain.LevelNone))
		lvlAcc := stats.Accuracy(filterLevel(m.runResult.Results, l))
		drop := (baseAcc - lvlAcc) * 100

		status := StyleSuccess.Render("STABLE")
		if drop >= 10.0 {
			status = StyleError.Render("CLIFF DETECTED")
		}

		rows = append(rows, fmt.Sprintf("• Level %-7s: Accuracy: %.1f%% (Drop: %+.1fpp) | 95%% CI: [%+.1f%%, %+.1f%%] | Status: %s",
			StyleSelected.Render(string(l)),
			lvlAcc*100,
			-drop,
			ciLow*100,
			ciHigh*100,
			status,
		))
	}

	return strings.Join(rows, "\n")
}

func filterLevel(cells []domain.CellResult, l domain.CompressionLevel) []domain.CellResult {
	var out []domain.CellResult
	for _, c := range cells {
		if c.CompressionLevel == l {
			out = append(out, c)
		}
	}
	return out
}

func (m ResultsViewModel) renderTaskLogView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Detailed Per-Task Benchmark Results:")+"\n")
	rows = append(rows, fmt.Sprintf("%-8s  %-24s  %-20s  %-8s  %-8s",
		"Level", "Task ID", "Tool Called", "Selection", "Params"))
	rows = append(rows, strings.Repeat("─", 76))

	limit := min(len(m.runResult.Results), 15)
	for i := 0; i < limit; i++ {
		c := m.runResult.Results[i]
		sel := StyleSuccess.Render("✓ Match")
		if !c.SelectionCorrect {
			sel = StyleError.Render("✗ Miss")
		}
		param := StyleSuccess.Render("✓ Match")
		if !c.ParamFillCorrect {
			param = StyleError.Render("✗ Miss")
		}
		rows = append(rows, fmt.Sprintf("%-8s  %-24s  %-20s  %-8s  %-8s",
			c.CompressionLevel,
			truncate(c.TaskID, 24),
			truncate(c.ToolSelected, 20),
			sel,
			param,
		))
	}
	if len(m.runResult.Results) > 15 {
		rows = append(rows, StyleSubtitle.Render(fmt.Sprintf("... and %d more task cells", len(m.runResult.Results)-15)))
	}

	return strings.Join(rows, "\n")
}

func (m ResultsViewModel) renderArtifactsView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Saved Reports & Artifacts:"))
	if m.jsonPath != "" {
		rows = append(rows, fmt.Sprintf("• Raw JSON Trace:   %s", StyleSelected.Render(m.jsonPath)))
	}
	if m.htmlPath != "" {
		rows = append(rows, fmt.Sprintf("• HTML Report:      %s", StyleSelected.Render(m.htmlPath)))
	}
	if m.mdPath != "" {
		rows = append(rows, fmt.Sprintf("• Markdown Table:   %s", StyleSelected.Render(m.mdPath)))
	}
	return strings.Join(rows, "\n")
}
