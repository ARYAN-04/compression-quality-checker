package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestNormalizeChatURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "http://192.168.1.50:1234",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://192.168.1.50:1234/",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://192.168.1.50:1234/v1",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://192.168.1.50:1234/v1/",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://192.168.1.50:1234/v1/chat/completions",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://192.168.1.50:1234/chat/completions",
			want:  "http://192.168.1.50:1234/v1/chat/completions",
		},
		{
			input: "http://localhost:11434",
			want:  "http://localhost:11434/v1/chat/completions",
		},
		{
			input: "http://localhost:11434/v1",
			want:  "http://localhost:11434/v1/chat/completions",
		},
		{
			input: "https://openrouter.ai/api/v1",
			want:  "https://openrouter.ai/api/v1/chat/completions",
		},
		{
			input: "https://openrouter.ai/api/v1/chat/completions",
			want:  "https://openrouter.ai/api/v1/chat/completions",
		},
		{
			input: "",
			want:  "http://localhost:1234/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		got := NormalizeChatURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeChatURL(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseArguments(t *testing.T) {
	// Stringified JSON
	args1 := parseArguments(StringOrRawJSON(`"{\"owner\":\"torvalds\",\"repo\":\"linux\"}"`))
	if args1["owner"] != "torvalds" || args1["repo"] != "linux" {
		t.Errorf("stringified JSON parse failed: %v", args1)
	}

	// Direct JSON object
	args2 := parseArguments(StringOrRawJSON(`{"owner":"torvalds","issue_number":42}`))
	if args2["owner"] != "torvalds" || args2["issue_number"] != float64(42) {
		t.Errorf("direct JSON object parse failed: %v", args2)
	}

	// Empty string or null
	args3 := parseArguments(StringOrRawJSON(`null`))
	if len(args3) != 0 {
		t.Errorf("null parse failed: %v", args3)
	}

	args4 := parseArguments(StringOrRawJSON(`""`))
	if len(args4) != 0 {
		t.Errorf("empty string parse failed: %v", args4)
	}
}

func TestExtractTextToolCalls(t *testing.T) {
	// Pattern 1: <tool_call> tag
	text1 := `<tool_call>{"name": "get_issue", "arguments": {"owner": "torvalds", "issue_number": 1}}</tool_call>`
	calls1 := extractTextToolCalls(text1)
	if len(calls1) != 1 || calls1[0].Name != "get_issue" || calls1[0].Arguments["owner"] != "torvalds" {
		t.Fatalf("extractTextToolCalls Pattern 1 failed: %+v", calls1)
	}

	// Pattern 2: [TOOL_CALLS] tag (Mistral / Hermes)
	text2 := `[TOOL_CALLS] [{"name": "update_status", "arguments": {"id": 123}}]`
	calls2 := extractTextToolCalls(text2)
	if len(calls2) != 1 || calls2[0].Name != "update_status" || calls2[0].Arguments["id"] != float64(123) {
		t.Fatalf("extractTextToolCalls Pattern 2 failed: %+v", calls2)
	}

	// Pattern 3: <function=name> tag
	text3 := `<function=search_repositories>{"query": "language:go"}</function>`
	calls3 := extractTextToolCalls(text3)
	if len(calls3) != 1 || calls3[0].Name != "search_repositories" || calls3[0].Arguments["query"] != "language:go" {
		t.Fatalf("extractTextToolCalls Pattern 3 failed: %+v", calls3)
	}

	// Pattern 4: ```json markdown code block
	text4 := "```json\n{\"name\": \"create_issue\", \"arguments\": {\"title\": \"Fix bug\"}}\n```"
	calls4 := extractTextToolCalls(text4)
	if len(calls4) != 1 || calls4[0].Name != "create_issue" || calls4[0].Arguments["title"] != "Fix bug" {
		t.Fatalf("extractTextToolCalls Pattern 4 failed: %+v", calls4)
	}

	// Pattern 5: ```python code block function call
	text5 := "```python\nupdate_issue_status(issue_key='ACME-77', new_status='In Review')\n```"
	calls5 := extractTextToolCalls(text5)
	if len(calls5) != 1 || calls5[0].Name != "update_issue_status" || calls5[0].Arguments["issue_key"] != "ACME-77" || calls5[0].Arguments["new_status"] != "In Review" {
		t.Fatalf("extractTextToolCalls Pattern 5 (python) failed: %+v", calls5)
	}

	// Pattern 6: ```bash code block function call
	text6 := "```bash\nsearch_files('/srv/projects', '*.log')\n```"
	calls6 := extractTextToolCalls(text6)
	if len(calls6) != 1 || calls6[0].Name != "search_files" || calls6[0].Arguments["arg0"] != "/srv/projects" || calls6[0].Arguments["arg1"] != "*.log" {
		t.Fatalf("extractTextToolCalls Pattern 6 (bash) failed: %+v", calls6)
	}

	// Pattern 7: Raw JSON object
	text7 := `{"tool": "get_weather", "parameters": {"city": "Tokyo"}}`
	calls7 := extractTextToolCalls(text7)
	if len(calls7) != 1 || calls7[0].Name != "get_weather" || calls7[0].Arguments["city"] != "Tokyo" {
		t.Fatalf("extractTextToolCalls Pattern 7 failed: %+v", calls7)
	}

	// Pattern 8: Bare function call in text
	text8 := `update_issue_status(issue_key="ACME-77", new_status="Done")`
	calls8 := extractTextToolCalls(text8)
	if len(calls8) != 1 || calls8[0].Name != "update_issue_status" || calls8[0].Arguments["issue_key"] != "ACME-77" || calls8[0].Arguments["new_status"] != "Done" {
		t.Fatalf("extractTextToolCalls Pattern 8 (bare) failed: %+v", calls8)
	}
}

