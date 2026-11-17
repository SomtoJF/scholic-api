package tools

import (
	"context"
	"encoding/json"
	"testing"

	"scholic/internal/agents"
	"scholic/internal/database"
)

func TestGetAnnotationHistoryRangeToolDefinition(t *testing.T) {
	definition := GetAnnotationHistoryRangeTool().Definition()
	if definition.Name != "GetAnnotationHistoryRange" {
		t.Fatalf("unexpected tool name: %q", definition.Name)
	}
	if _, ok := definition.Arguments.(GetAnnotationHistoryRangeInput); !ok {
		t.Fatalf("unexpected argument schema type: %T", definition.Arguments)
	}
}

func TestGetAnnotationHistoryRangeRejectsInvalidStart(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = oldDB })

	_, err := getAnnotationHistoryRange(context.Background(), agents.ToolContext{}, json.RawMessage(`{"start":-2}`))
	if err == nil || err.Error() != "start must be -1 or greater" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAnnotationHistoryLimit(t *testing.T) {
	for limit, want := range map[uint]uint{0: 10, 1: 1, 100: 100, 101: 100} {
		if got := normalizeAnnotationHistoryLimit(limit); got != want {
			t.Errorf("limit %d: got %d, want %d", limit, got, want)
		}
	}
}
