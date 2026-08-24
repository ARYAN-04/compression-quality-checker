package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
)

var testFlags struct {
	model      string
	endpoint   string
	apiKey     string
	prompt     string
	level      string
	compressor string
	mock       bool
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run an isolated single-tool test against an MCP surface",
	RunE: func(cmd *cobra.Command, _ []string) error {
		lvl, err := domain.ParseLevel(testFlags.level)
		if err != nil {
			return err
		}
		var model inference.Model
		if testFlags.mock {
			model = inference.NewMock(testFlags.model + "-mock")
		} else {
			model = inference.NewClient(testFlags.model, testFlags.endpoint, testFlags.apiKey)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		proxy, err := mcp.NewProxy(ctx, mcp.Config{Level: lvl, Binary: testFlags.compressor}, nil)
		if err != nil {
			return err
		}
		defer func() { _ = proxy.Close() }()

		tools, err := proxy.Tools(ctx)
		if err != nil {
			return err
		}
		resp, err := model.Chat(ctx, []inference.Message{
			{Role: "user", Content: testFlags.prompt},
		}, tools)
		if err != nil {
			return fmt.Errorf("model chat: %w", err)
		}
		out := cmd.OutOrStdout()
		if len(resp.ToolCalls) == 0 {
			fmt.Fprintf(out, "no tool call produced; content: %.200s\n", resp.Content)
			return nil
		}
		call := resp.ToolCalls[0]
		result, err := proxy.Call(ctx, call)
		fmt.Fprintf(out, "tool:    %s\nargs:    %v\nlatency: %dms\n", call.Name, call.Arguments, 0)
		if err != nil {
			return fmt.Errorf("proxy call: %w", err)
		}
		fmt.Fprintf(out, "result:  %v\n", result)
		return nil
	},
}

func init() {
	fs := testCmd.Flags()
	fs.StringVarP(&testFlags.model, "model", "m", "", "model name (required)")
	fs.StringVar(&testFlags.endpoint, "endpoint", "http://localhost:11434/v1", "endpoint base URL")
	fs.StringVar(&testFlags.apiKey, "api-key", "", "bearer token")
	fs.StringVarP(&testFlags.prompt, "prompt", "p", "", "task prompt (required)")
	fs.StringVar(&testFlags.level, "level", "none", "compression level: none|low|medium|high|max")
	fs.StringVar(&testFlags.compressor, "compressor", "", "path to mcp-compressor binary (omit for builtin)")
	fs.BoolVar(&testFlags.mock, "mock", false, "use deterministic mock model")
	_ = testCmd.MarkFlagRequired("model")
	_ = testCmd.MarkFlagRequired("prompt")
}
