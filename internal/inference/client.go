// Package inference drives local and reference language models over
// OpenAI-compatible chat completion APIs, plus a deterministic mock.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"can-i-compress-it/internal/domain"
)

// Response carries one model turn: the tool calls requested and usage stats.
type Response struct {
	Content    string
	ToolCalls  []domain.ToolCall
	TokensUsed int
}

// Model is a pluggable inference backend.
type Model interface {
	// Chat sends a conversation plus tool catalogue and returns the turn.
	Chat(ctx context.Context, msgs []Message, tools []domain.ToolDefinition) (Response, error)
	Name() string
}

// Message is one OpenAI-style conversation entry.
type Message struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// Client talks to any OpenAI-compatible /v1/chat/completions endpoint
// (LM Studio, Ollama, vLLM, llama.cpp server, MLX, frontier APIs).
type Client struct {
	model   string
	chatURL string
	http    *http.Client
	apiKey  string
}

// NormalizeChatURL resolves an OpenAI-compatible base URL or full endpoint URL
// to the complete /v1/chat/completions (or custom path /chat/completions) URL.
func NormalizeChatURL(endpoint string) string {
	ep := strings.TrimSpace(endpoint)
	ep = strings.TrimRight(ep, "/")
	if ep == "" {
		return "http://localhost:1234/v1/chat/completions"
	}
	if strings.HasSuffix(ep, "/chat/completions") {
		if !strings.Contains(ep, "/v1/") && !strings.HasSuffix(ep, "/v1/chat/completions") {
			return strings.TrimSuffix(ep, "/chat/completions") + "/v1/chat/completions"
		}
		return ep
	}
	if strings.HasSuffix(ep, "/v1") {
		return ep + "/chat/completions"
	}
	return ep + "/v1/chat/completions"
}

// NewClient builds a client for an OpenAI-compatible endpoint.
func NewClient(model, endpoint, apiKey string) *Client {
	return &Client{
		model:   model,
		chatURL: NormalizeChatURL(endpoint),
		http:    &http.Client{Timeout: 120 * time.Second},
		apiKey:  apiKey,
	}
}

func (c *Client) Name() string { return c.model }

// AssistantToolCall builds an assistant message representing one call,
// for feeding tool-call turns back into the conversation.
func AssistantToolCall(name, argumentsJSON, id string) Message {
	if id == "" {
		id = fmt.Sprintf("call_%s", name)
	}
	msg := Message{Role: "assistant"}
	msg.ToolCalls = []openAIToolCall{{
		ID:   id,
		Type: "function",
	}}
	msg.ToolCalls[0].Function.Name = name
	msg.ToolCalls[0].Function.Arguments = json.RawMessage(argumentsJSON)
	return msg
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type openAIFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type chatRequest struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Tools    []openAITool `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// Chat implements Model against /chat/completions.
func (c *Client) Chat(ctx context.Context, msgs []Message, tools []domain.ToolDefinition) (Response, error) {
	req := chatRequest{Model: c.model, Messages: msgs, Stream: false}
	for _, t := range tools {
		req.Tools = append(req.Tools, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.chatURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	respHTTP, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("post %s: %w", c.chatURL, err)
	}
	defer func() { _ = respHTTP.Body.Close() }()
	data, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if respHTTP.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("endpoint status %d: %s (url: %s)", respHTTP.StatusCode, truncateForLog(data), c.chatURL)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content      *string             `json:"content"`
				ToolCalls    []openAIToolCall    `json:"tool_calls"`
				FunctionCall *openAIFunctionCall `json:"function_call"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	out := Response{TokensUsed: parsed.Usage.TotalTokens}
	if len(parsed.Choices) > 0 {
		msg := parsed.Choices[0].Message
		if msg.Content != nil {
			out.Content = *msg.Content
		}
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				args := parseArguments(tc.Function.Arguments)
				id := tc.ID
				if id == "" {
					id = fmt.Sprintf("call_%s", tc.Function.Name)
				}
				out.ToolCalls = append(out.ToolCalls, domain.ToolCall{
					ID:        id,
					Name:      tc.Function.Name,
					Arguments: args,
				})
			}
		} else if msg.FunctionCall != nil && msg.FunctionCall.Name != "" {
			args := parseArguments(msg.FunctionCall.Arguments)
			out.ToolCalls = append(out.ToolCalls, domain.ToolCall{
				ID:        fmt.Sprintf("call_%s", msg.FunctionCall.Name),
				Name:      msg.FunctionCall.Name,
				Arguments: args,
			})
		} else if out.Content != "" {
			extracted := extractTextToolCalls(out.Content)
			if len(extracted) > 0 {
				out.ToolCalls = append(out.ToolCalls, extracted...)
			}
		}
	}
	return out, nil
}

