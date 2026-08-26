// Package cmd wires the can-i-compress-it command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/tui"
)

var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:          "can-i-compress-it",
	Short:        "Benchmark how MCP schema compression impacts tool-use accuracy of local models",
	Version:      version,
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		return tui.Run()
	},
}

func init() {
	rootCmd.AddCommand(runCmd, testCmd, suiteCmd, analyzeCmd, reportCmd)
}

// Execute runs the CLI and exits non-zero on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
