package suite

import (
	"can-i-compress-it/internal/domain"
)

func pgListSchemas() domain.ToolDefinition {
	t := tool("list_schemas", "List all schemas in the PostgreSQL database.")
	t.Parameters = params(nil)
	return t
}

func pgListTables() domain.ToolDefinition {
	t := tool("list_tables", "List tables in a PostgreSQL schema.")
	t.Parameters = params(map[string]any{
		"schema": obj("string", "Schema name to inspect (defaults to public)."),
	})
	return t
}

func pgDescribeTable() domain.ToolDefinition {
	t := tool("describe_table", "Describe table columns, data types, and constraints.")
	t.Parameters = params(map[string]any{
		"table_name": obj("string", "Name of the table to describe."),
		"schema":     obj("string", "Schema containing the table."),
	})
	return t
}

func pgExecuteQuery() domain.ToolDefinition {
	t := tool("execute_query", "Execute a data modification SQL query (INSERT, UPDATE, DELETE).")
	t.Parameters = params(map[string]any{
		"query": obj("string", "SQL write statement to execute."),
	})
	return t
}

func pgReadQuery() domain.ToolDefinition {
	t := tool("read_query", "Execute a read-only SQL query (SELECT).")
	t.Parameters = params(map[string]any{
		"query": obj("string", "SQL SELECT query to execute."),
	})
	return t
}

func postgresTasks() []domain.Task {
	tools := embeddedToolsMap("postgres")
	listSchemas := getOrFallbackTool(tools, "list_schemas", pgListSchemas())
	listTables := getOrFallbackTool(tools, "list_tables", pgListTables())
	describeTable := getOrFallbackTool(tools, "describe_table", pgDescribeTable())
	executeQuery := getOrFallbackTool(tools, "execute_query", pgExecuteQuery())
	readQuery := getOrFallbackTool(tools, "read_query", pgReadQuery())

	return []domain.Task{
		{
			ID:       "pg_list_schemas_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_schemas exactly once with no arguments.",
			Tools:    []domain.ToolDefinition{listSchemas, listTables},
			Expected: expected("list_schemas", map[string]any{}),
		},
		{
			ID:       "pg_list_tables_001",
			Category: domain.CategoryImported,
			Prompt:   "Call list_tables exactly once. Arguments: schema: analytics.",
			Tools:    []domain.ToolDefinition{listTables, describeTable},
			Expected: expected("list_tables", map[string]any{"schema": "analytics"}),
		},
		{
			ID:       "pg_describe_table_001",
			Category: domain.CategoryImported,
			Prompt:   "Call describe_table exactly once. Arguments: table_name: users, schema: public.",
			Tools:    []domain.ToolDefinition{describeTable, readQuery},
			Expected: expected("describe_table", map[string]any{"schema": "public", "table_name": "users"}),
		},
		{
			ID:       "pg_read_query_001",
			Category: domain.CategoryImported,
			Prompt:   "Call read_query exactly once. Arguments: query: SELECT id, email FROM users WHERE active = true.",
			Tools:    []domain.ToolDefinition{readQuery, executeQuery, describeTable},
			Expected: expected("read_query", map[string]any{"query": "SELECT id, email FROM users WHERE active = true"}),
		},
		{
			ID:       "pg_execute_query_001",
			Category: domain.CategoryImported,
			Prompt:   "Call execute_query exactly once. Arguments: query: UPDATE users SET active = false WHERE id = 42.",
			Tools:    []domain.ToolDefinition{executeQuery, readQuery},
			Expected: expected("execute_query", map[string]any{"query": "UPDATE users SET active = false WHERE id = 42"}),
		},
		{
			ID:       "pg_chain_inspect_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call list_tables. Arguments: schema: public. Step 2: call describe_table. Arguments: schema: public, table_name: <from:list_tables.table_name>.",
			Tools:    []domain.ToolDefinition{listTables, describeTable, readQuery},
			Expected: expected("list_tables", map[string]any{"schema": "public"}),
			MultiHop: []domain.ExpectedCall{
				expected("list_tables", map[string]any{"schema": "public"}),
				expected("describe_table", map[string]any{
					"schema":     "public",
					"table_name": "<from:list_tables.table_name>",
				}),
			},
		},
	}
}
