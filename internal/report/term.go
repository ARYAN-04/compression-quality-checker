package report

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TermTable renders the safety matrix as a styled terminal table.
func (t SafetyTable) TermTable(w io.Writer) {
	header := []string{"model", "params(b)"}
	for _, l := range t.Levels {
		header = append(header, string(l))
	}
	header = append(header, "cliff")

	rows := [][]string{header}
	for _, row := range t.Rows {
		r := []string{row.Model, formatB(row.B)}
		for _, l := range t.Levels {
			r = append(r, fmt.Sprintf("%.1f%%", row.Cells[l].Acc*100))
		}
		if row.HasCliff {
			r = append(r, fmt.Sprintf(">=%s", row.Cliff))
		} else {
			r = append(r, "-")
		}
		rows = append(rows, r)
	}

	style := lipgloss.NewStyle().Padding(0, 1)
	tbl := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("63"))).
		StyleFunc(func(int, int) lipgloss.Style { return style }).
		Rows(rows...)
	fmt.Fprintln(w, tbl.Render())
}
