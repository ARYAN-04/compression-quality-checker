package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/mcp"
)

// Mock is a deterministic offline model used to exercise the harness
// without a live endpoint. It emulates the documented small-model
// behaviour: selection relies on name/description matching, so heavier
// compression degrades it, and max compression forces the two-step
// meta-tool flow.
type Mock struct {
	model string
}

// NewMock returns a deterministic mock model.
func NewMock(model string) *Mock { return &Mock{model: model} }

func (m *Mock) Name() string { return m.model }

// argRe captures "key: value" pairs where a value is either a threaded
// placeholder (<from:tool.field>) or a bare token ending at punctuation.
var argRe = regexp.MustCompile(`\b([a-z_]+):\s*(<from:[^>]*>|[^,;\n.]*)`)

// Chat implements Model with deterministic behaviour.
func (m *Mock) Chat(_ context.Context, msgs []Message, tools []domain.ToolDefinition) (Response, error) {
	prompt := lastUserPrompt(msgs)
	if isMetaCatalogue(tools) {
		return m.metaTurn(msgs, prompt)
	}
	best, bestScore := "", -1.0
	for _, t := range tools {
		if score := m.matchScore(prompt, t); score > bestScore {
			best, bestScore = t.Name, score
		}
	}
	if len(tools) > 0 && bestScore <= 0 {
		best = tools[0].Name // forced wrong pick when nothing matches
	}
	if named := nthMentionedTool(prompt, countAssistantCalls(msgs)); named != "" {
		for _, t := range tools {
			if t.Name == named {
				best = named // chains are followed in prompt order
				break
			}
		}
	}
	return Response{
		ToolCalls:  []domain.ToolCall{{Name: best, Arguments: m.resolveThreaded(m.extractArgs(prompt), msgs)}},
		TokensUsed: 40 + len(tools)*12,
	}, nil
}

// metaTurn performs the two-step get_tool_schema then invoke_tool flow,
// degrading exactly like small models do: some fraction of tasks skip
// the schema fetch and lose the ability to thread intermediate values.
func (m *Mock) metaTurn(msgs []Message, prompt string) (Response, error) {
	target := firstSnakeToken(prompt)
	if !assistantCalled(msgs, mcp.MetaGetSchema) {
		if m.skipSchemaFetch(prompt) {
			args := m.extractArgs(prompt)
			return Response{
				ToolCalls: []domain.ToolCall{{Name: mcp.MetaInvoke, Arguments: map[string]any{
					"tool_name": target, "arguments": args,
				}}},
				TokensUsed: 45,
			}, nil
		}
		return Response{
			ToolCalls:  []domain.ToolCall{{Name: mcp.MetaGetSchema, Arguments: map[string]any{"tool_name": target}}},
			TokensUsed: 30,
		}, nil
	}
	args := m.resolveThreaded(m.extractArgs(prompt), msgs)
	return Response{
		ToolCalls: []domain.ToolCall{{Name: mcp.MetaInvoke, Arguments: map[string]any{
			"tool_name": target, "arguments": args,
		}}},
		TokensUsed: 60,
	}, nil
}

// skipSchemaFetch deterministically degrades some fraction of tasks per
// model, emulating the two-step lookup failure mode of weak instruction
// followers.
func (m *Mock) skipSchemaFetch(prompt string) bool {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s|%s", m.model, prompt)
	return h.Sum32()%10 < 4
}

func isMetaCatalogue(tools []domain.ToolDefinition) bool {
	for _, t := range tools {
		if t.Name == mcp.MetaGetSchema {
			return true
		}
	}
	return false
}

// matchScore scores prompt overlap with a tool's visible representation.
func (m *Mock) matchScore(prompt string, t domain.ToolDefinition) float64 {
	p := strings.ToLower(prompt)
	score := 0.0
	if strings.Contains(p, strings.ToLower(t.Name)) {
		score += 2.0
	}
	for _, word := range strings.Fields(strings.ToLower(t.Description)) {
		if len(word) > 3 && strings.Contains(p, word) {
			score += 0.1
		}
	}
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s/%s", m.model, t.Name)
	score -= float64(h.Sum32()%100) / 1000.0
	return score
}

// extractArgs parses "key: value" pairs from the prompt text.
func (m *Mock) extractArgs(prompt string) map[string]any {
	out := map[string]any{}
	for _, match := range argRe.FindAllStringSubmatch(prompt, -1) {
		key := strings.ToLower(match[1])
		raw := strings.TrimSpace(match[2])
		if raw == "" {
			continue
		}
		var v any = raw
		if n, err := parseInt64(raw); err == nil {
			v = n
		}
		out[key] = v
	}
	return out
}

// resolveThreaded replaces "<from:tool.field>" placeholders using prior
// tool result messages in the conversation.
func (m *Mock) resolveThreaded(args map[string]any, msgs []Message) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, "<from:") {
			out[k] = v
			continue
		}
		path := strings.TrimSuffix(strings.TrimPrefix(s, "<from:"), ">")
		tool, field, ok := strings.Cut(path, ".")
		if !ok {
			out[k] = v
			continue
		}
		out[k] = lookupInResults(msgs, tool, field)
	}
	return out
}

func lookupInResults(msgs []Message, tool, field string) any {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(msgs[i].Content), &payload); err != nil {
			continue
		}
		if payload["tool"] == tool {
			if v, ok := payload[field]; ok {
				return v
			}
		}
	}
	return nil
}

func assistantCalled(msgs []Message, tool string) bool {
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == tool {
				return true
			}
		}
	}
	return false
}

func countAssistantCalls(msgs []Message) int {
	n := 0
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			n += len(msg.ToolCalls)
		}
	}
	return n
}

// nthMentionedTool returns the nth snake_case tool name mentioned in the
// prompt (0-based), so chained calls follow prompt order.
func nthMentionedTool(prompt string, n int) string {
	toks := snakeRe.FindAllString(prompt, -1)
	if n < len(toks) {
		return toks[n]
	}
	return ""
}

var snakeRe = regexp.MustCompile(`[a-z]+(?:_[a-z]+)+`)

func firstSnakeToken(s string) string {
	if tok := snakeRe.FindString(s); tok != "" {
		return tok
	}
	return ""
}

func lastUserPrompt(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
