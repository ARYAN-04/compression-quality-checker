package suite

import (
	"can-i-compress-it/internal/domain"
)

func braveWebSearch() domain.ToolDefinition {
	t := tool("brave_web_search", "Performs a web search using the Brave Search API, returning relevant web pages.")
	t.Parameters = params(map[string]any{
		"query":  obj("string", "Search query keywords."),
		"count":  obj("integer", "Number of search results to return (default 10)."),
		"offset": obj("integer", "Pagination offset."),
	})
	return t
}

func braveLocalSearch() domain.ToolDefinition {
	t := tool("brave_local_search", "Searches for local businesses and places using the Brave Local Search API.")
	t.Parameters = params(map[string]any{
		"query": obj("string", "Local search query (e.g. coffee shops near Seattle)."),
		"count": obj("integer", "Number of local results to return."),
	})
	return t
}

func braveSearchTasks() []domain.Task {
	tools := embeddedToolsMap("brave-search")
	webSearch := getOrFallbackTool(tools, "brave_web_search", braveWebSearch())
	localSearch := getOrFallbackTool(tools, "brave_local_search", braveLocalSearch())

	return []domain.Task{
		{
			ID:       "search_web_001",
			Category: domain.CategoryImported,
			Prompt:   "Call brave_web_search exactly once. Arguments: query: context compression benchmarks LLM.",
			Tools:    []domain.ToolDefinition{webSearch, localSearch},
			Expected: expected("brave_web_search", map[string]any{"query": "context compression benchmarks LLM"}),
		},
		{
			ID:       "search_local_001",
			Category: domain.CategoryImported,
			Prompt:   "Call brave_local_search exactly once. Arguments: query: bookstore downtown Seattle.",
			Tools:    []domain.ToolDefinition{localSearch, webSearch},
			Expected: expected("brave_local_search", map[string]any{"query": "bookstore downtown Seattle"}),
		},
		{
			ID:       "search_chain_refine_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call brave_web_search. Arguments: query: nearest observatory coordinates. Step 2: call brave_local_search. Arguments: query: <from:brave_web_search.best_location>.",
			Tools:    []domain.ToolDefinition{webSearch, localSearch},
			Expected: expected("brave_web_search", map[string]any{"query": "nearest observatory coordinates"}),
			MultiHop: []domain.ExpectedCall{
				expected("brave_web_search", map[string]any{"query": "nearest observatory coordinates"}),
				expected("brave_local_search", map[string]any{
					"query": "<from:brave_web_search.best_location>",
				}),
			},
		},
	}
}
