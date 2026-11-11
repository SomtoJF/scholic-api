package prompts

import (
	"bytes"
	"embed"
	"log"
	"text/template"

	"scholic/internal/agents"
)

//go:embed system.go.tmpl user.go.tmpl
var chatPromptFiles embed.FS

type SystemPromptData struct {
	CurrentContext string
	Tools          []agents.ToolDefinition
}

type UserPromptData struct {
	CurrentDateTime string
	PageURL         string
	Message         string
	History         []agents.ContextMessage
}

var (
	systemPrompt *template.Template
	userPrompt   *template.Template
)

func init() {
	var err error
	systemPrompt, err = template.ParseFS(chatPromptFiles, "system.go.tmpl")
	if err != nil {
		log.Fatal("Failed to render chat agent system prompt")
	}

	userPrompt, err = template.ParseFS(chatPromptFiles, "user.go.tmpl")
	if err != nil {
		log.Fatal("Failed to render chat agent user prompt")
	}
}

func RenderSystemPrompt(tools []agents.ToolDefinition) (string, error) {
	var rendered bytes.Buffer
	if err := systemPrompt.Execute(&rendered, SystemPromptData{
		CurrentContext: "Use the page URL and recent chat history supplied in the user request.",
		Tools:          tools,
	}); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func RenderUserPrompt(data UserPromptData) (string, error) {
	var rendered bytes.Buffer
	if err := userPrompt.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}
