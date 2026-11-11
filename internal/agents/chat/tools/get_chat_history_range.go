package tools

import (
	"context"
	"encoding/json"
	"errors"

	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"
)

type GetChatHistoryRangeInput struct {
	Start int  `json:"start"`
	Limit uint `json:"limit"`
}

func GetChatHistoryRangeTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "GetChatHistoryRange", Description: "Read a message range. Start -1 returns the latest messages.", Arguments: GetChatHistoryRangeInput{}}, Handler: getChatHistoryRange}
}
func getChatHistoryRange(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
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
func reverseMessages(messages []models.Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}
