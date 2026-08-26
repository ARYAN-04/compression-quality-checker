package suite

import (
	"can-i-compress-it/internal/domain"
)

// TemplateNames lists the pre-annotated standard MCP servers.
func TemplateNames() []string {
	return []string{"github", "sqlite", "filesystem"}
}

// Template returns pre-annotated benchmark tasks for a standard MCP
// server ("github", "sqlite", "filesystem"). Ground truth matches the
// upstream @modelcontextprotocol server tool names and input schemas.
func Template(name string) ([]domain.Task, bool) {
	switch name {
	case "github":
		return githubTasks(), true
	case "sqlite":
		return sqliteTasks(), true
	case "filesystem":
		return filesystemTasks(), true
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
			Tools:    []domain.ToolDefinition{readQuery, describeTable},
			Expected: expected("read_query", map[string]any{
				"query": "SELECT count(*) FROM orders",
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
	}
}
