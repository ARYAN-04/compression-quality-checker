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
	"strconv"
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
	Content    string           `json:"content,omitempty"`
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

// StringOrRawJSON stores tool arguments and guarantees that when serialized
// in an outgoing OpenAI payload, it is always encoded as a JSON string.
type StringOrRawJSON string

func (s *StringOrRawJSON) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}
	if trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(trimmed, &str); err == nil {
			*s = StringOrRawJSON(str)
			return nil
		}
	}
	*s = StringOrRawJSON(string(trimmed))
	return nil
}

func (s StringOrRawJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

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
	msg.ToolCalls[0].Function.Arguments = StringOrRawJSON(argumentsJSON)
	return msg
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments StringOrRawJSON `json:"arguments"`
	} `json:"function"`
}

type openAIFunctionCall struct {
	Name      string          `json:"name"`
	Arguments StringOrRawJSON `json:"arguments"`
}

type chatRequest struct {
	Model      string       `json:"model"`
	Messages   []Message    `json:"messages"`
	Tools      []openAITool `json:"tools,omitempty"`
	ToolChoice string       `json:"tool_choice,omitempty"`
	Stream     bool         `json:"stream"`
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
	if len(req.Tools) > 0 {
		req.ToolChoice = "auto"
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
				name := normalizeName(tc.Function.Name)
				out.ToolCalls = append(out.ToolCalls, domain.ToolCall{
					ID:        id,
					Name:      name,
					Arguments: args,
				})
			}
		} else if msg.FunctionCall != nil && msg.FunctionCall.Name != "" {
			args := parseArguments(msg.FunctionCall.Arguments)
			name := normalizeName(msg.FunctionCall.Name)
			out.ToolCalls = append(out.ToolCalls, domain.ToolCall{
				ID:        fmt.Sprintf("call_%s", name),
				Name:      name,
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

func parseArguments(v any) map[string]any {
	var rawBytes []byte
	switch val := v.(type) {
	case StringOrRawJSON:
		rawBytes = []byte(val)
	case json.RawMessage:
		rawBytes = []byte(val)
	case []byte:
		rawBytes = val
	case string:
		rawBytes = []byte(val)
	default:
		if b, err := json.Marshal(v); err == nil {
			rawBytes = b
		}
	}
	trimmed := bytes.TrimSpace(rawBytes)
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

	// Pattern 1: <tool_call> JSON or code </tool_call>
	toolCallTagRe := regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)
	if matches := toolCallTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			if call, ok := parseJSONToolCall(m[1]); ok {
				out = append(out, call)
			} else if calls := parseCodeToolCalls(m[1]); len(calls) > 0 {
				out = append(out, calls...)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 2: [TOOL_CALLS] ... (Mistral / Hermes / Command-R format)
	toolCallsTagRe := regexp.MustCompile(`(?s)\[TOOL_CALLS\]\s*(.*?)(?:\[/TOOL_CALLS\]|$)`)
	if matches := toolCallsTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			if calls := parseJSONOrArrayCalls(m[1]); len(calls) > 0 {
				out = append(out, calls...)
			} else if calls := parseCodeToolCalls(m[1]); len(calls) > 0 {
				out = append(out, calls...)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 3: <function=name>args</function>
	funcTagRe := regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_\-]+)>\s*(.*?)\s*</function>`)
	if matches := funcTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			name := normalizeName(m[1])
			argsStr := strings.TrimSpace(m[2])
			var args map[string]any
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = parseCodeArguments(argsStr)
			}
			out = append(out, domain.ToolCall{
				ID:        fmt.Sprintf("call_%s", name),
				Name:      name,
				Arguments: args,
			})
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 4: ```json ... ``` code blocks
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

	// Pattern 5: ```(?:python|bash|plaintext)? ... ``` code blocks
	codeBlockRe := regexp.MustCompile("(?s)```(?:python|bash|sh|plaintext|text)?\\s*([a-zA-Z0-9_\\-]+)\\((.*?)\\)\\s*```")
	if matches := codeBlockRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, m := range matches {
			name := normalizeName(m[1])
			args := parseCodeArguments(m[2])
			out = append(out, domain.ToolCall{
				ID:        fmt.Sprintf("call_%s", name),
				Name:      name,
				Arguments: args,
			})
		}
		if len(out) > 0 {
			return out
		}
	}

	// Pattern 6: Whole content or substring is raw JSON object or array
	calls := parseJSONOrArrayCalls(content)
	if len(calls) > 0 {
		return calls
	}

	// Pattern 7: Bare function call in plain content
	if calls := parseCodeToolCalls(content); len(calls) > 0 {
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
	var args map[string]any

	if fnRaw, ok := raw["function"]; ok {
		var fnObj struct {
			Name       string          `json:"name"`
			Arguments  json.RawMessage `json:"arguments"`
			Parameters json.RawMessage `json:"parameters"`
		}
		if err := json.Unmarshal(fnRaw, &fnObj); err == nil && fnObj.Name != "" {
			name = fnObj.Name
			if len(fnObj.Arguments) > 0 {
				args = parseArguments(fnObj.Arguments)
			} else if len(fnObj.Parameters) > 0 {
				args = parseArguments(fnObj.Parameters)
			}
		}
	}

	if name == "" {
		for _, key := range []string{"name", "tool", "tool_name", "function"} {
			if val, ok := raw[key]; ok {
				var str string
				if err := json.Unmarshal(val, &str); err == nil && str != "" {
					name = str
					break
				}
			}
		}
	}
	if name == "" {
		return domain.ToolCall{}, false
	}
	name = normalizeName(name)

	if args == nil {
		for _, key := range []string{"arguments", "parameters", "args", "params", "input"} {
			if val, ok := raw[key]; ok {
				args = parseArguments(val)
				break
			}
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

var codeCallRe = regexp.MustCompile(`(?s)([a-zA-Z0-9_\-]+)\((.*?)\)`)

func parseCodeToolCalls(s string) []domain.ToolCall {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`")
	matches := codeCallRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	var out []domain.ToolCall
	for _, m := range matches {
		name := normalizeName(m[1])
		switch strings.ToLower(name) {
		case "if", "for", "while", "switch", "print", "echo", "return":
			continue
		}
		args := parseCodeArguments(m[2])
		out = append(out, domain.ToolCall{
			ID:        fmt.Sprintf("call_%s", name),
			Name:      name,
			Arguments: args,
		})
	}
	return out
}

func parseCodeArguments(rawArgs string) map[string]any {
	rawArgs = strings.TrimSpace(rawArgs)
	if rawArgs == "" {
		return map[string]any{}
	}
	if strings.HasPrefix(rawArgs, "{") && strings.HasSuffix(rawArgs, "}") {
		var m map[string]any
		if err := json.Unmarshal([]byte(rawArgs), &m); err == nil {
			return m
		}
	}
	args := splitCodeArguments(rawArgs)
	out := make(map[string]any, len(args))
	pos := 0
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		key, valStr, hasKey := splitKeyValue(arg)
		if hasKey {
			key = strings.TrimSpace(key)
			key = strings.Trim(key, `"'`)
			key = strings.TrimPrefix(key, "$")
			out[key] = parseScalarOrJSONValue(valStr)
		} else {
			out[fmt.Sprintf("arg%d", pos)] = parseScalarOrJSONValue(arg)
			pos++
		}
	}
	return out
}

func splitCodeArguments(s string) []string {
	var tokens []string
	var cur strings.Builder
	var inSingle, inDouble, escaped bool
	var braceDepth, bracketDepth, parenDepth int

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			cur.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			cur.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			cur.WriteByte(ch)
			continue
		}
		if !inSingle && !inDouble {
			switch ch {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			case ',':
				if braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
					tokens = append(tokens, cur.String())
					cur.Reset()
					continue
				}
			}
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func splitKeyValue(s string) (key, val string, ok bool) {
	var inSingle, inDouble, escaped bool
	var braceDepth, bracketDepth, parenDepth int

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble && braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
			if ch == '=' || ch == ':' {
				return s[:i], s[i+1:], true
			}
			switch ch {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			}
		}
	}
	return "", s, false
}

func parseScalarOrJSONValue(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2 {
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, `\'`, `'`)
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		var unquoted string
		if err := json.Unmarshal([]byte(s), &unquoted); err == nil {
			return unquoted
		}
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, `\"`, `"`)
	}
	lower := strings.ToLower(s)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	if lower == "none" || lower == "null" {
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) || (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			return v
		}
		jsonStr := strings.ReplaceAll(s, "'", `"`)
		if err := json.Unmarshal([]byte(jsonStr), &v); err == nil {
			return v
		}
	}
	return s
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "functions.")
	name = strings.TrimPrefix(name, "tools.")
	name = strings.TrimPrefix(name, "function.")
	return name
}

func truncateForLog(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
