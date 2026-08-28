#!/usr/bin/env python3
"""
fetch_mcp_schemas.py — MCP Tool Schema Extractor & Registry Synchronizer

Fetches, refreshes, and exports standard MCP tool catalogue schemas from
either live stdio MCP servers (via JSON-RPC 2.0 initialize + tools/list)
or built-in canonical fixture definitions for 11 popular MCP servers:
  - github
  - jira
  - slack
  - sqlite
  - filesystem
  - postgres
  - memory
  - fetch
  - brave-search
  - puppeteer
  - git

Usage:
  # Export all canonical fixture schemas to suites/schemas/ and internal/suite/schemas/
  uv run scripts/fetch_mcp_schemas.py --all

  # Export a single built-in server schema
  uv run scripts/fetch_mcp_schemas.py --server postgres --out suites/schemas/postgres.json

  # Query a live MCP server over stdio and dump its schema
  uv run scripts/fetch_mcp_schemas.py --cmd "npx -y @modelcontextprotocol/server-filesystem /tmp" --out suites/schemas/filesystem.json
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

# Canonical MCP fixture catalogue definitions for offline benchmarking.
CANONICAL_SCHEMAS: Dict[str, List[Dict[str, Any]]] = {
    "github": [
        {
            "name": "get_issue",
            "description": "Get details of a specific issue in a GitHub repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "issue_number": {"type": "integer", "description": "Issue number to fetch."},
                },
                "required": ["owner", "repo", "issue_number"],
            },
        },
        {
            "name": "list_issues",
            "description": "List issues in a GitHub repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "state": {"type": "string", "description": "Filter by issue state: open, closed, or all."},
                    "labels": {"type": "array", "items": {"type": "string"}, "description": "Filter by comma-separated label names."},
                },
                "required": ["owner", "repo"],
            },
        },
        {
            "name": "create_issue",
            "description": "Create a new issue in a GitHub repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "title": {"type": "string", "description": "Title of the new issue."},
                    "body": {"type": "string", "description": "Body text of the new issue."},
                    "labels": {"type": "array", "items": {"type": "string"}, "description": "Labels to attach to the issue."},
                },
                "required": ["owner", "repo", "title"],
            },
        },
        {
            "name": "get_pull_request",
            "description": "Get details of a specific pull request in a GitHub repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "pull_number": {"type": "integer", "description": "Pull request number to fetch."},
                },
                "required": ["owner", "repo", "pull_number"],
            },
        },
        {
            "name": "list_commits",
            "description": "List commits in a GitHub repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "sha": {"type": "string", "description": "Optional branch or commit SHA to start listing from."},
                    "path": {"type": "string", "description": "Filter commits by file path."},
                },
                "required": ["owner", "repo"],
            },
        },
        {
            "name": "create_pull_request_comment",
            "description": "Post a comment on a GitHub pull request.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "owner": {"type": "string", "description": "Repository owner login."},
                    "repo": {"type": "string", "description": "Repository name."},
                    "pull_number": {"type": "integer", "description": "Pull request number to comment on."},
                    "body": {"type": "string", "description": "Comment text to post."},
                },
                "required": ["owner", "repo", "pull_number", "body"],
            },
        },
        {
            "name": "search_repositories",
            "description": "Search for GitHub repositories by query.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query using GitHub search syntax."},
                    "sort": {"type": "string", "description": "Sort field: stars, forks, or updated."},
                    "order": {"type": "string", "description": "Sort direction: asc or desc."},
                },
                "required": ["query"],
            },
        },
    ],
    "jira": [
        {
            "name": "get_issue",
            "description": "Get full details of a single Jira issue by its key.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "issue_key": {"type": "string", "description": "Issue key such as ACME-42."},
                },
                "required": ["issue_key"],
            },
        },
        {
            "name": "create_issue",
            "description": "Create a new issue in a Jira project.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "project_key": {"type": "string", "description": "Key of the project that receives the issue."},
                    "summary": {"type": "string", "description": "Short one-line summary of the issue."},
                    "description": {"type": "string", "description": "Long-form description of the issue."},
                    "issue_type": {"type": "string", "description": "Issue type such as Bug, Task, or Story."},
                },
                "required": ["project_key", "summary"],
            },
        },
        {
            "name": "update_issue_status",
            "description": "Transition an issue to a new workflow status.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "issue_key": {"type": "string", "description": "Issue key to transition."},
                    "status": {"type": "string", "description": "Target workflow status such as In Review."},
                },
                "required": ["issue_key", "status"],
            },
        },
        {
            "name": "add_comment",
            "description": "Add a comment to an existing Jira issue.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "issue_key": {"type": "string", "description": "Issue key receiving the comment."},
                    "body": {"type": "string", "description": "Comment text to append to the issue."},
                },
                "required": ["issue_key", "body"],
            },
        },
        {
            "name": "search_issues_jql",
            "description": "Search Jira issues using a JQL query string.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "jql": {"type": "string", "description": "JQL query selecting the issues to return."},
                    "max_results": {"type": "integer", "description": "Maximum number of issues to return."},
                },
                "required": ["jql"],
            },
        },
    ],
    "slack": [
        {
            "name": "list_channels",
            "description": "List all channels in the Slack workspace.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "types": {"type": "string", "description": "Comma-separated channel types: public_channel, private_channel."},
                    "cursor": {"type": "string", "description": "Pagination cursor for listing next page."},
                },
                "required": [],
            },
        },
        {
            "name": "post_message",
            "description": "Post a new message to a Slack channel.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "channel_id": {"type": "string", "description": "ID of the channel to post in."},
                    "text": {"type": "string", "description": "Message text to post."},
                },
                "required": ["channel_id", "text"],
            },
        },
        {
            "name": "reply_to_thread",
            "description": "Reply to an existing message thread in a Slack channel.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "channel_id": {"type": "string", "description": "ID of the channel containing the thread."},
                    "thread_ts": {"type": "string", "description": "Timestamp of the parent message."},
                    "text": {"type": "string", "description": "Reply text to post in the thread."},
                },
                "required": ["channel_id", "thread_ts", "text"],
            },
        },
        {
            "name": "get_channel_history",
            "description": "Fetch recent messages from a Slack channel.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "channel_id": {"type": "string", "description": "ID of the channel to read."},
                    "limit": {"type": "integer", "description": "Number of messages to retrieve."},
                },
                "required": ["channel_id"],
            },
        },
        {
            "name": "list_users",
            "description": "List users in the Slack workspace.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "cursor": {"type": "string", "description": "Pagination cursor."},
                    "limit": {"type": "integer", "description": "Number of users to retrieve."},
                },
                "required": [],
            },
        },
    ],
    "sqlite": [
        {
            "name": "list_tables",
            "description": "List all tables in the SQLite database.",
            "inputSchema": {
                "type": "object",
                "properties": {},
                "required": [],
            },
        },
        {
            "name": "describe_table",
            "description": "Describe the columns of a specific table.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "table_name": {"type": "string", "description": "Name of the table to describe."},
                },
                "required": ["table_name"],
            },
        },
        {
            "name": "read_query",
            "description": "Execute a read-only SQL SELECT query.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "SQL SELECT query to execute."},
                },
                "required": ["query"],
            },
        },
        {
            "name": "execute_query",
            "description": "Execute a write SQL statement such as INSERT or UPDATE.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "SQL write statement to execute."},
                },
                "required": ["query"],
            },
        },
    ],
    "filesystem": [
        {
            "name": "list_directory",
            "description": "List the contents of a directory.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Directory path to list."},
                },
                "required": ["path"],
            },
        },
        {
            "name": "read_file",
            "description": "Read the complete contents of a single file.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Path of the file to read."},
                },
                "required": ["path"],
            },
        },
        {
            "name": "search_files",
            "description": "Recursively search for files matching a pattern.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Root directory to start the search from."},
                    "pattern": {"type": "string", "description": "Pattern to match file names against."},
                },
                "required": ["path", "pattern"],
            },
        },
        {
            "name": "write_file",
            "description": "Create or overwrite a file with the given contents.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Path of the file to write."},
                    "content": {"type": "string", "description": "Full contents to write to the file."},
                },
                "required": ["path", "content"],
            },
        },
        {
            "name": "move_file",
            "description": "Move or rename a file to a new location.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "source": {"type": "string", "description": "Path of the file to move."},
                    "destination": {"type": "string", "description": "New path for the file."},
                },
                "required": ["source", "destination"],
            },
        },
        {
            "name": "get_file_info",
            "description": "Get metadata about a file such as size and modification time.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Path of the file to inspect."},
                },
                "required": ["path"],
            },
        },
        {
            "name": "delete_file",
            "description": "Permanently delete a file.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Path of the file to delete."},
                },
                "required": ["path"],
            },
        },
    ],
    "postgres": [
        {
            "name": "list_schemas",
            "description": "List all schemas in the PostgreSQL database.",
            "inputSchema": {
                "type": "object",
                "properties": {},
                "required": [],
            },
        },
        {
            "name": "list_tables",
            "description": "List tables in a PostgreSQL schema.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "schema": {"type": "string", "description": "Schema name to inspect (defaults to public)."},
                },
                "required": [],
            },
        },
        {
            "name": "describe_table",
            "description": "Describe table columns, data types, and constraints.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "table_name": {"type": "string", "description": "Name of the table to describe."},
                    "schema": {"type": "string", "description": "Schema containing the table."},
                },
                "required": ["table_name"],
            },
        },
        {
            "name": "execute_query",
            "description": "Execute a data modification SQL query (INSERT, UPDATE, DELETE).",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "SQL write statement to execute."},
                },
                "required": ["query"],
            },
        },
        {
            "name": "read_query",
            "description": "Execute a read-only SQL query (SELECT).",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "SQL SELECT query to execute."},
                },
                "required": ["query"],
            },
        },
    ],
    "memory": [
        {
            "name": "create_entities",
            "description": "Create multiple new entities in the knowledge graph.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "entities": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "name": {"type": "string", "description": "Name of the entity."},
                                "entityType": {"type": "string", "description": "Type/category of the entity."},
                                "observations": {"type": "array", "items": {"type": "string"}, "description": "Observations or facts about the entity."},
                            },
                            "required": ["name", "entityType", "observations"],
                        },
                        "description": "List of entities to create.",
                    },
                },
                "required": ["entities"],
            },
        },
        {
            "name": "open_nodes",
            "description": "Open and retrieve specific nodes in the knowledge graph by name.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "names": {"type": "array", "items": {"type": "string"}, "description": "Names of nodes to retrieve."},
                },
                "required": ["names"],
            },
        },
        {
            "name": "create_relations",
            "description": "Create directional relations between existing entities in the graph.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "relations": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "from": {"type": "string", "description": "Source entity name."},
                                "to": {"type": "string", "description": "Target entity name."},
                                "relationType": {"type": "string", "description": "Relationship type label."},
                            },
                            "required": ["from", "to", "relationType"],
                        },
                        "description": "List of relations to establish.",
                    },
                },
                "required": ["relations"],
            },
        },
        {
            "name": "search_nodes",
            "description": "Search for nodes in the knowledge graph matching a query.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search term matching node names, types, or observations."},
                },
                "required": ["query"],
            },
        },
        {
            "name": "read_graph",
            "description": "Read the entire knowledge graph structure.",
            "inputSchema": {
                "type": "object",
                "properties": {},
                "required": [],
            },
        },
    ],
    "fetch": [
        {
            "name": "fetch",
            "description": "Fetch content from a web URL.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "url": {"type": "string", "description": "URL to fetch."},
                    "max_length": {"type": "integer", "description": "Maximum character length of content to return."},
                    "raw": {"type": "boolean", "description": "Return raw HTML instead of converted markdown text."},
                },
                "required": ["url"],
            },
        },
        {
            "name": "extract_markdown",
            "description": "Fetch and convert web page content to cleaned markdown.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "url": {"type": "string", "description": "URL of the page to extract markdown from."},
                },
                "required": ["url"],
            },
        },
        {
            "name": "extract_headers",
            "description": "Inspect HTTP response headers for a URL without downloading full body.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "url": {"type": "string", "description": "Target URL to inspect."},
                },
                "required": ["url"],
            },
        },
    ],
    "brave-search": [
        {
            "name": "brave_web_search",
            "description": "Performs a web search using the Brave Search API, returning relevant web pages.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query keywords."},
                    "count": {"type": "integer", "description": "Number of search results to return (default 10)."},
                    "offset": {"type": "integer", "description": "Pagination offset."},
                },
                "required": ["query"],
            },
        },
        {
            "name": "brave_local_search",
            "description": "Searches for local businesses and places using the Brave Local Search API.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Local search query (e.g. coffee shops near Seattle)."},
                    "count": {"type": "integer", "description": "Number of local results to return."},
                },
                "required": ["query"],
            },
        },
    ],
    "puppeteer": [
        {
            "name": "navigate",
            "description": "Navigate the browser to a specific URL.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "url": {"type": "string", "description": "URL to navigate to."},
                },
                "required": ["url"],
            },
        },
        {
            "name": "screenshot",
            "description": "Take a screenshot of the current page or a specific element.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Name or identifier for the screenshot image file."},
                    "width": {"type": "integer", "description": "Viewport width in pixels."},
                    "height": {"type": "integer", "description": "Viewport height in pixels."},
                },
                "required": ["name"],
            },
        },
        {
            "name": "click",
            "description": "Click an element on the page specified by a CSS selector.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "selector": {"type": "string", "description": "CSS selector of the element to click."},
                },
                "required": ["selector"],
            },
        },
        {
            "name": "fill",
            "description": "Fill out an input field on the page with text.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "selector": {"type": "string", "description": "CSS selector of the input element."},
                    "value": {"type": "string", "description": "Text value to fill into the input."},
                },
                "required": ["selector", "value"],
            },
        },
        {
            "name": "evaluate",
            "description": "Execute JavaScript code in the browser page context.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "script": {"type": "string", "description": "JavaScript code string to evaluate."},
                },
                "required": ["script"],
            },
        },
    ],
    "git": [
        {
            "name": "git_status",
            "description": "Show the working tree status.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "repo_path": {"type": "string", "description": "Path to the git repository."},
                },
                "required": [],
            },
        },
        {
            "name": "git_diff",
            "description": "Show changes between commits, commit and working tree, etc.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "repo_path": {"type": "string", "description": "Path to the git repository."},
                    "cached": {"type": "boolean", "description": "View staged changes."},
                    "target": {"type": "string", "description": "Target branch or commit SHA to compare against."},
                },
                "required": [],
            },
        },
        {
            "name": "git_commit",
            "description": "Record changes to the repository.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "message": {"type": "string", "description": "Commit message describing the changes."},
                    "repo_path": {"type": "string", "description": "Path to the git repository."},
                    "all": {"type": "boolean", "description": "Automatically stage modified and deleted files."},
                },
                "required": ["message"],
            },
        },
        {
            "name": "git_log",
            "description": "Show commit logs.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "repo_path": {"type": "string", "description": "Path to the git repository."},
                    "max_count": {"type": "integer", "description": "Maximum number of commit entries to return."},
                },
                "required": [],
            },
        },
        {
            "name": "git_branch",
            "description": "List, create, or inspect git branches.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "repo_path": {"type": "string", "description": "Path to the git repository."},
                    "branch_name": {"type": "string", "description": "Branch name to create or switch to."},
                },
                "required": [],
            },
        },
    ],
}


def fetch_from_stdio(command: str) -> List[Dict[str, Any]]:
    """Spawns an MCP server subprocess, executes handshake, and queries tools/list."""
    proc = subprocess.Popen(
        command,
        shell=True,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1,
    )
    if proc.stdin is None or proc.stdout is None:
        raise RuntimeError("Failed to open stdio pipes to MCP process")

    # Step 1: initialize handshake
    req_init = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "schema-fetcher", "version": "1.0.0"},
        },
    }
    proc.stdin.write(json.dumps(req_init) + "\n")
    proc.stdin.flush()

    line = proc.stdout.readline()
    if not line:
        proc.kill()
        raise RuntimeError("No response from MCP server on initialize")
    init_res = json.loads(line)
    if "error" in init_res:
        proc.kill()
        raise RuntimeError(f"Initialize error: {init_res['error']}")

    # Step 2: notifications/initialized
    notif = {"jsonrpc": "2.0", "method": "notifications/initialized"}
    proc.stdin.write(json.dumps(notif) + "\n")
    proc.stdin.flush()

    # Step 3: tools/list
    req_tools = {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}
    proc.stdin.write(json.dumps(req_tools) + "\n")
    proc.stdin.flush()

    line = proc.stdout.readline()
    if not line:
        proc.kill()
        raise RuntimeError("No response from MCP server on tools/list")
    tools_res = json.loads(line)
    proc.kill()

    if "error" in tools_res:
        raise RuntimeError(f"tools/list error: {tools_res['error']}")

    result = tools_res.get("result", {})
    return result.get("tools", [])


def export_schema(name: str, tools: List[Dict[str, Any]], out_path: Path) -> None:
    """Writes tool list to a pretty-printed JSON schema file."""
    out_path.parent.mkdir(parents=True, exist_ok=True)
    payload = {"tools": tools}
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)
        f.write("\n")
    print(f"  ✓ [{name}] wrote {len(tools)} tools to {out_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Fetch or export MCP tool schemas.")
    parser.add_argument("--all", action="store_true", help="Export all 11 popular MCP server schemas.")
    parser.add_argument("--server", type=str, choices=list(CANONICAL_SCHEMAS.keys()), help="Single server to export.")
    parser.add_argument("--cmd", type=str, help="Live MCP stdio command to query via JSON-RPC.")
    parser.add_argument("--output-dir", type=str, default="./suites/schemas", help="Output directory for schema files.")
    parser.add_argument("--out", type=str, help="Explicit output file path for single schema export.")

    args = parser.parse_args()

    out_dir = Path(args.output_dir)

    if args.cmd:
        out_file = Path(args.out) if args.out else out_dir / "custom.json"
        print(f"Querying live MCP server: {args.cmd}")
        try:
            tools = fetch_from_stdio(args.cmd)
            export_schema("live", tools, out_file)
        except Exception as e:
            print(f"Error querying live MCP server: {e}", file=sys.stderr)
            sys.exit(1)
        return

    if args.all or (not args.server and not args.cmd):
        print(f"Exporting all {len(CANONICAL_SCHEMAS)} canonical MCP schemas to {out_dir} and internal/suite/schemas/...")
        internal_dir = Path("./internal/suite/schemas")
        for name, tools in CANONICAL_SCHEMAS.items():
            export_schema(name, tools, out_dir / f"{name}.json")
            export_schema(name, tools, internal_dir / f"{name}.json")
        print("\nAll MCP schemas successfully exported.")
        return

    if args.server:
        tools = CANONICAL_SCHEMAS[args.server]
        out_file = Path(args.out) if args.out else out_dir / f"{args.server}.json"
        export_schema(args.server, tools, out_file)
        return


if __name__ == "__main__":
    main()
