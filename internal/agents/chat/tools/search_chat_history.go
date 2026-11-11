package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"
)

type SearchChatHistoryInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
	Limit      uint   `json:"limit"`
	Offset     uint   `json:"offset"`
}

func SearchChatHistoryTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "SearchChatHistory", Description: "Search this chat's messages using PostgreSQL full text search.", Arguments: SearchChatHistoryInput{}}, Handler: searchChatHistory}
}
func searchChatHistory(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request SearchChatHistoryInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	request.Limit, request.Offset = normalizePagination(request.Limit, request.Offset)
	if strings.TrimSpace(request.SearchText) == "" {
		return nil, errors.New("searchText is required")
	}
	var messages []models.Message
	err := database.DB.WithContext(ctx).Where("chat_id = ?", input.ChatID).Where("to_tsvector('english', message_text) @@ plainto_tsquery('english', ?)", request.SearchText).Order("created_at ASC, id ASC").Limit(int(request.Limit)).Offset(int(request.Offset)).Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return filterRelevant(ctx, input.Provider, request.Query, request.SearchText, messages)
}

func normalizePagination(limit, offset uint) (uint, uint) {
	if limit == 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	return limit, offset
}
