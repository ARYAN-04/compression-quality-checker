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
// (Ollama, vLLM, llama.cpp server, MLX, frontier APIs).
type Client struct {
	model    string
	endpoint string
	http     *http.Client
	apiKey   string
}

// NewClient builds a client for an OpenAI-compatible endpoint.
func NewClient(model, endpoint, apiKey string) *Client {
	return &Client{
		model:    model,
		endpoint: strings.TrimSuffix(endpoint, "/"),
		http:     &http.Client{Timeout: 120 * time.Second},
		apiKey:   apiKey,
	}
}

func (c *Client) Name() string { return c.model }

// AssistantToolCall builds an assistant message representing one call,
// for feeding tool-call turns back into the conversation.
func AssistantToolCall(name, argumentsJSON, id string) Message {
	msg := Message{Role: "assistant"}
	msg.ToolCalls = []openAIToolCall{{
		ID:   id,
		Type: "function",
	}}
	msg.ToolCalls[0].Function.Name = name
	msg.ToolCalls[0].Function.Arguments = argumentsJSON
	return msg
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
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
		c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	respHTTP, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("post %s: %w", c.endpoint, err)
	}
	defer func() { _ = respHTTP.Body.Close() }()
	data, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if respHTTP.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("endpoint status %d: %s", respHTTP.StatusCode, truncateForLog(data))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []openAIToolCall `json:"tool_calls"`
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
		out.Content = parsed.Choices[0].Message.Content
		for _, tc := range parsed.Choices[0].Message.ToolCalls {
			args := map[string]any{}
			if strings.TrimSpace(tc.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]any{"_raw": tc.Function.Arguments}
				}
			}
			out.ToolCalls = append(out.ToolCalls, domain.ToolCall{Name: tc.Function.Name, Arguments: args})
		}
	}
	return out, nil
}

func truncateForLog(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
