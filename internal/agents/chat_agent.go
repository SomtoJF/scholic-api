package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

type ChatAgent struct {
	Provider LLMProvider
	Tools    []Tool
	LoadChat func(context.Context, uint, int) ([]ContextMessage, error)
}

func NewChatAgent(provider LLMProvider, tools []Tool, loadChat func(context.Context, uint, int) ([]ContextMessage, error)) Agent {
	return &ChatAgent{Provider: provider, Tools: tools, LoadChat: loadChat}
}

func (a *ChatAgent) GetTools() []Tool { return a.Tools }

func (a *ChatAgent) Run(ctx context.Context, input RunInput) (RunOutput, error) {
	if a.Provider == nil {
		return RunOutput{}, errors.New("chat agent LLM provider is not configured")
	}
	messages, err := a.loadContext(ctx, input)
	if err != nil {
		return RunOutput{}, err
	}
	messages = append(messages, ContextMessage{Role: "user", Content: input.Message})
	definitions := make([]ToolDefinition, 0, len(a.Tools))
	for _, tool := range a.Tools {
		definitions = append(definitions, tool.Definition())
	}

	for turn := 0; turn < MaxLLMTurns; turn++ {
		modelTurn, err := a.Provider.Complete(ctx, ModelRequest{Messages: messages, Tools: definitions})
		if err != nil {
			return RunOutput{}, err
		}
		if modelTurn.Output != nil {
			return RunOutput{Output: modelTurn.Output}, nil
		}
		if len(modelTurn.ToolCalls) == 0 {
			return RunOutput{}, errors.New("LLM returned neither output nor tool calls")
		}
		results, err := a.executeToolCalls(ctx, input, modelTurn.ToolCalls)
		if err != nil {
			return RunOutput{}, err
		}
		messages = append(messages, ContextMessage{Role: "assistant", Content: encodeToolCalls(modelTurn.ToolCalls[:min(len(modelTurn.ToolCalls), MaxToolCallsPerTurn)])})
		messages = append(messages, ContextMessage{Role: "tool", Content: encodeToolResults(results)})
	}
	return RunOutput{}, fmt.Errorf("chat agent exceeded maximum of %d LLM turns", MaxLLMTurns)
}

func (a *ChatAgent) loadContext(ctx context.Context, input RunInput) ([]ContextMessage, error) {
	if a.LoadChat == nil {
		return nil, nil
	}
	return a.LoadChat(ctx, input.ChatID, RecentMessageLimit)
}

func (a *ChatAgent) executeToolCalls(ctx context.Context, input RunInput, calls []ToolCall) ([]ToolResult, error) {
	if len(calls) > MaxToolCallsPerTurn {
		calls = calls[:MaxToolCallsPerTurn]
	}
	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call ToolCall) { defer wg.Done(); results[i] = a.executeToolCall(ctx, input, call) }(i, call)
	}
	wg.Wait()
	return results, nil
}

func (a *ChatAgent) executeToolCall(ctx context.Context, input RunInput, call ToolCall) ToolResult {
	if call.Name == "SearchPageContent" && input.Transport == nil {
		return ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "page search transport is unavailable"}
	}
	if call.Name == "SearchPageContent" {
		result, err := input.Transport.SendToolCall(ctx, call)
		if err != nil {
			return ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		return result
	}
	for _, tool := range a.Tools {
		if tool.Definition().Name != call.Name {
			continue
		}
		output, err := tool.Execute(ctx, ToolContext{UserID: input.UserID, ChatID: input.ChatID, PageURL: input.PageURL}, call.Arguments)
		if err != nil {
			return ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		return ToolResult{ToolCallID: call.ID, Name: call.Name, Output: output}
	}
	return ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "unknown tool"}
}

func encodeToolCalls(calls []ToolCall) string { data, _ := json.Marshal(calls); return string(data) }
func encodeToolResults(results []ToolResult) string {
	data, _ := json.Marshal(results)
	return string(data)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
