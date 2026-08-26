package suite

import (
	"strings"
	"testing"
)

func TestTemplateCataloguesMatchExpectations(t *testing.T) {
	for _, name := range TemplateNames() {
		tasks, ok := Template(name)
		if !ok {
			t.Fatalf("template %s missing", name)
		}
		for _, task := range tasks {
			switch task.Category {
			case "imported", "multihop":
			default:
				t.Fatalf("task %s: category %q, want imported or multihop",
					task.ID, task.Category)
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

func TestTemplateMultiHopChains(t *testing.T) {
	for _, name := range TemplateNames() {
		tasks, ok := Template(name)
		if !ok {
			t.Fatalf("template %s missing", name)
		}
		for _, task := range tasks {
			if task.Category != "multihop" {
				continue
			}
			if len(task.MultiHop) < 2 {
				t.Fatalf("task %s: multihop chain has %d steps, want >= 2",
					task.ID, len(task.MultiHop))
			}
			catalogue := map[string]bool{}
			for _, tl := range task.Tools {
				catalogue[tl.Name] = true
			}
			for i, step := range task.MultiHop {
				for arg, val := range step.Arguments {
					ref, isStr := val.(string)
					if !isStr || !strings.HasPrefix(ref, "<from:") {
						if isStr && strings.Contains(ref, ",") {
							t.Fatalf("task %s step %d: argument %q value %q contains a comma",
								task.ID, i, arg, ref)
						}
						continue
					}
					srcTool, field, ok := strings.Cut(
						strings.TrimSuffix(strings.TrimPrefix(ref, "<from:"), ">"), ".")
					if !ok || field == "" {
						t.Fatalf("task %s step %d: malformed placeholder %q",
							task.ID, i, ref)
					}
					earlier := false
					for j := 0; j < i; j++ {
						if task.MultiHop[j].ToolName == srcTool {
							earlier = true
						}
					}
					if !earlier {
						t.Fatalf("task %s step %d: placeholder %q does not reference a strictly earlier step",
							task.ID, i, ref)
					}
					if !catalogue[srcTool] {
						t.Fatalf("task %s: referenced tool %q absent from catalogue",
							task.ID, srcTool)
					}
				}
			}
		}
	}
}

func TestTemplateUnknown(t *testing.T) {
	if _, ok := Template("nonexistent"); ok {
		t.Fatal("unknown template should not resolve")
	}
}
