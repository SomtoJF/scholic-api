package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"scholic/internal/agents"
	"scholic/internal/agents/chat/prompts"
)

const (
	maxToolCallsPerTurn = 4
	maxLLMTurns         = 10
	recentMessageLimit  = 20
	chatModel           = "openai/gpt-6-luna"
)

type ChatAgent struct {
	Provider         agents.LLMProvider
	Tools            []agents.Tool
	LoadChat         func(context.Context, uint, int) ([]agents.ContextMessage, error)
	PageSearchFilter func(context.Context, agents.LLMProvider, string, any) (any, error)
}

func NewChatAgent(provider agents.LLMProvider, tools []agents.Tool, loadChat func(context.Context, uint, int) ([]agents.ContextMessage, error), pageSearchFilter func(context.Context, agents.LLMProvider, string, any) (any, error)) agents.Agent {
	return &ChatAgent{Provider: provider, Tools: tools, LoadChat: loadChat, PageSearchFilter: pageSearchFilter}
}

func (a *ChatAgent) GetTools() []agents.Tool { return a.Tools }

func (a *ChatAgent) Run(ctx context.Context, input agents.RunInput) (agents.RunOutput, error) {
	if a.Provider == nil {
		return agents.RunOutput{}, errors.New("chat agent LLM provider is not configured")
	}
	messages, err := a.loadContext(ctx, input)
	if err != nil {
		return agents.RunOutput{}, err
	}
	definitions := make([]agents.ToolDefinition, 0, len(a.Tools))
	for _, tool := range a.Tools {
		definitions = append(definitions, tool.Definition())
	}
	systemPrompt, err := prompts.RenderSystemPrompt(definitions)
	if err != nil {
		return agents.RunOutput{}, err
	}
	userPrompt, err := prompts.RenderUserPrompt(prompts.UserPromptData{
		CurrentDateTime: time.Now().Format(time.RFC3339),
		PageURL:         input.PageURL,
		Message:         input.Message,
		History:         messages,
	})
	if err != nil {
		return agents.RunOutput{}, err
	}
	messages = []agents.ContextMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}

	for turn := 0; turn < maxLLMTurns; turn++ {
		completion, err := a.Provider.GetCompletions(ctx, agents.CompletionRequest{
			Model:      chatModel,
			ToolChoice: "auto",
			Messages:   messages,
			Tools:      definitions,
		})
		if err != nil {
			return agents.RunOutput{}, err
		}
		if completion.Output != nil {
			return agents.RunOutput{Output: completion.Output}, nil
		}
		if len(completion.ToolCalls) == 0 {
			return agents.RunOutput{}, errors.New("LLM returned neither output nor tool calls")
		}
		results, err := a.executeToolCalls(ctx, input, completion.ToolCalls)
		if err != nil {
			return agents.RunOutput{}, err
		}
		messages = append(messages, agents.ContextMessage{Role: "assistant", Content: encodeToolCalls(completion.ToolCalls[:min(len(completion.ToolCalls), maxToolCallsPerTurn)])})
		messages = append(messages, agents.ContextMessage{Role: "tool", Content: encodeToolResults(results)})
	}
	return agents.RunOutput{}, fmt.Errorf("chat agent exceeded maximum of %d LLM turns", maxLLMTurns)
}

func (a *ChatAgent) loadContext(ctx context.Context, input agents.RunInput) ([]agents.ContextMessage, error) {
	if a.LoadChat == nil {
		return nil, nil
	}
	return a.LoadChat(ctx, input.ChatID, recentMessageLimit)
}

func (a *ChatAgent) executeToolCalls(ctx context.Context, input agents.RunInput, calls []agents.ToolCall) ([]agents.ToolResult, error) {
	if len(calls) > maxToolCallsPerTurn {
		calls = calls[:maxToolCallsPerTurn]
	}
	results := make([]agents.ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call agents.ToolCall) {
			defer wg.Done()
			results[i] = a.executeToolCall(ctx, input, call)
		}(i, call)
	}
	wg.Wait()
	return results, nil
}

func (a *ChatAgent) executeToolCall(ctx context.Context, input agents.RunInput, call agents.ToolCall) agents.ToolResult {
	if call.Name == "SearchPageContent" && input.Transport == nil {
		return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "page search transport is unavailable"}
	}
	if call.Name == "SearchPageContent" {
		result, err := input.Transport.SendToolCall(ctx, call)
		if err != nil {
			return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		if result.Error != "" || result.Output == nil {
			return result
		}
		var request struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(call.Arguments, &request); err != nil {
			return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		if a.PageSearchFilter == nil {
			return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "page search relevance filter is unavailable"}
		}
		filtered, err := a.PageSearchFilter(ctx, a.Provider, request.Query, result.Output)
		if err != nil {
			return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		result.Output = filtered
		return result
	}
	for _, tool := range a.Tools {
		if tool.Definition().Name != call.Name {
			continue
		}
		output, err := tool.Execute(ctx, agents.ToolContext{UserID: input.UserID, ChatID: input.ChatID, PageURL: input.PageURL, Provider: a.Provider}, call.Arguments)
		if err != nil {
			return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Output: output}
	}
	return agents.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "unknown tool"}
}

func encodeToolCalls(calls []agents.ToolCall) string {
	data, _ := json.Marshal(calls)
	return string(data)
}

func encodeToolResults(results []agents.ToolResult) string {
	data, _ := json.Marshal(results)
	return string(data)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
