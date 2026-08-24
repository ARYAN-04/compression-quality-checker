package suite

import (
	"fmt"
	"math/rand"
	"sort"

	"can-i-compress-it/internal/domain"
)

// Options parameterize synthetic suite generation.
type Options struct {
	NumTasks  int
	Ambiguity string // "none", "low", "high"
	Seed      int64
}

// Generator synthesizes task suites with controlled ambiguity.
type Generator struct {
	opts Options
	rng  *rand.Rand
}

// NewGenerator returns a deterministic suite generator for the given seed.
func NewGenerator(opts Options) *Generator {
	return &Generator{opts: opts, rng: rand.New(rand.NewSource(opts.Seed))}
}

// name pools used to engineer tool-name ambiguity.
var (
	userVerbs    = []string{"query", "lookup", "fetch", "get", "find", "search"}
	userNouns    = []string{"user_by_id", "account_record", "user_profile", "person_entry", "member_details"}
	sharedParams = []string{"id", "filter", "query", "limit"}
)

// Generate produces numTasks tasks across the three failure-mode categories.
func (g *Generator) Generate() []domain.Task {
	var tasks []domain.Task
	id := 0
	for i := 0; i < g.opts.NumTasks; i++ {
		switch i % 3 {
		case 0:
			tasks = append(tasks, g.ambiguousNameTask(id))
		case 1:
			tasks = append(tasks, g.overlappingSchemaTask(id))
		default:
			tasks = append(tasks, g.multiHopTask(id))
		}
		id++
	}
	return tasks
}

func (g *Generator) pick(xs []string) string { return xs[g.rng.Intn(len(xs))] }

// ambiguousNameTask builds near-synonym tools where only one matches the prompt.
func (g *Generator) ambiguousNameTask(id int) domain.Task {
	target := g.pick(userVerbs) + "_" + g.pick(userNouns)
	distractor := target
	for distractor == target {
		distractor = g.pick(userVerbs) + "_" + g.pick(userNouns)
	}
	catalogue := []domain.ToolDefinition{
		tool(target, "Retrieves a user record by exact numeric identifier."),
		tool(distractor, "Searches account entries using fuzzy text matching."),
	}
	if g.opts.Ambiguity == "high" {
		catalogue = append(catalogue,
			tool("fetch_user_profile", "Loads the full profile document for a user."),
			tool("find_person_entry", "Locates a person entry in the directory index."),
		)
	}
	return domain.Task{
		ID:       fmt.Sprintf("amb_%03d", id),
		Category: domain.CategoryAmbiguousName,
		Prompt:   fmt.Sprintf("Call %s exactly once. Arguments: id: 42.", target),
		Tools:    catalogue,
		Expected: expected(target, map[string]any{"id": int64(42)}),
	}
}

// overlappingSchemaTask builds tools sharing parameter names but not types.
func (g *Generator) overlappingSchemaTask(id int) domain.Task {
	a := tool("update_issue_status", "Updates the status field of an issue.")
	a.Parameters = params(map[string]any{
		"id":     obj("string", "Issue identifier, e.g. ISS-104."),
		"filter": obj("object", "Structured status transition filter."),
		"limit":  obj("integer", "Maximum number of audit rows to return."),
	})
	b := tool("query_audit_log", "Queries the audit log for issue events.")
	b.Parameters = params(map[string]any{
		"id":     obj("integer", "Numeric log entry id."),
		"filter": obj("string", "Free-text log filter expression."),
		"limit":  obj("integer", "Row limit."),
	})
	return domain.Task{
		ID:       fmt.Sprintf("ovl_%03d", id),
		Category: domain.CategoryOverlappingSchem,
		Prompt:   "Call update_issue_status exactly once. Arguments: id: ISS-104. Pass no optional parameters.",
		Tools:    []domain.ToolDefinition{a, b},
		Expected: expected("update_issue_status", map[string]any{"id": "ISS-104"}),
	}
}

// multiHopTask builds a two-step chain threading an intermediate value.
func (g *Generator) multiHopTask(id int) domain.Task {
	s1 := tool("create_session", "Creates a session and returns its session_id.")
	s2 := tool("publish_report", "Publishes a report into an existing session.")
	return domain.Task{
		ID:       fmt.Sprintf("hop_%03d", id),
		Category: domain.CategoryMultiHop,
		Prompt:   "Step 1: call create_session. Arguments: workspace: alpha. Step 2: call publish_report. Arguments: report: q3, session_id: <from:create_session.result_id>.",
		Tools:    []domain.ToolDefinition{s1, s2},
		Expected: expected("create_session", map[string]any{"workspace": "alpha"}),
		MultiHop: []domain.ExpectedCall{
			expected("create_session", map[string]any{"workspace": "alpha"}),
			expected("publish_report", map[string]any{"session_id": "<from:create_session.result_id>", "report": "q3"}),
		},
	}
}

func tool(name, desc string) domain.ToolDefinition {
	return domain.ToolDefinition{Name: name, Description: desc, Parameters: params(nil)}
}

func params(m map[string]any) map[string]any {
	if m == nil {
		m = map[string]any{}
	}
	out := map[string]any{
		"type":       "object",
		"properties": m,
		"required":   sortedKeys(m),
	}
	return out
}

func obj(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func expected(name string, args map[string]any) domain.ExpectedCall {
	return domain.ExpectedCall{ToolName: name, Arguments: args}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
