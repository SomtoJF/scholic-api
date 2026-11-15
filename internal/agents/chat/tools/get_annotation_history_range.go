package tools

import (
	"context"
	"encoding/json"
	"errors"

	"scholic/internal/agents"
	"scholic/internal/database"
	"scholic/internal/models"
)

type GetAnnotationHistoryRangeInput struct {
	Start int  `json:"start"`
	Limit uint `json:"limit"`
}

func GetAnnotationHistoryRangeTool() agents.Tool {
	return agents.FuncTool{
		ToolDefinition: agents.ToolDefinition{
			Name:        "GetAnnotationHistoryRange",
			Description: "Read annotations on the current page. Start -1 returns the latest annotations.",
			Arguments:   GetAnnotationHistoryRangeInput{},
		},
		Handler: getAnnotationHistoryRange,
	}
}

func getAnnotationHistoryRange(ctx context.Context, input agents.ToolContext, raw json.RawMessage) (any, error) {
	var request GetAnnotationHistoryRangeInput
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.Start < -1 {
		return nil, errors.New("start must be -1 or greater")
	}
	request.Limit = normalizeAnnotationHistoryLimit(request.Limit)

	query := database.DB.WithContext(ctx).
		Model(&models.Annotation{}).
		Where("url = ?", input.PageURL).
		Where("(is_private = false OR user_id = ?)", input.UserID)

	var annotations []models.Annotation
	if request.Start == -1 {
		err := query.Order("created_at DESC, id DESC").Limit(int(request.Limit)).Find(&annotations).Error
		reverseAnnotations(annotations)
		return annotations, err
	}

	err := query.
		Order("created_at ASC, id ASC").
		Offset(request.Start).
		Limit(int(request.Limit)).
		Find(&annotations).Error
	return annotations, err
}

func normalizeAnnotationHistoryLimit(limit uint) uint {
	if limit == 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func reverseAnnotations(annotations []models.Annotation) {
	for left, right := 0, len(annotations)-1; left < right; left, right = left+1, right-1 {
		annotations[left], annotations[right] = annotations[right], annotations[left]
	}
}
