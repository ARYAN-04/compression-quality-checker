package suite

import (
	"can-i-compress-it/internal/domain"
)

// TemplateNames lists the pre-annotated standard MCP servers.
func TemplateNames() []string {
	return []string{"github", "sqlite", "filesystem", "jira", "slack"}
}

// Template returns pre-annotated benchmark tasks for a standard MCP
// server ("github", "sqlite", "filesystem", "jira", "slack"). Ground
// truth matches the upstream @modelcontextprotocol server tool names
// and input schemas.
func Template(name string) ([]domain.Task, bool) {
	switch name {
	case "github":
		return githubTasks(), true
	case "sqlite":
		return sqliteTasks(), true
	case "filesystem":
		return filesystemTasks(), true
	case "jira":
		return jiraTasks(), true
	case "slack":
		return slackTasks(), true
	default:
		return nil, false
	}
}

func githubTasks() []domain.Task {
	getIssue := tool("get_issue", "Get details of a specific issue in a GitHub repository.")
	getIssue.Parameters = params(map[string]any{
		"owner":        obj("string", "Repository owner login."),
		"repo":         obj("string", "Repository name."),
		"issue_number": obj("integer", "Issue number to fetch."),
	})
	searchRepos := tool("search_repositories", "Search for GitHub repositories by query.")
	searchRepos.Parameters = params(map[string]any{
		"query": obj("string", "Search query using GitHub search syntax."),
	})
	listIssues := tool("list_issues", "List issues in a GitHub repository.")
	listIssues.Parameters = params(map[string]any{
		"owner": obj("string", "Repository owner login."),
		"repo":  obj("string", "Repository name."),
		"state": obj("string", "Filter by issue state: open, closed, or all."),
	})
	createIssue := tool("create_issue", "Create a new issue in a GitHub repository.")
	createIssue.Parameters = params(map[string]any{
		"owner": obj("string", "Repository owner login."),
		"repo":  obj("string", "Repository name."),
		"title": obj("string", "Title of the new issue."),
		"body":  obj("string", "Body text of the new issue."),
	})
	getPullRequest := tool("get_pull_request", "Get details of a specific pull request in a GitHub repository.")
	getPullRequest.Parameters = params(map[string]any{
		"owner":       obj("string", "Repository owner login."),
		"repo":        obj("string", "Repository name."),
		"pull_number": obj("integer", "Pull request number to fetch."),
	})
	listCommits := tool("list_commits", "List commits in a GitHub repository.")
	listCommits.Parameters = params(map[string]any{
		"owner": obj("string", "Repository owner login."),
		"repo":  obj("string", "Repository name."),
		"sha":   obj("string", "Optional branch or commit SHA to start listing from."),
	})
	createPRComment := tool("create_pull_request_comment", "Post a comment on a GitHub pull request.")
	createPRComment.Parameters = params(map[string]any{
		"owner":       obj("string", "Repository owner login."),
		"repo":        obj("string", "Repository name."),
		"pull_number": obj("integer", "Pull request number to comment on."),
		"body":        obj("string", "Comment text to post."),
	})
	return []domain.Task{
		{
			ID:       "gh_get_issue_001",
			Category: domain.CategoryImported,
			Prompt:   "Call get_issue exactly once. Arguments: owner: acme, repo: api-gateway, issue_number: 1042.",
			Tools:    []domain.ToolDefinition{getIssue, listIssues},
			Expected: expected("get_issue", map[string]any{
				"owner": "acme", "repo": "api-gateway", "issue_number": int64(1042),
			}),
		},
		{
			ID:       "gh_search_repos_001",
			Category: domain.CategoryImported,
			Prompt:   "Call search_repositories exactly once. Arguments: query: language:go stars:>1000.",
			Tools:    []domain.ToolDefinition{searchRepos, getIssue},
			Expected: expected("search_repositories", map[string]any{
				"query": "language:go stars:>1000",
			}),
		},
		{
			ID:       "gh_list_issues_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_issues exactly once. Arguments: owner: acme, repo: api-gateway. Pass no optional parameters.",
			Tools:    []domain.ToolDefinition{listIssues, getIssue},
			Expected: expected("list_issues", map[string]any{"owner": "acme", "repo": "api-gateway"}),
		},
		{
			ID:       "gh_create_issue_001",
			Category: domain.CategoryImported,
			Prompt:   "Call create_issue exactly once. Arguments: owner: acme, repo: api-gateway, title: Add rate limiting, body: Implement token bucket rate limiting for the gateway.",
			Tools:    []domain.ToolDefinition{createIssue, createPRComment, getIssue},
			Expected: expected("create_issue", map[string]any{
				"owner": "acme", "repo": "api-gateway",
				"title": "Add rate limiting",
				"body":  "Implement token bucket rate limiting for the gateway",
			}),
		},
		{
			ID:       "gh_get_pull_request_001",
			Category: domain.CategoryImported,
			Prompt:   "Call get_pull_request exactly once. Arguments: owner: acme, repo: api-gateway, pull_number: 87.",
			Tools:    []domain.ToolDefinition{getPullRequest, getIssue},
			Expected: expected("get_pull_request", map[string]any{
				"owner": "acme", "repo": "api-gateway", "pull_number": int64(87),
			}),
		},
		{
			ID:       "gh_list_commits_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_commits exactly once. Arguments: owner: acme, repo: api-gateway. Pass no optional parameters.",
			Tools:    []domain.ToolDefinition{listCommits, getPullRequest},
			Expected: expected("list_commits", map[string]any{"owner": "acme", "repo": "api-gateway"}),
		},
		{
			ID:       "gh_create_pr_comment_001",
			Category: domain.CategoryImported,
			Prompt:   "Call create_pull_request_comment exactly once. Arguments: owner: acme, repo: api-gateway, pull_number: 87, body: LGTM once CI passes.",
			Tools:    []domain.ToolDefinition{createPRComment, createIssue, getPullRequest},
			Expected: expected("create_pull_request_comment", map[string]any{
				"owner": "acme", "repo": "api-gateway", "pull_number": int64(87),
				"body": "LGTM once CI passes",
			}),
		},
	}
}

