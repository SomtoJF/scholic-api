package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Agent interface {
	Run(context.Context, RunInput) (RunOutput, error)
	GetTools() []Tool
}

type RunInput struct {
	UserID    uint
	ChatID    uint
	PageURL   string
	Message   string
	Transport ToolTransport
}

type RunOutput struct {
	Output         *string  `json:"output"`
	PageReferences []string `json:"pageReferences"`
}

type ChatResponse struct {
	Output         string   `json:"output"`
	PageReferences []string `json:"pageReferences"`
}

type CompletionRequest struct {
	Model             string
	ToolChoice        string
	Messages          []ContextMessage
	Tools             []ToolDefinition
	MaxTokens         *int
	Temperature       *float64
	ParallelToolCalls *bool
	ResponseSchema    any
}

type CompletionResponse struct {
	Output       *string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
	TotalCost    float64
	Model        string
}

type EmbeddingRequest struct {
	Model string
	Input string
}

type EmbeddingResponse struct {
	Embedding []float64
	Model     string
}

type DecisionsRequest struct {
	Model      string                      `json:"model"`
	State      any                         `json:"state"`
	Questions  map[string]DecisionQuestion `json:"questions"`
	Screenshot string                      `json:"-"`
}

type DecisionQuestion struct {
	Type         string         `json:"type"`
	Instructions string         `json:"instructions"`
	Criteria     map[string]any `json:"criteria,omitempty"`
}

type DecisionsResponse struct {
	Answers map[string]DecisionAnswer `json:"answers"`
}

type DecisionAnswer struct {
	Type  string `json:"type"`
	Value *bool  `json:"value,omitempty"`
	Noul  *bool  `json:"noul,omitempty"`
}

// UnmarshalJSON accepts Decisions responses that encode boolean answers as
// booleans or numeric 0/1 values.
func (a *DecisionAnswer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
		Noul  json.RawMessage `json:"noul"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := decodeDecisionBool(raw.Value)
	if err != nil {
		return fmt.Errorf("value: %w", err)
	}
	noul, err := decodeDecisionBool(raw.Noul)
	if err != nil {
		return fmt.Errorf("noul: %w", err)
	}
	a.Type = raw.Type
	a.Value = value
	a.Noul = noul
	return nil
}

func decodeDecisionBool(data []byte) (*bool, error) {
	value := strings.TrimSpace(string(data))
	if value == "" || value == "null" {
		return nil, nil
	}
	if value == "true" || value == "false" {
		parsed := value == "true"
		return &parsed, nil
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		parsed := number != 0
		return &parsed, nil
	}
	if parsed, err := strconv.ParseBool(strings.Trim(value, `"`)); err == nil {
		return &parsed, nil
	}
	return nil, fmt.Errorf("expected boolean or 0/1, got %s", value)
}

func (a DecisionAnswer) IsTrue() bool {
	if a.Noul != nil {
		return *a.Noul
	}
	return a.Value != nil && *a.Value
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Name       string `json:"name"`
	Output     any    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ContextMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ImageURL   string     `json:"imageUrl,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
}

type LLMProvider interface {
	GetCompletions(context.Context, CompletionRequest) (CompletionResponse, error)
	GenerateEmbedding(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
	GetDecisions(context.Context, DecisionsRequest) (DecisionsResponse, error)
}

type ToolTransport interface {
	SendToolCall(context.Context, ToolCall) (ToolResult, error)
}

type ToolContext struct {
	UserID   uint
	ChatID   uint
	PageURL  string
	Provider LLMProvider
}

type ToolDefinition struct {
	Name        string
	Description string
	Arguments   any
}

type Tool interface {
	Definition() ToolDefinition
	Execute(context.Context, ToolContext, json.RawMessage) (any, error)
}

type FuncTool struct {
	ToolDefinition ToolDefinition
	Handler        func(context.Context, ToolContext, json.RawMessage) (any, error)
}

func (t FuncTool) Definition() ToolDefinition { return t.ToolDefinition }
func (t FuncTool) Execute(ctx context.Context, input ToolContext, args json.RawMessage) (any, error) {
	return t.Handler(ctx, input, args)
}
