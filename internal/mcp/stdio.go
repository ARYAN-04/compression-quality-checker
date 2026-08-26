package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"can-i-compress-it/internal/domain"
)

// StdioClient is a generic MCP client for one external server speaking
// newline-delimited JSON-RPC 2.0 over stdio. It performs the standard
// initialize handshake and supports tools/list cursor pagination, so it
// works against real MCP servers (not just mcp-compressor).
type StdioClient struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	nextID  int
}

// NewStdioClient spawns the server command and completes the MCP
// initialize handshake. The command is user-supplied, e.g.
// []string{"npx", "@modelcontextprotocol/server-github"}.
func NewStdioClient(ctx context.Context, command []string) (*StdioClient, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty mcp server command")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("server stdout: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command[0], err)
	}
	c := &StdioClient{cmd: cmd, stdin: stdin, scanner: bufio.NewScanner(stdout)}
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "can-i-compress-it", "version": "0.2.0"},
	}
	if _, err := c.rpc(context.Background(), "initialize", initParams); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialize server: %w", err)
	}
	if err := c.notify("notifications/initialized"); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialized notification: %w", err)
	}
	return c, nil
}

// ListTools returns every tool the server exposes, following pagination.
func (c *StdioClient) ListTools(ctx context.Context) ([]domain.ToolDefinition, error) {
	var out []domain.ToolDefinition
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.rpc(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		var page struct {
			Tools      []mcpServerTool `json:"tools"`
			NextCursor string          `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("decode tools/list: %w", err)
		}
		for _, t := range page.Tools {
			out = append(out, t.toolDefinition())
		}
		if page.NextCursor == "" {
			return out, nil
		}
		cursor = page.NextCursor
	}
}

// Close kills the server process and reaps it.
func (c *StdioClient) Close() error {
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}

// mcpServerTool tolerates both the standard inputSchema field and the
// legacy parameters field used by mcp-compressor.
type mcpServerTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Parameters  map[string]any `json:"parameters"`
}

func (t mcpServerTool) toolDefinition() domain.ToolDefinition {
	schema := t.Parameters
	if len(t.InputSchema) > 0 {
		schema = t.InputSchema
	}
	return domain.ToolDefinition{Name: t.Name, Description: t.Description, Parameters: schema}
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
}

func (c *StdioClient) notify(method string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	line, err := json.Marshal(rpcNotification{JSONRPC: "2.0", Method: method})
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

// rpc sends one request and waits for the matching response line.
func (c *StdioClient) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = b
	}
	c.nextID++
	req := jsonRPCRequest{JSONRPC: "2.0", ID: c.nextID, Method: method, Params: rawParams}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("write rpc: %w", err)
	}
	done := make(chan error, 1)
	var resp jsonRPCResponse
	go func() {
		for c.scanner.Scan() {
			if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
				done <- err
				return
			}
			if resp.ID == req.ID {
				done <- nil
				return
			}
			// Skip notifications and mismatched responses.
			resp = jsonRPCResponse{}
		}
		if err := c.scanner.Err(); err != nil {
			done <- err
			return
		}
		done <- io.ErrUnexpectedEOF
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("read rpc response: %w", err)
		}
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}
