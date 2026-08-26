package suite

import (
	"regexp"
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
