package cmd

import (
	"fmt"
	"math/rand"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/report"
	"can-i-compress-it/internal/stats"
)

var analyzeFlags struct {
	input      string
	iterations int
	outputTbl  string
	htmlReport string
	seed       int64
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Recalculate bootstrap intervals and failure cliffs from a trace",
	RunE: func(cmd *cobra.Command, _ []string) error {
		run, err := report.LoadJSON(analyzeFlags.input)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()

		table := report.BuildTable(run)
		table.TermTable(out)

		fmt.Fprintln(out, "\nbootstrap 95% CI of accuracy drop vs baseline:")
		rng := rand.New(rand.NewSource(analyzeFlags.seed))
		for _, lvl := range table.Levels {
			if lvl == domain.LevelNone {
				continue
			}
			base, comp := pairedVectors(run.Results, lvl)
			low, high := stats.BootstrapDropCI(base, comp, analyzeFlags.iterations, rng)
			fmt.Fprintf(out, "  %-6s drop CI95 [%.3f, %.3f]\n", lvl, low, high)
		}

		cliffs := cliffReports(table)
		if len(cliffs) > 0 {
			alpha, beta, r2 := stats.LogRegression(cliffPoints(cliffs))
			fmt.Fprintf(out, "\nlog-scale regression: cliffParamB = %.3f + %.3f * ln(paramsB)  (R^2=%.3f)\n",
				alpha, beta, r2)
		} else {
			fmt.Fprintln(out, "\nno failure cliffs observed; regression not applicable")
		}

		if analyzeFlags.outputTbl != "" {
			if err := report.SaveMarkdown(analyzeFlags.outputTbl, table.Markdown()); err != nil {
				return err
			}
			fmt.Fprintf(out, "\nmarkdown table: %s\n", analyzeFlags.outputTbl)
		}
		if analyzeFlags.htmlReport != "" {
			title := "Compression Safety Matrix — " + run.Model
			if err := report.SaveHTML(analyzeFlags.htmlReport, table.HTML(title)); err != nil {
				return err
			}
			fmt.Fprintf(out, "html report: %s\n", analyzeFlags.htmlReport)
		}
		return nil
	},
}

// pairedVectors extracts baseline and level accuracy vectors ordered by task.
func pairedVectors(results []domain.CellResult, lvl domain.CompressionLevel) (base, comp []float64) {
	idxBase := map[string]float64{}
	idxComp := map[string]float64{}
	for _, r := range results {
		v := 0.0
		if r.Correct() {
			v = 1
		}
		switch r.CompressionLevel {
		case domain.LevelNone:
			idxBase[r.TaskID] = v
		case lvl:
			idxComp[r.TaskID] = v
		}
	}
	for id, b := range idxBase {
		base = append(base, b)
		comp = append(comp, idxComp[id])
	}
	return base, comp
}

func cliffReports(table report.SafetyTable) []domain.FailureCliffReport {
	var out []domain.FailureCliffReport
	for _, row := range table.Rows {
		scores := map[domain.CompressionLevel]float64{}
		for l, cell := range row.Cells {
			scores[l] = cell.Acc
		}
		cliff, delta, found := stats.DetectCliff(table.Levels, scores)
		report1 := domain.FailureCliffReport{
			Model:            row.Model,
			BaselineAccuracy: row.Baseline,
			CliffLevel:       cliff,
			CliffAccuracy:    scores[cliff],
			AccuracyDelta:    -delta,
		}
		if found {
			out = append(out, report1)
		}
	}
	return out
}

// cliffPoints maps each model's parameter count to its observed cliff depth.
func cliffPoints(reports []domain.FailureCliffReport) [][2]float64 {
	params := map[string]float64{
		"little-lamb-0.3b":   0.3,
		"qwen-3.5-0.8b":      0.8,
		"phi-3.5-mini":       3.8,
		"zaya-1":             8,
		"gemma-4":            12,
		"frontier-reference": 175,
	}
	points := make([][2]float64, 0, len(reports))
	for _, r := range reports {
		b, ok := params[r.Model]
		if !ok || r.CliffLevel == "" {
			continue
		}
		points = append(points, [2]float64{b, levelOrdinal(r.CliffLevel)})
	}
	return points
}

func levelOrdinal(l domain.CompressionLevel) float64 {
	for i, v := range domain.AllLevels {
		if v == l {
			return float64(i)
		}
	}
	return 0
}

func init() {
	fs := analyzeCmd.Flags()
	fs.StringVarP(&analyzeFlags.input, "input", "i", "./results/results.json", "raw JSON trace input path")
	fs.IntVar(&analyzeFlags.iterations, "bootstrap-iterations", 1000, "paired bootstrap resamples per level")
	fs.StringVar(&analyzeFlags.outputTbl, "output-table", "", "markdown safety table output path")
	fs.StringVar(&analyzeFlags.htmlReport, "html-report", "", "standalone HTML report output path")
	fs.Int64Var(&analyzeFlags.seed, "seed", 7, "bootstrap RNG seed")
}
