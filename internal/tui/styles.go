package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme color constants.
const (
	ColorElectricCyan = lipgloss.Color("#00E5FF")
	ColorSoftGreen    = lipgloss.Color("#00E676")
	ColorAmber        = lipgloss.Color("#FFB300")
	ColorSlateGray    = lipgloss.Color("#64748B")
	ColorDarkSlate    = lipgloss.Color("#1E293B")
	ColorMutedBorder  = lipgloss.Color("#334155")
	ColorBrightText   = lipgloss.Color("#F8FAFC")
	ColorDimText      = lipgloss.Color("#94A3B8")
	ColorDangerRed    = lipgloss.Color("#EF4444")
	ColorPurpleAccent = lipgloss.Color("#818CF8")
)

// Core Lipgloss styles.
var (
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorElectricCyan)

	StyleSubtitle = lipgloss.NewStyle().
			Foreground(ColorSlateGray)

	StyleSectionTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBrightText)

	StyleLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBrightText)

	StyleHelp = lipgloss.NewStyle().
			Foreground(ColorSlateGray).
			Italic(true)

	StyleError = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorDangerRed)

	StyleSuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSoftGreen)

	StyleWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAmber)

	StyleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorElectricCyan)

	StyleUnselected = lipgloss.NewStyle().
			Foreground(ColorDimText)

	StyleActiveItem = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorElectricCyan).
			Background(lipgloss.Color("#0F172A")).
			Padding(0, 1)

	StyleInactiveItem = lipgloss.NewStyle().
				Foreground(ColorDimText).
				Padding(0, 1)

	StylePillActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorElectricCyan).
			Padding(0, 1)

	StylePillDone = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorSoftGreen).
			Padding(0, 1)

	StylePillInactive = lipgloss.NewStyle().
				Foreground(ColorDimText).
				Background(ColorMutedBorder).
				Padding(0, 1)

	StyleBadgeGood = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorSoftGreen).
			Padding(0, 1)

	StyleBadgeWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorAmber).
			Padding(0, 1)

	StyleBadgeBad = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorDangerRed).
			Padding(0, 1)
)

var stepNames = []string{
	"1. Provider & Model",
	"2. MCP Suite",
	"3. Compression",
	"4. Live Runner",
	"5. Results",
}

// RenderBreadcrumbs returns a styled navigation bar across wizard steps.
func RenderBreadcrumbs(currentStep int) string {
	var pills []string
	for i, name := range stepNames {
		switch {
		case i == currentStep:
			pills = append(pills, StylePillActive.Render(name))
		case i < currentStep:
			pills = append(pills, StylePillDone.Render("✓ "+name))
		default:
			pills = append(pills, StylePillInactive.Render(name))
		}
	}
	separator := lipgloss.NewStyle().Foreground(ColorSlateGray).Render(" ── ")
	return strings.Join(pills, separator)
}

// RenderHeader renders the common top header banner for the wizard.
func RenderHeader(currentStep int, subtitle string) string {
	var b strings.Builder
	title := StyleTitle.Render("⚡ CAN-I-COMPRESS-IT") + " " +
		StyleSubtitle.Render("— MCP Schema Compression Benchmark Wizard")
	b.WriteString(title + "\n\n")
	b.WriteString(RenderBreadcrumbs(currentStep) + "\n")
	if subtitle != "" {
		b.WriteString("\n" + StyleSubtitle.Render(subtitle) + "\n")
	}
	return b.String()
}
