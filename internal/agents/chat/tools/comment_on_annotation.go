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

type CommentOnAnnotationInput struct {
	AnnotationID uint   `json:"annotationId"`
	Text         string `json:"text"`
}

func CommentOnAnnotationTool() agents.Tool {
	return agents.FuncTool{ToolDefinition: agents.ToolDefinition{Name: "CommentOnAnnotation", Description: "Comment on an annotation.", Arguments: CommentOnAnnotationInput{}}, Handler: commentOnAnnotation}
}
func commentOnAnnotation(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
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
