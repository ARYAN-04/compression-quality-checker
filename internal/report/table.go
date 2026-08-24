package report

import (
	"fmt"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// SafetyCell is one model x level accuracy entry of the safety matrix.
type SafetyCell struct {
	Model  string
	B      float64
	Level  domain.CompressionLevel
	Acc    float64
	CILow  float64
	CIHigh float64
}

// SafetyTable aggregates per-model cliffs and per-cell accuracies.
type SafetyTable struct {
	Levels []domain.CompressionLevel
	Rows   []SafetyRow
}

// SafetyRow is one model's row across all levels plus its verdict.
type SafetyRow struct {
	Model    string
	B        float64
	Cells    map[domain.CompressionLevel]SafetyCell
	Cliff    domain.CompressionLevel
	HasCliff bool
	Baseline float64
}

// BuildTable groups cell results into a safety matrix and computes cliffs.
func BuildTable(run domain.RunResult) SafetyTable {
	levels := run.Levels
	if len(levels) == 0 {
		levels = domain.AllLevels
	}
	byModel := map[string][]domain.CellResult{}
	for _, r := range run.Results {
		byModel[r.Model] = append(byModel[r.Model], r)
	}
	table := SafetyTable{Levels: levels}
	models := make([]string, 0, len(byModel))
	for m := range byModel {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		results := byModel[m]
		row := SafetyRow{Model: m, Cells: map[domain.CompressionLevel]SafetyCell{}}
		scores := map[domain.CompressionLevel]float64{}
		for _, lvl := range levels {
			acc := levelAccuracy(results, lvl)
			scores[lvl] = acc
			row.Cells[lvl] = SafetyCell{Model: m, Level: lvl, Acc: acc}
			if lvl == domain.LevelNone {
				row.Baseline = acc
			}
		}
		cliff, _, found := cliffOf(levels, scores)
		row.Cliff, row.HasCliff = cliff, found
		table.Rows = append(table.Rows, row)
	}
	return table
}

// Markdown renders the safety matrix as a markdown table.
func (t SafetyTable) Markdown() string {
	var b strings.Builder
	b.WriteString("| Model |")
	for _, l := range t.Levels {
		fmt.Fprintf(&b, " %s |", l)
	}
	b.WriteString(" Failure Cliff |\n")
	b.WriteString("|---|")
	for range t.Levels {
		b.WriteString("---|")
	}
	b.WriteString("---|\n")
	for _, row := range t.Rows {
		fmt.Fprintf(&b, "| %s |", row.Model)
		for _, l := range t.Levels {
			fmt.Fprintf(&b, " %.1f%% |", row.Cells[l].Acc*100)
		}
		if row.HasCliff {
			fmt.Fprintf(&b, " >= %s |\n", row.Cliff)
		} else {
			b.WriteString(" none observed |\n")
		}
	}
	return b.String()
}
