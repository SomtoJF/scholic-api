package agents

import (
	"bytes"
	"embed"
	"text/template"
	"time"
)

//go:embed prompt/chat/systemm.go.tmpl prompt/chat/user.go.tmpl
var chatPromptFiles embed.FS

type SystemPromptData struct {
	CurrentContext string
	Tools          []ToolDefinition
}

type UserPromptData struct {
	CurrentDateTime string
	PageURL         string
	Message         string
	History         []ContextMessage
}

func RenderSystemPrompt(tools []ToolDefinition) (string, error) {
	return renderChatPrompt("prompt/chat/systemm.go.tmpl", SystemPromptData{
		CurrentContext: "Use the page URL and recent chat history supplied in the user request.",
		Tools:          tools,
	})
}

func RenderUserPrompt(data UserPromptData) (string, error) {
	return renderChatPrompt("prompt/chat/user.go.tmpl", data)
}

func renderChatPrompt(name string, data any) (string, error) {
	tmpl, err := template.ParseFS(chatPromptFiles, name)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func promptDateTime(now time.Time) string { return now.Format(time.RFC3339) }
