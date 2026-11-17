package endpoints

import (
	"context"
	"errors"
	"sync"

	"scholic/internal/agents"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type chatToolCallEnvelope struct {
	Type     string          `json:"type"`
	ToolCall agents.ToolCall `json:"toolCall"`
}

type chatToolResultEnvelope struct {
	Type   string            `json:"type"`
	Result agents.ToolResult `json:"result"`
}

type webSocketToolTransport struct {
	conn *websocket.Conn
	ioMu sync.Mutex
}

func (t *webSocketToolTransport) SendToolCall(ctx context.Context, call agents.ToolCall) (agents.ToolResult, error) {
	t.ioMu.Lock()
	defer t.ioMu.Unlock()
	writeErr := wsjson.Write(ctx, t.conn, chatToolCallEnvelope{Type: "tool_call", ToolCall: call})
	if writeErr != nil {
		return agents.ToolResult{}, writeErr
	}

	// Keep write/read paired. coder/websocket permits one concurrent reader and
	// one concurrent writer, but result correlation requires this pairing.
	for {
		var envelope chatToolResultEnvelope
		if err := wsjson.Read(ctx, t.conn, &envelope); err != nil {
			return agents.ToolResult{}, err
		}
		if envelope.Type != "tool_result" {
			continue
		}
		if envelope.Result.ToolCallID != call.ID {
			return agents.ToolResult{}, errors.New("tool result does not match request")
		}
		return envelope.Result, nil
	}
}
