package suite

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// Options parameterize synthetic suite generation.
type Options struct {
	NumTasks    int
	Ambiguity   string // "none", "low", "high"
	PromptStyle string // "explicit", "intent"
	Distractors int    // filler tools padded into every catalogue; 0 disables
	Seed        int64
}

// Prompt styles supported by the generator.
const (
	StyleExplicit = "explicit"
	StyleIntent   = "intent"
)

// Generator synthesizes task suites with controlled ambiguity.
type Generator struct {
	opts Options
	rng  *rand.Rand
}

// NewGenerator returns a deterministic suite generator for the given seed.
func NewGenerator(opts Options) *Generator {
	if opts.PromptStyle == "" {
		opts.PromptStyle = StyleExplicit
	}
	return &Generator{opts: opts, rng: rand.New(rand.NewSource(opts.Seed))}
}

// name pools used to engineer tool-name ambiguity.
var (
	userVerbs    = []string{"query", "lookup", "fetch", "get", "find", "search"}
	userNouns    = []string{"user_by_id", "account_record", "user_profile", "person_entry", "member_details"}
	sharedParams = []string{"id", "filter", "query", "limit"}
)

// filler pools used to pad catalogues with plausible background tools.
var (
	padVerbs = []string{"list", "create", "update", "delete", "archive",
		"restore", "export", "import", "validate", "sync"}
	padNouns = []string{"attachment", "comment", "label", "webhook", "token",
		"secret", "policy", "rule", "alert", "metric", "dashboard", "widget",
		"invoice", "order", "customer", "product", "shipment", "category",
		"collection", "channel", "invite", "bookmark", "snippet", "draft",
		"queue", "topic", "subscription", "credential", "endpoint", "quota"}
)

// Generate produces numTasks tasks across the three failure-mode categories.
func (g *Generator) Generate() []domain.Task {
	tasks := g.buildTasks()
	if g.opts.Distractors > 0 {
		g.padCatalogues(tasks)
	}
	return tasks
}

func (g *Generator) buildTasks() []domain.Task {
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

// padCatalogues appends Distractors unique filler tools to every task so
// models must discriminate against a realistically large tool surface.
func (g *Generator) padCatalogues(tasks []domain.Task) {
	reserved := map[string]bool{}
	for _, t := range tasks {
		for _, tl := range t.Tools {
			reserved[tl.Name] = true
		}
	}
	pool := g.padPool(reserved, g.opts.Distractors)
	for i := range tasks {
		tasks[i].Tools = append(tasks[i].Tools, pool...)
	}
}

// padPool builds deterministic filler tools, skipping names already taken.
func (g *Generator) padPool(reserved map[string]bool, n int) []domain.ToolDefinition {
	pool := make([]domain.ToolDefinition, 0, n)
	for _, verb := range padVerbs {
		for _, noun := range padNouns {
			if len(pool) == n {
				return pool
			}
			name := verb + "_" + noun
			if reserved[name] {
				continue
			}
			reserved[name] = true
			t := tool(name, fmt.Sprintf("Manages %s resources in the workspace.", noun))
			t.Parameters = params(map[string]any{"id": obj("string", "Resource identifier.")})
			pool = append(pool, t)
		}
	}
	return pool
}

func (g *Generator) pick(xs []string) string { return xs[g.rng.Intn(len(xs))] }

// ambiguousNameTask builds near-synonym tools where only one matches the prompt.
func (g *Generator) ambiguousNameTask(id int) domain.Task {
	target := g.pick(userVerbs) + "_" + g.pick(userNouns)
	distractor := target
	for distractor == target {
		distractor = g.pick(userVerbs) + "_" + g.pick(userNouns)
	}
	withID := func(name, desc string) domain.ToolDefinition {
		t := tool(name, desc)
		t.Parameters = params(map[string]any{"id": obj("integer", "Numeric user identifier.")})
		return t
	}
	catalogue := []domain.ToolDefinition{
		withID(target, "Retrieves a user record by exact numeric identifier."),
		withID(distractor, "Searches account entries using fuzzy text matching."),
	}
	if g.opts.Ambiguity == "high" {
		catalogue = append(catalogue,
			withID("fetch_user_profile", "Loads the full profile document for a user."),
			withID("find_person_entry", "Locates a person entry in the directory index."),
		)
	}
	prompt := fmt.Sprintf("Call %s exactly once. Arguments: id: 42.", target)
	if g.opts.PromptStyle == StyleIntent {
		prompt = fmt.Sprintf("Please use the %s tool to retrieve the user record tied to numeric identifier 42 for our compliance audit.",
			strings.ReplaceAll(target, "_", " "))
	}
	return domain.Task{
		ID:       fmt.Sprintf("amb_%03d", id),
		Category: domain.CategoryAmbiguousName,
		Prompt:   prompt,
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
	prompt := "Call update_issue_status exactly once. Arguments: id: ISS-104. Pass no optional parameters."
	if g.opts.PromptStyle == StyleIntent {
		prompt = "We need the status field updated on issue ISS-104, please use the update issue status tool to take care of it."
	}
	return domain.Task{
		ID:       fmt.Sprintf("ovl_%03d", id),
		Category: domain.CategoryOverlappingSchem,
		Prompt:   prompt,
		Tools:    []domain.ToolDefinition{a, b},
		Expected: expected("update_issue_status", map[string]any{"id": "ISS-104"}),
	}
}

// multiHopTask builds a two-step chain threading an intermediate value.
func (g *Generator) multiHopTask(id int) domain.Task {
	s1 := tool("create_session", "Creates a session and returns its session_id.")
	s1.Parameters = params(map[string]any{"workspace": obj("string", "Workspace name for the new session.")})
	s2 := tool("publish_report", "Publishes a report into an existing session.")
	s2.Parameters = params(map[string]any{
		"report":     obj("string", "Report identifier to publish."),
		"session_id": obj("string", "Target session receiving the report."),
	})
	prompt := "Step 1: call create_session. Arguments: workspace: alpha. Step 2: call publish_report. Arguments: report: q3, session_id: <from:create_session.result_id>."
	if g.opts.PromptStyle == StyleIntent {
		prompt = "First, create a new session in workspace 'alpha' with the create session tool and give me its session id. Then use the publish report tool to publish the quarterly report 'q3' into that session."
	}
	return domain.Task{
		ID:       fmt.Sprintf("hop_%03d", id),
		Category: domain.CategoryMultiHop,
		Prompt:   prompt,
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
