package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// TestStdioClientListTools drives the client against a re-executed test
// binary acting as a paginated fake MCP server.
func TestStdioClientListTools(t *testing.T) {
	if os.Getenv("BE_FAKE_MCP") == "1" {
		runFakeMCPServer()
		os.Exit(0)
	}
	command := []string{
		"/usr/bin/env", "BE_FAKE_MCP=1", os.Args[0],
		"-test.run=TestStdioClientListTools", "--",
	}
	client, err := NewStdioClient(context.Background(), command)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools across pages, got %d: %+v", len(tools), tools)
	}
	want := map[string]string{
		"read_file":  "Read a file.",
		"list_dir":   "List a directory.",
		"write_file": "Write a file.",
	}
	for _, tl := range tools {
		if want[tl.Name] != tl.Description {
			t.Fatalf("tool %s = %q, want %q", tl.Name, tl.Description, want[tl.Name])
		}
		if len(tl.Parameters["properties"].(map[string]any)) == 0 {
			t.Fatalf("tool %s lost its schema: %+v", tl.Name, tl.Parameters)
		}
	}
}

func runFakeMCPServer() {
	in := bufio.NewScanner(os.Stdin)
	page := 0
	for in.Scan() {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"-"`
		}
		_ = json.Unmarshal(in.Bytes(), &req)
		if req.Method == "" {
			continue // notifications carry no id or method response
		}
		switch req.Method {
		case "initialize":
			writeLine(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"protocolVersion": "2024-11-05"},
			})
		case "tools/list":
			page++
			schema := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
			var tools []map[string]any
			next := ""
			switch page {
			case 1:
				tools = []map[string]any{{"name": "read_file", "description": "Read a file.", "inputSchema": schema}}
				next = "p2"
			case 2:
				tools = []map[string]any{
					{"name": "list_dir", "description": "List a directory.", "inputSchema": schema},
					{"name": "write_file", "description": "Write a file.", "parameters": schema},
				}
			}
			result := map[string]any{"tools": tools}
			if next != "" {
				result["nextCursor"] = next
			}
			writeLine(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
	}
}

func writeLine(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}
