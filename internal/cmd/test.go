package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/runner"
	"can-i-compress-it/internal/suite"
)

var testFlags struct {
	model      string
	endpoint   string
	apiKey     string
	prompt     string
	level      string
	compressor string
	mock       bool
	suitePath  string
	schema     string
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

		var tools []domain.ToolDefinition
		schemaTarget := testFlags.schema
		if schemaTarget == "" && testFlags.suitePath == "" {
			schemaTarget = "github"
		}

		if schemaTarget != "" {
			if schemaTarget == "all" {
				for _, name := range suite.EmbeddedSchemaNames() {
					if st, ok := suite.EmbeddedSchemaTools(name); ok {
						tools = append(tools, st...)
					}
				}
			} else if st, ok := suite.EmbeddedSchemaTools(schemaTarget); ok {
				tools = st
			} else {
				tasks, sErr := suite.FromSchemaFile(schemaTarget)
				if sErr != nil {
					return fmt.Errorf("load schema %s: %w", schemaTarget, sErr)
				}
				tools = suite.UniqueTools(tasks)
			}
		} else if testFlags.suitePath != "" {
			tasks, sErr := loadSuiteOrBuiltin(testFlags.suitePath, cmd.ErrOrStderr())
			if sErr != nil {
				return fmt.Errorf("load suite %s: %w", testFlags.suitePath, sErr)
			}
			tools = suite.UniqueTools(tasks)
		}

		proxy, err := mcp.NewProxy(ctx, mcp.Config{Level: lvl, Binary: testFlags.compressor}, tools)
		if err != nil {
			return err
		}
		defer func() { _ = proxy.Close() }()

		compressedTools, err := proxy.Tools(ctx)
		if err != nil {
			return err
		}

		var catalogue []string
		if lvl == domain.LevelMax {
			for _, t := range tools {
				catalogue = append(catalogue, t.Name)
			}
		}
		msgs := []inference.Message{
			{Role: "system", Content: runner.SystemPrompt(lvl, compressedTools, catalogue...)},
			{Role: "user", Content: testFlags.prompt},
		}

		start := time.Now()
		resp, err := model.Chat(ctx, msgs, compressedTools)
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
		if err != nil {
			return fmt.Errorf("proxy call: %w", err)
		}

		// At LevelMax, resolve the two-step get_tool_schema -> invoke_tool flow
		if (call.Name == mcp.MetaGetSchema || strings.Contains(call.Name, "get_tool_schema")) && lvl == domain.LevelMax {
			callID := call.ID
			if callID == "" {
				callID = "call_get_schema"
				call.ID = callID
			}
			msgs = append(msgs,
				runner.AssistantToolCall(call),
				runner.ToolResultMessage(callID, result, nil),
			)
			resp2, err2 := model.Chat(ctx, msgs, compressedTools)
			if err2 == nil && len(resp2.ToolCalls) > 0 {
				call = resp2.ToolCalls[0]
				result, err = proxy.Call(ctx, call)
				if err != nil {
					return fmt.Errorf("proxy invoke call: %w", err)
				}
			}
		}

		latency := time.Since(start).Milliseconds()
		fmt.Fprintf(out, "tool:    %s\nargs:    %v\nlatency: %dms\n", call.Name, call.Arguments, latency)
		fmt.Fprintf(out, "result:  %v\n", result)
		return nil
	},
}

func init() {
	fs := testCmd.Flags()
	fs.StringVarP(&testFlags.model, "model", "m", "", "model name (required)")
	fs.StringVar(&testFlags.endpoint, "endpoint", "http://localhost:1234/v1",
		"OpenAI-compatible endpoint URL (e.g. http://localhost:1234/v1 or http://<ip>:1234)")
	fs.StringVar(&testFlags.apiKey, "api-key", "", "bearer token")
	fs.StringVarP(&testFlags.prompt, "prompt", "p", "", "task prompt (required)")
	fs.StringVar(&testFlags.level, "level", "none", "compression level: none|low|medium|high|max")
	fs.StringVar(&testFlags.compressor, "compressor", "", "path to mcp-compressor binary (omit for builtin)")
	fs.BoolVar(&testFlags.mock, "mock", false, "use deterministic mock model")
	fs.StringVar(&testFlags.suitePath, "suite", "", "task suite JSON path (optional)")
	fs.StringVar(&testFlags.schema, "schema", "", "MCP schema file path or bundled name (e.g. github, git, postgres)")
	fs.StringVar(&testFlags.schema, "from-schema", "", "alias for --schema")
	_ = testCmd.MarkFlagRequired("model")
	_ = testCmd.MarkFlagRequired("prompt")
}
