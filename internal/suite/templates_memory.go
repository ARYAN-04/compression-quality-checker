package suite

import (
	"can-i-compress-it/internal/domain"
)

func memCreateEntities() domain.ToolDefinition {
	t := tool("create_entities", "Create multiple new entities in the knowledge graph.")
	t.Parameters = params(map[string]any{
		"entities": obj("array", "List of entities to create."),
	})
	return t
}

func memOpenNodes() domain.ToolDefinition {
	t := tool("open_nodes", "Open and retrieve specific nodes in the knowledge graph by name.")
	t.Parameters = params(map[string]any{
		"names": obj("array", "Names of nodes to retrieve."),
	})
	return t
}

func memCreateRelations() domain.ToolDefinition {
	t := tool("create_relations", "Create directional relations between existing entities in the graph.")
	t.Parameters = params(map[string]any{
		"relations": obj("array", "List of relations to establish."),
	})
	return t
}

func memSearchNodes() domain.ToolDefinition {
	t := tool("search_nodes", "Search for nodes in the knowledge graph matching a query.")
	t.Parameters = params(map[string]any{
		"query": obj("string", "Search term matching node names or attributes."),
	})
	return t
}

func memReadGraph() domain.ToolDefinition {
	t := tool("read_graph", "Read the entire knowledge graph structure.")
	t.Parameters = params(nil)
	return t
}

func memoryTasks() []domain.Task {
	createEntities := memCreateEntities()
	openNodes := memOpenNodes()
	createRelations := memCreateRelations()
	searchNodes := memSearchNodes()
	readGraph := memReadGraph()

	return []domain.Task{
		{
			ID:       "mem_read_graph_001",
			Category: domain.CategoryImported,
			Prompt:   "Call read_graph exactly once with no arguments.",
			Tools:    []domain.ToolDefinition{readGraph, searchNodes},
			Expected: expected("read_graph", map[string]any{}),
		},
		{
			ID:       "mem_search_nodes_001",
			Category: domain.CategoryImported,
			Prompt:   "Call search_nodes exactly once. Arguments: query: authentication service.",
			Tools:    []domain.ToolDefinition{searchNodes, openNodes},
			Expected: expected("search_nodes", map[string]any{"query": "authentication service"}),
		},
		{
			ID:       "mem_open_nodes_001",
			Category: domain.CategoryImported,
			Prompt:   "Call open_nodes exactly once. Arguments: names: AuthServer.",
			Tools:    []domain.ToolDefinition{openNodes, searchNodes, createEntities},
			Expected: expected("open_nodes", map[string]any{"names": []any{"AuthServer"}}),
		},
		{
			ID:       "mem_create_entities_001",
			Category: domain.CategoryImported,
			Prompt:   "Call create_entities exactly once. Arguments: entities: AuthService.",
			Tools:    []domain.ToolDefinition{createEntities, createRelations},
			Expected: expected("create_entities", map[string]any{"entities": []any{"AuthService"}}),
		},
		{
			ID:       "mem_create_relations_001",
			Category: domain.CategoryImported,
			Prompt:   "Call create_relations exactly once. Arguments: relations: AuthToDB.",
			Tools:    []domain.ToolDefinition{createRelations, openNodes},
			Expected: expected("create_relations", map[string]any{"relations": []any{"AuthToDB"}}),
		},
		{
			ID:       "mem_chain_explore_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call search_nodes. Arguments: query: gateway. Step 2: call open_nodes. Arguments: names: <from:search_nodes.node_name>.",
			Tools:    []domain.ToolDefinition{searchNodes, openNodes, readGraph},
			Expected: expected("search_nodes", map[string]any{"query": "gateway"}),
			MultiHop: []domain.ExpectedCall{
				expected("search_nodes", map[string]any{"query": "gateway"}),
				expected("open_nodes", map[string]any{
					"names": "<from:search_nodes.node_name>",
				}),
			},
		},
	}
}
