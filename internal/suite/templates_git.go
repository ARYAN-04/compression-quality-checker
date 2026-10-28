package suite

import (
	"can-i-compress-it/internal/domain"
)

func gitStatus() domain.ToolDefinition {
	t := tool("git_status", "Show the working tree status.")
	t.Parameters = params(map[string]any{
		"repo_path": obj("string", "Path to the git repository."),
	})
	return t
}

func gitDiff() domain.ToolDefinition {
	t := tool("git_diff", "Show changes between commits, commit and working tree, etc.")
	t.Parameters = params(map[string]any{
		"repo_path": obj("string", "Path to the git repository."),
		"cached":    obj("boolean", "View staged changes."),
		"target":    obj("string", "Target branch or commit SHA to compare against."),
	})
	return t
}

func gitCommit() domain.ToolDefinition {
	t := tool("git_commit", "Record changes to the repository.")
	t.Parameters = params(map[string]any{
		"message":   obj("string", "Commit message describing the changes."),
		"repo_path": obj("string", "Path to the git repository."),
		"all":       obj("boolean", "Automatically stage modified and deleted files."),
	})
	return t
}

func gitLog() domain.ToolDefinition {
	t := tool("git_log", "Show commit logs.")
	t.Parameters = params(map[string]any{
		"repo_path": obj("string", "Path to the git repository."),
		"max_count": obj("integer", "Maximum number of commit entries to return."),
	})
	return t
}

func gitBranch() domain.ToolDefinition {
	t := tool("git_branch", "List, create, or inspect git branches.")
	t.Parameters = params(map[string]any{
		"repo_path":   obj("string", "Path to the git repository."),
		"branch_name": obj("string", "Branch name to create or switch to."),
	})
	return t
}

func gitTasks() []domain.Task {
	tools := embeddedToolsMap("git")
	status := getOrFallbackTool(tools, "git_status", gitStatus())
	diff := getOrFallbackTool(tools, "git_diff", gitDiff())
	commit := getOrFallbackTool(tools, "git_commit", gitCommit())
	log := getOrFallbackTool(tools, "git_log", gitLog())
	branch := getOrFallbackTool(tools, "git_branch", gitBranch())

	return []domain.Task{
		{
			ID:       "git_status_001",
			Category: domain.CategoryImported,
			Prompt:   "Call git_status exactly once. Arguments: repo_path: /srv/repo.",
			Tools:    []domain.ToolDefinition{status, diff},
			Expected: expected("git_status", map[string]any{"repo_path": "/srv/repo"}),
		},
		{
			ID:       "git_diff_001",
			Category: domain.CategoryImported,
			Prompt:   "Call git_diff exactly once. Arguments: repo_path: /srv/repo, target: main.",
			Tools:    []domain.ToolDefinition{diff, status},
			Expected: expected("git_diff", map[string]any{"repo_path": "/srv/repo", "target": "main"}),
		},
		{
			ID:       "git_commit_001",
			Category: domain.CategoryImported,
			Prompt:   "Call git_commit exactly once. Arguments: message: feat: add schema parser, repo_path: /srv/repo.",
			Tools:    []domain.ToolDefinition{commit, diff, status},
			Expected: expected("git_commit", map[string]any{"message": "feat: add schema parser", "repo_path": "/srv/repo"}),
		},
		{
			ID:       "git_log_001",
			Category: domain.CategoryImported,
			Prompt:   "Call git_log exactly once. Arguments: repo_path: /srv/repo, max_count: 5.",
			Tools:    []domain.ToolDefinition{log, status},
			Expected: expected("git_log", map[string]any{"max_count": int64(5), "repo_path": "/srv/repo"}),
		},
		{
			ID:       "git_branch_001",
			Category: domain.CategoryImported,
			Prompt:   "Call git_branch exactly once. Arguments: branch_name: feature/auth, repo_path: /srv/repo.",
			Tools:    []domain.ToolDefinition{branch, status},
			Expected: expected("git_branch", map[string]any{"branch_name": "feature/auth", "repo_path": "/srv/repo"}),
		},
		{
			ID:       "git_chain_review_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call git_status. Arguments: repo_path: /srv/repo. Step 2: call git_diff. Arguments: repo_path: /srv/repo, target: <from:git_status.target_branch>.",
			Tools:    []domain.ToolDefinition{status, diff, commit},
			Expected: expected("git_status", map[string]any{"repo_path": "/srv/repo"}),
			MultiHop: []domain.ExpectedCall{
				expected("git_status", map[string]any{"repo_path": "/srv/repo"}),
				expected("git_diff", map[string]any{
					"repo_path": "/srv/repo",
					"target":    "<from:git_status.target_branch>",
				}),
			},
		},
	}
}
