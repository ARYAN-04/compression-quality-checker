package suite

import (
	"can-i-compress-it/internal/domain"
)

// serverTools extracts specific tool definitions from an embedded MCP schema.
func serverTools(serverName string, toolNames ...string) []domain.ToolDefinition {
	tools, ok := EmbeddedSchemaTools(serverName)
	if !ok {
		panic("missing embedded schema: " + serverName)
	}
	if len(toolNames) == 0 {
		return tools
	}
	byName := make(map[string]domain.ToolDefinition, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	out := make([]domain.ToolDefinition, 0, len(toolNames))
	for _, name := range toolNames {
		if t, found := byName[name]; found {
			out = append(out, t)
		}
	}
	return out
}

// BuiltinTasks constructs the canonical 30 benchmark tasks purpose-built
// for in-built MCP schemas across ambiguous_name, overlapping_schema, and
// multihop categories.
func BuiltinTasks() []domain.Task {
	return []domain.Task{
		// =====================================================================
		// Category 1: Ambiguous Name (10 tasks)
		// Semantic discrimination between similar tools based on descriptions.
		// =====================================================================
		{
			ID:       "amb_001",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Retrieve the email addresses of all active customers who signed up in 2024 from the database.",
			Tools:    serverTools("postgres", "read_query", "execute_query", "describe_table", "list_tables"),
			Expected: expected("read_query", map[string]any{
				"query": "SELECT email FROM customers WHERE active = true AND signup_date >= '2024-01-01'",
			}),
		},
		{
			ID:       "amb_002",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "List all files and contents located inside the directory /var/log/nginx.",
			Tools:    serverTools("filesystem", "list_directory", "search_files", "get_file_info", "read_file"),
			Expected: expected("list_directory", map[string]any{
				"path": "/var/log/nginx",
			}),
		},
		{
			ID:       "amb_003",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Check the disk size and last modified date of the archive at /backups/db_dump.tar.gz without opening its contents.",
			Tools:    serverTools("filesystem", "get_file_info", "read_file", "list_directory", "delete_file"),
			Expected: expected("get_file_info", map[string]any{
				"path": "/backups/db_dump.tar.gz",
			}),
		},
		{
			ID:       "amb_004",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Look up all unresolved blocker tickets assigned to Sarah Chen in the INFRA project.",
			Tools:    serverTools("jira", "search_issues_jql", "get_issue", "create_issue", "update_issue_status"),
			Expected: expected("search_issues_jql", map[string]any{
				"jql": "project = INFRA AND assignee = 'Sarah Chen' AND priority = Blocker AND resolution = Unresolved",
			}),
		},
		{
			ID:       "amb_005",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Inspect the details, reviewers, and branch merge status for pull request #248 in the kubernetes/minikube repository.",
			Tools:    serverTools("github", "get_pull_request", "get_issue", "list_issues", "create_pull_request_comment"),
			Expected: expected("get_pull_request", map[string]any{
				"owner":       "kubernetes",
				"repo":        "minikube",
				"pull_number": int64(248),
			}),
		},
		{
			ID:       "amb_006",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Find top-rated Italian espresso bars near North Beach in San Francisco.",
			Tools:    serverTools("brave-search", "brave_local_search", "brave_web_search"),
			Expected: expected("brave_local_search", map[string]any{
				"query": "Italian espresso bars North Beach San Francisco",
			}),
		},
		{
			ID:       "amb_007",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Check the HTTP response headers and caching policies returned by https://api.stripe.com/v1/charges without downloading the full page body.",
			Tools:    serverTools("fetch", "extract_headers", "extract_markdown", "fetch"),
			Expected: expected("extract_headers", map[string]any{
				"url": "https://api.stripe.com/v1/charges",
			}),
		},
		{
			ID:       "amb_008",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Show me the uncommitted code changes currently staged in the index at /repos/auth-service.",
			Tools:    serverTools("git", "git_diff", "git_log", "git_status", "git_commit"),
			Expected: expected("git_diff", map[string]any{
				"repo_path": "/repos/auth-service",
				"cached":    true,
			}),
		},
		{
			ID:       "amb_009",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Send a fresh announcement message to the general channel C0123ABCD stating that the scheduled maintenance window is complete.",
			Tools:    serverTools("slack", "post_message", "reply_to_thread", "get_channel_history", "list_channels"),
			Expected: expected("post_message", map[string]any{
				"channel_id": "C0123ABCD",
				"text":       "The scheduled maintenance window is complete.",
			}),
		},
		{
			ID:       "amb_010",
			Category: domain.CategoryAmbiguousName,
			Prompt:   "Find all knowledge graph entities related to quantum cryptography protocols.",
			Tools:    serverTools("memory", "search_nodes", "open_nodes", "create_entities", "read_graph"),
			Expected: expected("search_nodes", map[string]any{
				"query": "quantum cryptography protocols",
			}),
		},

		// =====================================================================
		// Category 2: Overlapping Schema (10 tasks)
		// Tools sharing parameter names with differing types, optional/required.
		// =====================================================================
		{
			ID:       "ovl_001",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Inspect the schema and column definitions for the billing_events table.",
			Tools:    serverTools("sqlite", "describe_table", "read_query", "execute_query", "list_tables"),
			Expected: expected("describe_table", map[string]any{
				"table_name": "billing_events",
			}),
		},
		{
			ID:       "ovl_002",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Fetch the complete issue record and metadata for PROJ-882.",
			Tools:    serverTools("jira", "get_issue", "update_issue_status", "add_comment", "search_issues_jql"),
			Expected: expected("get_issue", map[string]any{
				"issue_key": "PROJ-882",
			}),
		},
		{
			ID:       "ovl_003",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Scan recursively under /home/ubuntu/project to find all configuration files ending in .yaml.",
			Tools:    serverTools("filesystem", "search_files", "get_file_info", "list_directory", "read_file"),
			Expected: expected("search_files", map[string]any{
				"path":    "/home/ubuntu/project",
				"pattern": "*.yaml",
			}),
		},
		{
			ID:       "ovl_004",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Show all open issues in the expressjs/express repository that have the bug label.",
			Tools:    serverTools("github", "list_issues", "get_issue", "create_issue", "list_commits"),
			Expected: expected("list_issues", map[string]any{
				"owner":  "expressjs",
				"repo":   "express",
				"state":  "open",
				"labels": []any{"bug"},
			}),
		},
		{
			ID:       "ovl_005",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "List all database tables located in the analytics schema.",
			Tools:    serverTools("postgres", "list_tables", "describe_table", "list_schemas", "read_query"),
			Expected: expected("list_tables", map[string]any{
				"schema": "analytics",
			}),
		},
		{
			ID:       "ovl_006",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Reply directly to discussion thread 1712839201.004200 in channel C9999XYZ saying 'Reviewed and approved'.",
			Tools:    serverTools("slack", "reply_to_thread", "post_message", "get_channel_history"),
			Expected: expected("reply_to_thread", map[string]any{
				"channel_id": "C9999XYZ",
				"thread_ts":  "1712839201.004200",
				"text":       "Reviewed and approved",
			}),
		},
		{
			ID:       "ovl_007",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Click the primary checkout button matching selector button#checkout-btn on the page.",
			Tools:    serverTools("puppeteer", "click", "fill", "navigate", "screenshot"),
			Expected: expected("click", map[string]any{
				"selector": "button#checkout-btn",
			}),
		},
		{
			ID:       "ovl_008",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Download raw payload from https://example.com/data.bin with a maximum length cap of 50000 bytes.",
			Tools:    serverTools("fetch", "fetch", "extract_markdown", "extract_headers"),
			Expected: expected("fetch", map[string]any{
				"url":        "https://example.com/data.bin",
				"max_length": int64(50000),
				"raw":        true,
			}),
		},
		{
			ID:       "ovl_009",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Record all modified files in repository /workspace/backend with commit message 'fix: handle nil pointer dereference'.",
			Tools:    serverTools("git", "git_commit", "git_status", "git_diff", "git_branch"),
			Expected: expected("git_commit", map[string]any{
				"repo_path": "/workspace/backend",
				"message":   "fix: handle nil pointer dereference",
				"all":       true,
			}),
		},
		{
			ID:       "ovl_010",
			Category: domain.CategoryOverlappingSchem,
			Prompt:   "Leave a code review comment 'LGTM! Great test coverage.' on pull request #45 for facebook/react.",
			Tools:    serverTools("github", "create_pull_request_comment", "create_issue", "get_pull_request", "list_issues"),
			Expected: expected("create_pull_request_comment", map[string]any{
				"owner":       "facebook",
				"repo":        "react",
				"pull_number": int64(45),
				"body":        "LGTM! Great test coverage.",
			}),
		},

		// =====================================================================
		// Category 3: Multi-Hop (10 tasks)
		// Multi-step workflows where step 2 consumes step 1's output.
		// =====================================================================
		{
			ID:       "hop_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Search for open tickets regarding 'checkout button error' in the FRONTEND project. Then add a comment on that ticket stating 'Reproduced on Safari 17.4.'.",
			Tools:    serverTools("jira", "search_issues_jql", "add_comment", "get_issue", "update_issue_status"),
			Expected: expected("search_issues_jql", map[string]any{
				"jql": "project = FRONTEND AND text ~ 'checkout button error'",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("search_issues_jql", map[string]any{
					"jql": "project = FRONTEND AND text ~ 'checkout button error'",
				}),
				expected("add_comment", map[string]any{
					"issue_key": "<from:search_issues_jql.issue_key>",
					"body":      "Reproduced on Safari 17.4.",
				}),
			},
		},
		{
			ID:       "hop_002",
			Category: domain.CategoryMultiHop,
			Prompt:   "Inspect directory /etc/nginx/sites-available to discover the site configuration file. Once found, read the complete file contents of that configuration file.",
			Tools:    serverTools("filesystem", "list_directory", "read_file", "search_files", "get_file_info"),
			Expected: expected("list_directory", map[string]any{
				"path": "/etc/nginx/sites-available",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("list_directory", map[string]any{
					"path": "/etc/nginx/sites-available",
				}),
				expected("read_file", map[string]any{
					"path": "<from:list_directory.path>",
				}),
			},
		},
		{
			ID:       "hop_003",
			Category: domain.CategoryMultiHop,
			Prompt:   "Explore the database tables located in the inventory schema. Once the tables are listed, inspect the columns and constraints of the primary warehouse table.",
			Tools:    serverTools("postgres", "list_tables", "describe_table", "list_schemas", "read_query"),
			Expected: expected("list_tables", map[string]any{
				"schema": "inventory",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("list_tables", map[string]any{
					"schema": "inventory",
				}),
				expected("describe_table", map[string]any{
					"table_name": "<from:list_tables.table_name>",
					"schema":     "inventory",
				}),
			},
		},
		{
			ID:       "hop_004",
			Category: domain.CategoryMultiHop,
			Prompt:   "Search GitHub for repositories matching 'modelcontextprotocol go'. From the top repository found, retrieve full details for pull request #12.",
			Tools:    serverTools("github", "search_repositories", "get_pull_request", "get_issue", "list_issues"),
			Expected: expected("search_repositories", map[string]any{
				"query": "modelcontextprotocol go",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("search_repositories", map[string]any{
					"query": "modelcontextprotocol go",
				}),
				expected("get_pull_request", map[string]any{
					"owner":       "<from:search_repositories.owner>",
					"repo":        "<from:search_repositories.repo>",
					"pull_number": int64(12),
				}),
			},
		},
		{
			ID:       "hop_005",
			Category: domain.CategoryMultiHop,
			Prompt:   "Fetch the list of public channels in the workspace to locate the announcements channel. Then post the update 'Deployment pipeline v2 is now active.' to that channel.",
			Tools:    serverTools("slack", "list_channels", "post_message", "reply_to_thread", "list_users"),
			Expected: expected("list_channels", map[string]any{
				"types": "public_channel",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("list_channels", map[string]any{
					"types": "public_channel",
				}),
				expected("post_message", map[string]any{
					"channel_id": "<from:list_channels.channel_id>",
					"text":       "Deployment pipeline v2 is now active.",
				}),
			},
		},
		{
			ID:       "hop_006",
			Category: domain.CategoryMultiHop,
			Prompt:   "Search the knowledge graph for nodes relating to 'database cluster architecture'. Then retrieve and open the full node details for the identified cluster entity.",
			Tools:    serverTools("memory", "search_nodes", "open_nodes", "read_graph", "create_entities"),
			Expected: expected("search_nodes", map[string]any{
				"query": "database cluster architecture",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("search_nodes", map[string]any{
					"query": "database cluster architecture",
				}),
				expected("open_nodes", map[string]any{
					"names": []any{"<from:search_nodes.name>"},
				}),
			},
		},
		{
			ID:       "hop_007",
			Category: domain.CategoryMultiHop,
			Prompt:   "Navigate the browser to https://status.github.com and wait for it to load. Once the page is loaded, capture a full screenshot named 'status-dashboard.png' with width 1280 and height 800.",
			Tools:    serverTools("puppeteer", "navigate", "screenshot", "click", "evaluate"),
			Expected: expected("navigate", map[string]any{
				"url": "https://status.github.com",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("navigate", map[string]any{
					"url": "https://status.github.com",
				}),
				expected("screenshot", map[string]any{
					"name":   "status-dashboard.png",
					"width":  int64(1280),
					"height": int64(800),
				}),
			},
		},
		{
			ID:       "hop_008",
			Category: domain.CategoryMultiHop,
			Prompt:   "Check the working tree status of the git repository at /workspace/compiler. If there are modified files, generate a diff of the unstaged modifications.",
			Tools:    serverTools("git", "git_status", "git_diff", "git_log", "git_commit"),
			Expected: expected("git_status", map[string]any{
				"repo_path": "/workspace/compiler",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("git_status", map[string]any{
					"repo_path": "/workspace/compiler",
				}),
				expected("git_diff", map[string]any{
					"repo_path": "<from:git_status.repo_path>",
					"cached":    false,
				}),
			},
		},
		{
			ID:       "hop_009",
			Category: domain.CategoryMultiHop,
			Prompt:   "Run a web search for 'latest Go 1.24 release notes'. Take the official release notes web page URL from the search results and download its clean page content.",
			Tools: append(
				serverTools("brave-search", "brave_web_search", "brave_local_search"),
				serverTools("fetch", "fetch", "extract_markdown")...,
			),
			Expected: expected("brave_web_search", map[string]any{
				"query": "latest Go 1.24 release notes",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("brave_web_search", map[string]any{
					"query": "latest Go 1.24 release notes",
				}),
				expected("fetch", map[string]any{
					"url": "<from:brave_web_search.url>",
					"raw": false,
				}),
			},
		},
		{
			ID:       "hop_010",
			Category: domain.CategoryMultiHop,
			Prompt:   "First inspect the columns of the 'customers' table. After verifying its structure, execute a read-only query to select customer_id, email, and balance for all customers with balance greater than 1000.",
			Tools:    serverTools("sqlite", "describe_table", "read_query", "list_tables", "execute_query"),
			Expected: expected("describe_table", map[string]any{
				"table_name": "customers",
			}),
			MultiHop: []domain.ExpectedCall{
				expected("describe_table", map[string]any{
					"table_name": "customers",
				}),
				expected("read_query", map[string]any{
					"query": "SELECT customer_id, email, balance FROM customers WHERE balance > 1000",
				}),
			},
		},
	}
}
