package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/mcp"
)

// Mock is a deterministic offline model used to exercise the harness
// without a live endpoint. It emulates the documented small-model
// behaviour: selection relies on name/description matching, so heavier
// compression degrades it, and max compression forces the two-step
// meta-tool flow.
type Mock struct {
	model string
}

// NewMock returns a deterministic mock model.
func NewMock(model string) *Mock { return &Mock{model: model} }

func (m *Mock) Name() string { return m.model }

// argRe captures "key: value" pairs where a value is either a threaded
// placeholder (<from:tool.field>) or a bare token ending at punctuation
// (comma, semicolon, newline, or sentence period). Interior dots are
// consumed when followed by more token characters, so values like
// /srv/README.md or v1.2 survive extraction.
var argRe = regexp.MustCompile(`\b([a-z_]+):\s*(<from:[^>]*>|(?:[^,;\n.]|\.[\w/@*])*)`)

// Chat implements Model with deterministic behaviour. Explicit-style
// prompts (containing snake_case tool names) follow the original path;
// natural-language intent prompts are served by a schema-driven path.
func (m *Mock) Chat(_ context.Context, msgs []Message, tools []domain.ToolDefinition) (Response, error) {
	prompt := lastUserPrompt(msgs)
	if isMetaCatalogue(tools) {
		return m.metaTurn(msgs, prompt, tools)
	}
	if snakeRe.MatchString(prompt) {
		return m.explicitTurn(msgs, prompt, tools)
	}
	return m.intentTurn(msgs, prompt, tools)
}

// explicitTurn handles "Call <tool> exactly once. Arguments: ..." prompts.
func (m *Mock) explicitTurn(msgs []Message, prompt string, tools []domain.ToolDefinition) (Response, error) {
	best, bestScore := "", -1.0
	for _, t := range tools {
		if score := m.matchScore(prompt, t); score > bestScore {
			best, bestScore = t.Name, score
		}
	}
	if len(tools) > 0 && bestScore <= 0 {
		best = tools[0].Name // forced wrong pick when nothing matches
	}
	if named := nextMentionedTool(prompt, invokedTools(msgs)); named != "" {
		for _, t := range tools {
			if t.Name == named {
				best = named // chains are followed in prompt order
				break
			}
		}
	}
	segment := promptSegmentForTool(prompt, best)
	return Response{
		ToolCalls:  []domain.ToolCall{{Name: best, Arguments: m.resolveThreaded(m.extractArgs(segment), msgs)}},
		TokensUsed: 40 + len(tools)*12,
	}, nil
}

// quotedRe captures 'quoted' or "quoted" literal values in intent prompts.
var quotedRe = regexp.MustCompile(`['"]([A-Za-z0-9][A-Za-z0-9 _.-]*?)['"]`)

// dashedRe captures uppercase dashed identifiers such as ISS-104.
var dashedRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)+\b`)

// pathRe captures absolute file system paths like /var/log/nginx.
var pathRe = regexp.MustCompile(`/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*`)

// urlRe captures web URLs like https://api.stripe.com.
var urlRe = regexp.MustCompile(`https?://[^\s'"]+`)

// intTokenRe captures bare integer values.
var intTokenRe = regexp.MustCompile(`\b\d+\b`)

// mentionRe captures descriptive tool references like "the create session tool".
var mentionRe = regexp.MustCompile(`(?:the|this) ([a-z][a-z ]*?) tool`)

// wordRe extracts lowercase words for stem-based description matching.
var wordRe = regexp.MustCompile(`[a-z]+`)

