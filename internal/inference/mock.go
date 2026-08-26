package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
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
// placeholder (<from:tool.field>) or a bare token ending at punctuation.
var argRe = regexp.MustCompile(`\b([a-z_]+):\s*(<from:[^>]*>|[^,;\n.]*)`)

// Chat implements Model with deterministic behaviour. Explicit-style
// prompts (containing snake_case tool names) follow the original path;
// natural-language intent prompts are served by a schema-driven path.
func (m *Mock) Chat(_ context.Context, msgs []Message, tools []domain.ToolDefinition) (Response, error) {
	prompt := lastUserPrompt(msgs)
	if isMetaCatalogue(tools) {
		return m.metaTurn(msgs, prompt)
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
	if named := nthMentionedTool(prompt, countAssistantCalls(msgs)); named != "" {
		for _, t := range tools {
			if t.Name == named {
				best = named // chains are followed in prompt order
				break
			}
		}
	}
	return Response{
		ToolCalls:  []domain.ToolCall{{Name: best, Arguments: m.resolveThreaded(m.extractArgs(prompt), msgs)}},
		TokensUsed: 40 + len(tools)*12,
	}, nil
}

// quotedRe captures 'quoted' or "quoted" literal values in intent prompts.
var quotedRe = regexp.MustCompile(`['"]([A-Za-z0-9][A-Za-z0-9 _.-]*?)['"]`)

// dashedRe captures uppercase dashed identifiers such as ISS-104.
var dashedRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)+\b`)

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
	if named := metaTarget(prompt, countAssistantCalls(msgs)); named != "" && !called[named] {
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
func (m *Mock) metaTurn(msgs []Message, prompt string) (Response, error) {
	target := metaTarget(prompt, countAssistantCalls(msgs))
	if !assistantCalled(msgs, mcp.MetaGetSchema) {
		if m.skipSchemaFetch(prompt) {
			args := m.metaInvokeArgs(prompt, target, msgs)
			return Response{
				ToolCalls: []domain.ToolCall{{Name: mcp.MetaInvoke, Arguments: map[string]any{
					"tool_name": target, "arguments": args,
				}}},
				TokensUsed: 45,
			}, nil
		}
		return Response{
			ToolCalls:  []domain.ToolCall{{Name: mcp.MetaGetSchema, Arguments: map[string]any{"tool_name": target}}},
			TokensUsed: 30,
		}, nil
	}
	args := m.metaInvokeArgs(prompt, target, msgs)
	return Response{
		ToolCalls: []domain.ToolCall{{Name: mcp.MetaInvoke, Arguments: map[string]any{
			"tool_name": target, "arguments": args,
		}}},
		TokensUsed: 60,
	}, nil
}

// metaTarget picks the nth tool referenced by the prompt: explicit
// prompts mention snake_case names directly; intent prompts use
// descriptive "the create session tool" phrasing. Single-step prompts
// keep resolving to their only mention on later turns.
func metaTarget(prompt string, n int) string {
	for _, i := range []int{n, 0} {
		if named := nthMentionedTool(prompt, i); named != "" {
			return named
		}
	}
	mentions := mentionRe.FindAllStringSubmatch(prompt, -1)
	for _, i := range []int{n, 0} {
		if i < len(mentions) {
			return strings.Join(strings.Fields(mentions[i][1]), "_")
		}
	}
	return ""
}

// metaInvokeArgs builds invoke_tool arguments. Explicit prompts carry
// their own key/value pairs; intent prompts need the fetched schema to
// bind values — which is exactly what skipped lookups lose.
func (m *Mock) metaInvokeArgs(prompt, target string, msgs []Message) map[string]any {
	if snakeRe.MatchString(prompt) {
		return m.resolveThreaded(m.extractArgs(prompt), msgs)
	}
	schemaParams := fetchedSchema(msgs, target)
	fake := domain.ToolDefinition{Name: target, Parameters: schemaParams}
	return m.bindIntentArgs(prompt, fake, msgs)
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
		if t.Name == mcp.MetaGetSchema {
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
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, "<from:") {
			out[k] = v
			continue
		}
		path := strings.TrimSuffix(strings.TrimPrefix(s, "<from:"), ">")
		tool, field, ok := strings.Cut(path, ".")
		if !ok {
			out[k] = v
			continue
		}
		out[k] = lookupInResults(msgs, tool, field)
	}
	return out
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
			if tc.Function.Name == tool {
				return true
			}
		}
	}
	return false
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

// nthMentionedTool returns the nth snake_case tool name mentioned in the
// prompt (0-based), so chained calls follow prompt order.
func nthMentionedTool(prompt string, n int) string {
	toks := snakeRe.FindAllString(prompt, -1)
	if n < len(toks) {
		return toks[n]
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

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
