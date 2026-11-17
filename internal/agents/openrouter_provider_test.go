package agents

import "testing"

func TestToolChoice(t *testing.T) {
	if got := toolChoice("auto"); got != "auto" {
		t.Fatalf("expected auto, got %#v", got)
	}
	if got := toolChoice("SearchAnnotations"); got == "SearchAnnotations" {
		t.Fatal("expected named function tool choice object")
	}
}
