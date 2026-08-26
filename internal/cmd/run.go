package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/report"
	"can-i-compress-it/internal/runner"
	"can-i-compress-it/internal/suite"
)

const defaultSuitePath = "./suites/synthetic_v1.json"

var runFlags struct {
	model      string
	endpoint   string
	apiKey     string
	suitePath  string
	output     string
	htmlReport string
	mdReport   string
	levels     []string
	compressor string
	paramsB    float64
	mock       bool
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute a full factorial matrix against a model endpoint",
	RunE:  runMatrix,
}

func init() {
	fs := runCmd.Flags()
	fs.StringVar(&runFlags.model, "model", "", "subject model name (required)")
	fs.StringVar(&runFlags.endpoint, "endpoint", "http://localhost:11434/v1",
		"OpenAI-compatible endpoint base URL")
	fs.StringVar(&runFlags.apiKey, "api-key", "", "bearer token for remote endpoints")
	fs.StringVar(&runFlags.suitePath, "suite", defaultSuitePath, "task suite JSON path")
	fs.StringVar(&runFlags.output, "output", "./results/results.json", "raw JSON trace output path")
	fs.StringVar(&runFlags.htmlReport, "html-report", "", "standalone HTML report path")
	fs.StringVar(&runFlags.mdReport, "md-table", "", "markdown safety table path")
	fs.StringSliceVar(&runFlags.levels, "levels",
		[]string{"none", "low", "medium", "high", "max"}, "compression levels to sweep")
	fs.StringVar(&runFlags.compressor, "compressor", "",
		"path to mcp-compressor binary (omit for builtin compressor)")
	fs.Float64Var(&runFlags.paramsB, "params-b", 0,
		"subject model parameter count in billions (recorded in the trace for regression analysis)")
	fs.BoolVar(&runFlags.mock, "mock", false, "use deterministic mock model (offline verification)")
	_ = runCmd.MarkFlagRequired("model")
}

func parseLevels(spec []string) ([]domain.CompressionLevel, error) {
	out := make([]domain.CompressionLevel, 0, len(spec))
	for _, s := range spec {
		lvl, err := domain.ParseLevel(s)
		if err != nil {
			return nil, fmt.Errorf("parse levels: %w", err)
		}
		out = append(out, lvl)
	}
	return out, nil
}

// loadSuiteOrBuiltin falls back to the embedded suite when the default
// path is missing so the tool works out of the box.
func loadSuiteOrBuiltin(path string, warn io.Writer) ([]domain.Task, error) {
	tasks, err := suite.Load(path)
	if err == nil {
		return tasks, nil
	}
	if path == defaultSuitePath && warn != nil {
		fmt.Fprintln(warn, "note: no suite file found; using embedded synthetic_v1 suite")
		return suite.Builtin(), nil
	}
	return nil, err
}

func newModel() inference.Model {
	if runFlags.mock {
		return inference.NewMock(runFlags.model)
	}
	return inference.NewClient(runFlags.model, runFlags.endpoint, runFlags.apiKey)
}

func runMatrix(cmd *cobra.Command, _ []string) error {
	levels, err := parseLevels(runFlags.levels)
	if err != nil {
		return err
	}
	tasks, err := loadSuiteOrBuiltin(runFlags.suitePath, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	sched := runner.NewScheduler(newModel(), func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
		return mcp.NewProxy(ctx, mcp.Config{
			Level:        level,
			Binary:       runFlags.compressor,
			TargetServer: "suite",
			ThreadFields: suite.ThreadedFields(tasks),
		}, suite.UniqueTools(tasks))
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cells, err := sched.ExecuteMatrix(ctx, tasks, levels)
	if err != nil {
		return fmt.Errorf("matrix execution: %w", err)
	}

	run := domain.RunResult{
		RunID:           fmt.Sprintf("run_%d", time.Now().Unix()),
		Model:           runFlags.model,
		Endpoint:        runFlags.endpoint,
		SuitePath:       runFlags.suitePath,
		Levels:          levels,
		Tasks:           len(tasks),
		Results:         cells,
		ParameterCountB: runFlags.paramsB,
	}
	out := cmd.OutOrStdout()
	if err := report.SaveJSON(runFlags.output, run); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %d cell results to %s\n\n", len(cells), runFlags.output)

	table := report.BuildTable([]domain.RunResult{run})
	table.TermTable(out)
	if runFlags.htmlReport != "" {
		title := "Compression Safety Matrix — " + runFlags.model
		if err := report.SaveHTML(runFlags.htmlReport, table.HTML(title)); err != nil {
			return err
		}
		fmt.Fprintf(out, "\nhtml report: %s\n", runFlags.htmlReport)
	}
	if runFlags.mdReport != "" {
		if err := report.SaveMarkdown(runFlags.mdReport, table.Markdown()); err != nil {
			return err
		}
		fmt.Fprintf(out, "markdown table: %s\n", runFlags.mdReport)
	}
	return nil
}
