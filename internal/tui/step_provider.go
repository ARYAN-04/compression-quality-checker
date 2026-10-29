package tui

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ProviderPreset defines a preset inference provider configuration.
type ProviderPreset struct {
	Name           string
	Endpoint       string
	DefaultModel   string
	DefaultParamsB float64
	RequiresAPIKey bool
	IsMock         bool
	Description    string
}

// Available provider presets.
var defaultPresets = []ProviderPreset{
	{
		Name:           "LM Studio",
		Endpoint:       "http://localhost:1234/v1",
		DefaultModel:   "qwen2.5-7b-instruct",
		DefaultParamsB: 7.0,
		RequiresAPIKey: false,
		Description:    "Local or remote LM Studio inference server (/v1)",
	},
	{
		Name:           "Ollama",
		Endpoint:       "http://localhost:11434/v1",
		DefaultModel:   "llama3.1:8b",
		DefaultParamsB: 8.0,
		RequiresAPIKey: false,
		Description:    "Local Ollama daemon (/v1)",
	},
	{
		Name:           "vLLM",
		Endpoint:       "http://localhost:8000/v1",
		DefaultModel:   "meta-llama/Meta-Llama-3-8B-Instruct",
		DefaultParamsB: 8.0,
		RequiresAPIKey: false,
		Description:    "High-throughput local vLLM server",
	},
	{
		Name:           "OpenAI / OpenRouter",
		Endpoint:       "https://openrouter.ai/api/v1",
		DefaultModel:   "meta-llama/llama-3.1-8b-instruct",
		DefaultParamsB: 8.0,
		RequiresAPIKey: true,
		Description:    "Remote OpenAI-compatible cloud API",
	},
	{
		Name:           "Offline Mock",
		Endpoint:       "mock",
		DefaultModel:   "mock-7b",
		DefaultParamsB: 7.0,
		RequiresAPIKey: false,
		IsMock:         true,
		Description:    "Deterministic offline test simulator",
	},
	{
		Name:           "Custom",
		Endpoint:       "http://localhost:8080/v1",
		DefaultModel:   "custom-model-8b",
		DefaultParamsB: 8.0,
		RequiresAPIKey: false,
		Description:    "Custom OpenAI-compatible HTTP endpoint",
	},
}

// Focus indices for step 1.
const (
	focusPreset = iota
	focusModel
	focusEndpoint
	focusAPIKey
	focusParams
	focusCount
)

// ProviderStepModel manages Step 1 of the wizard.
type ProviderStepModel struct {
	presetIndex   int
	presets       []ProviderPreset
	focused       int
	modelInput    textinput.Model
	endpointInput textinput.Model
	apiKeyInput   textinput.Model
	paramsInput   textinput.Model
	errMessage    string
}

// NewProviderStepModel creates an initialized Step 1 model.
func NewProviderStepModel() ProviderStepModel {
	m := ProviderStepModel{
		presets:       defaultPresets,
		presetIndex:   0,
		focused:       focusModel,
		modelInput:    textinput.New(),
		endpointInput: textinput.New(),
		apiKeyInput:   textinput.New(),
		paramsInput:   textinput.New(),
	}

	m.modelInput.Placeholder = "e.g. llama-3.1-8b-instruct or qwen2.5-7b"
	m.modelInput.Prompt = "Model Name:     "
	m.modelInput.PromptStyle = StyleLabel
	m.modelInput.Focus()

	m.endpointInput.Placeholder = "http://localhost:1234/v1 or http://<ip>:1234"
	m.endpointInput.Prompt = "Endpoint URL:   "
	m.endpointInput.PromptStyle = StyleLabel

	m.apiKeyInput.Placeholder = "Optional bearer token (masked)"
	m.apiKeyInput.Prompt = "API Key:        "
	m.apiKeyInput.PromptStyle = StyleLabel
	m.apiKeyInput.EchoMode = textinput.EchoPassword
	m.apiKeyInput.EchoCharacter = '•'

	m.paramsInput.Placeholder = "e.g. 8.0 or 70.0"
	m.paramsInput.Prompt = "Parameters (B): "
	m.paramsInput.PromptStyle = StyleLabel

	m.applyPreset(0)

	if envEndpoint := os.Getenv("CAN_I_COMPRESS_IT_ENDPOINT"); envEndpoint != "" {
		m.endpointInput.SetValue(envEndpoint)
	}
	if envModel := os.Getenv("CAN_I_COMPRESS_IT_MODEL"); envModel != "" {
		m.modelInput.SetValue(envModel)
	}
	if envParams := os.Getenv("CAN_I_COMPRESS_IT_PARAMS_B"); envParams != "" {
		m.paramsInput.SetValue(envParams)
	}

	return m
}

func (m *ProviderStepModel) applyPreset(idx int) {
	if idx < 0 || idx >= len(m.presets) {
		return
	}
	m.presetIndex = idx
	p := m.presets[idx]
	if m.modelInput.Value() == "" || m.isDefaultModelOfAnyPreset(m.modelInput.Value()) {
		m.modelInput.SetValue(p.DefaultModel)
	}
	m.endpointInput.SetValue(p.Endpoint)
	if p.DefaultParamsB > 0 {
		m.paramsInput.SetValue(strconv.FormatFloat(p.DefaultParamsB, 'f', -1, 64))
	}
}

// SetSize adapts input component widths to window size.
func (m *ProviderStepModel) SetSize(width, height int) {
	w := max(20, min(60, width-25))
	m.modelInput.Width = w
	m.endpointInput.Width = w
	m.apiKeyInput.Width = w
	m.paramsInput.Width = max(10, min(30, width-25))
}

