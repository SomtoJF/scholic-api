package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"scholic/internal/database"
	"scholic/internal/models"

	"gorm.io/gorm"
)

type SearchChatHistoryInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
	Limit      uint   `json:"limit"`
	Offset     uint   `json:"offset"`
}

type GetChatHistoryRangeInput struct {
	Start int  `json:"start"`
	Limit uint `json:"limit"`
}

type SearchAnnotationsInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
	Limit      uint   `json:"limit"`
	Offset     uint   `json:"offset"`
}

type SearchPageContentInput struct {
	Query      string `json:"query"`
	SearchText string `json:"searchText"`
}

type CommentOnAnnotationInput struct {
	AnnotationID uint   `json:"annotationId"`
	Text         string `json:"text"`
}

func SearchChatHistoryTool() Tool {
	return FuncTool{ToolDefinition: ToolDefinition{Name: "SearchChatHistory", Description: "Search this chat's messages using PostgreSQL full text search.", Arguments: SearchChatHistoryInput{}}, Handler: searchChatHistory}
}

func GetChatHistoryRangeTool() Tool {
	return FuncTool{ToolDefinition: ToolDefinition{Name: "GetChatHistoryRange", Description: "Read a message range. Start -1 returns the latest messages.", Arguments: GetChatHistoryRangeInput{}}, Handler: getChatHistoryRange}
}

func SearchAnnotationsTool() Tool {
	return FuncTool{ToolDefinition: ToolDefinition{Name: "SearchAnnotations", Description: "Search annotations visible to the user using full text search.", Arguments: SearchAnnotationsInput{}}, Handler: searchAnnotations}
}

func SearchPageContentTool() Tool {
	return FuncTool{ToolDefinition: ToolDefinition{Name: "SearchPageContent", Description: "Ask the browser frontend to search the current page.", Arguments: SearchPageContentInput{}}}
}

func CommentOnAnnotationTool(handler func(context.Context, ToolContext, json.RawMessage) (any, error)) Tool {
	if handler == nil {
		handler = func(context.Context, ToolContext, json.RawMessage) (any, error) {
			return nil, errors.New("comment tool is not configured")
		}
	}
	return FuncTool{ToolDefinition: ToolDefinition{Name: "CommentOnAnnotation", Description: "Comment on an annotation.", Arguments: CommentOnAnnotationInput{}}, Handler: handler}
}

func searchChatHistory(ctx context.Context, input ToolContext, raw json.RawMessage) (any, error) {
	var request SearchChatHistoryInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	request.Limit, request.Offset = normalizePagination(request.Limit, request.Offset)
	if strings.TrimSpace(request.SearchText) == "" {
		return nil, errors.New("searchText is required")
	}
	var messages []models.Message
	err := database.DB.WithContext(ctx).
		Where("chat_id = ?", input.ChatID).
		Where("to_tsvector('english', message_text) @@ plainto_tsquery('english', ?)", request.SearchText).
		Order("created_at ASC, id ASC").Limit(int(request.Limit)).Offset(int(request.Offset)).Find(&messages).Error
	return messages, err
}

func getChatHistoryRange(ctx context.Context, input ToolContext, raw json.RawMessage) (any, error) {
	var request GetChatHistoryRangeInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.Limit == 0 {
		request.Limit = 10
	}
	if request.Limit > 100 {
		request.Limit = 100
	}
	query := database.DB.WithContext(ctx).Where("chat_id = ?", input.ChatID)
	var messages []models.Message
	if request.Start == -1 {
		err := query.Order("created_at DESC, id DESC").Limit(int(request.Limit)).Find(&messages).Error
		reverseMessages(messages)
		return messages, err
	}
	if request.Start < -1 {
		return nil, errors.New("start must be -1 or greater")
	}
	err := query.Order("created_at ASC, id ASC").Offset(request.Start).Limit(int(request.Limit)).Find(&messages).Error
	return messages, err
}

func searchAnnotations(ctx context.Context, input ToolContext, raw json.RawMessage) (any, error) {
	var request SearchAnnotationsInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	request.Limit, request.Offset = normalizePagination(request.Limit, request.Offset)
	if strings.TrimSpace(request.SearchText) == "" {
		return nil, errors.New("searchText is required")
	}

	query := database.DB.WithContext(ctx).Model(&models.Annotation{}).
		Where("(is_private = false OR user_id = ?)", input.UserID).
		Where("(to_tsvector('english', coalesce(quote_text, '')) @@ plainto_tsquery('english', ?) OR to_tsvector('english', coalesce(text, '')) @@ plainto_tsquery('english', ?))", request.SearchText, request.SearchText)
	if input.PageURL != "" {
		query = query.Where("url = ?", input.PageURL)
	}
	var annotations []models.Annotation
	err := query.Order("created_at DESC, id DESC").Limit(int(request.Limit)).Offset(int(request.Offset)).Find(&annotations).Error
	return annotations, err
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

func reverseMessages(messages []models.Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func LoadRecentChatMessages(ctx context.Context, chatID uint, limit int) ([]ContextMessage, error) {
	if limit <= 0 {
		return nil, nil
	}
	var messages []models.Message
	err := database.DB.WithContext(ctx).Where("chat_id = ?", chatID).Order("created_at DESC, id DESC").Limit(limit).Find(&messages).Error
	if err != nil {
		return nil, err
	}
	reverseMessages(messages)
	result := make([]ContextMessage, 0, len(messages))
	for _, message := range messages {
		role := "assistant"
		if message.IsUserMessage {
			role = "user"
		}
		result = append(result, ContextMessage{Role: role, Content: message.MessageText})
	}
	return result, nil
}

func ValidateChatExists(ctx context.Context, chatID uint) error {
	var chat models.Chat
	if err := database.DB.WithContext(ctx).First(&chat, chatID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("chat %d not found", chatID)
		}
		return err
	}
	return nil
}
