package chat

import (
	"testing"

	"scholic/internal/agents"
)

func TestValidatePageReferences(t *testing.T) {
	result, err := validatePageReferences(agents.ChatResponse{
		Output:         "Alpha [2]. Unused [1]. Invalid [3]. Duplicate [4].",
		PageReferences: []string{"unused", "Alpha page text", "not on page", "Alpha page text"},
	}, []string{"Alpha page text"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "Alpha [1]. Unused . Invalid . Duplicate [1]." {
		t.Fatalf("output = %q", result.Output)
	}
	if len(result.PageReferences) != 1 || result.PageReferences[0] != "Alpha page text" {
		t.Fatalf("references = %#v", result.PageReferences)
	}
}

func TestValidatePageReferencesEmptyForUnsupportedClaims(t *testing.T) {
	result, err := validatePageReferences(agents.ChatResponse{Output: "No page claim [1].", PageReferences: []string{"not supplied"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "No page claim ." || len(result.PageReferences) != 0 {
		t.Fatalf("result = %#v", result)
	}
}