func parseArguments(raw json.RawMessage) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]any{}
	}
	if trimmed[0] == '{' {
		var args map[string]any
		if err := json.Unmarshal(trimmed, &args); err == nil {
			return args
		}
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			sTrim := strings.TrimSpace(s)
			if sTrim == "" || sTrim == "{}" {
				return map[string]any{}
			}
			var inner map[string]any
			if err := json.Unmarshal([]byte(sTrim), &inner); err == nil {
				return inner
			}
			return map[string]any{"_raw": s}
		}
	}
	return map[string]any{"_raw": string(trimmed)}
}

func extractTextToolCalls(content string) []domain.ToolCall {
	var out []domain.ToolCall

	// Pattern 1: <tool_call> JSON </tool_call>
	toolCallTagRe := regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)
	if matches := toolCallTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			if call, ok := parseJSONToolCall(m[1]); ok {
				out = append(out, call)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 2: <function=name>args_json</function>
	funcTagRe := regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_\-]+)>\s*(.*?)\s*</function>`)
	if matches := funcTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			var args map[string]any
			if err := json.Unmarshal([]byte(m[2]), &args); err == nil {
				out = append(out, domain.ToolCall{
					ID:        fmt.Sprintf("call_%s", m[1]),
					Name:      m[1],
					Arguments: args,
				})
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 3: ```json ... ``` code blocks
	jsonBlockRe := regexp.MustCompile("(?s)```(?:json)?\\s*([\\{\\[].*?[\\}\\]])\\s*```")
	if matches := jsonBlockRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			calls := parseJSONOrArrayCalls(m[1])
			out = append(out, calls...)
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 4: Whole content or substring is raw JSON object or array
	calls := parseJSONOrArrayCalls(content)
	if len(calls) > 0 {
		return calls
	}

	return nil
}

func parseJSONOrArrayCalls(s string) []domain.ToolCall {
	s = strings.TrimSpace(s)
	var list []json.RawMessage
	if err := json.Unmarshal([]byte(s), &list); err == nil {
		var out []domain.ToolCall
		for _, item := range list {
			if call, ok := parseJSONToolCall(string(item)); ok {
				out = append(out, call)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if call, ok := parseJSONToolCall(s); ok {
		return []domain.ToolCall{call}
	}
	return nil
}

func parseJSONToolCall(s string) (domain.ToolCall, bool) {
	s = strings.TrimSpace(s)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return domain.ToolCall{}, false
	}
	var name string
	for _, key := range []string{"name", "tool", "tool_name", "function"} {
		if val, ok := raw[key]; ok {
			var str string
			if err := json.Unmarshal(val, &str); err == nil && str != "" {
				name = str
				break
			}
		}
	}
	if name == "" {
		return domain.ToolCall{}, false
	}
	var args map[string]any
	for _, key := range []string{"arguments", "parameters", "args", "params", "input"} {
		if val, ok := raw[key]; ok {
			args = parseArguments(val)
			break
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	return domain.ToolCall{
		ID:        fmt.Sprintf("call_%s", name),
		Name:      name,
		Arguments: args,
	}, true
}

func truncateForLog(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
