package tools

import "scholic/internal/agents"

type SearchPageContentInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
	Limit      uint   `json:"limit"`
	Offset     uint   `json:"offset"`
}

func SearchPageContentTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "SearchPageContent", Description: "Use for questions about the current webpage, especially open-ended questions about webpage content; for open-ended questions, the current page is the primary source of truth. Set query to one free-text sentence describing what you are looking for, and set searchText to the exact word or phrase to find on the page. Use distinctive terms or phrases likely to appear in the target content; avoid filler-only searches such as 'the' or 'and'. Results preserve logical page blocks and nearby lines and may be dense. For broad questions, prefer several focused searches over one broad search, and use offset to paginate matching results when needed. Tool results persist only during the current agent run, not across later user turns. For uncertain open-ended questions, search the page before relying on annotations.", Arguments: SearchPageContentInput{}}}
}
