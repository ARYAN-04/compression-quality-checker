package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"can-i-compress-it/internal/domain"
)

// jsonRPCRequest is one JSON-RPC 2.0 call over stdio.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// subprocessProxy manages an external mcp-compressor or direct target MCP process.
type subprocessProxy struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	nextID  int
	level   domain.CompressionLevel
	tools   []domain.ToolDefinition
}

func expandHome(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home dir: %w", err)
		}
		rel := filepath.FromSlash(strings.ReplaceAll(path[2:], `\`, "/"))
		return filepath.Join(home, rel), nil
	}
	return path, nil
}

func newSubprocessProxy(ctx context.Context, cfg Config, suiteTools []domain.ToolDefinition) (Proxy, error) {
	targetCmd := cfg.TargetCmd
	if len(targetCmd) == 0 {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("self binary path: %w", err)
		}
		targetServer := cfg.TargetServer
		if targetServer == "" {
			targetServer = "suite"
		}
		targetCmd = []string{self, "serve-mcp", "--suite", targetServer}
	}

	var cmd *exec.Cmd
	if cfg.Level == domain.LevelNone {
		cmd = exec.CommandContext(ctx, targetCmd[0], targetCmd[1:]...)
	} else {
		bin, err := expandHome(cfg.Binary)
		if err != nil {
			return nil, fmt.Errorf("expand compressor binary: %w", err)
		}
		if bin == "" {
			return nil, fmt.Errorf("compressor binary path is required for level %s", cfg.Level)
		}
		args := []string{"-c", string(cfg.Level)}
		if len(targetCmd) > 0 && targetCmd[0] == "--" {
			args = append(args, targetCmd...)
		} else {
			args = append(args, "--")
			args = append(args, targetCmd...)
		}
		cmd = exec.CommandContext(ctx, bin, args...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("compressor stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("compressor stdout: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start subprocess: %w", err)
	}

	p := &subprocessProxy{
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		level:   cfg.Level,
		tools:   suiteTools,
	}

	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "can-i-compress-it", "version": "0.4.0"},
	}
	if _, err := p.rpc(context.Background(), "initialize", initParams); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("initialize compressor: %w", err)
	}
	if err := p.notify("notifications/initialized"); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("initialized notification: %w", err)
	}
	return p, nil
}

func (p *subprocessProxy) notify(method string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	req := map[string]any{"jsonrpc": "2.0", "method": method}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

func (p *subprocessProxy) Tools(_ context.Context) ([]domain.ToolDefinition, error) {
	raw, err := p.rpc(context.Background(), "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	var out struct {
		Tools []mcpServerTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	tools := make([]domain.ToolDefinition, len(out.Tools))
	for i, t := range out.Tools {
		tools[i] = t.toolDefinition()
	}
	return tools, nil
}

func (p *subprocessProxy) Call(ctx context.Context, call domain.ToolCall) (any, error) {
	args := cloneMap(call.Arguments)
	if strings.Contains(call.Name, "invoke_tool") {
		if _, ok := args["tool_input"]; !ok {
			if innerArgs, ok := args["arguments"].(map[string]any); ok {
				args["tool_input"] = innerArgs
			} else {
				args["tool_input"] = map[string]any{}
			}
		}
	}
	params := map[string]any{"name": call.Name, "arguments": args}
	raw, err := p.rpc(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", call.Name, err)
	}
	return unwrapCallResult(raw)
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func unwrapCallResult(raw json.RawMessage) (any, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err == nil && len(resp.Content) > 0 {
		for _, c := range resp.Content {
			if c.Type == "text" && c.Text != "" {
				var inner any
				if err := json.Unmarshal([]byte(c.Text), &inner); err == nil {
					return inner, nil
				}
				return c.Text, nil
			}
		}
	}
	var fallback any
	if err := json.Unmarshal(raw, &fallback); err != nil {
		return nil, fmt.Errorf("decode tools/call result: %w", err)
	}
	return fallback, nil
}

func (p *subprocessProxy) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		return p.cmd.Wait()
	}
	return nil
}

func (p *subprocessProxy) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = b
	}
	p.nextID++
	req := jsonRPCRequest{JSONRPC: "2.0", ID: p.nextID, Method: method, Params: rawParams}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("write rpc: %w", err)
	}
	done := make(chan error, 1)
	var resp jsonRPCResponse
	go func() {
		for p.scanner.Scan() {
			data := p.scanner.Bytes()
			if len(data) == 0 {
				continue
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				done <- err
				return
			}
			if resp.ID == req.ID {
				done <- nil
				return
			}
			resp = jsonRPCResponse{}
		}
		if err := p.scanner.Err(); err != nil {
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
