package suite

import (
	"can-i-compress-it/internal/domain"
)

func jiraGetIssue() domain.ToolDefinition {
	t := tool("get_issue", "Get full details of a single Jira issue by its key.")
	t.Parameters = params(map[string]any{
		"issue_key": obj("string", "Issue key such as ACME-42."),
	})
	return t
}

func jiraSearchJQL() domain.ToolDefinition {
	t := tool("search_issues_jql", "Search Jira issues using a JQL query string.")
	t.Parameters = params(map[string]any{
		"jql": obj("string", "JQL query selecting the issues to return."),
	})
	return t
}

func jiraCreateIssue() domain.ToolDefinition {
	t := tool("create_issue", "Create a new issue in a Jira project.")
	t.Parameters = params(map[string]any{
		"project_key": obj("string", "Key of the project that receives the issue."),
		"summary":     obj("string", "Short one-line summary of the issue."),
		"description": obj("string", "Long-form description of the issue."),
	})
	return t
}

func jiraUpdateStatus() domain.ToolDefinition {
	t := tool("update_issue_status", "Transition an issue to a new workflow status.")
	t.Parameters = params(map[string]any{
		"issue_key": obj("string", "Issue key to transition."),
		"status":    obj("string", "Target workflow status such as In Review."),
	})
	return t
}

func jiraAddComment() domain.ToolDefinition {
	t := tool("add_comment", "Add a comment to an existing Jira issue.")
	t.Parameters = params(map[string]any{
		"issue_key": obj("string", "Issue key receiving the comment."),
		"body":      obj("string", "Comment text to append to the issue."),
	})
	return t
}

func jiraTasks() []domain.Task {
	getIssue := jiraGetIssue()
	searchJQL := jiraSearchJQL()
	createIssue := jiraCreateIssue()
	updateStatus := jiraUpdateStatus()
	addComment := jiraAddComment()
	return []domain.Task{
		{
			ID:       "jr_get_issue_001",
			Category: domain.CategoryImported,
			Prompt:   "Call get_issue exactly once. Arguments: issue_key: ACME-42.",
			Tools:    []domain.ToolDefinition{getIssue, searchJQL},
			Expected: expected("get_issue", map[string]any{"issue_key": "ACME-42"}),
		},
		{
			ID:       "jr_search_jql_001",
			Category: domain.CategoryImported,
			Prompt:   "Call search_issues_jql exactly once. Arguments: jql: project = ACME AND status = Open ORDER BY created DESC.",
			Tools:    []domain.ToolDefinition{searchJQL, getIssue},
			Expected: expected("search_issues_jql", map[string]any{
				"jql": "project = ACME AND status = Open ORDER BY created DESC",
			}),
		},
		{
			ID:       "jr_create_issue_001",
			Category: domain.CategoryImported,
			Prompt:   "Call create_issue exactly once. Arguments: project_key: ACME, summary: Fix login timeout on staging. Pass no optional parameters.",
			Tools:    []domain.ToolDefinition{createIssue, updateStatus, addComment},
			Expected: expected("create_issue", map[string]any{
				"project_key": "ACME", "summary": "Fix login timeout on staging",
			}),
		},
		{
			ID:       "jr_update_status_001",
			Category: domain.CategoryImported,
			Prompt:   "Call update_issue_status exactly once. Arguments: issue_key: ACME-77, status: In Review.",
			Tools:    []domain.ToolDefinition{updateStatus, addComment, getIssue},
			Expected: expected("update_issue_status", map[string]any{
				"issue_key": "ACME-77", "status": "In Review",
			}),
		},
		{
			ID:       "jr_add_comment_001",
			Category: domain.CategoryImported,
			Prompt:   "Call add_comment exactly once. Arguments: issue_key: ACME-42, body: Verified fix on staging.",
			Tools:    []domain.ToolDefinition{addComment, createIssue, getIssue},
			Expected: expected("add_comment", map[string]any{
				"issue_key": "ACME-42", "body": "Verified fix on staging",
			}),
		},
		{
			ID:       "jr_chain_comment_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call search_issues_jql. Arguments: jql: project = ACME AND summary ~ payment retry. Step 2: call get_issue. Arguments: issue_key: <from:search_issues_jql.issue_key>. Step 3: call add_comment. Arguments: issue_key: <from:get_issue.issue_key>, body: Confirmed duplicate of ACME-99.",
			Tools:    []domain.ToolDefinition{searchJQL, getIssue, addComment},
			Expected: expected("search_issues_jql", map[string]any{
				"jql": "project = ACME AND summary ~ payment retry",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("search_issues_jql", map[string]any{
					"jql": "project = ACME AND summary ~ payment retry",
				}),
				expected("get_issue", map[string]any{
					"issue_key": "<from:search_issues_jql.issue_key>",
				}),
				expected("add_comment", map[string]any{
					"issue_key": "<from:get_issue.issue_key>",
					"body":      "Confirmed duplicate of ACME-99",
				}),
			},
		},
	}
}
