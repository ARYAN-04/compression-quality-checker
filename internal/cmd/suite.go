package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

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

func init() {
	suiteCmd.AddCommand(suiteGenerateCmd)
	fs := suiteGenerateCmd.Flags()
	fs.IntVarP(&suiteGenFlags.numTasks, "num-tasks", "n", 90, "number of tasks to generate")
	fs.StringVar(&suiteGenFlags.ambiguity, "ambiguity", "high", "ambiguity profile: none|low|high")
	fs.StringVar(&suiteGenFlags.promptStyle, "prompt-style", suite.StyleExplicit, "prompt phrasing: explicit|intent")
	fs.IntVar(&suiteGenFlags.distractors, "distractors", 0, "filler tools padded into every catalogue (10-90, 0 disables)")
	fs.Int64Var(&suiteGenFlags.seed, "seed", int64(time.Now().UnixNano()), "RNG seed for reproducibility")
	fs.StringVarP(&suiteGenFlags.out, "out", "o", "", "output suite path")
}
