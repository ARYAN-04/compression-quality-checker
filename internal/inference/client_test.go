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
	args1 := parseArguments(json.RawMessage(`"{\"owner\":\"torvalds\",\"repo\":\"linux\"}"`))
	if args1["owner"] != "torvalds" || args1["repo"] != "linux" {
		t.Errorf("stringified JSON parse failed: %v", args1)
	}

	// Direct JSON object
	args2 := parseArguments(json.RawMessage(`{"owner":"torvalds","issue_number":42}`))
	if args2["owner"] != "torvalds" || args2["issue_number"] != float64(42) {
		t.Errorf("direct JSON object parse failed: %v", args2)
	}

	// Empty string or null
	args3 := parseArguments(json.RawMessage(`null`))
	if len(args3) != 0 {
		t.Errorf("null parse failed: %v", args3)
	}

	args4 := parseArguments(json.RawMessage(`""`))
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

	// Pattern 2: <function=name> tag
	text2 := `<function=search_repositories>{"query": "language:go"}</function>`
	calls2 := extractTextToolCalls(text2)
	if len(calls2) != 1 || calls2[0].Name != "search_repositories" || calls2[0].Arguments["query"] != "language:go" {
		t.Fatalf("extractTextToolCalls Pattern 2 failed: %+v", calls2)
	}

	// Pattern 3: ```json markdown code block
	text3 := "```json\n{\"name\": \"create_issue\", \"arguments\": {\"title\": \"Fix bug\"}}\n```"
	calls3 := extractTextToolCalls(text3)
	if len(calls3) != 1 || calls3[0].Name != "create_issue" || calls3[0].Arguments["title"] != "Fix bug" {
		t.Fatalf("extractTextToolCalls Pattern 3 failed: %+v", calls3)
	}

	// Pattern 4: Raw JSON object
	text4 := `{"tool": "get_weather", "parameters": {"city": "Tokyo"}}`
	calls4 := extractTextToolCalls(text4)
	if len(calls4) != 1 || calls4[0].Name != "get_weather" || calls4[0].Arguments["city"] != "Tokyo" {
		t.Fatalf("extractTextToolCalls Pattern 4 failed: %+v", calls4)
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
