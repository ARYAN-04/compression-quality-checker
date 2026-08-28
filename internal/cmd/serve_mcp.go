package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"

	"github.com/spf13/cobra"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/suite"
)

var serveMcpFlags struct {
	suitePath string
}

var serveMcpCmd = &cobra.Command{
	Use:    "serve-mcp",
	Short:  "Run stdio JSON-RPC 2.0 MCP server for task suite tools",
	Hidden: true,
	RunE:   runServeMcp,
}

func init() {
	serveMcpCmd.Flags().StringVar(&serveMcpFlags.suitePath, "suite", defaultSuitePath, "task suite JSON path")
}

func runServeMcp(cmd *cobra.Command, _ []string) error {
	tasks, err := loadSuiteOrBuiltin(serveMcpFlags.suitePath, cmd.ErrOrStderr())
	if err != nil {
		return fmt.Errorf("load suite: %w", err)
	}
	tools := suite.UniqueTools(tasks)
	threadFields := suite.ThreadedFields(tasks)

	return serveStdio(cmd.InOrStdin(), cmd.OutOrStdout(), tools, threadFields)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func serveStdio(in io.Reader, out io.Writer, tools []domain.ToolDefinition, threadFields map[string][]string) error {
	scanner := bufio.NewScanner(in)
	callCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &rpcError{
					Code:    -32700,
					Message: "parse error",
				},
			}
			if writeErr := writeRPCResponse(out, resp); writeErr != nil {
				return writeErr
			}
			continue
		}

		if req.Method == "" && req.ID == nil {
			continue
		}

		switch req.Method {
		case "initialize":
			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "can-i-compress-it-suite",
						"version": "0.4.0",
					},
				},
			}
			if err := writeRPCResponse(out, resp); err != nil {
				return err
			}
		case "notifications/initialized":
			// Notification acknowledged (no response required).
		case "ping":
			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]any{},
			}
			if err := writeRPCResponse(out, resp); err != nil {
				return err
			}
		case "tools/list":
			mcpTools := make([]map[string]any, 0, len(tools))
			for _, t := range tools {
				schema := t.Parameters
				if schema == nil {
					schema = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				mcpTools = append(mcpTools, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": schema,
					"parameters":  schema,
				})
			}
			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"tools": mcpTools,
				},
			}
			if err := writeRPCResponse(out, resp); err != nil {
				return err
			}
		case "tools/call":
			var callParams struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params, &callParams)
			}
			callCount++
			res := synthesizeCallResult(callParams.Name, callCount, threadFields)
			jsonBytes, err := json.Marshal(res)
			if err != nil {
				resp := rpcResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &rpcError{
						Code:    -32603,
						Message: fmt.Sprintf("marshal synthetic result: %v", err),
					},
				}
				if writeErr := writeRPCResponse(out, resp); writeErr != nil {
					return writeErr
				}
				continue
			}

			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{
							"type": "text",
							"text": string(jsonBytes),
						},
					},
				},
			}
			if err := writeRPCResponse(out, resp); err != nil {
				return err
			}
		default:
			if req.ID != nil {
				resp := rpcResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &rpcError{
						Code:    -32601,
						Message: fmt.Sprintf("method %q not found", req.Method),
					},
				}
				if err := writeRPCResponse(out, resp); err != nil {
					return err
				}
			}
		}
	}
	return scanner.Err()
}

func synthesizeCallResult(name string, callIndex int, threadFields map[string][]string) map[string]any {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	res := map[string]any{
		"ok":        true,
		"tool":      name,
		"result_id": fmt.Sprintf("res-%d-%08x", callIndex, h.Sum32()),
	}
	for _, field := range threadFields[name] {
		h.Reset()
		_, _ = h.Write([]byte(name + "." + field))
		res[field] = fmt.Sprintf("%s-%d-%08x", field, callIndex, h.Sum32())
	}
	return res
}

func writeRPCResponse(out io.Writer, resp rpcResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = out.Write(append(b, '\n'))
	return err
}