// intentTurn handles natural-language prompts: tools are selected by
// description overlap and arguments are bound against the selected
// tool's required parameter types.
func (m *Mock) intentTurn(msgs []Message, prompt string, tools []domain.ToolDefinition) (Response, error) {
	called := calledNames(msgs)
	scope := prompt
	if len(called) == 0 {
		scope = firstSentence(prompt) // chains start at the first step
	}
	best, bestScore := "", -1.0
	for _, t := range tools {
		if called[t.Name] {
			continue
		}
		if score := intentScore(scope, t) - jitter(m.model, t.Name); score > bestScore {
			best, bestScore = t.Name, score
		}
	}
	if best == "" || (bestScore <= 0 && len(tools) > 0) {
		best = firstUncalled(tools, called) // forced wrong pick when nothing matches
	}
	// Descriptive "the create session tool" references are the intent
	// counterpart of explicit name mentions: they drive step order.
	if named := metaTarget(prompt, msgs); named != "" && !called[named] {
		for _, t := range tools {
			if t.Name == named {
				best = named
				break
			}
		}
	}
	var args map[string]any
	for _, t := range tools {
		if t.Name == best {
			args = m.bindIntentArgs(prompt, t, msgs)
			break
		}
	}
	return Response{
		ToolCalls:  []domain.ToolCall{{Name: best, Arguments: args}},
		TokensUsed: 40 + len(tools)*12,
	}, nil
}

// bindIntentArgs fills required parameters from literal candidates found
// in the prompt (quoted strings, dashed identifiers, integers), then
// threads unbound parameters from prior tool results by field suffix.
func (m *Mock) bindIntentArgs(prompt string, t domain.ToolDefinition, msgs []Message) map[string]any {
	props := propertiesOf(t.Parameters)
	out := map[string]any{}
	used := usedValues(msgs)
	stringCandidates := quotedValues(prompt)
	for _, tok := range dashedRe.FindAllString(prompt, -1) {
		stringCandidates = append(stringCandidates, tok)
	}
	for _, tok := range pathRe.FindAllString(prompt, -1) {
		stringCandidates = append(stringCandidates, tok)
	}
	for _, tok := range urlRe.FindAllString(prompt, -1) {
		stringCandidates = append(stringCandidates, tok)
	}
	intCandidates := intTokens(stripDashed(prompt))
	for _, key := range requiredOf(t.Parameters) {
		typ := propType(props, key)
		switch typ {
		case "integer", "number":
			for _, n := range intCandidates {
				if !used[keyOf(n)] {
					out[key] = n
					used[keyOf(n)] = true
					break
				}
			}
		case "string", "":
			for _, s := range stringCandidates {
				if !used[keyOf(s)] {
					out[key] = s
					used[keyOf(s)] = true
					break
				}
			}
		}
		if _, ok := out[key]; !ok {
			if v, bound := threadFromResults(msgs, key); bound {
				out[key] = v
			}
		}
	}
	return out
}

// metaTurn performs the two-step get_tool_schema then invoke_tool flow,
// degrading exactly like small models do: some fraction of tasks skip
// the schema fetch and lose the ability to thread intermediate values.
func (m *Mock) metaTurn(msgs []Message, prompt string, tools []domain.ToolDefinition) (Response, error) {
	getSchemaTool, invokeTool := findMetaTools(tools)
	target := metaTarget(prompt, msgs)
	if !assistantCalledMeta(msgs, "get_tool_schema") {
		if m.skipSchemaFetch(prompt) {
			args := m.metaInvokeArgs(prompt, target, msgs)
			return Response{
				ToolCalls: []domain.ToolCall{{Name: invokeTool, Arguments: map[string]any{
					"tool_name": target, "arguments": args, "tool_input": args,
				}}},
				TokensUsed: 45,
			}, nil
		}
		return Response{
			ToolCalls:  []domain.ToolCall{{Name: getSchemaTool, Arguments: map[string]any{"tool_name": target}}},
			TokensUsed: 30,
		}, nil
	}
	args := m.metaInvokeArgs(prompt, target, msgs)
	return Response{
		ToolCalls: []domain.ToolCall{{Name: invokeTool, Arguments: map[string]any{
			"tool_name": target, "arguments": args, "tool_input": args,
		}}},
		TokensUsed: 60,
	}, nil
}

func findMetaTools(tools []domain.ToolDefinition) (getSchema, invoke string) {
	getSchema = mcp.MetaGetSchema
	invoke = mcp.MetaInvoke
	for _, t := range tools {
		if strings.Contains(t.Name, "get_tool_schema") {
			getSchema = t.Name
		}
		if strings.Contains(t.Name, "invoke_tool") {
			invoke = t.Name
		}
	}
	return getSchema, invoke
}

