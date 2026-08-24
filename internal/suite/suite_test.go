package suite

import (
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
