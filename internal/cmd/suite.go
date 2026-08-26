package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/suite"
)

var suiteGenFlags struct {
	numTasks    int
	ambiguity   string
	promptStyle string
	distractors int
	seed        int64
	out         string
}

var suiteCmd = &cobra.Command{
	Use:   "suite",
	Short: "Task suite operations",
}

var suiteGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Synthesize a task suite with parameterized ambiguity",
	RunE: func(cmd *cobra.Command, _ []string) error {
		switch suiteGenFlags.ambiguity {
		case "none", "low", "high":
		default:
			return fmt.Errorf("ambiguity must be none, low, or high")
		}
		switch suiteGenFlags.promptStyle {
		case "", suite.StyleExplicit, suite.StyleIntent:
		default:
			return fmt.Errorf("prompt-style must be explicit or intent")
		}
		if suiteGenFlags.distractors != 0 && (suiteGenFlags.distractors < 10 || suiteGenFlags.distractors > 90) {
			return fmt.Errorf("distractors must be between 10 and 90, or 0 to disable")
		}
		gen := suite.NewGenerator(suite.Options{
			NumTasks:    suiteGenFlags.numTasks,
			Ambiguity:   suiteGenFlags.ambiguity,
			PromptStyle: suiteGenFlags.promptStyle,
			Distractors: suiteGenFlags.distractors,
			Seed:        suiteGenFlags.seed,
		})
		tasks := gen.Generate()
		if suiteGenFlags.out == "" {
			suiteGenFlags.out = fmt.Sprintf("./suites/synthetic_%d.json", time.Now().Unix())
		}
		if err := suite.Save(suiteGenFlags.out, tasks); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote %d tasks to %s\n", len(tasks), suiteGenFlags.out)
		return nil
	},
}

var suiteImportFlags struct {
	template string
	out      string
}

var suiteImportCmd = &cobra.Command{
	Use:   "import -- <server command...>",
	Short: "Import a suite from a live MCP server's tool schemas",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if suiteImportFlags.template != "" {
			if _, ok := suite.Template(suiteImportFlags.template); !ok {
				return fmt.Errorf("unknown template %q (want one of %s)",
					suiteImportFlags.template, strings.Join(suite.TemplateNames(), ", "))
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client, err := mcp.NewStdioClient(ctx, args)
		if err != nil {
			return err
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close()
			return err
		}
		if err := client.Close(); err != nil {
			_ = err // Close kills the server; exit status after kill is expected
		}

		tasks := importedStubs(tools)
		if suiteImportFlags.template != "" {
			annotated, _ := suite.Template(suiteImportFlags.template)
			tasks = mergeTemplate(tasks, annotated, tools)
		}
		out := suiteImportFlags.out
		if out == "" {
			out = fmt.Sprintf("./suites/imported_%s.json", sanitizeName(args[0]))
		}
		if err := suite.Save(out, tasks); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "imported %d tools as %d tasks to %s\n",
			len(tools), len(tasks), out)
		return nil
	},
}

// importedStubs converts live server schemas into annotation-ready task
// stubs: one task per tool with an empty prompt to fill in.
func importedStubs(tools []domain.ToolDefinition) []domain.Task {
	tasks := make([]domain.Task, 0, len(tools))
	for _, t := range tools {
		tasks = append(tasks, domain.Task{
			ID:       fmt.Sprintf("imp_%s", sanitizeName(t.Name)),
			Category: domain.CategoryImported,
			Prompt:   "",
			Tools:    []domain.ToolDefinition{t},
			Expected: domain.ExpectedCall{ToolName: t.Name},
		})
	}
	return tasks
}

// mergeTemplate replaces stubs with matching pre-annotated template
// tasks and reports template tools the live server does not expose.
func mergeTemplate(stubs []domain.Task, annotated []domain.Task, tools []domain.ToolDefinition) []domain.Task {
	exposed := map[string]bool{}
	for _, t := range tools {
		exposed[t.Name] = true
	}
	byTool := map[string]domain.Task{}
	for _, stub := range stubs {
		byTool[stub.Expected.ToolName] = stub
	}
	var missing []string
	for _, task := range annotated {
		if !exposed[task.Expected.ToolName] {
			missing = append(missing, task.ID)
			continue
		}
		byTool[task.Expected.ToolName] = task
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		fmt.Fprintf(os.Stderr, "note: skipped %d template tasks for absent tools: %s\n",
			len(missing), strings.Join(missing, ", "))
	}
	out := make([]domain.Task, 0, len(byTool))
	names := make([]string, 0, len(byTool))
	for name := range byTool {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, byTool[name])
	}
	return out
}

// sanitizeName reduces a command or tool name to a filename-safe token.
func sanitizeName(name string) string {
	lower := strings.ToLower(name)
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func init() {
	suiteCmd.AddCommand(suiteGenerateCmd)
	fs := suiteGenerateCmd.Flags()
	fs.IntVarP(&suiteGenFlags.numTasks, "num-tasks", "n", 90, "number of tasks to generate")
	fs.StringVar(&suiteGenFlags.ambiguity, "ambiguity", "high", "ambiguity profile: none|low|high")
	fs.StringVar(&suiteGenFlags.promptStyle, "prompt-style", suite.StyleExplicit, "prompt phrasing: explicit|intent")
	fs.IntVar(&suiteGenFlags.distractors, "distractors", 0, "filler tools padded into every catalogue (10-90, 0 disables)")
	fs.Int64Var(&suiteGenFlags.seed, "seed", int64(time.Now().UnixNano()), "RNG seed for reproducibility")
	fs.StringVarP(&suiteGenFlags.out, "out", "o", "", "output suite path")

	suiteCmd.AddCommand(suiteImportCmd)
	ifs := suiteImportCmd.Flags()
	ifs.StringVar(&suiteImportFlags.template, "template", "",
		"merge pre-annotated tasks for a standard server: github|sqlite|filesystem")
	ifs.StringVarP(&suiteImportFlags.out, "out", "o", "", "output suite path")
}
