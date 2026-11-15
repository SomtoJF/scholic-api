package prompts

import (
	"strings"
	"testing"

	"scholic/internal/agents"
)

func TestRenderSystemPromptIncludesSearchDiscoveryGuidance(t *testing.T) {
	rendered, err := RenderSystemPrompt([]agents.ToolDefinition{{Name: "SearchPageContent"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"iterative discovery",
		"focused searches with distinctive terms",
		"focused follow-up search",
		"page content, annotations, or chat history",
		"Use pagination when results are truncated",
		"extra searches are not mandatory",
	} {
		if !strings.Contains(rendered, phrase) {
			t.Errorf("system prompt missing %q", phrase)
		}
	}
}