// metaTarget picks the next tool referenced by the prompt that has not
// been invoked yet: explicit prompts mention snake_case names directly;
// intent prompts use descriptive "the create session tool" phrasing.
// Placeholder references duplicate earlier mentions, so progress is
// tracked by invocation history rather than a positional index.
func metaTarget(prompt string, msgs []Message) string {
	invoked := invokedTools(msgs)
	done := map[string]bool{}
	for _, name := range invoked {
		done[name] = true
	}
	next := func(names []string) string {
		for _, m := range names {
			if !done[m] {
				return m
			}
		}
		return ""
	}
	if target := next(explicitToolMentions(prompt)); target != "" {
		return target
	}
	var descriptive []string
	for _, match := range mentionRe.FindAllStringSubmatch(prompt, -1) {
		descriptive = append(descriptive, strings.Join(strings.Fields(match[1]), "_"))
	}
	if target := next(descriptive); target != "" {
		return target
	}
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "Available tools in catalogue: [") {
			start := strings.Index(msg.Content, "Available tools in catalogue: [") + len("Available tools in catalogue: [")
			end := strings.Index(msg.Content[start:], "]")
			if end >= 0 {
				raw := msg.Content[start : start+end]
				tools := strings.Split(raw, ",")
				var cleaned []string
				for _, t := range tools {
					cleaned = append(cleaned, strings.TrimSpace(t))
				}
				if target := next(cleaned); target != "" {
					return target
				}
			}
		}
	}
	if len(invoked) > 0 {
		return invoked[len(invoked)-1] // single-step prompts keep their tool
	}
	return ""
}

// metaInvokeArgs builds invoke_tool arguments. Explicit prompts carry
// their own key/value pairs; intent prompts need the fetched schema to
// bind values — which is exactly what skipped lookups lose.
func (m *Mock) metaInvokeArgs(prompt, target string, msgs []Message) map[string]any {
	if snakeRe.MatchString(prompt) {
		segment := promptSegmentForTool(prompt, target)
		return m.resolveThreaded(m.extractArgs(segment), msgs)
	}
	schemaParams := fetchedSchema(msgs, target)
	fake := domain.ToolDefinition{Name: target, Parameters: schemaParams}
	return m.bindIntentArgs(prompt, fake, msgs)
}

var stepRe = regexp.MustCompile(`(?i)(?:^|\bStep\s+\d+:)\s*`)

// promptSegmentForTool extracts the step substring relevant to tool,
// preventing multi-step prompts with shared argument names from
// overwriting earlier step parameters.
func promptSegmentForTool(prompt, tool string) string {
	if tool == "" {
		return prompt
	}
	steps := stepRe.Split(prompt, -1)
	if len(steps) <= 1 {
		return prompt
	}
	targetCall := "call " + strings.ToLower(tool)
	for _, step := range steps {
		if strings.Contains(strings.ToLower(step), targetCall) {
			return step
		}
	}
	for _, step := range steps {
		if strings.Contains(strings.ToLower(step), strings.ToLower(tool)) {
			return step
		}
	}
	return prompt
}

// skipSchemaFetch deterministically degrades some fraction of tasks per
// model, emulating the two-step lookup failure mode of weak instruction
// followers.
func (m *Mock) skipSchemaFetch(prompt string) bool {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s|%s", m.model, prompt)
	return h.Sum32()%10 < 4
}

func isMetaCatalogue(tools []domain.ToolDefinition) bool {
	for _, t := range tools {
		if strings.Contains(t.Name, "get_tool_schema") || strings.Contains(t.Name, "invoke_tool") {
			return true
		}
	}
	return false
}

// matchScore scores prompt overlap with a tool's visible representation.
func (m *Mock) matchScore(prompt string, t domain.ToolDefinition) float64 {
	p := strings.ToLower(prompt)
	score := 0.0
	if strings.Contains(p, strings.ToLower(t.Name)) {
		score += 2.0
	}
	for _, word := range strings.Fields(strings.ToLower(t.Description)) {
		if len(word) > 3 && strings.Contains(p, word) {
			score += 0.1
		}
	}
	return score - jitter(m.model, t.Name)
}

