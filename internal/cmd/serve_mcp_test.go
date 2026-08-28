package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"can-i-compress-it/internal/domain"
)

func sampleSuiteTools() []domain.ToolDefinition {
	return []domain.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for city.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string", "description": "City name"},
				},
				"required": []string{"city"},
			},
		},
		{
			Name:        "send_alert",
			Description: "Send alert message.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alert_id": map[string]any{"type": "string", "description": "Alert ID"},
				},
				"required": []string{"alert_id"},
			},
		},
	}
}

func TestServeStdioProtocol(t *testing.T) {
	tools := sampleSuiteTools()
	threadFields := map[string][]string{
		"get_weather": {"forecast_id", "temp_c"},
	}

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_weather","arguments":{"city":"London"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"unknown/method"}`,
		`invalid-json`,
	}

	input := strings.Join(requests, "\n") + "\n"
	in := strings.NewReader(input)
	var out bytes.Buffer

	err := serveStdio(in, &out, tools, threadFields)
	if err != nil {
		t.Fatalf("serveStdio failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 responses (notification skipped, 1 parse error), got %d: %v", len(lines), lines)
	}

	// 1. Initialize
	var respInit rpcResponse
	if err := json.Unmarshal([]byte(lines[0]), &respInit); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	initResult, ok := respInit.Result.(map[string]any)
	if !ok {
		t.Fatalf("init result not map: %v", respInit.Result)
	}
	if initResult["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocolVersion: %v", initResult["protocolVersion"])
	}
	serverInfo, _ := initResult["serverInfo"].(map[string]any)
	if serverInfo["name"] != "can-i-compress-it-suite" || serverInfo["version"] != "0.4.0" {
		t.Fatalf("unexpected serverInfo: %v", serverInfo)
	}

	// 2. Ping
	var respPing rpcResponse
	if err := json.Unmarshal([]byte(lines[1]), &respPing); err != nil {
		t.Fatalf("decode ping response: %v", err)
	}
	if respPing.Error != nil {
		t.Fatalf("ping error: %v", respPing.Error)
	}

	// 3. Tools List
	var respTools rpcResponse
	if err := json.Unmarshal([]byte(lines[2]), &respTools); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	toolsResult, ok := respTools.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result not map: %v", respTools.Result)
	}
	toolsList, ok := toolsResult["tools"].([]any)
	if !ok || len(toolsList) != 2 {
		t.Fatalf("expected 2 tools, got: %v", toolsResult)
	}
	firstTool := toolsList[0].(map[string]any)
	if firstTool["name"] != "get_weather" {
		t.Fatalf("unexpected tool name: %v", firstTool["name"])
	}
	if firstTool["inputSchema"] == nil || firstTool["parameters"] == nil {
		t.Fatalf("tool missing inputSchema or parameters: %v", firstTool)
	}

	// 4. Tools Call
	var respCall rpcResponse
	if err := json.Unmarshal([]byte(lines[3]), &respCall); err != nil {
		t.Fatalf("decode tools/call response: %v", err)
	}
	callResult, ok := respCall.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/call result not map: %v", respCall.Result)
	}
	content, ok := callResult["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected content in call result: %v", callResult)
	}
	firstContent := content[0].(map[string]any)
	if firstContent["type"] != "text" {
		t.Fatalf("expected content type text, got %v", firstContent["type"])
	}
	textStr, ok := firstContent["text"].(string)
	if !ok {
		t.Fatalf("expected text string in content: %v", firstContent)
	}
	var callPayload map[string]any
	if err := json.Unmarshal([]byte(textStr), &callPayload); err != nil {
		t.Fatalf("failed to decode text JSON payload: %v", err)
	}
	if callPayload["ok"] != true || callPayload["tool"] != "get_weather" {
		t.Fatalf("unexpected call payload: %v", callPayload)
	}
	if callPayload["forecast_id"] == nil || callPayload["temp_c"] == nil {
		t.Fatalf("missing thread fields in call payload: %v", callPayload)
	}
	if !strings.HasPrefix(callPayload["result_id"].(string), "res-1-") {
		t.Fatalf("unexpected result_id: %v", callPayload["result_id"])
	}

	// 5. Unknown method error
	var respUnknown rpcResponse
	if err := json.Unmarshal([]byte(lines[4]), &respUnknown); err != nil {
		t.Fatalf("decode unknown method response: %v", err)
	}
	if respUnknown.Error == nil || respUnknown.Error.Code != -32601 {
		t.Fatalf("expected code -32601 for unknown method, got: %v", respUnknown.Error)
	}

	// 6. Parse error
	var respParse rpcResponse
	if err := json.Unmarshal([]byte(lines[5]), &respParse); err != nil {
		t.Fatalf("decode parse error response: %v", err)
	}
	if respParse.Error == nil || respParse.Error.Code != -32700 {
		t.Fatalf("expected code -32700 for parse error, got: %v", respParse.Error)
	}
}

func TestSynthesizeCallResultDeterministic(t *testing.T) {
	fields := map[string][]string{
		"sample_tool": {"id_val", "status_val"},
	}

	res1 := synthesizeCallResult("sample_tool", 1, fields)
	res2 := synthesizeCallResult("sample_tool", 1, fields)

	if fmt.Sprint(res1) != fmt.Sprint(res2) {
		t.Fatalf("synthesizeCallResult not deterministic:\n res1=%v\n res2=%v", res1, res2)
	}

	res3 := synthesizeCallResult("sample_tool", 2, fields)
	if res1["result_id"] == res3["result_id"] {
		t.Fatalf("call index 1 and 2 produced same result_id")
	}
}
