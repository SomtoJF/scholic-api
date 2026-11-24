package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"

	"gorm.io/gorm"
)

type AddMemoryInput struct {
	Text   string `json:"text"`
	Global bool   `json:"global"`
}

type UpdateMemoryInput struct {
	ID   uint   `json:"id"`
	Text string `json:"text"`
}

type DeleteMemoryInput struct {
	ID uint `json:"id"`
}

func AddMemoryTool() agents.Tool {
	return agents.FuncTool{
		ToolDefinition: agents.ToolDefinition{
			Name:        "AddMemory",
			Description: "Save a memory for future use. Use this when the user gives a durable preference, instruction, or fact, especially language like 'never do...', 'always...', 'remember to...', or anything clearly meant to persist. Do not save ordinary one-off requests. text must be a concise, useful fact. Set global=true for use across chats; set global=false for this chat only. If the intended scope is uncertain, ask the user before calling this tool.",
			Arguments:   AddMemoryInput{},
		},
		Handler: addMemory,
	}
}

func UpdateMemoryTool() agents.Tool {
	return agents.FuncTool{
		ToolDefinition: agents.ToolDefinition{
			Name:        "UpdateMemory",
			Description: "Replace an existing memory by ID. This is a PUT operation: text completely overwrites the old text. Use only a memory owned by the user and visible in the current context.",
			Arguments:   UpdateMemoryInput{},
		},
		Handler: updateMemory,
	}
}

func DeleteMemoryTool() agents.Tool {
	return agents.FuncTool{
		ToolDefinition: agents.ToolDefinition{
			Name:        "DeleteMemory",
			Description: "Delete an existing memory by ID. Use only a memory owned by the user and visible in the current context. Ask for confirmation when the user's deletion request is ambiguous.",
			Arguments:   DeleteMemoryInput{},
		},
		Handler: deleteMemory,
	}
}

func addMemory(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request AddMemoryInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return nil, errors.New("text is required")
	}
	memory := models.Memory{UserID: input.UserID, Text: request.Text}
	if !request.Global {
		if input.ChatID == 0 {
			return nil, errors.New("chat context is required for chat-scoped memory")
		}
		memory.ChatID = &input.ChatID
	}
	if err := database.DB.WithContext(ctx).Create(&memory).Error; err != nil {
		return nil, err
	}
	return memory, nil
}

func updateMemory(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request UpdateMemoryInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.ID == 0 {
		return nil, errors.New("id is required")
	}
	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return nil, errors.New("text is required")
	}
	var memory models.Memory
	if err := memoryQuery(ctx, input).First(&memory, request.ID).Error; err != nil {
		return nil, err
	}
	memory.Text = request.Text
	if err := database.DB.WithContext(ctx).Save(&memory).Error; err != nil {
		return nil, err
	}
	return memory, nil
}

func deleteMemory(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request DeleteMemoryInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.ID == 0 {
		return nil, errors.New("id is required")
	}
	var memory models.Memory
	if err := memoryQuery(ctx, input).First(&memory, request.ID).Error; err != nil {
		return nil, err
	}
	if err := database.DB.WithContext(ctx).Delete(&memory).Error; err != nil {
		return nil, err
	}
	return map[string]any{"id": request.ID, "deleted": true}, nil
}

func memoryQuery(ctx context.Context, input agents.ToolContext) *gorm.DB {
	return database.DB.WithContext(ctx).
		Where("user_id = ? AND (chat_id IS NULL OR chat_id = ?)", input.UserID, input.ChatID)
}
