package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	got, err := expandHome("~")
	if err != nil || got != home {
		t.Fatalf("expandHome(~) = (%q, %v), want (%q, nil)", got, err, home)
	}

	got, err = expandHome("~/bin/mcp-compressor")
	want := filepath.Join(home, "bin/mcp-compressor")
	if err != nil || got != want {
		t.Fatalf("expandHome(~/bin/mcp-compressor) = (%q, %v), want (%q, nil)", got, err, want)
	}

	got, err = expandHome(`~\bin\mcp-compressor`)
	wantWin := filepath.Join(home, "bin", "mcp-compressor")
	if err != nil || got != wantWin {
		t.Fatalf("expandHome(~\\bin\\mcp-compressor) = (%q, %v), want (%q, nil)", got, err, wantWin)
	}

	got, err = expandHome("/usr/local/bin/mcp-compressor")
	if err != nil || got != "/usr/local/bin/mcp-compressor" {
		t.Fatalf("expandHome(abs) = (%q, %v), want abs path", got, err)
	}
}

func TestUnwrapCallResult(t *testing.T) {
	// 1. Standard MCP format with JSON text
	mcpTextJSON := []byte(`{"content":[{"type":"text","text":"{\"ok\":true,\"tool\":\"test_tool\",\"result_id\":\"res-1-abc\"}"}]}`)
	res, err := unwrapCallResult(mcpTextJSON)
	if err != nil {
		t.Fatalf("unwrap MCP JSON text failed: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["tool"] != "test_tool" || m["ok"] != true {
		t.Fatalf("unexpected unwrap result: %+v", res)
	}

	// 2. Standard MCP format with plain text
	mcpPlain := []byte(`{"content":[{"type":"text","text":"plain string output"}]}`)
	res, err = unwrapCallResult(mcpPlain)
	if err != nil {
		t.Fatalf("unwrap plain text failed: %v", err)
	}
	if str, ok := res.(string); !ok || str != "plain string output" {
		t.Fatalf("unexpected plain text result: %+v", res)
	}

	// 3. Raw JSON map fallback
	rawMap := []byte(`{"ok":true,"tool":"direct_tool"}`)
	res, err = unwrapCallResult(rawMap)
	if err != nil {
		t.Fatalf("unwrap raw map failed: %v", err)
	}
	m, ok = res.(map[string]any)
	if !ok || m["tool"] != "direct_tool" {
		t.Fatalf("unexpected raw map result: %+v", res)
	}
}

func TestSubprocessProxyLevelNoneDirect(t *testing.T) {
	if os.Getenv("BE_SUBPROCESS_MCP_FAKE") == "1" {
		runSubprocessFakeServer()
		os.Exit(0)
	}

	targetCmd := []string{
		"/usr/bin/env", "BE_SUBPROCESS_MCP_FAKE=1", os.Args[0],
		"-test.run=TestSubprocessProxyLevelNoneDirect", "--",
	}

	cfg := Config{
		Level:     domain.LevelNone,
		Binary:    "/should/be/bypassed",
		TargetCmd: targetCmd,
	}

	ctx := context.Background()
	p, err := newSubprocessProxy(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("newSubprocessProxy failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	tools, err := p.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "fake_tool" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	callRes, err := p.Call(ctx, domain.ToolCall{
		Name:      "fake_tool",
		Arguments: map[string]any{"arg": "val"},
	})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	wantRes := map[string]any{"ok": true, "tool": "fake_tool"}
	if !reflect.DeepEqual(callRes, wantRes) {
		t.Fatalf("Call result = %v, want %v", callRes, wantRes)
	}
}

func runSubprocessFakeServer() {
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(line, &req)
		if req.Method == "" {
			continue
		}

		switch req.Method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		case "tools/list":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "fake_tool",
							"description": "A fake tool.",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		case "tools/call":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{
							"type": "text",
							"text": `{"ok":true,"tool":"fake_tool"}`,
						},
					},
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		}
	}
}
