package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"scholic/internal/agents"
	"scholic/internal/agents/chat/prompts"
	"scholic/internal/agents/chat/tools"
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

var pageReferenceMarker = regexp.MustCompile(`\[(\d+)\]`)

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
	pageCandidates := make([]string, 0)

	for turn := 0; turn < maxLLMTurns; turn++ {
		completion, err := a.Provider.GetCompletions(ctx, agents.CompletionRequest{
			Model:          chatModel,
			ToolChoice:     "auto",
			Messages:       messages,
			Tools:          definitions,
			ResponseSchema: agents.ChatResponse{},
		})
		if err != nil {
			return agents.RunOutput{}, err
		}
		if completion.Output != nil {
			response, err := parseChatResponse(*completion.Output)
			if err != nil {
				return agents.RunOutput{}, err
			}
			validated, err := validatePageReferences(response, pageCandidates)
			if err != nil {
				return agents.RunOutput{}, err
			}
			return agents.RunOutput{Output: &validated.Output, PageReferences: validated.PageReferences}, nil
		}
		if len(completion.ToolCalls) == 0 {
			return agents.RunOutput{}, errors.New("LLM returned neither output nor tool calls")
		}
		results, err := a.executeToolCalls(ctx, input, completion.ToolCalls)
		if err != nil {
			return agents.RunOutput{}, err
		}
		pageCandidates = append(pageCandidates, pageReferenceCandidates(results)...)
		toolCalls := completion.ToolCalls[:min(len(completion.ToolCalls), maxToolCallsPerTurn)]
		messages = append(messages, agents.ContextMessage{Role: "assistant", ToolCalls: toolCalls})
		for _, result := range results {
			messages = append(messages, agents.ContextMessage{
				Role:       "tool",
				ToolCallID: result.ToolCallID,
				Content:    encodeToolResult(result),
			})
		}
	}
	return agents.RunOutput{}, fmt.Errorf("chat agent exceeded maximum of %d LLM turns", maxLLMTurns)
}

func parseChatResponse(raw string) (agents.ChatResponse, error) {
	var response agents.ChatResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return agents.ChatResponse{}, fmt.Errorf("invalid chat response schema: %w", err)
	}
	return response, nil
}

func pageReferenceCandidates(results []agents.ToolResult) []string {
	var candidates []string
	for _, result := range results {
		if result.Name != "SearchPageContent" || result.Error != "" {
			continue
		}
		var pageResult tools.PageSearchResult
		data, err := json.Marshal(result.Output)
		if err != nil || json.Unmarshal(data, &pageResult) != nil {
			continue
		}
		for _, match := range pageResult.Matches {
			if text := strings.TrimSpace(match.Text); text != "" {
				candidates = append(candidates, text)
			}
			if context := strings.TrimSpace(match.Context); context != "" {
				candidates = append(candidates, context)
			}
		}
	}
	return candidates
}

func validatePageReferences(response agents.ChatResponse, candidates []string) (agents.ChatResponse, error) {
	valid := make([]string, 0, len(response.PageReferences))
	for _, reference := range response.PageReferences {
		reference = strings.TrimSpace(reference)
		if reference == "" || containsString(valid, reference) || !matchesCandidate(reference, candidates) {
			continue
		}
		valid = append(valid, reference)
	}
	indexMap := make(map[int]int)
	referenceMap := make(map[string]int)
	refs := make([]string, 0, len(valid))
	for _, match := range pageReferenceMarker.FindAllStringSubmatchIndex(response.Output, -1) {
		old, err := strconv.Atoi(response.Output[match[2]:match[3]])
		if err != nil || old < 1 || old > len(response.PageReferences) {
			continue
		}
		reference := strings.TrimSpace(response.PageReferences[old-1])
		if !containsString(valid, reference) {
			continue
		}
		newIndex, ok := referenceMap[reference]
		if !ok {
			newIndex = len(refs) + 1
			referenceMap[reference] = newIndex
			refs = append(refs, reference)
		}
		indexMap[old] = newIndex
		_ = newIndex
	}
	output := pageReferenceMarker.ReplaceAllStringFunc(response.Output, func(marker string) string {
		match := pageReferenceMarker.FindStringSubmatch(marker)
		old, _ := strconv.Atoi(match[1])
		newIndex, ok := indexMap[old]
		if !ok {
			return ""
		}
		return fmt.Sprintf("[%d]", newIndex)
	})
	return agents.ChatResponse{Output: output, PageReferences: refs}, nil
}

func matchesCandidate(reference string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(candidate, reference) {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
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
		log.Printf("[SearchPageContent] frontend request: %+v", call.Arguments)
		result, err := input.Transport.SendToolCall(ctx, call)
		log.Printf("[SearchPageContent] frontend response: %+v (err=%v)", result, err)
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
			log.Printf("[SearchPageContent] relevance filter failed; using raw result: %v", err)
			return result
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

func encodeToolResult(result agents.ToolResult) string {
	data, _ := json.Marshal(result)
	return string(data)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
