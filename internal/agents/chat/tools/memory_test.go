package tools

import "testing"

func TestMemoryToolDefinitions(t *testing.T) {
	if got := AddMemoryTool().Definition().Name; got != "AddMemory" {
		t.Fatalf("unexpected add tool name: %q", got)
	}
	if got := UpdateMemoryTool().Definition().Name; got != "UpdateMemory" {
		t.Fatalf("unexpected update tool name: %q", got)
	}
	if got := DeleteMemoryTool().Definition().Name; got != "DeleteMemory" {
		t.Fatalf("unexpected delete tool name: %q", got)
	}
	if description := AddMemoryTool().Definition().Description; description == "" {
		t.Fatal("missing add memory description")
	}
}
