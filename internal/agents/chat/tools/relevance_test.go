package tools

import (
	"context"
	"testing"

	"scholic/internal/agents"
)

type relevanceProvider struct {
	decisions agents.DecisionsRequest
}

func (p *relevanceProvider) GetCompletions(context.Context, agents.CompletionRequest) (agents.CompletionResponse, error) {
	return agents.CompletionResponse{}, nil
}

func (p *relevanceProvider) GenerateEmbedding(context.Context, agents.EmbeddingRequest) (agents.EmbeddingResponse, error) {
	return agents.EmbeddingResponse{}, nil
}

func (p *relevanceProvider) GetDecisions(_ context.Context, request agents.DecisionsRequest) (agents.DecisionsResponse, error) {
	p.decisions = request
	answers := make(map[string]agents.DecisionAnswer, len(request.Questions))
	for key := range request.Questions {
		keep := key == "match_0"
		answers[key] = agents.DecisionAnswer{Type: "noul", Noul: &keep}
	}
	return agents.DecisionsResponse{Answers: answers}, nil
}

func TestFilterPageSearchResult(t *testing.T) {
	provider := &relevanceProvider{}
	output, err := FilterPageSearchResult(context.Background(), provider, "find alpha", map[string]any{
		"query": "find alpha",
		"matches": []any{
			map[string]any{"text": "alpha", "context": "alpha context", "start": 1, "end": 6},
			map[string]any{"text": "beta", "context": "beta context", "start": 8, "end": 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := output.(PageSearchResult)
	if len(result.Matches) != 1 || result.Matches[0].Text != "alpha" {
		t.Fatalf("unexpected filtered matches: %+v", result.Matches)
	}
	if len(provider.decisions.Questions) != 2 {
		t.Fatalf("expected two Decisions questions, got %d", len(provider.decisions.Questions))
	}
}

func TestFilterPageSearchResultEmptySkipsProvider(t *testing.T) {
	output, err := FilterPageSearchResult(context.Background(), nil, "find alpha", PageSearchResult{Query: "find alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.(PageSearchResult).Matches) != 0 {
		t.Fatal("expected no matches")
	}
}