func sqliteTasks() []domain.Task {
	listTables := tool("list_tables", "List all tables in the SQLite database.")
	listTables.Parameters = params(nil)
	describeTable := tool("describe_table", "Describe the columns of a specific table.")
	describeTable.Parameters = params(map[string]any{
		"table_name": obj("string", "Name of the table to describe."),
	})
	readQuery := tool("read_query", "Execute a read-only SQL SELECT query.")
	readQuery.Parameters = params(map[string]any{
		"query": obj("string", "SQL SELECT query to execute."),
	})
	executeQuery := tool("execute_query", "Execute a write SQL statement such as INSERT or UPDATE.")
	executeQuery.Parameters = params(map[string]any{
		"query": obj("string", "SQL write statement to execute."),
	})
	return []domain.Task{
		{
			ID:       "sq_list_tables_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_tables exactly once with no arguments.",
			Tools:    []domain.ToolDefinition{listTables, describeTable},
			Expected: expected("list_tables", map[string]any{}),
		},
		{
			ID:       "sq_describe_001",
			Category: domain.CategoryImported,
			Prompt:   "Call describe_table exactly once. Arguments: table_name: users.",
			Tools:    []domain.ToolDefinition{describeTable, readQuery},
			Expected: expected("describe_table", map[string]any{"table_name": "users"}),
		},
		{
			ID:       "sq_read_query_001",
			Category: domain.CategoryImported,
			Prompt:   "Call read_query exactly once. Arguments: query: SELECT count(*) FROM orders.",
			Tools:    []domain.ToolDefinition{readQuery, describeTable, executeQuery},
			Expected: expected("read_query", map[string]any{
				"query": "SELECT count(*) FROM orders",
			}),
		},
		{
			ID:       "sq_execute_query_001",
			Category: domain.CategoryImported,
			Prompt:   "Call execute_query exactly once. Arguments: query: INSERT INTO users (name) VALUES ('alice').",
			Tools:    []domain.ToolDefinition{executeQuery, readQuery},
			Expected: expected("execute_query", map[string]any{
				"query": "INSERT INTO users (name) VALUES ('alice')",
			}),
		},
	}
}

