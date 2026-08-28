package runner

import (
	"context"
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
