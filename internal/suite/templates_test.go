package suite

import (
	"testing"
)

func TestTemplateCataloguesMatchExpectations(t *testing.T) {
	for _, name := range TemplateNames() {
		tasks, ok := Template(name)
		if !ok {
			t.Fatalf("template %s missing", name)
		}
		for _, task := range tasks {
			if task.Category != "imported" {
				t.Fatalf("task %s: category %q, want imported", task.ID, task.Category)
			}
			if task.Prompt == "" || task.Expected.ToolName == "" {
				t.Fatalf("task %s not annotated", task.ID)
			}
			found := false
			for _, tl := range task.Tools {
				if tl.Name == task.Expected.ToolName {
					found = true
				}
			}
			if !found {
				t.Fatalf("task %s: expected tool %q absent from catalogue",
					task.ID, task.Expected.ToolName)
			}
		}
	}
}

func TestTemplateUnknown(t *testing.T) {
	if _, ok := Template("slack"); ok {
		t.Fatal("unknown template should not resolve")
	}
}
