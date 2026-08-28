package eval

import (
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestScoreCallSelection(t *testing.T) {
	m := Matcher{}
	exp := domain.ExpectedCall{ToolName: "query_user_by_id", Arguments: map[string]any{"id": int64(42)}}
	got := domain.ToolCall{Name: "lookup_account_record", Arguments: map[string]any{"id": int64(42)}}
	sel, params := m.ScoreCall(exp, got)
	if sel {
		t.Fatal("wrong tool must not pass selection")
	}
	if params {
		t.Fatal("params should not pass when selection fails")
	}
}

func TestScoreCallPrefixStripping(t *testing.T) {
	m := Matcher{}
	exp := domain.ExpectedCall{ToolName: "get_issue", Arguments: map[string]any{"id": int64(42)}}

	cases := []string{
		"get_issue",
		"functions.get_issue",
		"tools.get_issue",
		"function.get_issue",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			sel, params := m.ScoreCall(exp, domain.ToolCall{Name: name, Arguments: map[string]any{"id": int64(42)}})
			if !sel || !params {
				t.Fatalf("expected prefix stripping to match for %s, got sel=%v params=%v", name, sel, params)
			}
		})
	}
}

func TestScoreCallParamsIgnoreOrderAndWhitespace(t *testing.T) {
	m := Matcher{}
	cases := []struct {
		name       string
		exp        map[string]any
		got        map[string]any
		strictType bool
		want       bool
	}{
		{"string trim", map[string]any{"q": "alpha "}, map[string]any{"q": " alpha"}, false, true},
		{"string to int non-strict", map[string]any{"id": int64(7)}, map[string]any{"id": "7"}, false, true},
		{"string to int strict", map[string]any{"id": int64(7)}, map[string]any{"id": "7"}, true, false},
		{"string to bool non-strict true", map[string]any{"active": true}, map[string]any{"active": "true"}, false, true},
		{"string to bool non-strict false", map[string]any{"active": false}, map[string]any{"active": "false"}, false, true},
		{"string to bool strict", map[string]any{"active": true}, map[string]any{"active": "true"}, true, false},
		{"int float equal non-strict", map[string]any{"n": int64(3)}, map[string]any{"n": 3.0}, false, true},
		{"missing key", map[string]any{"a": 1.0, "b": 2.0}, map[string]any{"a": 1.0}, false, false},
		{"nested object", map[string]any{"f": map[string]any{"x": 1.0}}, map[string]any{"f": map[string]any{"x": 1.0}}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := m.ScoreCall(domain.ExpectedCall{ToolName: "t", Arguments: tc.exp, StrictType: tc.strictType},
				domain.ToolCall{Name: "t", Arguments: tc.got})
			if ok != tc.want {
				t.Fatalf("got %v want %v", ok, tc.want)
			}
		})
	}
}

func TestVerifyChain(t *testing.T) {
	hops := []domain.ExpectedCall{
		{ToolName: "create_session"},
		{ToolName: "publish_report", Arguments: map[string]any{"session_id": "<from:create_session.session_id>", "report": "q3"}},
	}
	returns := []map[string]any{
		{"tool": "create_session", "session_id": "s-1"},
	}
	executedOK := []domain.ToolCall{
		{Name: "create_session"},
		{Name: "publish_report", Arguments: map[string]any{"session_id": "s-1", "report": "q3"}},
	}
	if r := VerifyChain(hops, executedOK, returns); !r.Complete {
		t.Fatalf("expected complete, got %+v", r)
	}
	executedWrong := []domain.ToolCall{
		{Name: "publish_report"},
		{Name: "publish_report", Arguments: map[string]any{"session_id": "s-1", "report": "q3"}},
	}
	if r := VerifyChain(hops, executedWrong, nil); r.Complete || r.CorrectStep != 0 {
		t.Fatalf("expected incomplete at step 0, got %+v", r)
	}
	executedShort := []domain.ToolCall{
		{Name: "create_session"},
	}
	if r := VerifyChain(hops, executedShort, nil); r.Complete || r.CorrectStep != 1 {
		t.Fatalf("expected incomplete at step 1 for short chain, got %+v", r)
	}
}
