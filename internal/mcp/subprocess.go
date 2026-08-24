package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

// subprocessProxy manages an external mcp-compressor process.
type subprocessProxy struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   interface{ Write([]byte) (int, error) }
	scanner *bufio.Scanner
	nextID  int
	level   domain.CompressionLevel
	tools   []domain.ToolDefinition
}

func newSubprocessProxy(ctx context.Context, cfg Config, suiteTools []domain.ToolDefinition) (Proxy, error) {
	args := append([]string{
		"--level", string(cfg.Level),
	}, cfg.TargetCmd...)
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("compressor stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("compressor stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.Binary, err)
	}
	p := &subprocessProxy{
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		level:   cfg.Level,
		tools:   suiteTools,
	}
	if _, err := p.rpc(context.Background(), "initialize", map[string]any{}); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("initialize compressor: %w", err)
	}
	return p, nil
}

func (p *subprocessProxy) Tools(_ context.Context) ([]domain.ToolDefinition, error) {
	raw, err := p.rpc(context.Background(), "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	var out struct {
		Tools []domain.ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return out.Tools, nil
}

func (p *subprocessProxy) Call(ctx context.Context, call domain.ToolCall) (any, error) {
	if p.level == domain.LevelMax && call.Name != MetaGetSchema && call.Name != MetaInvoke {
		return nil, fmt.Errorf("at max compression only meta-tools are callable, got %q", call.Name)
	}
	params := map[string]any{"name": call.Name, "arguments": call.Arguments}
	raw, err := p.rpc(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", call.Name, err)
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/call result: %w", err)
	}
	return result, nil
}

func (p *subprocessProxy) Close() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.cmd.Wait()
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
		if !p.scanner.Scan() {
			done <- p.scanner.Err()
			return
		}
		done <- json.Unmarshal(p.scanner.Bytes(), &resp)
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
