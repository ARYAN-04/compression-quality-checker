package inference

import (
	"context"
	"testing"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/suite"
)

func firstCall(t *testing.T, resp Response) domain.ToolCall {
	t.Helper()
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	return resp.ToolCalls[0]
}

func chatOnce(t *testing.T, m *Mock, msgs []Message, tools []domain.ToolDefinition) domain.ToolCall {
	t.Helper()
	resp, err := m.Chat(context.Background(), msgs, tools)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	return firstCall(t, resp)
}

func intentTask(t *testing.T, category string, id int) domain.Task {
	t.Helper()
	tasks := suite.NewGenerator(suite.Options{NumTasks: id + 1, PromptStyle: suite.StyleIntent, Seed: 9}).Generate()
	for _, task := range tasks {
		if task.Category == category {
			return task
		}
	}
	t.Fatalf("no %s task generated", category)
	return domain.Task{}
}

func TestIntentAmbiguousSelectionAndArgs(t *testing.T) {
	task := intentTask(t, domain.CategoryAmbiguousName, 0)
	m := NewMock("mock-intent")
	call := chatOnce(t, m, []Message{{Role: "user", Content: task.Prompt}}, task.Tools)
	if call.Name != task.Expected.ToolName {
		t.Fatalf("selected %q, want %q", call.Name, task.Expected.ToolName)
	}
	if call.Arguments["id"] != int64(42) {
		t.Fatalf("args = %v, want id=42", call.Arguments)
	}
}

func TestIntentOverlappingSchemaArgs(t *testing.T) {
	task := intentTask(t, domain.CategoryOverlappingSchem, 1)
	m := NewMock("mock-intent")
	call := chatOnce(t, m, []Message{{Role: "user", Content: task.Prompt}}, task.Tools)
	if call.Name != "update_issue_status" {
		t.Fatalf("selected %q, want update_issue_status", call.Name)
	}
	if call.Arguments["id"] != "ISS-104" {
		t.Fatalf("args = %v, want id=ISS-104", call.Arguments)
	}
	if _, has := call.Arguments["filter"]; has {
		t.Fatalf("optional filter should not be bound: %v", call.Arguments)
	}
}

func TestIntentMultiHopChain(t *testing.T) {
	task := intentTask(t, domain.CategoryMultiHop, 2)
	m := NewMock("mock-intent")
	msgs := []Message{{Role: "user", Content: task.Prompt}}

	first := chatOnce(t, m, msgs, task.Tools)
	if first.Name != "create_session" || first.Arguments["workspace"] != "alpha" {
		t.Fatalf("step 1 = %+v", first)
	}
	msgs = append(msgs,
		AssistantToolCall(first.Name, `{"workspace":"alpha"}`, "call-1"),
		Message{Role: "tool", Content: `{"ok":true,"tool":"create_session","result_id":"res-0-abcd1234"}`},
	)

	second := chatOnce(t, m, msgs, task.Tools)
	if second.Name != "publish_report" {
		t.Fatalf("step 2 = %+v", second)
	}
	if second.Arguments["report"] != "q3" {
		t.Fatalf("step 2 args = %v, want report=q3", second.Arguments)
	}
	if second.Arguments["session_id"] != "res-0-abcd1234" {
		t.Fatalf("step 2 args = %v, want threaded session_id", second.Arguments)
	}
}

func TestIntentMetaFlowAtMaxCompression(t *testing.T) {
	task := intentTask(t, domain.CategoryAmbiguousName, 0)
	m := NewMock("mock-intent")
	metaTools := []domain.ToolDefinition{
		{Name: "get_tool_schema", Description: "Schema lookup."},
		{Name: "invoke_tool", Description: "Invoke a tool."},
	}
	msgs := []Message{{Role: "user", Content: task.Prompt}}
	call := chatOnce(t, m, msgs, metaTools)
	switch call.Name {
	case "get_tool_schema":
		if call.Arguments["tool_name"] != task.Expected.ToolName {
			t.Fatalf("schema target = %v, want %q", call.Arguments["tool_name"], task.Expected.ToolName)
		}
	case "invoke_tool":
		if call.Arguments["tool_name"] != task.Expected.ToolName {
			t.Fatalf("invoke target = %v, want %q", call.Arguments["tool_name"], task.Expected.ToolName)
		}
	default:
		t.Fatalf("unexpected meta call %q", call.Name)
	}
}

func TestExplicitPathUnchanged(t *testing.T) {
	tasks := suite.NewGenerator(suite.Options{NumTasks: 3, Ambiguity: "high", Seed: 42}).Generate()
	m := NewMock("mock")
	for _, task := range tasks {
		if task.Category == domain.CategoryMultiHop {
			continue
		}
		call := chatOnce(t, m, []Message{{Role: "user", Content: task.Prompt}}, task.Tools)
		if call.Name != task.Expected.ToolName {
			t.Fatalf("task %s: selected %q, want %q", task.ID, call.Name, task.Expected.ToolName)
		}
		for k, want := range task.Expected.Arguments {
			if got := call.Arguments[k]; got != want {
				t.Fatalf("task %s: arg %s = %v, want %v", task.ID, k, got, want)
			}
		}
	}
}

func TestStemInflections(t *testing.T) {
	cases := [][2]string{
		{"creates", "creating"},
		{"retrieve", "retrieves"},
		{"updated", "updates"},
		{"issue", "issues"},
		{"publishes", "publish"},
	}
	for _, c := range cases {
		if stem(c[0]) != stem(c[1]) {
			t.Fatalf("stem(%q)=%q != stem(%q)=%q", c[0], stem(c[0]), c[1], stem(c[1]))
		}
	}
}
