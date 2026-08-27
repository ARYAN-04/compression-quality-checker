package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/suite"
)

// SuiteMode constants.
type SuiteMode int

const (
	SuiteModeStandard  SuiteMode = iota // [1] Full Factorial Matrix (synthetic_v1)
	SuiteModeTemplates                  // [2] Pre-annotated Templates
	SuiteModeGenerator                  // [3] Synthetic Generator
	SuiteModeLive                       // [4] Live MCP Server
	SuiteModeFile                       // [5] Custom JSON File
)

// TemplateItem holds state for template checklist.
type TemplateItem struct {
	Name        string
	DisplayName string
	Description string
	ToolCount   int
	Selected    bool
}

// SuiteStepModel manages Step 2 of the wizard.
type SuiteStepModel struct {
	mode          SuiteMode
	templates     []TemplateItem
	templateIdx   int
	genTasksInput textinput.Model
	genAmbiguity  int // 0: none, 1: low, 2: high
	genStyle      int // 0: explicit, 1: intent
	genDistracts  int // 0: 0, 1: 5, 2: 10, 3: 20
	genSeedInput  textinput.Model
	genFieldIdx   int
	liveCmdInput  textinput.Model
	filePathInput textinput.Model
	errMessage    string
}

var (
	ambiguityOptions  = []string{"none", "low", "high"}
	styleOptions      = []string{"explicit", "intent"}
	distractorOptions = []int{0, 5, 10, 20}
)

// NewSuiteStepModel creates an initialized Step 2 model.
func NewSuiteStepModel() SuiteStepModel {
	m := SuiteStepModel{
		mode:         SuiteModeStandard,
		genAmbiguity: 2, // high
		genStyle:     0, // explicit
		genDistracts: 0,
		templates: []TemplateItem{
			{Name: "github", DisplayName: "GitHub MCP Server", Description: "7 tools (issues, pull requests, comments, repos)", ToolCount: 7, Selected: true},
			{Name: "jira", DisplayName: "Jira MCP Server", Description: "5 tools (issue tracking, transitions, JQL search)", ToolCount: 5, Selected: true},
			{Name: "slack", DisplayName: "Slack MCP Server", Description: "5 tools (channels, post message, threads, users)", ToolCount: 5, Selected: true},
			{Name: "sqlite", DisplayName: "SQLite MCP Server", Description: "4 tools (tables, schema describe, read/execute query)", ToolCount: 4, Selected: true},
			{Name: "filesystem", DisplayName: "Filesystem MCP Server", Description: "6 tools (list dir, read/search/move/delete file)", ToolCount: 6, Selected: true},
		},
		genTasksInput: textinput.New(),
		genSeedInput:  textinput.New(),
		liveCmdInput:  textinput.New(),
		filePathInput: textinput.New(),
	}

	m.genTasksInput.Placeholder = "12"
	m.genTasksInput.Prompt = "Task Count:      "
	m.genTasksInput.PromptStyle = StyleLabel
	m.genTasksInput.SetValue("12")

	m.genSeedInput.Placeholder = "42"
	m.genSeedInput.Prompt = "Random Seed:     "
	m.genSeedInput.PromptStyle = StyleLabel
	m.genSeedInput.SetValue("42")

	m.liveCmdInput.Placeholder = "npx -y @modelcontextprotocol/server-filesystem /tmp"
	m.liveCmdInput.Prompt = "MCP Command:     "
	m.liveCmdInput.PromptStyle = StyleLabel

	m.filePathInput.Placeholder = "./suites/synthetic_v1.json"
	m.filePathInput.Prompt = "JSON File Path:  "
	m.filePathInput.PromptStyle = StyleLabel
	m.filePathInput.SetValue("./suites/synthetic_v1.json")

	return m
}

// SetSize adapts input component widths to window size.
func (m *SuiteStepModel) SetSize(width, height int) {
	w := max(20, min(70, width-25))
	m.liveCmdInput.Width = w
	m.filePathInput.Width = w
}