func filesystemTasks() []domain.Task {
	listDir := tool("list_directory", "List the contents of a directory.")
	listDir.Parameters = params(map[string]any{
		"path": obj("string", "Directory path to list."),
	})
	readFile := tool("read_file", "Read the complete contents of a single file.")
	readFile.Parameters = params(map[string]any{
		"path": obj("string", "Path of the file to read."),
	})
	searchFiles := tool("search_files", "Recursively search for files matching a pattern.")
	searchFiles.Parameters = params(map[string]any{
		"path":    obj("string", "Root directory to start the search from."),
		"pattern": obj("string", "Pattern to match file names against."),
	})
	writeFile := tool("write_file", "Create or overwrite a file with the given contents.")
	writeFile.Parameters = params(map[string]any{
		"path":    obj("string", "Path of the file to write."),
		"content": obj("string", "Full contents to write to the file."),
	})
	moveFile := tool("move_file", "Move or rename a file to a new location.")
	moveFile.Parameters = params(map[string]any{
		"source":      obj("string", "Path of the file to move."),
		"destination": obj("string", "New path for the file."),
	})
	getFileInfo := tool("get_file_info", "Get metadata about a file such as size and modification time.")
	getFileInfo.Parameters = params(map[string]any{
		"path": obj("string", "Path of the file to inspect."),
	})
	deleteFile := tool("delete_file", "Permanently delete a file.")
	deleteFile.Parameters = params(map[string]any{
		"path": obj("string", "Path of the file to delete."),
	})
	return []domain.Task{
		{
			ID:       "fs_list_dir_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_directory exactly once. Arguments: path: /srv/projects.",
			Tools:    []domain.ToolDefinition{listDir, readFile, writeFile},
			Expected: expected("list_directory", map[string]any{"path": "/srv/projects"}),
		},
		{
			ID:       "fs_read_file_001",
			Category: domain.CategoryImported,
			Prompt:   "Call read_file exactly once. Arguments: path: /srv/projects/README.md.",
			Tools:    []domain.ToolDefinition{readFile, listDir},
			Expected: expected("read_file", map[string]any{"path": "/srv/projects/README.md"}),
		},
		{
			ID:       "fs_search_files_001",
			Category: domain.CategoryImported,
			Prompt:   "Call search_files exactly once. Arguments: path: /srv/projects, pattern: *.log.",
			Tools:    []domain.ToolDefinition{searchFiles, listDir},
			Expected: expected("search_files", map[string]any{
				"path": "/srv/projects", "pattern": "*.log",
			}),
		},
		{
			ID:       "fs_move_file_001",
			Category: domain.CategoryImported,
			Prompt:   "Call move_file exactly once. Arguments: source: /srv/projects/notes.txt, destination: /srv/archive/notes.txt.",
			Tools:    []domain.ToolDefinition{moveFile, writeFile, deleteFile},
			Expected: expected("move_file", map[string]any{
				"source": "/srv/projects/notes.txt", "destination": "/srv/archive/notes.txt",
			}),
		},
		{
			ID:       "fs_get_file_info_001",
			Category: domain.CategoryImported,
			Prompt:   "Call get_file_info exactly once. Arguments: path: /srv/projects/build.log. Do not read the file contents.",
			Tools:    []domain.ToolDefinition{getFileInfo, readFile},
			Expected: expected("get_file_info", map[string]any{"path": "/srv/projects/build.log"}),
		},
		{
			ID:       "fs_delete_file_001",
			Category: domain.CategoryImported,
			Prompt:   "Call delete_file exactly once. Arguments: path: /tmp/cache/session.dat.",
			Tools:    []domain.ToolDefinition{deleteFile, moveFile, readFile},
			Expected: expected("delete_file", map[string]any{"path": "/tmp/cache/session.dat"}),
		},
	}
}
