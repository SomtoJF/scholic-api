package tools

import "scholic/internal/agents"

type SearchPageContentInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
}

func SearchPageContentTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "SearchPageContent", Description: "Ask the browser frontend to search the current page.", Arguments: SearchPageContentInput{}}}
}
