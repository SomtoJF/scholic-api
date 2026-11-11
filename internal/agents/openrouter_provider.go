package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/models/operations"
	"github.com/google/jsonschema-go/jsonschema"
)

const (
	ChatModel      = "openai/gpt-6-luna"
	DecisionsModel = "openai/gpt-6-luna-decisions"
)

type OpenRouterProvider struct {
	client *openrouter.OpenRouter
}

func NewOpenRouterProvider(apiKey string) (*OpenRouterProvider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("OPENROUTER_API_KEY is required")
	}
	return &OpenRouterProvider{client: openrouter.New(openrouter.WithSecurity(apiKey))}, nil
}

func (p *OpenRouterProvider) GetCompletions(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	if strings.TrimSpace(request.Model) == "" {
		return CompletionResponse{}, errors.New("completion model is required")
	}
	messages := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		content := any(message.Content)
		if message.ImageURL != "" {
			content = []map[string]any{{"type": "text", "text": message.Content}, {"type": "image_url", "image_url": map[string]string{"url": message.ImageURL}}}
		}
		messages = append(messages, map[string]any{"role": message.Role, "content": content})
	}
	tools := make([]map[string]any, 0, len(request.Tools))
	for _, tool := range request.Tools {
		parameters, err := schemaFor(tool.Arguments)
		if err != nil {
			return CompletionResponse{}, fmt.Errorf("tool %s schema: %w", tool.Name, err)
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": parameters, "strict": true}})
	}
	payload := map[string]any{"model": request.Model, "messages": messages}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if request.ToolChoice != "" {
		payload["tool_choice"] = toolChoice(request.ToolChoice)
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.ParallelToolCalls != nil {
		payload["parallel_tool_calls"] = *request.ParallelToolCalls
	}
	if request.ResponseSchema != nil {
		schema, err := schemaFor(request.ResponseSchema)
		if err != nil {
			return CompletionResponse{}, fmt.Errorf("response schema: %w", err)
		}
		payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "response_schema", "schema": schema, "strict": true}}
	}
	requestJSON, err := json.Marshal(payload)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal OpenRouter completion request: %w", err)
	}
	var providerRequest components.ChatRequest
	if err := json.Unmarshal(requestJSON, &providerRequest); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode OpenRouter completion request: %w", err)
	}
	response, err := p.client.Chat.Send(ctx, providerRequest, nil)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("OpenRouter completion: %w", err)
	}
	if response == nil || response.ChatResult == nil {
		return CompletionResponse{}, errors.New("OpenRouter completion returned no result")
	}
	return normalizeCompletionResponse(response.ChatResult)
}

func (p *OpenRouterProvider) GenerateEmbedding(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	if strings.TrimSpace(request.Model) == "" {
		return EmbeddingResponse{}, errors.New("embedding model is required")
	}
	if strings.TrimSpace(request.Input) == "" {
		return EmbeddingResponse{}, errors.New("embedding input is required")
	}
	response, err := p.client.Embeddings.Generate(ctx, operations.CreateEmbeddingsRequest{Model: request.Model, Input: operations.CreateInputUnionStr(request.Input)}, nil)
	if err != nil {
		return EmbeddingResponse{}, fmt.Errorf("OpenRouter embedding: %w", err)
	}
	if response == nil || response.CreateEmbeddingsResponseBody == nil || len(response.CreateEmbeddingsResponseBody.Data) == 0 {
		return EmbeddingResponse{}, errors.New("OpenRouter embedding returned no data")
	}
	embedding := response.CreateEmbeddingsResponseBody.Data[0].GetEmbedding()
	if embedding.Type != operations.EmbeddingTypeArrayOfNumber {
		return EmbeddingResponse{}, errors.New("OpenRouter embedding returned a non-vector result")
	}
	return EmbeddingResponse{Embedding: embedding.ArrayOfNumber, Model: request.Model}, nil
}

func (p *OpenRouterProvider) GetDecisions(ctx context.Context, request DecisionsRequest) (DecisionsResponse, error) {
	if strings.TrimSpace(request.Model) == "" {
		return DecisionsResponse{}, errors.New("decisions model is required")
	}
	state := request.State
	if request.Screenshot != "" {
		state = []any{state, map[string]any{"type": "image_url", "image_url": map[string]string{"url": request.Screenshot}}}
	}
	body, err := json.Marshal(map[string]any{"model": request.Model, "state": state, "questions": request.Questions})
	if err != nil {
		return DecisionsResponse{}, fmt.Errorf("marshal Decisions request: %w", err)
	}
	var sdkRequest components.DecisionsRequest
	if err := json.Unmarshal(body, &sdkRequest); err != nil {
		return DecisionsResponse{}, fmt.Errorf("decode Decisions request: %w", err)
	}
	response, err := p.client.Alpha.Decisions.Create(ctx, sdkRequest)
	if err != nil {
		return DecisionsResponse{}, fmt.Errorf("OpenRouter Decisions: %w", err)
	}
	if response == nil {
		return DecisionsResponse{}, errors.New("OpenRouter Decisions returned no result")
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return DecisionsResponse{}, fmt.Errorf("marshal Decisions response: %w", err)
	}
	var result DecisionsResponse
	if err := json.Unmarshal(responseJSON, &result); err != nil {
		return DecisionsResponse{}, fmt.Errorf("decode Decisions response: %w", err)
	}
	return result, nil
}

func schemaFor(value any) (any, error) {
	if value == nil {
		return map[string]any{"type": "object", "additionalProperties": true}, nil
	}
	if schema, ok := value.(map[string]any); ok {
		return schema, nil
	}
	schema, err := jsonschema.ForType(reflect.TypeOf(value), nil)
	if err != nil {
		return nil, err
	}
	return schema, nil
}

func toolChoice(value string) any {
	switch value {
	case "auto", "none", "required":
		return value
	default:
		return map[string]any{"type": "function", "function": map[string]string{"name": value}}
	}
}

func normalizeCompletionResponse(response json.Marshaler) (CompletionResponse, error) {
	data, err := response.MarshalJSON()
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal completion response: %w", err)
	}
	var result struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			Cost             float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return CompletionResponse{}, fmt.Errorf("decode completion response: %w", err)
	}
	output := CompletionResponse{Model: result.Model, InputTokens: result.Usage.PromptTokens, OutputTokens: result.Usage.CompletionTokens, TotalCost: result.Usage.Cost}
	if len(result.Choices) == 0 {
		return output, nil
	}
	if result.Choices[0].Message.Content != "" {
		text := result.Choices[0].Message.Content
		output.Output = &text
	}
	for _, call := range result.Choices[0].Message.ToolCalls {
		output.ToolCalls = append(output.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
	}
	return output, nil
}