func (m *ProviderStepModel) isDefaultModelOfAnyPreset(val string) bool {
	for _, p := range m.presets {
		if p.DefaultModel == val {
			return true
		}
	}
	return false
}

// DetectParamsB extracts parameter size in billions from model naming conventions.
func DetectParamsB(name string) float64 {
	// Look for pattern like 8b, 70b, 1.5b, 0.5b, 3.8b, 14b, 32b, 671b
	reB := regexp.MustCompile(`(?i)(?:^|[-_./:\s])(\d+(?:\.\d+)?)\s*[bB](?:[-_./:\s]|$)`)
	if match := reB.FindStringSubmatch(name); len(match) > 1 {
		if v, err := strconv.ParseFloat(match[1], 64); err == nil {
			return v
		}
	}
	// Look for pattern like 360m, 135m (millions -> billions)
	reM := regexp.MustCompile(`(?i)(?:^|[-_./:\s])(\d+(?:\.\d+)?)\s*[mM](?:[-_./:\s]|$)`)
	if match := reM.FindStringSubmatch(name); len(match) > 1 {
		if v, err := strconv.ParseFloat(match[1], 64); err == nil {
			return v / 1000.0
		}
	}
	return 0
}

// Update handles key messages for Step 1.
func (m ProviderStepModel) Update(msg tea.Msg) (ProviderStepModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.errMessage = ""
			m.focused = (m.focused + 1) % focusCount
			m.updateFocus()
			return m, nil

		case "shift+tab", "up":
			m.errMessage = ""
			m.focused = (m.focused - 1 + focusCount) % focusCount
			m.updateFocus()
			return m, nil

		case "left":
			if m.focused == focusPreset {
				m.presetIndex = (m.presetIndex - 1 + len(m.presets)) % len(m.presets)
				m.applyPreset(m.presetIndex)
				return m, nil
			}

		case "right":
			if m.focused == focusPreset {
				m.presetIndex = (m.presetIndex + 1) % len(m.presets)
				m.applyPreset(m.presetIndex)
				return m, nil
			}
		}
	}

	// Update text inputs
	var cmd tea.Cmd
	switch m.focused {
	case focusModel:
		prevVal := m.modelInput.Value()
		m.modelInput, cmd = m.modelInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.modelInput.Value() != prevVal {
			if detected := DetectParamsB(m.modelInput.Value()); detected > 0 {
				m.paramsInput.SetValue(strconv.FormatFloat(detected, 'f', -1, 64))
			}
		}
	case focusEndpoint:
		m.endpointInput, cmd = m.endpointInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusAPIKey:
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusParams:
		m.paramsInput, cmd = m.paramsInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *ProviderStepModel) updateFocus() {
	m.modelInput.Blur()
	m.endpointInput.Blur()
	m.apiKeyInput.Blur()
	m.paramsInput.Blur()

	switch m.focused {
	case focusModel:
		m.modelInput.Focus()
	case focusEndpoint:
		m.endpointInput.Focus()
	case focusAPIKey:
		m.apiKeyInput.Focus()
	case focusParams:
		m.paramsInput.Focus()
	}
}

// Validate checks whether the step configuration is ready to advance.
func (m *ProviderStepModel) Validate() error {
	if strings.TrimSpace(m.modelInput.Value()) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(m.endpointInput.Value()) == "" {
		return fmt.Errorf("endpoint URL is required")
	}
	return nil
}

// ParamsB returns the parsed parameter count.
func (m ProviderStepModel) ParamsB() float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(m.paramsInput.Value()), 64)
	if err != nil {
		return 0
	}
	return val
}

// IsMock returns true if the selected preset is the offline mock.
func (m ProviderStepModel) IsMock() bool {
	return m.presets[m.presetIndex].IsMock || m.endpointInput.Value() == "mock"
}

// View renders Step 1.
func (m ProviderStepModel) View() string {
	var b strings.Builder
	b.WriteString(RenderHeader(0, "Configure the inference provider endpoint and target model."))
	b.WriteString("\n\n")

	// Preset selector section
	b.WriteString(StyleSectionTitle.Render("Inference Endpoint Preset:") + "\n")
	var presetPills []string
	for i, p := range m.presets {
		label := p.Name
		if i == m.presetIndex {
			if m.focused == focusPreset {
				presetPills = append(presetPills, StyleActiveItem.Render("● "+label))
			} else {
				presetPills = append(presetPills, StyleSelected.Render("● "+label))
			}
		} else {
			presetPills = append(presetPills, StyleUnselected.Render("○ "+label))
		}
	}
	b.WriteString("  " + strings.Join(presetPills, "  ") + "\n")
	b.WriteString("  " + StyleHelp.Render("Description: "+m.presets[m.presetIndex].Description) + "\n\n")

	// Input fields section
	b.WriteString(StyleSectionTitle.Render("Target Model & Connection Parameters:") + "\n")
	b.WriteString("  " + m.modelInput.View() + "\n")
	b.WriteString("  " + m.endpointInput.View() + "\n")
	b.WriteString("  " + m.apiKeyInput.View() + "\n")
	b.WriteString("  " + m.paramsInput.View() + "\n")

	b.WriteString("\n" + StyleSectionTitle.Render("Execution Shortcuts:") + "\n")
	b.WriteString("  " + StyleActiveItem.Render("🚀 [F] Run Full Factorial Benchmark (All 5 Levels)") + "  " + StyleSubtitle.Render("[Enter] Configure Wizard Steps") + "\n")

	if m.errMessage != "" {
		b.WriteString("\n" + StyleError.Render("⚠ "+m.errMessage) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("Controls: [Tab/Shift+Tab] Navigate • [←/→] Switch preset • [F] Quick Full Factorial • [Enter] Next Step • [Ctrl+C] Exit"))
	return b.String()
}
