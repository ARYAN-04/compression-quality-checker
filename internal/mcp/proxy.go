// Package mcp spawns and controls compressed MCP tool surfaces.
//
// Two backends are supported:
//   - "subprocess": spawns an external mcp-compressor binary speaking
//     JSON-RPC over stdio (the production configuration under study).
//   - "builtin": an in-process deterministic compressor implementing the
//     same level semantics, used when mcp-compressor is unavailable.
package mcp

import (
	"context"
	"fmt"

	"can-i-compress-it/internal/domain"
)

// Proxy exposes a compressed tool catalogue and dispatches calls against it.
type Proxy interface {
	// Tools returns the (possibly compressed) catalogue visible to a model.
	Tools(ctx context.Context) ([]domain.ToolDefinition, error)
	// Call routes one invocation through the compressed surface, handling
	// meta-tool unwrapping at max compression.
	Call(ctx context.Context, call domain.ToolCall) (any, error)
	Close() error
}

// Config selects the backend and compression behaviour.
type Config struct {
	Level        domain.CompressionLevel
	Binary       string   // path to mcp-compressor; empty selects builtin backend
	TargetCmd    []string // MCP server command to wrap in subprocess mode
	TargetServer string   // builtin backend: "suite" serves the task suite tools
}

// NewProxy builds a proxy for cfg at ctx cancellation-safe lifecycle.
func NewProxy(ctx context.Context, cfg Config, suiteTools []domain.ToolDefinition) (Proxy, error) {
	if err := validateLevel(cfg.Level); err != nil {
		return nil, err
	}
	if cfg.Binary == "" {
		return newBuiltinProxy(cfg.Level, suiteTools), nil
	}
	return newSubprocessProxy(ctx, cfg, suiteTools)
}

func validateLevel(l domain.CompressionLevel) error {
	for _, v := range domain.AllLevels {
		if v == l {
			return nil
		}
	}
	return fmt.Errorf("invalid compression level %q", l)
}
