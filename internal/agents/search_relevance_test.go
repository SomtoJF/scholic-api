package agents

import (
	"context"
	"testing"
)

type relevanceProvider struct {
	decisions DecisionsRequest
}

func (p *relevanceProvider) GetCompletions(context.Context, CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, nil
}

func (p *relevanceProvider) GenerateEmbedding(context.Context, EmbeddingRequest) (EmbeddingResponse, error) {
	return EmbeddingResponse{}, nil
}

func (p *relevanceProvider) GetDecisions(_ context.Context, request DecisionsRequest) (DecisionsResponse, error) {
	p.decisions = request
	answers := make(map[string]DecisionAnswer, len(request.Questions))
	for key := range request.Questions {
		keep := key == "match_0"
		answers[key] = DecisionAnswer{Type: "noul", Noul: &keep}
	}
	return DecisionsResponse{Answers: answers}, nil
}

func TestFilterPageSearchResult(t *testing.T) {
	provider := &relevanceProvider{}
	output, err := filterPageSearchResult(context.Background(), provider, "find alpha", map[string]any{
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
	output, err := filterPageSearchResult(context.Background(), nil, "find alpha", PageSearchResult{Query: "find alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.(PageSearchResult).Matches) != 0 {
		t.Fatal("expected no matches")
	}
}

func TestToolChoice(t *testing.T) {
	if got := toolChoice("auto"); got != "auto" {
		t.Fatalf("expected auto, got %#v", got)
	}
	if got := toolChoice("SearchAnnotations"); got == "SearchAnnotations" {
		t.Fatal("expected named function tool choice object")
	}
}
