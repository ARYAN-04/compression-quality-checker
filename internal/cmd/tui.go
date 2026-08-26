package cmd

import (
	"github.com/spf13/cobra"

	"can-i-compress-it/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive Bubble Tea guided wizard and safety explorer",
	RunE: func(_ *cobra.Command, _ []string) error {
		return tui.Run()
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
