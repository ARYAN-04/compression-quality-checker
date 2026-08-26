// External test package: the end-to-end matrix below wires the proxy
// into the runner, which itself depends on mcp.
package mcp_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"reflect"
	"testing"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
	"can-i-compress-it/internal/runner"
	"can-i-compress-it/internal/suite"
)

func threadTools() []domain.ToolDefinition {
	return []domain.ToolDefinition{
		{
			Name:        "first_step",
			Description: "Fetches a record by name.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Record name."},
			}, "required": []any{"name"}},
		},
		{
			Name:        "second_step",
			Description: "Consumes an item id.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"item_id": map[string]any{"type": "string", "description": "Item to consume."},
			}, "required": []any{"item_id"}},
		},
	}
}

// synthesizedValue mirrors the builtin proxy's deterministic field value
// after exactly one recorded underlying call.
func synthesizedValue(tool, field string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(tool + "." + field))
	return fmt.Sprintf("%s-%d-%08x", field, 1, h.Sum32())
}

func TestSynthesizeResultEmitsThreadFieldsDeterministically(t *testing.T) {
	ctx := context.Background()
	fields := map[string][]string{"first_step": {"my_field"}}
	call := domain.ToolCall{Name: "first_step", Arguments: map[string]any{"name": "alpha"}}

	var results []map[string]any
	for i := 0; i < 2; i++ {
		p, err := mcp.NewProxy(ctx, mcp.Config{Level: domain.LevelNone, ThreadFields: fields}, threadTools())
		if err != nil {
			t.Fatalf("proxy %d: %v", i, err)
		}
		res, err := p.Call(ctx, call)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
		results = append(results, res.(map[string]any))
	}
	want := map[string]any{
		"ok":        true,
		"tool":      "first_step",
		"result_id": results[0]["result_id"],
		"my_field":  synthesizedValue("first_step", "my_field"),
	}
	if !reflect.DeepEqual(results[0], want) || !reflect.DeepEqual(results[1], want) {
		t.Fatalf("nondeterministic or missing fields:\n got %v\nwant %v", results[0], want)
	}
}

func TestThreadFieldsAtMaxLevelMetaPath(t *testing.T) {
	ctx := context.Background()
	p, err := mcp.NewProxy(ctx, mcp.Config{
		Level:        domain.LevelMax,
		ThreadFields: map[string][]string{"first_step": {"my_field"}},
	}, threadTools())
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	defer func() { _ = p.Close() }()
	res, err := p.Call(ctx, domain.ToolCall{
		Name:      mcp.MetaInvoke,
		Arguments: map[string]any{"tool_name": "first_step", "arguments": map[string]any{"name": "alpha"}},
	})
	if err != nil {
		t.Fatalf("invoke_tool: %v", err)
	}
	payload := res.(map[string]any)
	if got, want := payload["my_field"], synthesizedValue("first_step", "my_field"); got != want {
		t.Fatalf("meta path my_field = %v, want %q", got, want)
	}
}

func TestSchedulerThreadsFieldsEndToEnd(t *testing.T) {
	task := domain.Task{
		ID:       "chain_two",
		Category: domain.CategoryMultiHop,
		Prompt:   "Step 1: call first_step. Arguments: name: alpha. Step 2: call second_step. Arguments: item_id: <from:first_step.my_field>.",
		Tools:    threadTools(),
		Expected: domain.ExpectedCall{
			ToolName:  "first_step",
			Arguments: map[string]any{"name": "alpha"},
		},
		MultiHop: []domain.ExpectedCall{
			{ToolName: "first_step", Arguments: map[string]any{"name": "alpha"}},
			{ToolName: "second_step", Arguments: map[string]any{"item_id": "<from:first_step.my_field>"}},
		},
	}
	tasks := []domain.Task{task}
	sched := runner.NewScheduler(inference.NewMock("mock-chain"),
		func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error) {
			return mcp.NewProxy(ctx, mcp.Config{
				Level:        level,
				TargetServer: "suite",
				ThreadFields: suite.ThreadedFields(tasks),
			}, suite.UniqueTools(tasks))
		}, slog.Default())

	cells, err := sched.ExecuteMatrix(context.Background(), tasks,
		[]domain.CompressionLevel{domain.LevelNone})
	if err != nil {
		t.Fatalf("matrix failed: %v", err)
	}
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(cells))
	}
	cell := cells[0]
	if !cell.SelectionCorrect || !cell.ParamFillCorrect || !cell.MultiHopComplete {
		t.Fatalf("expected threaded chain pass, got %+v", cell)
	}
}
