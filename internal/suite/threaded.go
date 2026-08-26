package suite

import (
	"sort"
	"strings"

	"can-i-compress-it/internal/domain"
)

// ThreadedFields scans every task's expected arguments (including all
// multi-hop hops) for "<from:tool.field>" placeholders and returns, per
// referenced tool, the sorted result fields its synthesized payload must
// carry so offline compressors can satisfy chain threading.
func ThreadedFields(tasks []domain.Task) map[string][]string {
	fields := map[string]map[string]bool{}
	for _, t := range tasks {
		collectThreaded(fields, t.Expected.Arguments)
		for _, hop := range t.MultiHop {
			collectThreaded(fields, hop.Arguments)
		}
	}
	out := make(map[string][]string, len(fields))
	for tool, set := range fields {
		names := make([]string, 0, len(set))
		for f := range set {
			names = append(names, f)
		}
		sort.Strings(names)
		out[tool] = names
	}
	return out
}

// collectThreaded records the field referenced by each placeholder in
// args under its source tool key.
func collectThreaded(fields map[string]map[string]bool, args map[string]any) {
	for _, v := range args {
		ref, ok := v.(string)
		if !ok || !strings.HasPrefix(ref, "<from:") {
			continue
		}
		path := strings.TrimSuffix(strings.TrimPrefix(ref, "<from:"), ">")
		tool, field, ok := strings.Cut(path, ".")
		if !ok || field == "" {
			continue
		}
		if fields[tool] == nil {
			fields[tool] = map[string]bool{}
		}
		fields[tool][field] = true
	}
}