// intentScore scores stem-level overlap between the prompt and a tool's
// description, so natural-language requests can find their tool.
func intentScore(prompt string, t domain.ToolDefinition) float64 {
	stems := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(prompt), -1) {
		stems[stem(w)] = true
	}
	score := 0.0
	for _, word := range strings.Fields(strings.ToLower(t.Description)) {
		word = strings.Trim(word, ".,;:!?")
		if len(word) > 3 && stems[stem(word)] {
			score += 0.1
		}
	}
	return score
}

// stem reduces a word to a coarse form so inflections compare equal.
func stem(w string) string {
	for {
		trimmed := false
		for _, suf := range []string{"ing", "ies", "ed", "es", "s", "e"} {
			if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 4 {
				w = w[:len(w)-len(suf)]
				trimmed = true
				break
			}
		}
		if !trimmed {
			return w
		}
	}
}

// jitter applies a deterministic per-model/per-tool penalty so scoring
// ties break reproducibly.
func jitter(model, tool string) float64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s/%s", model, tool)
	return float64(h.Sum32()%100) / 1000.0
}

// extractArgs parses "key: value" pairs from the prompt text.
func (m *Mock) extractArgs(prompt string) map[string]any {
	out := map[string]any{}
	for _, match := range argRe.FindAllStringSubmatch(prompt, -1) {
		key := strings.ToLower(match[1])
		raw := strings.TrimSpace(match[2])
		if raw == "" {
			continue
		}
		var v any = raw
		if n, err := parseInt64(raw); err == nil {
			v = n
		}
		out[key] = v
	}
	return out
}

// resolveThreaded replaces "<from:tool.field>" placeholders using prior
// tool result messages in the conversation.
func (m *Mock) resolveThreaded(args map[string]any, msgs []Message) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = resolveMockValue(v, msgs)
	}
	return out
}

func resolveMockValue(v any, msgs []Message) any {
	switch val := v.(type) {
	case string:
		if !strings.HasPrefix(val, "<from:") {
			return val
		}
		path := strings.TrimSuffix(strings.TrimPrefix(val, "<from:"), ">")
		tool, field, ok := strings.Cut(path, ".")
		if !ok {
			return val
		}
		return lookupInResults(msgs, tool, field)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = resolveMockValue(item, msgs)
		}
		return out
	default:
		return v
	}
}

func lookupInResults(msgs []Message, tool, field string) any {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(msgs[i].Content), &payload); err != nil {
			continue
		}
		if payload["tool"] == tool {
			if v, ok := payload[field]; ok {
				return v
			}
		}
	}
	return nil
}

// fetchedSchema returns the parameters payload returned by an earlier
// get_tool_schema call for target, or nil when no schema was fetched.
func fetchedSchema(msgs []Message, target string) map[string]any {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(msgs[i].Content), &payload); err != nil {
			continue
		}
		if name, _ := payload["name"].(string); name == target {
			params, _ := payload["parameters"].(map[string]any)
			return params
		}
	}
	return nil
}

// threadFromResults binds an unbound parameter to a prior result field
// sharing its trailing segment (session_id <-> result_id).
func threadFromResults(msgs []Message, param string) (any, bool) {
	want := lastSegment(param)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(msgs[i].Content), &payload); err != nil {
			continue
		}
		keys := make([]string, 0, len(payload))
		for k := range payload {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "ok" || k == "tool" {
				continue
			}
			if lastSegment(k) == want {
				return payload[k], true
			}
		}
	}
	return nil, false
}

func lastSegment(s string) string {
	if i := strings.LastIndexByte(s, '_'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// usedValues collects literals already sent in earlier assistant calls
// so chained turns do not re-bind consumed candidates.
func usedValues(msgs []Message) map[string]bool {
	used := map[string]bool{}
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			var args any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				continue
			}
			collectLiterals(args, used)
		}
	}
	return used
}

func collectLiterals(v any, used map[string]bool) {
	switch t := v.(type) {
	case string:
		used[t] = true
	case float64:
		used[keyOf(int64(t))] = true
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectLiterals(t[k], used)
		}
	case []any:
		for _, item := range t {
			collectLiterals(item, used)
		}
	}
}

