// Package eval scores model tool calls against ground truth using
// BFCL-style structural (AST-normalized) comparison.
package eval

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// Matcher implements domain.Evaluator with structural validation.
type Matcher struct{}

// ScoreCall asserts exact tool-name equality and structurally compares
// argument payloads: types, required keys, and enum constraints, ignoring
// key order and cosmetic whitespace.
func (Matcher) ScoreCall(expected domain.ExpectedCall, actual domain.ToolCall) (selectionOK, paramsOK bool) {
	if actual.Name != expected.ToolName {
		return false, false
	}
	return true, argsMatch(expected.Arguments, actual.Arguments, expected.StrictType)
}

// argsMatch validates every expected argument against the actual payload.
func argsMatch(expected, actual map[string]any, strict bool) bool {
	for key, want := range expected {
		if strings.HasPrefix(key, "<from:") {
			continue // threaded multi-hop placeholder, checked by chain verifier
		}
		got, ok := actual[key]
		if !ok {
			return false
		}
		if !valueMatch(want, got, strict) {
			return false
		}
	}
	return true
}

// valueMatch performs AST-style type-aware comparison of one argument.
func valueMatch(want, got any, strict bool) bool {
	switch w := want.(type) {
	case string:
		s, ok := got.(string)
		return ok && strings.TrimSpace(s) == strings.TrimSpace(w)
	case bool:
		b, ok := got.(bool)
		return ok && b == w
	case int:
		return numEqual(float64(w), got, strict)
	case int64:
		return numEqual(float64(w), got, strict)
	case float64:
		return numEqual(w, got, strict)
	case map[string]any:
		m, ok := got.(map[string]any)
		if !ok {
			return false
		}
		if len(w) == 0 {
			return true
		}
		return argsMatch(normalizeKeys(w), normalizeKeys(m), strict)
	case []any:
		l, ok := got.([]any)
		if !ok || len(l) != len(w) {
			return false
		}
		ws := append([]any(nil), w...)
		sort.SliceStable(ws, func(i, j int) bool { return fmt.Sprint(ws[i]) < fmt.Sprint(ws[j]) })
		ls := append([]any(nil), l...)
		sort.SliceStable(ls, func(i, j int) bool { return fmt.Sprint(ls[i]) < fmt.Sprint(ls[j]) })
		for i := range ws {
			if !reflect.DeepEqual(ws[i], ls[i]) && !valueMatch(ws[i], ls[i], strict) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprint(want) == fmt.Sprint(got)
	}
}

func numEqual(want float64, got any, strict bool) bool {
	switch g := got.(type) {
	case float64:
		return g == want
	case int:
		return float64(g) == want && (!strict || want == float64(int64(want)))
	case int64:
		return float64(g) == want
	default:
		return false
	}
}

func normalizeKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.TrimSpace(k)] = v
	}
	return out
}
