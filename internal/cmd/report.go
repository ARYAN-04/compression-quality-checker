package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/report"
)

var reportFlags struct {
	input string
	md    string
	html  string
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Format existing result files into markdown or HTML reports",
	RunE: func(cmd *cobra.Command, _ []string) error {
		run, err := report.LoadJSON(reportFlags.input)
		if err != nil {
			return err
		}
		table := report.BuildTable(run)
		out := cmd.OutOrStdout()
		table.TermTable(out)
		if reportFlags.md != "" {
			if err := report.SaveMarkdown(reportFlags.md, table.Markdown()); err != nil {
				return err
			}
			fmt.Fprintf(out, "markdown: %s\n", reportFlags.md)
		}
		if reportFlags.html != "" {
			title := "Compression Safety Matrix — " + run.Model
			if err := report.SaveHTML(reportFlags.html, table.HTML(title)); err != nil {
				return err
			}
			fmt.Fprintf(out, "html: %s\n", reportFlags.html)
		}
		return nil
	},
}

func init() {
	fs := reportCmd.Flags()
	fs.StringVarP(&reportFlags.input, "input", "i", "./results/results.json", "raw JSON trace input path")
	fs.StringVar(&reportFlags.md, "md-table", "", "markdown output path")
	fs.StringVar(&reportFlags.html, "html-report", "", "HTML output path")
}
