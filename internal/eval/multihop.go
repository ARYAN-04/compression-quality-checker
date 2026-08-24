package eval

import (
	"strings"

	"can-i-compress-it/internal/domain"
)

// MultiHopResult captures per-step verification of a chained task.
type MultiHopResult struct {
	Complete    bool
	CorrectStep int // number of leading steps verified correct
}

// VerifyChain checks that executed calls realize every expected hop in
// dependency order, resolving "<from:tool.field>" placeholders from the
// return value of the referenced prior step.
func VerifyChain(expected []domain.ExpectedCall, executed []domain.ToolCall, returns []map[string]any) MultiHopResult {
	if len(executed) < len(expected) {
		return MultiHopResult{Complete: false, CorrectStep: len(executed)}
	}
	for i, exp := range expected {
		got := executed[i]
		if got.Name != exp.ToolName {
			return MultiHopResult{CorrectStep: i}
		}
		resolved := resolveThreaded(exp.Arguments, returns[:i])
		if !argsMatch(resolved, got.Arguments, exp.StrictType) {
			return MultiHopResult{CorrectStep: i}
		}
	}
	return MultiHopResult{Complete: true, CorrectStep: len(expected)}
}

// resolveThreaded replaces "<from:tool.field>" values with values observed
// in earlier step returns (returns[i] is the payload returned by step i).
func resolveThreaded(args map[string]any, returns []map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		ref, ok := v.(string)
		if !ok || !strings.HasPrefix(ref, "<from:") {
			out[k] = v
			continue
		}
		path := strings.TrimSuffix(strings.TrimPrefix(ref, "<from:"), ">")
		tool, field, ok := strings.Cut(path, ".")
		if !ok {
			out[k] = v
			continue
		}
		out[k] = lookupField(returns, tool, field)
	}
	return out
}

func lookupField(returns []map[string]any, tool, field string) any {
	for _, r := range returns {
		if r == nil {
			continue
		}
		if t, ok := r["tool"].(string); ok && t == tool {
			if v, ok := r[field]; ok {
				return v
			}
		}
	}
	return nil
}
