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

type SearchAnnotationsInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
	Limit      uint   `json:"limit"`
	Offset     uint   `json:"offset"`
}

func SearchAnnotationsTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "SearchAnnotations", Description: "Search annotations visible to the user using full text search.", Arguments: SearchAnnotationsInput{}}, Handler: searchAnnotations}
}
func searchAnnotations(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request SearchAnnotationsInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	request.Limit, request.Offset = normalizePagination(request.Limit, request.Offset)
	if strings.TrimSpace(request.SearchText) == "" {
		return nil, errors.New("searchText is required")
	}
	query := database.DB.WithContext(ctx).Model(&models.Annotation{}).Where("(is_private = false OR user_id = ?)", input.UserID).Where("(to_tsvector('english', coalesce(quote_text, '')) @@ plainto_tsquery('english', ?) OR to_tsvector('english', coalesce(text, '')) @@ plainto_tsquery('english', ?))", request.SearchText, request.SearchText)
	if input.PageURL != "" {
		query = query.Where("url = ?", input.PageURL)
	}
	var annotations []models.Annotation
	if err := query.Order("created_at DESC, id DESC").Limit(int(request.Limit)).Offset(int(request.Offset)).Find(&annotations).Error; err != nil {
		return nil, err
	}
	return filterRelevant(ctx, input.Provider, request.Query, request.SearchText, annotations)
}
