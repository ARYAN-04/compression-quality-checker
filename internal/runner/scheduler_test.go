package runner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/suite"
)

func TestExecuteMatrixEndToEndMock(t *testing.T) {
	tasks := suite.NewGenerator(suite.Options{NumTasks: 12, Ambiguity: "high", Seed: 42}).Generate()
	sched := NewScheduler(inference.NewMock("mock-0.5b"),
		func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{Level: level}, suite.UniqueTools(tasks))
		}, slog.Default())

	cells, err := sched.ExecuteMatrix(context.Background(), tasks, domain.AllLevels)
	if err != nil {
		t.Fatalf("matrix failed: %v", err)
	}
	if len(cells) != len(tasks)*len(domain.AllLevels) {
		t.Fatalf("expected %d cells, got %d", len(tasks)*len(domain.AllLevels), len(cells))
	}
	for _, c := range cells {
		if c.ToolSelected == "" || c.TokensConsumed == 0 {
			t.Fatalf("cell %+v missing fields", c)
		}
	}
}

func TestMaxLevelMetaUnwrapping(t *testing.T) {
	tasks := suite.Builtin()[:3]
	sched := NewScheduler(inference.NewMock("mock"),
		func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{Level: level}, suite.UniqueTools(tasks))
		}, slog.Default())
	cells, err := sched.ExecuteMatrix(context.Background(), tasks, []domain.CompressionLevel{domain.LevelMax})
	if err != nil {
		t.Fatalf("max-level run failed: %v", err)
	}
	for _, c := range cells {
		if c.ToolSelected == mcp.MetaInvoke && c.SelectionCorrect {
			t.Fatalf("meta wrapper name leaked into scoring: %+v", c)
		}
	}
}

func TestSchedulerOnCellResult(t *testing.T) {
	tasks := suite.Builtin()[:2]
	sched := NewScheduler(inference.NewMock("mock"),
		func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{Level: level}, suite.UniqueTools(tasks))
		}, slog.Default())
	var count int
	sched.SetOnCellResult(func(c domain.CellResult) {
		count++
	})
	_, err := sched.ExecuteMatrix(context.Background(), tasks, []domain.CompressionLevel{domain.LevelNone})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 onCell callbacks, got %d", count)
	}
}

func TestSystemPromptFormatting(t *testing.T) {
	tools := []domain.ToolDefinition{
		{Name: "get_weather", Description: "Get current weather"},
	}

	// LevelNone
	promptNone := systemPrompt(domain.LevelNone, tools)
	if !strings.Contains(promptNone, "You are a tool-using assistant") {
		t.Errorf("expected tool-using assistant text, got %q", promptNone)
	}
	if !strings.Contains(promptNone, "get_weather") {
		t.Errorf("expected get_weather in prompt, got %q", promptNone)
	}
	if !strings.Contains(promptNone, "To invoke a tool, respond with a tool call") {
		t.Errorf("expected tool invocation instructions, got %q", promptNone)
	}

	// LevelMax with catalogue
	metaTools := []domain.ToolDefinition{
		{Name: mcp.MetaGetSchema},
		{Name: mcp.MetaInvoke},
	}
	promptMax := systemPrompt(domain.LevelMax, metaTools, "get_weather", "lookup_user")
	if !strings.Contains(promptMax, "Available tools in catalogue: [get_weather, lookup_user]") {
		t.Errorf("expected catalogue tool names in LevelMax prompt, got %q", promptMax)
	}
	if !strings.Contains(promptMax, "get_tool_schema") || !strings.Contains(promptMax, "invoke_tool") {
		t.Errorf("expected meta tools in LevelMax prompt, got %q", promptMax)
	}
}

type flakyModel struct {
	failOn string
}

func (f flakyModel) Name() string { return "flaky" }

func (f flakyModel) Chat(ctx context.Context, msgs []inference.Message, tools []domain.ToolDefinition) (inference.Response, error) {
	for _, m := range msgs {
		if strings.Contains(m.Content, f.failOn) {
			return inference.Response{}, fmt.Errorf("channel error 400")
		}
	}
	return inference.Response{
		ToolCalls: []domain.ToolCall{{Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}}},
	}, nil
}

func TestSchedulerToleratesModelChatError(t *testing.T) {
	tasks := []domain.Task{
		{ID: "task_fail", Prompt: "Trigger failure", Expected: domain.ExpectedCall{ToolName: "get_weather"}},
		{ID: "task_pass", Prompt: "Run normally", Expected: domain.ExpectedCall{ToolName: "get_weather"}},
	}
	sched := NewScheduler(flakyModel{failOn: "Trigger failure"},
		func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{Level: level}, []domain.ToolDefinition{{Name: "get_weather"}})
		}, slog.Default())

	cells, err := sched.ExecuteMatrix(context.Background(), tasks, []domain.CompressionLevel{domain.LevelNone})
	if err != nil {
		t.Fatalf("expected matrix to complete despite chat error, got: %v", err)
	}
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].SelectionCorrect {
		t.Errorf("expected cell 0 to fail due to chat error")
	}
	if !cells[1].SelectionCorrect {
		t.Errorf("expected cell 1 to pass")
	}
}
