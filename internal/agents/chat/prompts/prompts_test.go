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

func TestRenderSystemPromptIncludesMemories(t *testing.T) {
	rendered, err := RenderSystemPromptWithMemories(nil, []string{"[global] User prefers concise answers"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "[global] User prefers concise answers") {
		t.Fatal("system prompt missing memory")
	}
	if !strings.Contains(rendered, "Memory IDs identify the exact memories") {
		t.Fatal("system prompt missing memory ID guidance")
	}
}
