package agents

import (
	"context"
	"encoding/json"
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
	Output *string `json:"output"`
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
	Role     string `json:"role"`
	Content  string `json:"content"`
	ImageURL string `json:"imageUrl,omitempty"`
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
