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

type PageSearchResult struct {
	Query   string            `json:"query"`
	Matches []PageSearchMatch `json:"matches"`
}

type PageSearchMatch struct {
	Text    string `json:"text"`
	Context string `json:"context"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
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

func CommentOnAnnotationTool() Tool {
	return FuncTool{ToolDefinition: ToolDefinition{Name: "CommentOnAnnotation", Description: "Comment on an annotation.", Arguments: CommentOnAnnotationInput{}}, Handler: commentOnAnnotation}
}

func commentOnAnnotation(ctx context.Context, input ToolContext, raw json.RawMessage) (any, error) {
	var request CommentOnAnnotationInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.AnnotationID == 0 {
		return nil, errors.New("annotationId is required")
	}
	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return nil, errors.New("text is required")
	}

	db := database.DB.WithContext(ctx)
	var annotation models.Annotation
	if err := db.First(&annotation, request.AnnotationID).Error; err != nil {
		return nil, err
	}
	comment := models.Comment{AnnotationID: annotation.ID, UserID: input.UserID, Text: request.Text}
	if err := db.Create(&comment).Error; err != nil {
		return nil, err
	}
	if err := db.Preload("User").First(&comment, comment.ID).Error; err != nil {
		return nil, err
	}
	return comment, nil
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
	if err != nil {
		return nil, err
	}
	return filterRelevant(ctx, input.Provider, request.Query, request.SearchText, messages)
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
	if err != nil {
		return nil, err
	}
	return filterRelevant(ctx, input.Provider, request.Query, request.SearchText, annotations)
}

type relevanceCandidate struct {
	Key   string
	Value any
}

func filterRelevant(ctx context.Context, provider LLMProvider, query, searchText string, values any) (any, error) {
	candidates := make([]relevanceCandidate, 0)
	switch typed := values.(type) {
	case []models.Message:
		for i, value := range typed {
			candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: map[string]any{"messageText": value.MessageText, "isUserMessage": value.IsUserMessage, "createdAt": value.CreatedAt}})
		}
	case []models.Annotation:
		for i, value := range typed {
			candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: map[string]any{"quoteText": value.QuoteText, "text": value.Text, "url": value.URL}})
		}
	default:
		return nil, errors.New("unsupported search result type")
	}
	if len(candidates) == 0 {
		switch typed := values.(type) {
		case []models.Message:
			return typed, nil
		case []models.Annotation:
			return typed, nil
		}
	}
	if provider == nil {
		return nil, errors.New("LLM provider is required for search relevance")
	}
	keep, err := decideRelevant(ctx, provider, query, searchText, candidates)
	if err != nil {
		return nil, err
	}
	switch typed := values.(type) {
	case []models.Message:
		filtered := make([]models.Message, 0, len(typed))
		for i, value := range typed {
			if keep[i] {
				filtered = append(filtered, value)
			}
		}
		return filtered, nil
	case []models.Annotation:
		filtered := make([]models.Annotation, 0, len(typed))
		for i, value := range typed {
			if keep[i] {
				filtered = append(filtered, value)
			}
		}
		return filtered, nil
	default:
		panic("unreachable")
	}
}

func decideRelevant(ctx context.Context, provider LLMProvider, query, searchText string, candidates []relevanceCandidate) ([]bool, error) {
	keep := make([]bool, len(candidates))
	if len(candidates) == 0 {
		return keep, nil
	}
	questions := make(map[string]DecisionQuestion, len(candidates))
	stateCandidates := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		questions[candidate.Key] = DecisionQuestion{
			Type:         "noul",
			Instructions: "Is this search result relevant to the user's query?",
			Criteria: map[string]any{
				"true":  "The result directly helps answer the query.",
				"false": "The result is irrelevant or does not help answer the query.",
			},
		}
		stateCandidates = append(stateCandidates, map[string]any{"id": candidate.Key, "result": candidate.Value})
	}
	response, err := provider.GetDecisions(ctx, DecisionsRequest{
		Model:     DecisionsModel,
		State:     map[string]any{"query": query, "searchText": searchText, "candidates": stateCandidates},
		Questions: questions,
	})
	if err != nil {
		return nil, err
	}
	for i, candidate := range candidates {
		answer, ok := response.Answers[candidate.Key]
		if !ok || answer.Type != "noul" && answer.Type != "" {
			return nil, fmt.Errorf("missing or invalid relevance decision for %s", candidate.Key)
		}
		keep[i] = answer.IsTrue()
	}
	return keep, nil
}

func filterPageSearchResult(ctx context.Context, provider LLMProvider, query string, output any) (any, error) {
	var result PageSearchResult
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal page search result: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode page search result: %w", err)
	}
	candidates := make([]relevanceCandidate, 0, len(result.Matches))
	for i, match := range result.Matches {
		candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: match})
	}
	keep, err := decideRelevant(ctx, provider, query, "", candidates)
	if err != nil {
		return nil, err
	}
	filtered := result.Matches[:0]
	for i, match := range result.Matches {
		if keep[i] {
			filtered = append(filtered, match)
		}
	}
	result.Matches = filtered
	return result, nil
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
