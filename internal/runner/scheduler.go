// Package runner executes the full factorial evaluation matrix:
// compression levels x tasks, driving the model through the MCP proxy
// and scoring every produced call with the eval engine.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/eval"
	"can-i-compress-it/internal/inference"
	"can-i-compress-it/internal/mcp"
)

// ProxyFactory builds one proxy per matrix cell level.
type ProxyFactory func(ctx context.Context, level domain.CompressionLevel) (mcp.Proxy, error)

// Scheduler executes matrices for one model.
type Scheduler struct {
	model        inference.Model
	newProxy     ProxyFactory
	evaluator    domain.Evaluator
	log          *slog.Logger
	maxTurns     int
	onCellResult func(domain.CellResult)
}

// NewScheduler wires a scheduler for a model backend.
func NewScheduler(model inference.Model, newProxy ProxyFactory, log *slog.Logger) *Scheduler {
	return &Scheduler{
		model:     model,
		newProxy:  newProxy,
		evaluator: eval.Matcher{},
		log:       log,
		maxTurns:  4,
	}
}

// SetOnCellResult registers a listener called immediately when each cell completes.
func (s *Scheduler) SetOnCellResult(fn func(domain.CellResult)) {
	s.onCellResult = fn
}

// ExecuteMatrix implements domain.Runner: every task x every level.
func (s *Scheduler) ExecuteMatrix(ctx context.Context, suite []domain.Task, levels []domain.CompressionLevel) ([]domain.CellResult, error) {
	var out []domain.CellResult
	for _, level := range levels {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		results, err := s.runLevel(ctx, suite, level)
		if err != nil {
			return out, fmt.Errorf("level %s: %w", level, err)
		}
		out = append(out, results...)
		s.log.Info("level complete", "level", level, "tasks", len(results))
	}
	return out, nil
}

func (s *Scheduler) runLevel(ctx context.Context, suite []domain.Task, level domain.CompressionLevel) ([]domain.CellResult, error) {
	proxy, err := s.newProxy(ctx, level)
	if err != nil {
		return nil, err
	}
	defer func() { _ = proxy.Close() }()
	tools, err := proxy.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	var out []domain.CellResult
	for _, task := range suite {
		res, err := s.runTask(ctx, proxy, tools, level, task)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", task.ID, err)
		}
		if s.onCellResult != nil {
			s.onCellResult(res)
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *Scheduler) runTask(ctx context.Context, proxy mcp.Proxy, tools []domain.ToolDefinition,
	level domain.CompressionLevel, task domain.Task,
) (domain.CellResult, error) {
	start := time.Now()
	var catalogue []string
	if level == domain.LevelMax {
		for _, t := range task.Tools {
			catalogue = append(catalogue, t.Name)
		}
	}
	msgs := []inference.Message{
		{Role: "system", Content: systemPrompt(level, tools, catalogue...)},
		{Role: "user", Content: task.Prompt},
	}
	var executed []domain.ToolCall
	var returns []map[string]any
	tokens := schemaTokens(tools)

	for turn := 0; turn < s.maxTurns; turn++ {
		resp, err := s.model.Chat(ctx, msgs, tools)
		if err != nil {
			if ctx.Err() != nil {
				return domain.CellResult{}, ctx.Err()
			}
			s.log.Warn("model chat error on task", "task", task.ID, "level", level, "err", err)
			break
		}
		tokens += resp.TokensUsed
		if len(resp.ToolCalls) == 0 {
			break
		}
		for _, call := range resp.ToolCalls {
			result, err := proxy.Call(ctx, call)
			if err != nil {
				s.log.Warn("tool call failed", "tool", call.Name, "err", err)
			} else if m, ok := result.(map[string]any); ok {
				returns = append(returns, m)
			}
			if inner, ok := unwrapMeta(call); ok {
				executed = append(executed, inner)
			}
			callID := call.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%s", call.Name)
				call.ID = callID
			}
			msgs = append(msgs, assistantToolCall(call), toolResultMessage(callID, result, err))
		}
		if len(executed) >= expectedCalls(task) {
			break
		}
	}

	res := domain.CellResult{
		Model:            s.model.Name(),
		CompressionLevel: level,
		TaskID:           task.ID,
		Category:         task.Category,
		TokensConsumed:   tokens,
		LatencyMs:        time.Since(start).Milliseconds(),
	}
	s.scoreTask(&res, task, executed, returns)
	return res, nil
}

func (s *Scheduler) scoreTask(res *domain.CellResult, task domain.Task, executed []domain.ToolCall, returns []map[string]any) {
	if len(executed) == 0 {
		return
	}
	first := executed[0]
	res.ToolSelected = first.Name
	res.SelectionCorrect, res.ParamFillCorrect = s.evaluator.ScoreCall(task.Expected, first)
	if len(task.MultiHop) > 0 {
		chain := eval.VerifyChain(task.MultiHop, executed, returns)
		res.MultiHopComplete = chain.Complete
	} else {
		res.MultiHopComplete = res.SelectionCorrect && res.ParamFillCorrect
	}
}

func systemPrompt(level domain.CompressionLevel, tools []domain.ToolDefinition, catalogue ...string) string {
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		toolsJSON = []byte("[]")
	}
	var sb strings.Builder
	sb.WriteString("You are a tool-using assistant. You have access to the following tools:\n")
	sb.WriteString(string(toolsJSON))
	sb.WriteString("\n\nTo invoke a tool, respond with a tool call (either native tool_call or JSON object: {\"name\": \"<tool_name>\", \"arguments\": {<args>}}).")

	if level == domain.LevelMax {
		if len(catalogue) > 0 {
			sb.WriteString(fmt.Sprintf("\nAvailable tools in catalogue: [%s]. First call get_tool_schema(tool_name=\"...\"), then call invoke_tool(tool_name=\"...\", arguments={...}).", strings.Join(catalogue, ", ")))
		} else {
			sb.WriteString("\nFirst call get_tool_schema(tool_name=\"...\"), then call invoke_tool(tool_name=\"...\", arguments={...}).")
		}
	}
	return sb.String()
}

func schemaTokens(tools []domain.ToolDefinition) int {
	data, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return len(data) / 4
}

// unwrapMeta converts meta-tool traffic into the underlying invocation.
// Schema lookups are protocol overhead, not scored calls.
func unwrapMeta(call domain.ToolCall) (domain.ToolCall, bool) {
	if isMetaGetSchema(call.Name) {
		return domain.ToolCall{}, false
	}
	if isMetaInvoke(call.Name) {
		name, _ := call.Arguments["tool_name"].(string)
		args, ok := call.Arguments["arguments"].(map[string]any)
		if !ok {
			args, _ = call.Arguments["tool_input"].(map[string]any)
		}
		if args == nil {
			args = map[string]any{}
		}
		return domain.ToolCall{Name: name, Arguments: args}, true
	}
	return call, true
}

func isMetaGetSchema(name string) bool {
	return name == mcp.MetaGetSchema || strings.HasSuffix(name, "_"+mcp.MetaGetSchema) || strings.Contains(name, "get_tool_schema")
}

func isMetaInvoke(name string) bool {
	return name == mcp.MetaInvoke || strings.HasSuffix(name, "_"+mcp.MetaInvoke) || strings.Contains(name, "invoke_tool")
}
