package suite

import (
	"can-i-compress-it/internal/domain"
)

func pupNavigate() domain.ToolDefinition {
	t := tool("navigate", "Navigate the browser to a specific URL.")
	t.Parameters = params(map[string]any{
		"url": obj("string", "URL to navigate to."),
	})
	return t
}

func pupScreenshot() domain.ToolDefinition {
	t := tool("screenshot", "Take a screenshot of the current page or a specific element.")
	t.Parameters = params(map[string]any{
		"name":   obj("string", "Name or identifier for the screenshot image file."),
		"width":  obj("integer", "Viewport width in pixels."),
		"height": obj("integer", "Viewport height in pixels."),
	})
	return t
}

func pupClick() domain.ToolDefinition {
	t := tool("click", "Click an element on the page specified by a CSS selector.")
	t.Parameters = params(map[string]any{
		"selector": obj("string", "CSS selector of the element to click."),
	})
	return t
}

func pupFill() domain.ToolDefinition {
	t := tool("fill", "Fill out an input field on the page with text.")
	t.Parameters = params(map[string]any{
		"selector": obj("string", "CSS selector of the input element."),
		"value":    obj("string", "Text value to fill into the input."),
	})
	return t
}

func pupEvaluate() domain.ToolDefinition {
	t := tool("evaluate", "Execute JavaScript code in the browser page context.")
	t.Parameters = params(map[string]any{
		"script": obj("string", "JavaScript code string to evaluate."),
	})
	return t
}

func puppeteerTasks() []domain.Task {
	navigate := pupNavigate()
	screenshot := pupScreenshot()
	click := pupClick()
	fill := pupFill()
	evaluate := pupEvaluate()

	return []domain.Task{
		{
			ID:       "pup_navigate_001",
			Category: domain.CategoryImported,
			Prompt:   "Call navigate exactly once. Arguments: url: https://dashboard.example.com.",
			Tools:    []domain.ToolDefinition{navigate, screenshot},
			Expected: expected("navigate", map[string]any{"url": "https://dashboard.example.com"}),
		},
		{
			ID:       "pup_screenshot_001",
			Category: domain.CategoryImported,
			Prompt:   "Call screenshot exactly once. Arguments: name: login_page.png.",
			Tools:    []domain.ToolDefinition{screenshot, navigate},
			Expected: expected("screenshot", map[string]any{"name": "login_page.png"}),
		},
		{
			ID:       "pup_click_001",
			Category: domain.CategoryImported,
			Prompt:   "Call click exactly once. Arguments: selector: button#submit.",
			Tools:    []domain.ToolDefinition{click, fill},
			Expected: expected("click", map[string]any{"selector": "button#submit"}),
		},
		{
			ID:       "pup_fill_001",
			Category: domain.CategoryImported,
			Prompt:   "Call fill exactly once. Arguments: selector: input#username, value: admin.",
			Tools:    []domain.ToolDefinition{fill, click},
			Expected: expected("fill", map[string]any{"selector": "input#username", "value": "admin"}),
		},
		{
			ID:       "pup_evaluate_001",
			Category: domain.CategoryImported,
			Prompt:   "Call evaluate exactly once. Arguments: script: document.title.",
			Tools:    []domain.ToolDefinition{evaluate, screenshot},
			Expected: expected("evaluate", map[string]any{"script": "document.title"}),
		},
		{
			ID:       "pup_chain_flow_001",
			Category: domain.CategoryMultiHop,
			Prompt:   "Step 1: call navigate. Arguments: url: https://app.example.com. Step 2: call fill. Arguments: selector: #search, value: reports. Step 3: call click. Arguments: selector: <from:fill.next_selector>.",
			Tools:    []domain.ToolDefinition{navigate, fill, click, screenshot},
			Expected: expected("navigate", map[string]any{"url": "https://app.example.com"}),
			MultiHop: []domain.ExpectedCall{
				expected("navigate", map[string]any{"url": "https://app.example.com"}),
				expected("fill", map[string]any{
					"selector": "#search",
					"value":    "reports",
				}),
				expected("click", map[string]any{
					"selector": "<from:fill.next_selector>",
				}),
			},
		},
	}
}