func keyOf[T ~int64 | ~string](v T) string { return fmt.Sprint(v) }

func quotedValues(prompt string) []string {
	var out []string
	for _, m := range quotedRe.FindAllStringSubmatch(prompt, -1) {
		out = append(out, m[1])
	}
	return out
}

func intTokens(prompt string) []int64 {
	var out []int64
	for _, tok := range intTokenRe.FindAllString(prompt, -1) {
		if n, err := parseInt64(tok); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// stripDashed removes dashed identifiers so their numeric segments are
// not mistaken for standalone integer arguments.
func stripDashed(prompt string) string {
	return dashedRe.ReplaceAllString(prompt, "")
}

func propertiesOf(parameters map[string]any) map[string]any {
	props, _ := parameters["properties"].(map[string]any)
	return props
}

func requiredOf(parameters map[string]any) []string {
	out := []string{}
	switch raw := parameters["required"].(type) {
	case []string:
		out = append(out, raw...)
	case []any:
		for _, r := range raw {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

func propType(props map[string]any, key string) string {
	prop, _ := props[key].(map[string]any)
	typ, _ := prop["type"].(string)
	return typ
}

func calledNames(msgs []Message) map[string]bool {
	called := map[string]bool{}
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			called[tc.Function.Name] = true
		}
	}
	return called
}

func firstUncalled(tools []domain.ToolDefinition, called map[string]bool) string {
	for _, t := range tools {
		if !called[t.Name] {
			return t.Name
		}
	}
	if len(tools) > 0 {
		return tools[0].Name
	}
	return ""
}

// firstSentence returns the text up to the first sentence boundary so
// multi-step intent prompts score their initial step first.
func firstSentence(prompt string) string {
	if i := strings.Index(prompt, ". "); i >= 0 {
		return prompt[:i+1]
	}
	return prompt
}

func assistantCalled(msgs []Message, tool string) bool {
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == tool || strings.Contains(tc.Function.Name, tool) {
				return true
			}
		}
	}
	return false
}

func assistantCalledMeta(msgs []Message, substr string) bool {
	return assistantCalled(msgs, substr)
}

func countAssistantCalls(msgs []Message) int {
	n := 0
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			n += len(msg.ToolCalls)
		}
	}
	return n
}

// invokedTools lists underlying tool names already executed, in order:
// direct calls at normal levels, invoke_tool targets at max. Schema
// fetches are protocol overhead and do not advance the chain.
func invokedTools(msgs []Message) []string {
	var out []string
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if strings.Contains(tc.Function.Name, "get_tool_schema") {
				continue
			}
			if strings.Contains(tc.Function.Name, "invoke_tool") {
				var args struct {
					ToolName string `json:"tool_name"`
				}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil && args.ToolName != "" {
					out = append(out, args.ToolName)
				}
			} else {
				out = append(out, tc.Function.Name)
			}
		}
	}
	return out
}

var callRe = regexp.MustCompile(`(?i)\bcall\s+([a-z]+(?:_[a-z]+)*)`)

// explicitToolMentions extracts tool names referenced via "call <tool>"
// phrases, falling back to snake_case tokens if no call prefix is found.
func explicitToolMentions(prompt string) []string {
	var out []string
	for _, match := range callRe.FindAllStringSubmatch(prompt, -1) {
		out = append(out, strings.ToLower(match[1]))
	}
	if len(out) == 0 {
		out = snakeRe.FindAllString(prompt, -1)
	}
	return out
}

// nextMentionedTool returns the first prompt-mentioned tool that has not
// been invoked yet, so chains advance step by step even when placeholder
// references duplicate earlier mentions.
func nextMentionedTool(prompt string, invoked []string) string {
	done := map[string]bool{}
	for _, name := range invoked {
		done[name] = true
	}
	for _, m := range explicitToolMentions(prompt) {
		if !done[m] {
			return m
		}
	}
	return ""
}

var snakeRe = regexp.MustCompile(`[a-z]+(?:_[a-z]+)+`)

func lastUserPrompt(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

// parseInt64 accepts only strings that are entirely an integer, so
// values like Slack's "1756000000.000100" stay strings.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
