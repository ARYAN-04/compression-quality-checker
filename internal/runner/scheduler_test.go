package runner

import (
	"context"
	"log/slog"
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
