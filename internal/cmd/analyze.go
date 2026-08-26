package cmd

import (
	"fmt"
	"math/rand"
	"strings"

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
	Short: "Recalculate bootstrap intervals and failure cliffs from one or more traces",
	RunE: func(cmd *cobra.Command, _ []string) error {
		runs, err := report.LoadRuns(analyzeFlags.input)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()

		table := report.BuildTable(runs)
		table.TermTable(out)

		results := report.MergeRuns(runs).Results
		fmt.Fprintln(out, "\nbootstrap 95% CI of accuracy drop vs baseline:")
		rng := rand.New(rand.NewSource(analyzeFlags.seed))
		for _, lvl := range table.Levels {
			if lvl == domain.LevelNone {
				continue
			}
			base, comp := pairedVectors(results, lvl)
			low, high := stats.BootstrapDropCI(base, comp, analyzeFlags.iterations, rng)
			fmt.Fprintf(out, "  %-6s drop CI95 [%.3f, %.3f]\n", lvl, low, high)
		}

		cliffs := cliffReports(table)
		if len(cliffs) > 0 {
			points, skipped := cliffPoints(cliffs)
			if len(skipped) > 0 {
				fmt.Fprintf(out, "\nregression skips %s (unknown parameter count; re-run with --params-b)",
					strings.Join(skipped, ", "))
			}
			if len(points) >= 2 {
				alpha, beta, r2 := stats.LogRegression(points)
				fmt.Fprintf(out, "\nlog-scale regression: cliffParamB = %.3f + %.3f * ln(paramsB)  (R^2=%.3f)\n",
					alpha, beta, r2)
			} else {
				fmt.Fprint(out, "\nnot enough models with known parameter counts for regression\n")
			}
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
			title := "Compression Safety Matrix — " + report.MergeRuns(runs).Model
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
			ParameterCountB:  row.B,
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

// cliffPoints maps each model's parameter count to its observed cliff
// depth, using counts recorded in the traces. Models without a recorded
// count are returned separately so callers can surface the gap.
func cliffPoints(reports []domain.FailureCliffReport) (points [][2]float64, skipped []string) {
	for _, r := range reports {
		if r.ParameterCountB <= 0 || r.CliffLevel == "" {
			skipped = append(skipped, r.Model)
			continue
		}
		points = append(points, [2]float64{r.ParameterCountB, levelOrdinal(r.CliffLevel)})
	}
	return points, skipped
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
	fs.StringVarP(&analyzeFlags.input, "input", "i", "./results/results.json",
		"raw JSON trace input path or glob (e.g. \"results/*.json\")")
	fs.IntVar(&analyzeFlags.iterations, "bootstrap-iterations", 1000, "paired bootstrap resamples per level")
	fs.StringVar(&analyzeFlags.outputTbl, "output-table", "", "markdown safety table output path")
	fs.StringVar(&analyzeFlags.htmlReport, "html-report", "", "standalone HTML report output path")
	fs.Int64Var(&analyzeFlags.seed, "seed", 7, "bootstrap RNG seed")
}
