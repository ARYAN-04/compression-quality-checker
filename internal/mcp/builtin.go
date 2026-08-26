package mcp

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// Meta-tool names exposed at max compression, per mcp-compressor semantics.
const (
	MetaGetSchema = "get_tool_schema"
	MetaInvoke    = "invoke_tool"
)

type builtinProxy struct {
	level        domain.CompressionLevel
	tools        []domain.ToolDefinition
	schemas      map[string]map[string]any // original full schemas for meta lookup
	calls        []domain.ToolCall         // executed call history (for multihop eval)
	threadFields map[string][]string       // tool -> result fields to synthesize
}

// newBuiltinProxy applies deterministic in-process compression to tools.
func newBuiltinProxy(level domain.CompressionLevel, threadFields map[string][]string, tools []domain.ToolDefinition) Proxy {
	compressed := compressTools(level, tools)
	schemas := make(map[string]map[string]any, len(tools))
	for _, t := range tools {
		schemas[t.Name] = map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		}
	}
	return &builtinProxy{level: level, tools: compressed, schemas: schemas, threadFields: threadFields}
}

func (p *builtinProxy) Tools(context.Context) ([]domain.ToolDefinition, error) {
	return p.tools, nil
}

func (p *builtinProxy) Call(_ context.Context, call domain.ToolCall) (any, error) {
	switch p.level {
	case domain.LevelMax:
		return p.callMeta(call)
	default:
		p.calls = append(p.calls, call)
		return p.synthesizeResult(call.Name), nil
	}
}

// synthesizeResult produces a deterministic success payload including a
// result_id so multi-hop tasks can thread values between steps. Fields
// declared in threadFields are emitted with deterministic values so any
// "<from:tool.field>" chain placeholder resolves offline.
func (p *builtinProxy) synthesizeResult(name string) map[string]any {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	res := map[string]any{
		"ok":        true,
		"tool":      name,
		"result_id": fmt.Sprintf("res-%d-%08x", len(p.calls), h.Sum32()),
	}
	for _, field := range p.threadFields[name] {
		h.Reset()
		_, _ = h.Write([]byte(name + "." + field))
		res[field] = fmt.Sprintf("%s-%d-%08x", field, len(p.calls), h.Sum32())
	}
	return res
}

// callMeta resolves get_tool_schema / invoke_tool wrappers. A direct call to
// an underlying tool is rejected to faithfully model two-step lookup.
func (p *builtinProxy) callMeta(call domain.ToolCall) (any, error) {
	switch call.Name {
	case MetaGetSchema:
		name, _ := call.Arguments["tool_name"].(string)
		schema, ok := p.schemas[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		return schema, nil
	case MetaInvoke:
		name, _ := call.Arguments["tool_name"].(string)
		argsRaw, ok := call.Arguments["arguments"].(map[string]any)
		if !ok {
			argsRaw = map[string]any{}
		}
		inner := domain.ToolCall{Name: name, Arguments: argsRaw}
		p.calls = append(p.calls, inner)
		return p.synthesizeResult(name), nil
	default:
		return nil, fmt.Errorf("at max compression only %s/%s are callable, got %q",
			MetaGetSchema, MetaInvoke, call.Name)
	}
}

func (p *builtinProxy) Close() error { return nil }

// Calls returns executed underlying invocations in order.
func callsOf(p Proxy) []domain.ToolCall {
	bp, ok := p.(*builtinProxy)
	if !ok {
		return nil
	}
	return bp.calls
}

// compressTools transforms a catalogue according to level semantics.
func compressTools(level domain.CompressionLevel, tools []domain.ToolDefinition) []domain.ToolDefinition {
	out := make([]domain.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		ct := domain.ToolDefinition{Name: t.Name, Parameters: cloneParams(t.Parameters)}
		switch level {
		case domain.LevelNone:
			ct.Description = t.Description
		case domain.LevelLow:
			ct.Description = firstSentence(t.Description)
			ct.Parameters = keepFullParams(ct.Parameters)
		case domain.LevelMedium:
			ct.Description = truncate(t.Description, 60)
			ct.Parameters = summarizeOptional(ct.Parameters)
		case domain.LevelHigh:
			ct.Description = ""
			ct.Parameters = stripDescriptions(ct.Parameters)
		case domain.LevelMax:
			continue // replaced wholesale below
		}
		out = append(out, ct)
	}
	if level == domain.LevelMax {
		out = append(out, metaTool(MetaGetSchema), metaTool(MetaInvoke))
	}
	return out
}

func metaTool(name string) domain.ToolDefinition {
	t := domain.ToolDefinition{Name: name, Description: "Meta-tool for schema-on-demand access."}
	switch name {
	case MetaGetSchema:
		t.Parameters = params(map[string]any{"tool_name": obj("string", "Tool to inspect.")})
	default:
		t.Parameters = params(map[string]any{
			"tool_name": obj("string", "Tool to invoke."),
			"arguments": obj("object", "Argument payload."),
		})
	}
	return t
}

func params(m map[string]any) map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]any{"type": "object", "properties": m, "required": keys}
}

func obj(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func cloneParams(p map[string]any) map[string]any {
	if p == nil {
		return nil
	}
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

func firstSentence(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i+1]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// keepFullParams retains parameter descriptions but drops whitespace noise.
func keepFullParams(p map[string]any) map[string]any { return cloneParams(p) }

// summarizeOptional drops non-required parameter entries entirely.
func summarizeOptional(p map[string]any) map[string]any {
	props, _ := p["properties"].(map[string]any)
	req := requiredSet(p)
	for k, v := range props {
		if vm, ok := v.(map[string]any); ok {
			vm["description"] = truncate(firstSentence(desc(vm)), 40)
		}
		if !req[k] {
			delete(props, k)
		}
	}
	return p
}

func stripDescriptions(p map[string]any) map[string]any {
	props, _ := p["properties"].(map[string]any)
	for _, v := range props {
		if vm, ok := v.(map[string]any); ok {
			delete(vm, "description")
		}
	}
	return p
}

func desc(vm map[string]any) string {
	d, _ := vm["description"].(string)
	return d
}

func requiredSet(p map[string]any) map[string]bool {
	out := map[string]bool{}
	switch raw := p["required"].(type) {
	case []string:
		for _, r := range raw {
			out[r] = true
		}
	case []any:
		for _, r := range raw {
			if s, ok := r.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}