func TestClientChatToolChoiceAuto(t *testing.T) {
	var capturedReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "get_issue",
							"arguments": {"owner": "torvalds"}
						}
					}]
				}
			}],
			"usage": {"total_tokens": 100}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewClient("test-model", server.URL, "")
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "get issue"}}, []domain.ToolDefinition{
		{Name: "get_issue", Description: "Get issue"},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if capturedReq.ToolChoice != "auto" {
		t.Errorf("expected tool_choice 'auto', got %q", capturedReq.ToolChoice)
	}
}

func TestClientChatLMStudioObjectArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Response with arguments as a JSON object instead of string
		resp := `{
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_lmstudio_123",
						"type": "function",
						"function": {
							"name": "get_issue",
							"arguments": {"owner": "torvalds", "repo": "linux", "issue_number": 42}
						}
					}]
				}
			}],
			"usage": {"total_tokens": 150}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	// Pass server root URL without /v1/chat/completions to verify normalization
	client := NewClient("qwen2.5-7b-instruct", server.URL, "")
	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "get issue 42"}}, []domain.ToolDefinition{
		{Name: "get_issue", Description: "Get an issue"},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "get_issue" || tc.ID != "call_lmstudio_123" {
		t.Errorf("tool call mismatch: %+v", tc)
	}
	if tc.Arguments["owner"] != "torvalds" || tc.Arguments["issue_number"] != float64(42) {
		t.Errorf("tool arguments mismatch: %+v", tc.Arguments)
	}
	if resp.TokensUsed != 150 {
		t.Errorf("tokens used mismatch: got %d, want 150", resp.TokensUsed)
	}
}

func TestClientChatLegacyFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"choices": [{
				"message": {
					"role": "assistant",
					"function_call": {
						"name": "search_repos",
						"arguments": "{\"query\": \"stars:>1000\"}"
					}
				}
			}],
			"usage": {"total_tokens": 80}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewClient("legacy-model", server.URL+"/v1", "")
	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "find popular repos"}}, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "search_repos" {
		t.Fatalf("legacy function call parsing failed: %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].Arguments["query"] != "stars:>1000" {
		t.Errorf("legacy function call args mismatch: %+v", resp.ToolCalls[0].Arguments)
	}
}

func TestClientChatTextContentFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "<tool_call>{\"name\": \"delete_file\", \"arguments\": {\"path\": \"/tmp/test.txt\"}}</tool_call>"
				}
			}],
			"usage": {"total_tokens": 60}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewClient("text-tool-model", server.URL, "")
	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "delete /tmp/test.txt"}}, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "delete_file" {
		t.Fatalf("content fallback parsing failed: %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("content fallback args mismatch: %+v", resp.ToolCalls[0].Arguments)
	}
}

func TestClientChat404ErrorFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Route not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("test-model", server.URL+"/nonexistent", "")
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err == nil {
		t.Fatalf("expected error on 404, got nil")
	}
	if errString := err.Error(); !testing.Short() {
		if !strings.Contains(errString, "endpoint status 404") || !strings.Contains(errString, "url:") {
			t.Errorf("error string missing 404 or URL context: %s", errString)
		}
	}
}

func TestAssistantToolCallArgumentsSerializedAsString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if len(req.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %d", len(req.Messages))
		}
		asst := req.Messages[1]
		if len(asst.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(asst.ToolCalls))
		}
		rawArgs := string(asst.ToolCalls[0].Function.Arguments)
		// Arguments in OpenAI spec MUST be serialized as a JSON string
		if !strings.HasPrefix(rawArgs, `"`) || !strings.HasSuffix(rawArgs, `"`) {
			t.Fatalf("arguments was serialized as raw JSON object instead of JSON string: %s", rawArgs)
		}

		resp := `{"choices": [{"message": {"role": "assistant", "content": "Done"}}], "usage": {"total_tokens": 10}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	client := NewClient("test-model", server.URL, "")
	msgs := []Message{
		{Role: "user", Content: "create a session"},
		AssistantToolCall("create_session", `{"workspace":"alpha"}`, "call_123"),
		{Role: "tool", Content: `{"ok":true,"session_id":"s_1"}`, ToolCallID: "call_123"},
	}
	_, err := client.Chat(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
}
