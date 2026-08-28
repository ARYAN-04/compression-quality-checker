package suite

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

//go:embed schemas/*.json
var embeddedSchemas embed.FS

// EmbeddedSchemaNames returns the names of all bundled static MCP tool schemas.
func EmbeddedSchemaNames() []string {
	return []string{
		"github",
		"jira",
		"slack",
		"sqlite",
		"filesystem",
		"postgres",
		"memory",
		"fetch",
		"brave-search",
		"puppeteer",
		"git",
	}
}

// EmbeddedSchema returns the raw JSON bytes for a bundled MCP tool schema.
func EmbeddedSchema(name string) ([]byte, bool) {
	clean := strings.TrimSuffix(name, ".json")
	data, err := embeddedSchemas.ReadFile("schemas/" + clean + ".json")
	if err != nil {
		return nil, false
	}
	return data, true
}

// EmbeddedSchemaTools returns the parsed ToolDefinitions for a bundled MCP tool schema.
func EmbeddedSchemaTools(name string) ([]domain.ToolDefinition, bool) {
	data, ok := EmbeddedSchema(name)
	if !ok {
		return nil, false
	}
	tools, err := ParseSchemaTools(data)
	if err != nil {
		return nil, false
	}
	return tools, true
}

// rawToolJSON handles both MCP inputSchema and legacy parameters formats.
type rawToolJSON struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Parameters  map[string]any `json:"parameters"`
}

func (r rawToolJSON) toToolDefinition() domain.ToolDefinition {
	params := r.InputSchema
	if len(params) == 0 {
		params = r.Parameters
	}
	if params == nil {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []any{},
		}
	}
	return domain.ToolDefinition{
		Name:        r.Name,
		Description: r.Description,
		Parameters:  params,
	}
}

// ParseSchemaTools extracts ToolDefinitions from arbitrary MCP JSON schemas.
// Tolerates {"tools": [...]}, {"result": {"tools": [...]}}, and direct arrays [...].
func ParseSchemaTools(data []byte) ([]domain.ToolDefinition, error) {
	// Try {"tools": [...]}
	var wrapper struct {
		Tools  []rawToolJSON `json:"tools"`
		Result *struct {
			Tools []rawToolJSON `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil {
		rawList := wrapper.Tools
		if len(rawList) == 0 && wrapper.Result != nil {
			rawList = wrapper.Result.Tools
		}
		if len(rawList) > 0 {
			out := make([]domain.ToolDefinition, 0, len(rawList))
			for _, r := range rawList {
				out = append(out, r.toToolDefinition())
			}
			return out, nil
		}
	}

	// Try direct array [...]
	var arrayList []rawToolJSON
	if err := json.Unmarshal(data, &arrayList); err == nil && len(arrayList) > 0 {
		out := make([]domain.ToolDefinition, 0, len(arrayList))
		for _, r := range arrayList {
			out = append(out, r.toToolDefinition())
		}
		return out, nil
	}

	return nil, fmt.Errorf("could not parse tool schemas from JSON data")
}

// FromSchema generates benchmark tasks from any MCP tool catalogue schema.
// Every tool in the schema becomes a benchmark task evaluated against the full catalogue.
func FromSchema(data []byte) ([]domain.Task, error) {
	tools, err := ParseSchemaTools(data)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("schema contains no tools")
	}

	tasks := make([]domain.Task, 0, len(tools))
	for _, t := range tools {
		args := synthesizeSampleArguments(t.Parameters)
		prompt := synthesizePrompt(t.Name, t.Description, args)
		tasks = append(tasks, domain.Task{
			ID:       fmt.Sprintf("imp_%s", SanitizeName(t.Name)),
			Category: domain.CategoryImported,
			Prompt:   prompt,
			Tools:    tools,
			Expected: domain.ExpectedCall{
				ToolName:  t.Name,
				Arguments: args,
			},
		})
	}
	return tasks, nil
}

// FromSchemaFile loads a task suite from a local schema file path or bundled schema name.
func FromSchemaFile(path string) ([]domain.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Check if path matches an embedded schema name
		if embedded, ok := EmbeddedSchema(path); ok {
			return FromSchema(embedded)
		}
		return nil, fmt.Errorf("read schema file %s: %w", path, err)
	}
	return FromSchema(data)
}

// SanitizeName normalizes a string into a safe identifier.
func SanitizeName(name string) string {
	lower := strings.ToLower(name)
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func synthesizeSampleArguments(params map[string]any) map[string]any {
	out := map[string]any{}
	if params == nil {
		return out
	}
	props, _ := params["properties"].(map[string]any)
	if len(props) == 0 {
		return out
	}

	reqMap := map[string]bool{}
	if reqSlice, ok := params["required"].([]any); ok {
		for _, item := range reqSlice {
			if s, ok := item.(string); ok {
				reqMap[s] = true
			}
		}
	} else if reqStrSlice, ok := params["required"].([]string); ok {
		for _, s := range reqStrSlice {
			reqMap[s] = true
		}
	}

	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if len(reqMap) > 0 && !reqMap[k] {
			continue
		}
		propDef, _ := props[k].(map[string]any)
		typ, _ := propDef["type"].(string)
		out[k] = sampleValueForType(k, typ)
	}
	return out
}

func sampleValueForType(name, typ string) any {
	switch typ {
	case "integer":
		if strings.Contains(name, "number") || strings.Contains(name, "id") || strings.Contains(name, "count") {
			return int64(42)
		}
		return int64(10)
	case "number":
		return 3.14
	case "boolean":
		return true
	case "array":
		return []any{"sample_item"}
	case "object":
		return map[string]any{"key": "value"}
	default:
		switch {
		case strings.Contains(name, "owner"):
			return "acme"
		case strings.Contains(name, "repo"):
			return "service-api"
		case strings.Contains(name, "url"):
			return "https://example.com"
		case strings.Contains(name, "path"):
			return "/tmp/workspace/file.txt"
		case strings.Contains(name, "query"):
			return "SELECT 1"
		case strings.Contains(name, "key"):
			return "KEY-101"
		case strings.Contains(name, "channel"):
			return "C0123456789"
		default:
			return "sample_" + name
		}
	}
}

func synthesizePrompt(toolName, desc string, args map[string]any) string {
	if len(args) == 0 {
		return fmt.Sprintf("Call %s exactly once with no arguments.", toolName)
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var argParts []string
	for _, k := range keys {
		argParts = append(argParts, fmt.Sprintf("%s: %v", k, args[k]))
	}
	return fmt.Sprintf("Call %s exactly once. Arguments: %s.", toolName, strings.Join(argParts, ", "))
}
