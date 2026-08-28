package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"can-i-compress-it/internal/domain"
)

// LevelCheckItem represents a compression level checkbox in the UI.
type LevelCheckItem struct {
	Level       domain.CompressionLevel
	Name        string
	Description string
	Selected    bool
}

// CompressorBackend represents built-in vs subprocess binary.
type CompressorBackend int

const (
	BackendBuiltin CompressorBackend = iota
	BackendSubprocess
)

// CompressionStepModel manages Step 3 of the wizard.
type CompressionStepModel struct {
	levels        []LevelCheckItem
	levelIdx      int
	backend       CompressorBackend
	compressorBin textinput.Model
	jsonPathInput textinput.Model
	htmlPathInput textinput.Model
	mdPathInput   textinput.Model
	focusedField  int
	errMessage    string
}

const (
	focusLevels = iota
	focusBackend
	focusCompressorBin
	focusJSONPath
	focusHTMLPath
	focusMDPath
	focusStartButton
	focusCompCount
)

// NewCompressionStepModel creates an initialized Step 3 model.
func NewCompressionStepModel() CompressionStepModel {
	m := CompressionStepModel{
		levels: []LevelCheckItem{
			{Level: domain.LevelNone, Name: "none", Description: "Uncompressed raw baseline schema", Selected: true},
			{Level: domain.LevelLow, Name: "low", Description: "Prune description whitespace, omit optional titles", Selected: true},
			{Level: domain.LevelMedium, Name: "medium", Description: "Compress field descriptions, retain strict types", Selected: true},
			{Level: domain.LevelHigh, Name: "high", Description: "Aggressive token pruning (short keys & min types)", Selected: true},
			{Level: domain.LevelMax, Name: "max", Description: "Two-stage meta discovery: get_tool_schema + invoke_tool", Selected: true},
		},
		levelIdx:      0,
		backend:       BackendBuiltin,
		focusedField:  focusLevels,
		compressorBin: textinput.New(),
		jsonPathInput: textinput.New(),
		htmlPathInput: textinput.New(),
		mdPathInput:   textinput.New(),
	}

	m.compressorBin.Placeholder = "mcp-compressor"
	m.compressorBin.Prompt = "Binary Path:    "
	m.compressorBin.PromptStyle = StyleLabel
	m.compressorBin.SetValue("mcp-compressor")

	m.jsonPathInput.Placeholder = "./results/results.json"
	m.jsonPathInput.Prompt = "JSON Trace:     "
	m.jsonPathInput.PromptStyle = StyleLabel
	m.jsonPathInput.SetValue("./results/results.json")

	m.htmlPathInput.Placeholder = "./results/report.html"
	m.htmlPathInput.Prompt = "HTML Report:    "
	m.htmlPathInput.PromptStyle = StyleLabel
	m.htmlPathInput.SetValue("./results/report.html")

	m.mdPathInput.Placeholder = "./results/table.md"
	m.mdPathInput.Prompt = "Markdown Table: "
	m.mdPathInput.PromptStyle = StyleLabel
	m.mdPathInput.SetValue("./results/table.md")

	return m
}

// SetSize adapts input component widths to window size.
func (m *CompressionStepModel) SetSize(width, height int) {
	w := max(20, min(60, width-25))
	m.compressorBin.Width = w
	m.jsonPathInput.Width = w
	m.htmlPathInput.Width = w
	m.mdPathInput.Width = w
}

// Update handles user input on Step 3.
func (m CompressionStepModel) Update(msg tea.Msg) (CompressionStepModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.errMessage = ""
			m.handleNavigate(1)
			return m, nil

		case "shift+tab", "up":
			m.errMessage = ""
			m.handleNavigate(-1)
			return m, nil

		case "left", "right":
			if m.focusedField == focusBackend {
				if m.backend == BackendBuiltin {
					m.backend = BackendSubprocess
				} else {
					m.backend = BackendBuiltin
				}
				return m, nil
			}

		case "a", "A":
			if m.focusedField == focusLevels {
				for i := range m.levels {
					m.levels[i].Selected = true
				}
				return m, nil
			}

		case " ":
			if m.focusedField == focusLevels {
				m.levels[m.levelIdx].Selected = !m.levels[m.levelIdx].Selected
				return m, nil
			}
			if m.focusedField == focusBackend {
				if m.backend == BackendBuiltin {
					m.backend = BackendSubprocess
				} else {
					m.backend = BackendBuiltin
				}
				return m, nil
			}
		}
	}

	// Update text inputs
	var cmd tea.Cmd
	switch m.focusedField {
	case focusCompressorBin:
		m.compressorBin, cmd = m.compressorBin.Update(msg)
		cmds = append(cmds, cmd)
	case focusJSONPath:
		m.jsonPathInput, cmd = m.jsonPathInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusHTMLPath:
		m.htmlPathInput, cmd = m.htmlPathInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusMDPath:
		m.mdPathInput, cmd = m.mdPathInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *CompressionStepModel) handleNavigate(delta int) {
	if m.focusedField == focusLevels {
		nextIdx := m.levelIdx + delta
		if nextIdx >= 0 && nextIdx < len(m.levels) {
			m.levelIdx = nextIdx
			return
		}
	}

	m.focusedField = (m.focusedField + delta + focusCompCount) % focusCompCount
	if m.focusedField == focusCompressorBin && m.backend != BackendSubprocess {
		m.focusedField = (m.focusedField + delta + focusCompCount) % focusCompCount
	}
	m.updateFocus()
}

