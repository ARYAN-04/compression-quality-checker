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
// Multiple runs (one per subject model) aggregate into a unified table.
func BuildTable(runs []domain.RunResult) SafetyTable {
	levels := unionLevels(runs)
	byModel := map[string][]domain.CellResult{}
	params := map[string]float64{}
	for _, run := range runs {
		if run.ParameterCountB > 0 {
			params[run.Model] = run.ParameterCountB
		}
		for _, r := range run.Results {
			byModel[r.Model] = append(byModel[r.Model], r)
		}
	}
	table := SafetyTable{Levels: levels}
	models := make([]string, 0, len(byModel))
	for m := range byModel {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		results := byModel[m]
		row := SafetyRow{Model: m, B: params[m], Cells: map[domain.CompressionLevel]SafetyCell{}}
		scores := map[domain.CompressionLevel]float64{}
		for _, lvl := range levels {
			acc := levelAccuracy(results, lvl)
			scores[lvl] = acc
			row.Cells[lvl] = SafetyCell{Model: m, B: params[m], Level: lvl, Acc: acc}
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

// unionLevels merges all runs' level sweeps in canonical intensity order.
func unionLevels(runs []domain.RunResult) []domain.CompressionLevel {
	seen := map[domain.CompressionLevel]bool{}
	out := make([]domain.CompressionLevel, 0, len(domain.AllLevels))
	for _, run := range runs {
		for _, l := range run.Levels {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return levelOrder(out[i]) < levelOrder(out[j]) })
	return out
}

// formatB renders a parameter count, or "-" when unknown.
func formatB(b float64) string {
	if b <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", b)
}

// Markdown renders the safety matrix as a markdown table.
func (t SafetyTable) Markdown() string {
	var b strings.Builder
	b.WriteString("| Model | Params (B) |")
	for _, l := range t.Levels {
		fmt.Fprintf(&b, " %s |", l)
	}
	b.WriteString(" Failure Cliff |\n")
	b.WriteString("|---|---|")
	for range t.Levels {
		b.WriteString("---|")
	}
	b.WriteString("---|\n")
	for _, row := range t.Rows {
		fmt.Fprintf(&b, "| %s | %s |", row.Model, formatB(row.B))
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
