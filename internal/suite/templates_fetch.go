package suite

import (
	"can-i-compress-it/internal/domain"
)

func fetchTool() domain.ToolDefinition {
	t := tool("fetch", "Fetch content from a web URL.")
	t.Parameters = params(map[string]any{
		"url":        obj("string", "URL to fetch."),
		"max_length": obj("integer", "Maximum character length of content to return."),
		"raw":        obj("boolean", "Return raw HTML instead of converted markdown text."),
	})
	return t
}

func fetchExtractMarkdown() domain.ToolDefinition {
	t := tool("extract_markdown", "Fetch and convert web page content to cleaned markdown.")
	t.Parameters = params(map[string]any{
		"url": obj("string", "URL of the page to extract markdown from."),
	})
	return t
}

func fetchExtractHeaders() domain.ToolDefinition {
	t := tool("extract_headers", "Inspect HTTP response headers for a URL without downloading full body.")
	t.Parameters = params(map[string]any{
		"url": obj("string", "Target URL to inspect."),
	})
	return t
}

func fetchTasks() []domain.Task {
	fetch := fetchTool()
	extractMarkdown := fetchExtractMarkdown()
	extractHeaders := fetchExtractHeaders()

	return []domain.Task{
		{
			ID:       "fetch_url_001",
			Category: domain.CategoryImported,
			Prompt:   "Call fetch exactly once. Arguments: url: https://docs.anthropic.com/en/docs/overview.",
			Tools:    []domain.ToolDefinition{fetch, extractMarkdown},
			Expected: expected("fetch", map[string]any{"url": "https://docs.anthropic.com/en/docs/overview"}),
		},
		{
			ID:       "fetch_markdown_001",
			Category: domain.CategoryImported,
			Prompt:   "Call extract_markdown exactly once. Arguments: url: https://modelcontextprotocol.io/introduction.",
			Tools:    []domain.ToolDefinition{extractMarkdown, fetch},
			Expected: expected("extract_markdown", map[string]any{"url": "https://modelcontextprotocol.io/introduction"}),
		},
		{
			ID:       "fetch_headers_001",
			Category: domain.CategoryImported,
			Prompt:   "Call extract_headers exactly once. Arguments: url: https://api.github.com.",
			Tools:    []domain.ToolDefinition{extractHeaders, fetch},
			Expected: expected("extract_headers", map[string]any{"url": "https://api.github.com"}),
		},
		{
			ID:       "fetch_chain_verify_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call extract_headers. Arguments: url: https://example.org. Step 2: call extract_markdown. Arguments: url: <from:extract_headers.final_url>.",
			Tools:    []domain.ToolDefinition{extractHeaders, extractMarkdown, fetch},
			Expected: expected("extract_headers", map[string]any{"url": "https://example.org"}),
			MultiHop: []domain.ExpectedCall{
				expected("extract_headers", map[string]any{"url": "https://example.org"}),
				expected("extract_markdown", map[string]any{
					"url": "<from:extract_headers.final_url>",
				}),
			},
		},
	}
}
