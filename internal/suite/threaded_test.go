package suite

import (
	"reflect"
	"testing"

	"can-i-compress-it/internal/domain"
)

func TestThreadedFieldsCollectsAndSorts(t *testing.T) {
	tasks := []domain.Task{
		{
			ID: "t1",
			Expected: domain.ExpectedCall{
				ToolName:  "search_issues_jql",
				Arguments: map[string]any{"jql": "project = DEMO"},
			},
			MultiHop: []domain.ExpectedCall{
				{ToolName: "get_issue", Arguments: map[string]any{"issue_key": "<from:search_issues_jql.issue_key>"}},
				{ToolName: "add_comment", Arguments: map[string]any{
					"issue_key": "<from:get_issue.issue_key>",
					"author":    "<from:get_issue.author>",
				}},
			},
		},
		{
			ID: "t2",
			MultiHop: []domain.ExpectedCall{
				// duplicate references must dedupe
				{ToolName: "get_issue", Arguments: map[string]any{"issue_key": "<from:search_issues_jql.issue_key>"}},
				{ToolName: "add_reaction", Arguments: map[string]any{"channel": "<from:list_channels.channel_id>"}},
			},
		},
	}
	got := ThreadedFields(tasks)
	want := map[string][]string{
		"search_issues_jql": {"issue_key"},
		"get_issue":         {"author", "issue_key"},
		"list_channels":     {"channel_id"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ThreadedFields = %v, want %v", got, want)
	}
}

func TestThreadedFieldsEmptySuite(t *testing.T) {
	if got := ThreadedFields(nil); len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
	single := []domain.Task{{Expected: domain.ExpectedCall{
		ToolName:  "get_issue",
		Arguments: map[string]any{"issue_key": "ISS-1"},
	}}}
	if got := ThreadedFields(single); len(got) != 0 {
		t.Fatalf("placeholders absent: expected empty map, got %v", got)
	}
}
