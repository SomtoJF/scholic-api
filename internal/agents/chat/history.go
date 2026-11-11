package chat

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"
)

func LoadRecentChatMessages(ctx context.Context, chatID uint, limit int) ([]agents.ContextMessage, error) {
	if limit <= 0 {
		return nil, nil
	}
	var messages []models.Message
	if err := database.DB.WithContext(ctx).Where("chat_id = ?", chatID).Order("created_at DESC, id DESC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, err
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	result := make([]agents.ContextMessage, 0, len(messages))
	for _, message := range messages {
		role := "assistant"
		if message.IsUserMessage {
			role = "user"
		}
		result = append(result, agents.ContextMessage{Role: role, Content: message.MessageText})
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