func (m *CompressionStepModel) updateFocus() {
	m.compressorBin.Blur()
	m.jsonPathInput.Blur()
	m.htmlPathInput.Blur()
	m.mdPathInput.Blur()

	switch m.focusedField {
	case focusCompressorBin:
		m.compressorBin.Focus()
	case focusJSONPath:
		m.jsonPathInput.Focus()
	case focusHTMLPath:
		m.htmlPathInput.Focus()
	case focusMDPath:
		m.mdPathInput.Focus()
	}
}

// SelectedLevels returns the parsed compression levels.
func (m CompressionStepModel) SelectedLevels() ([]domain.CompressionLevel, error) {
	var out []domain.CompressionLevel
	for _, l := range m.levels {
		if l.Selected {
			out = append(out, l.Level)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one compression level must be selected")
	}
	return out, nil
}

// CompressorBinary returns compressor path if subprocess mode is chosen.
func (m CompressionStepModel) CompressorBinary() string {
	if m.backend != BackendSubprocess {
		return ""
	}
	bin := strings.TrimSpace(m.compressorBin.Value())
	if bin == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	} else if strings.HasPrefix(bin, "~/") || strings.HasPrefix(bin, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, bin[2:])
		}
	}
	return bin
}

// View renders Step 3.
func (m CompressionStepModel) View() string {
	var b strings.Builder
	b.WriteString(RenderHeader(2, "Select compression levels to sweep, backend engine, and artifact outputs."))
	b.WriteString("\n\n")

	// Levels section
	b.WriteString(StyleSectionTitle.Render("1. Compression Levels Sweep:") + "\n")
	for i, l := range m.levels {
		check := "[ ]"
		if l.Selected {
			check = StyleSuccess.Render("[✓]")
		}
		cursor := "  "
		if m.focusedField == focusLevels && i == m.levelIdx {
			cursor = StyleSelected.Render("▶ ")
		}
		b.WriteString(fmt.Sprintf("%s%s %-8s %s\n",
			cursor, check, StyleLabel.Render(string(l.Level)), StyleSubtitle.Render(l.Description)))
	}
	b.WriteString("\n")

	// Backend section
	var backendPills []string
	if m.backend == BackendBuiltin {
		backendPills = append(backendPills, StyleActiveItem.Render("● Built-in Pure Go (Default)"))
		backendPills = append(backendPills, StyleUnselected.Render("○ Subprocess Binary (mcp-compressor)"))
	} else {
		backendPills = append(backendPills, StyleUnselected.Render("○ Built-in Pure Go (Default)"))
		backendPills = append(backendPills, StyleActiveItem.Render("● Subprocess Binary (mcp-compressor)"))
	}
	cursorBackend := "  "
	if m.focusedField == focusBackend {
		cursorBackend = StyleSelected.Render("▶ ")
	}

	b.WriteString(StyleSectionTitle.Render("2. Compressor Backend:") + "\n")
	b.WriteString(cursorBackend + strings.Join(backendPills, "   ") + "\n")
	if m.backend == BackendSubprocess {
		cursorBin := "  "
		if m.focusedField == focusCompressorBin {
			cursorBin = StyleSelected.Render("▶ ")
		}
		b.WriteString(cursorBin + m.compressorBin.View() + "\n")
	}
	b.WriteString("\n")

	// Output paths section
	b.WriteString(StyleSectionTitle.Render("3. Report & Artifact Targets:") + "\n")
	b.WriteString("  " + m.jsonPathInput.View() + "\n")
	b.WriteString("  " + m.htmlPathInput.View() + "\n")
	b.WriteString("  " + m.mdPathInput.View() + "\n\n")

	// Start button
	btnText := "  🚀 START BENCHMARK SWEEP  "
	if m.focusedField == focusStartButton {
		b.WriteString(StyleActiveItem.Render(btnText) + "\n")
	} else {
		b.WriteString(StylePillActive.Render(btnText) + "\n")
	}

	if m.errMessage != "" {
		b.WriteString("\n" + StyleError.Render("⚠ "+m.errMessage) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("Controls: [Tab/↑↓] Navigate • [Space] Toggle • [A] Select All Levels • [Enter] Start Benchmark • [Esc] Back"))
	return b.String()
}
