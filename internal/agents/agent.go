package agents

import (
	"context"
	"encoding/json"
)

const (
	MaxToolCallsPerTurn = 4
	MaxLLMTurns         = 10
	RecentMessageLimit  = 20
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

type ModelRequest struct {
	Messages []ContextMessage
	Tools    []ToolDefinition
}

type ModelTurn struct {
	Output    *string    `json:"output"`
	ToolCalls []ToolCall `json:"toolCalls"`
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
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMProvider interface {
	Complete(context.Context, ModelRequest) (ModelTurn, error)
}

type ToolTransport interface {
	SendToolCall(context.Context, ToolCall) (ToolResult, error)
}

type ToolContext struct {
	UserID  uint
	ChatID  uint
	PageURL string
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
