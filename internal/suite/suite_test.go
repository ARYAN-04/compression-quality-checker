package suite

import (
	"regexp"
	"strings"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestGenerateIsDeterministic(t *testing.T) {
	a := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", Seed: 7}).Generate()
	b := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", Seed: 7}).Generate()
	if len(a) != len(b) || len(a) != 9 {
		t.Fatalf("expected 9 tasks each, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Expected.ToolName != b[i].Expected.ToolName {
			t.Fatalf("task %d differs between seeds runs", i)
		}
	}
}

func TestCategoriesCoverAllThree(t *testing.T) {
	tasks := NewGenerator(Options{NumTasks: 30, Ambiguity: "high", Seed: 3}).Generate()
	cats := map[string]bool{}
	for _, task := range tasks {
		cats[task.Category] = true
		switch task.Category {
		case domain.CategoryMultiHop:
			if len(task.MultiHop) < 2 {
				t.Fatalf("multihop task %s needs >=2 hops", task.ID)
			}
		case domain.CategoryAmbiguousName:
			if len(task.Tools) < 2 {
				t.Fatalf("ambiguous task %s needs distractors", task.ID)
			}
		}
	}
	for _, want := range []string{domain.CategoryAmbiguousName, domain.CategoryOverlappingSchem, domain.CategoryMultiHop} {
		if !cats[want] {
			t.Fatalf("missing category %s", want)
		}
	}
}

func TestBuiltinSuiteQuality(t *testing.T) {
	tasks := Builtin()
	if len(tasks) != 30 {
		t.Fatalf("expected 30 tasks in Builtin(), got %d", len(tasks))
	}

	catCounts := map[string]int{}
	taskIDs := map[string]bool{}
	prompts := map[string]bool{}

	for i, task := range tasks {
		if task.ID == "" {
			t.Fatalf("task %d: empty ID", i)
		}
		if taskIDs[task.ID] {
			t.Fatalf("duplicate task ID: %s", task.ID)
		}
		taskIDs[task.ID] = true

		if task.Prompt == "" {
			t.Fatalf("task %s: empty prompt", task.ID)
		}
		if prompts[task.Prompt] {
			t.Fatalf("duplicate prompt in task %s", task.ID)
		}
		prompts[task.Prompt] = true

		catCounts[task.Category]++

		if len(task.Tools) < 2 {
			t.Fatalf("task %s: catalogue must have >= 2 tools, got %d", task.ID, len(task.Tools))
		}

		// Ensure tools are valid
		for _, tl := range task.Tools {
			if tl.Name == "" || tl.Description == "" {
				t.Fatalf("task %s: tool has empty name/description: %+v", task.ID, tl)
			}
		}

		// Prompt must NOT leak exact tool name (e.g. "Call get_issue" or "use get_issue tool")
		for _, tl := range task.Tools {
			if strings.Contains(strings.ToLower(task.Prompt), "call "+strings.ToLower(tl.Name)) {
				t.Fatalf("task %s leaks tool name with 'Call %s'", task.ID, tl.Name)
			}
			if strings.Contains(strings.ToLower(task.Prompt), strings.ToLower(tl.Name)+" tool") {
				t.Fatalf("task %s leaks tool name with '%s tool'", task.ID, tl.Name)
			}
		}

		if task.Category == domain.CategoryMultiHop {
			if len(task.MultiHop) < 2 {
				t.Fatalf("multihop task %s must have >= 2 hops, got %d", task.ID, len(task.MultiHop))
			}
			if task.Expected.ToolName != task.MultiHop[0].ToolName {
				t.Fatalf("multihop task %s: Expected.ToolName (%s) != MultiHop[0].ToolName (%s)",
					task.ID, task.Expected.ToolName, task.MultiHop[0].ToolName)
			}
		}
	}

	if catCounts[domain.CategoryAmbiguousName] != 10 {
		t.Fatalf("expected 10 ambiguous_name tasks, got %d", catCounts[domain.CategoryAmbiguousName])
	}
	if catCounts[domain.CategoryOverlappingSchem] != 10 {
		t.Fatalf("expected 10 overlapping_schema tasks, got %d", catCounts[domain.CategoryOverlappingSchem])
	}
	if catCounts[domain.CategoryMultiHop] != 10 {
		t.Fatalf("expected 10 multihop tasks, got %d", catCounts[domain.CategoryMultiHop])
	}
}

func TestSyncSyntheticV1(t *testing.T) {
	tasks := Builtin()
	path := "../../suites/synthetic_v1.json"
	if err := Save(path, tasks); err != nil {
		t.Fatalf("failed to save synthetic_v1.json: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("failed to load saved synthetic_v1.json: %v", err)
	}
	if len(loaded) != len(tasks) {
		t.Fatalf("saved suite has %d tasks, want %d", len(loaded), len(tasks))
	}
}

func TestUniqueToolsDeduplicates(t *testing.T) {
	tasks := Builtin()
	got := UniqueTools(tasks)
	names := map[string]bool{}
	for _, tool := range got {
		names[tool.Name] = true
	}
	if len(names) != len(got) {
		t.Fatal("duplicate tool names in UniqueTools output")
	}
}

func TestDistractorPadding(t *testing.T) {
	tasks := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", Distractors: 20, Seed: 5}).Generate()
	for _, task := range tasks {
		names := map[string]bool{}
		for _, tl := range task.Tools {
			if names[tl.Name] {
				t.Fatalf("task %s has duplicate tool %s", task.ID, tl.Name)
			}
			names[tl.Name] = true
		}
		base := 2
		if task.Category == domain.CategoryAmbiguousName {
			base = 4
		}
		if len(task.Tools) != base+20 {
			t.Fatalf("task %s: expected %d tools, got %d", task.ID, base+20, len(task.Tools))
		}
	}
	base := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", Seed: 5}).Generate()
	want := make(map[string]bool)
	for _, task := range base {
		for _, tl := range task.Tools {
			want[tl.Name] = true
		}
	}
	unique := UniqueTools(tasks)
	got := make(map[string]bool)
	for _, tl := range unique {
		got[tl.Name] = true
	}
	if len(got) != len(unique) {
		t.Fatal("duplicate tool names in UniqueTools output")
	}
	if len(got) != len(want)+20 {
		t.Fatalf("expected %d unique tools after padding, got %d", len(want)+20, len(got))
	}
}

func TestPromptStyles(t *testing.T) {
	explicit := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", Seed: 11}).Generate()
	intent := NewGenerator(Options{NumTasks: 9, Ambiguity: "high", PromptStyle: StyleIntent, Seed: 11}).Generate()
	snake := regexp.MustCompile(`[a-z]+_[a-z]+`)
	for i := range intent {
		if intent[i].Prompt == explicit[i].Prompt {
			t.Fatalf("task %d: intent prompt matches explicit", i)
		}
		if snake.MatchString(intent[i].Prompt) {
			t.Fatalf("intent prompt leaks tool name: %q", intent[i].Prompt)
		}
		if intent[i].Expected.ToolName != explicit[i].Expected.ToolName {
			t.Fatalf("task %d: styles disagree on expected tool", i)
		}
	}
}