// Update handles key messages for Step 2.
func (m SuiteStepModel) Update(msg tea.Msg) (SuiteStepModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "1":
			m.mode = SuiteModeStandard
			return m, nil
		case "2":
			m.mode = SuiteModeTemplates
			return m, nil
		case "3":
			m.mode = SuiteModeGenerator
			return m, nil
		case "4":
			m.mode = SuiteModeLive
			return m, nil
		case "5":
			m.mode = SuiteModeFile
			return m, nil

		case "tab":
			m.errMessage = ""
			m.handleTab(1)
			return m, nil

		case "shift+tab":
			m.errMessage = ""
			m.handleTab(-1)
			return m, nil

		case "up":
			m.handleUp()
			return m, nil

		case "down":
			m.handleDown()
			return m, nil

		case "left":
			m.handleLeft()
			return m, nil

		case "right":
			m.handleRight()
			return m, nil

		case " ":
			m.handleSpace()
			return m, nil
		}
	}

	// Update text inputs based on active mode
	var cmd tea.Cmd
	switch m.mode {
	case SuiteModeGenerator:
		if m.genFieldIdx == 0 {
			m.genTasksInput, cmd = m.genTasksInput.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.genFieldIdx == 4 {
			m.genSeedInput, cmd = m.genSeedInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	case SuiteModeLive:
		m.liveCmdInput, cmd = m.liveCmdInput.Update(msg)
		cmds = append(cmds, cmd)
	case SuiteModeFile:
		m.filePathInput, cmd = m.filePathInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *SuiteStepModel) handleTab(delta int) {
	switch m.mode {
	case SuiteModeTemplates:
		m.templateIdx = (m.templateIdx + delta + len(m.templates)) % len(m.templates)
	case SuiteModeGenerator:
		m.genFieldIdx = (m.genFieldIdx + delta + 5) % 5
		m.updateGenFocus()
	}
}

func (m *SuiteStepModel) handleUp() {
	switch m.mode {
	case SuiteModeTemplates:
		m.templateIdx = (m.templateIdx - 1 + len(m.templates)) % len(m.templates)
	case SuiteModeGenerator:
		m.genFieldIdx = (m.genFieldIdx - 1 + 5) % 5
		m.updateGenFocus()
	}
}

func (m *SuiteStepModel) handleDown() {
	switch m.mode {
	case SuiteModeTemplates:
		m.templateIdx = (m.templateIdx + 1) % len(m.templates)
	case SuiteModeGenerator:
		m.genFieldIdx = (m.genFieldIdx + 1) % 5
		m.updateGenFocus()
	}
}

func (m *SuiteStepModel) handleLeft() {
	switch m.mode {
	case SuiteModeGenerator:
		switch m.genFieldIdx {
		case 1:
			m.genAmbiguity = (m.genAmbiguity - 1 + len(ambiguityOptions)) % len(ambiguityOptions)
		case 2:
			m.genStyle = (m.genStyle - 1 + len(styleOptions)) % len(styleOptions)
		case 3:
			m.genDistracts = (m.genDistracts - 1 + len(distractorOptions)) % len(distractorOptions)
		}
	}
}

func (m *SuiteStepModel) handleRight() {
	switch m.mode {
	case SuiteModeGenerator:
		switch m.genFieldIdx {
		case 1:
			m.genAmbiguity = (m.genAmbiguity + 1) % len(ambiguityOptions)
		case 2:
			m.genStyle = (m.genStyle + 1) % len(styleOptions)
		case 3:
			m.genDistracts = (m.genDistracts + 1) % len(distractorOptions)
		}
	}
}

func (m *SuiteStepModel) handleSpace() {
	if m.mode == SuiteModeTemplates {
		m.templates[m.templateIdx].Selected = !m.templates[m.templateIdx].Selected
	}
}

func (m *SuiteStepModel) updateGenFocus() {
	m.genTasksInput.Blur()
	m.genSeedInput.Blur()
	if m.genFieldIdx == 0 {
		m.genTasksInput.Focus()
	} else if m.genFieldIdx == 4 {
		m.genSeedInput.Focus()
	}
}

// ResolveTasks synthesizes or loads tasks matching user configuration.
func (m SuiteStepModel) ResolveTasks() ([]domain.Task, error) {
	switch m.mode {
	case SuiteModeStandard:
		return suite.Builtin(), nil

	case SuiteModeTemplates:
		var allTasks []domain.Task
		for _, t := range m.templates {
			if t.Selected {
				tasks, ok := suite.Template(t.Name)
				if ok {
					allTasks = append(allTasks, tasks...)
				}
			}
		}
		if len(allTasks) == 0 {
			return nil, fmt.Errorf("at least one MCP template must be selected")
		}
		return allTasks, nil

	case SuiteModeGenerator:
		numTasks, err := strconv.Atoi(strings.TrimSpace(m.genTasksInput.Value()))
		if err != nil || numTasks <= 0 {
			numTasks = 12
		}
		seed, err := strconv.ParseInt(strings.TrimSpace(m.genSeedInput.Value()), 10, 64)
		if err != nil {
			seed = 42
		}
		opts := suite.Options{
			NumTasks:    numTasks,
			Ambiguity:   ambiguityOptions[m.genAmbiguity],
			PromptStyle: styleOptions[m.genStyle],
			Distractors: distractorOptions[m.genDistracts],
			Seed:        seed,
		}
		gen := suite.NewGenerator(opts)
		return gen.Generate(), nil

	case SuiteModeLive:
		if strings.TrimSpace(m.liveCmdInput.Value()) == "" {
			return nil, fmt.Errorf("MCP command is required for live server benchmark")
		}
		return suite.Builtin(), nil

	case SuiteModeFile:
		path := strings.TrimSpace(m.filePathInput.Value())
		if path == "" {
			return nil, fmt.Errorf("file path cannot be empty")
		}
		tasks, err := suite.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load suite: %w", err)
		}
		return tasks, nil
	}
	return suite.Builtin(), nil
}

// View renders Step 2.
func (m SuiteStepModel) View() string {
	var b strings.Builder
	b.WriteString(RenderHeader(1, "Choose benchmark task suite source or configure synthetic generator."))
	b.WriteString("\n")

	// Suite mode tabs
	modes := []string{
		"[1] Full Factorial Matrix (synthetic_v1)",
		"[2] MCP Templates",
		"[3] Synthetic Generator",
		"[4] Live MCP Server",
		"[5] Custom JSON File",
	}
	var modePills []string
	for i, name := range modes {
		if SuiteMode(i) == m.mode {
			modePills = append(modePills, StyleActiveItem.Render(name))
		} else {
			modePills = append(modePills, StyleInactiveItem.Render(name))
		}
	}
	b.WriteString(strings.Join(modePills, " ") + "\n\n")

	switch m.mode {
	case SuiteModeStandard:
		b.WriteString(m.renderStandardView())
	case SuiteModeTemplates:
		b.WriteString(m.renderTemplatesView())
	case SuiteModeGenerator:
		b.WriteString(m.renderGeneratorView())
	case SuiteModeLive:
		b.WriteString(m.renderLiveView())
	case SuiteModeFile:
		b.WriteString(m.renderFileView())
	}

	if m.errMessage != "" {
		b.WriteString("\n" + StyleError.Render("⚠ "+m.errMessage) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("Controls: [1-5] Switch tab • [Tab/↑↓] Navigate • [Space] Toggle • [Enter] Next • [Esc] Back"))
	return b.String()
}

func (m SuiteStepModel) renderStandardView() string {
	builtin := suite.Builtin()
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Canonical Full Factorial Benchmark Suite (synthetic_v1):"))
	rows = append(rows, StyleSubtitle.Render(fmt.Sprintf("Standard %d-task benchmark suite engineered to detect compression failure cliffs.", len(builtin))))
	rows = append(rows, "")

	var ambCount, overlapCount, hopCount int
	for _, t := range builtin {
		switch t.Category {
		case domain.CategoryAmbiguousName:
			ambCount++
		case domain.CategoryOverlappingSchem:
			overlapCount++
		case domain.CategoryMultiHop:
			hopCount++
		}
	}

	rows = append(rows, fmt.Sprintf("  %s %s (%d tasks)",
		StyleSuccess.Render("●"),
		StyleLabel.Render("Ambiguous Tool Names:"),
		ambCount,
	))
	rows = append(rows, "    Hallucination resistance across similar tool names (e.g. search_issues vs list_issues).")
	rows = append(rows, "")

	rows = append(rows, fmt.Sprintf("  %s %s (%d tasks)",
		StyleSuccess.Render("●"),
		StyleLabel.Render("Overlapping Schema Signatures:"),
		overlapCount,
	))
	rows = append(rows, "    Parameter type and enum binding accuracy when schema descriptions are heavily compressed.")
	rows = append(rows, "")

	rows = append(rows, fmt.Sprintf("  %s %s (%d tasks)",
		StyleSuccess.Render("●"),
		StyleLabel.Render("Multi-Hop Dependency Chains:"),
		hopCount,
	))
	rows = append(rows, "    Chained tool calls with threaded output variables (e.g. session_id, issue_key).")
	rows = append(rows, "")

	rows = append(rows, StyleHelp.Render(fmt.Sprintf("Full Matrix Size: %d tasks × 5 compression levels = %d factorial evaluation cells.", len(builtin), len(builtin)*5)))
	return strings.Join(rows, "\n")
}

func (m SuiteStepModel) renderTemplatesView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Select MCP Server Suites to Benchmark:")+"\n")

	for i, t := range m.templates {
		check := "[ ]"
		if t.Selected {
			check = StyleSuccess.Render("[✓]")
		}
		cursor := "  "
		if i == m.templateIdx {
			cursor = StyleSelected.Render("▶ ")
		}
		name := StyleLabel.Render(t.DisplayName)
		desc := StyleSubtitle.Render(fmt.Sprintf("(%s)", t.Description))
		rows = append(rows, fmt.Sprintf("%s%s %s %s", cursor, check, name, desc))
	}

	return strings.Join(rows, "\n")
}

func (m SuiteStepModel) renderGeneratorView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Synthetic Benchmark Generator Configuration:")+"\n")

	// Task count
	cursor0 := "  "
	if m.genFieldIdx == 0 {
		cursor0 = StyleSelected.Render("▶ ")
	}
	rows = append(rows, cursor0+m.genTasksInput.View())

	// Ambiguity
	cursor1 := "  "
	if m.genFieldIdx == 1 {
		cursor1 = StyleSelected.Render("▶ ")
	}
	var ambPills []string
	for i, a := range ambiguityOptions {
		if i == m.genAmbiguity {
			ambPills = append(ambPills, StyleActiveItem.Render("● "+a))
		} else {
			ambPills = append(ambPills, StyleUnselected.Render("○ "+a))
		}
	}
	rows = append(rows, fmt.Sprintf("%s%s %s", cursor1, StyleLabel.Render("Ambiguity:       "), strings.Join(ambPills, " ")))

	// Style
	cursor2 := "  "
	if m.genFieldIdx == 2 {
		cursor2 = StyleSelected.Render("▶ ")
	}
	var stylePills []string
	for i, s := range styleOptions {
		if i == m.genStyle {
			stylePills = append(stylePills, StyleActiveItem.Render("● "+s))
		} else {
			stylePills = append(stylePills, StyleUnselected.Render("○ "+s))
		}
	}
	rows = append(rows, fmt.Sprintf("%s%s %s", cursor2, StyleLabel.Render("Prompt Style:    "), strings.Join(stylePills, " ")))

	// Distractors
	cursor3 := "  "
	if m.genFieldIdx == 3 {
		cursor3 = StyleSelected.Render("▶ ")
	}
	var distPills []string
	for i, d := range distractorOptions {
		label := fmt.Sprintf("%d tools", d)
		if d == 0 {
			label = "None"
		}
		if i == m.genDistracts {
			distPills = append(distPills, StyleActiveItem.Render("● "+label))
		} else {
			distPills = append(distPills, StyleUnselected.Render("○ "+label))
		}
	}
	rows = append(rows, fmt.Sprintf("%s%s %s", cursor3, StyleLabel.Render("Distractor Tools:"), strings.Join(distPills, " ")))

	// Seed
	cursor4 := "  "
	if m.genFieldIdx == 4 {
		cursor4 = StyleSelected.Render("▶ ")
	}
	rows = append(rows, cursor4+m.genSeedInput.View())

	return strings.Join(rows, "\n\n")
}

func (m SuiteStepModel) renderLiveView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Live MCP Stdio Server Integration:"))
	rows = append(rows, StyleSubtitle.Render("Spawns a local MCP server subprocess via stdio, extracts schemas, and validates proxy compatibility."))
	rows = append(rows, "")
	rows = append(rows, "  "+m.liveCmdInput.View())
	return strings.Join(rows, "\n")
}

func (m SuiteStepModel) renderFileView() string {
	var rows []string
	rows = append(rows, StyleSectionTitle.Render("Custom Benchmark Suite File:"))
	rows = append(rows, StyleSubtitle.Render("Load task definitions and ground truth from a JSON file."))
	rows = append(rows, "")
	rows = append(rows, "  "+m.filePathInput.View())
	return strings.Join(rows, "\n")
}
