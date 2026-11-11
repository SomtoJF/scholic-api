package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"scholic/internal/agents"
	"scholic/internal/models"
)

const decisionsModel = "openai/gpt-6-luna-decisions"

type PageSearchResult struct {
	Query   string            `json:"query"`
	Matches []PageSearchMatch `json:"matches"`
}
type PageSearchMatch struct {
	Text    string `json:"text"`
	Context string `json:"context"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}
type relevanceCandidate struct {
	Key   string
	Value any
}

func filterRelevant(ctx context.Context, provider agents.LLMProvider, query, searchText string, values any) (any, error) {
	candidates := make([]relevanceCandidate, 0)
	switch typed := values.(type) {
	case []models.Message:
		for i, value := range typed {
			candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: map[string]any{"messageText": value.MessageText, "isUserMessage": value.IsUserMessage, "createdAt": value.CreatedAt}})
		}
	case []models.Annotation:
		for i, value := range typed {
			candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: map[string]any{"quoteText": value.QuoteText, "text": value.Text, "url": value.URL}})
		}
	default:
		return nil, errors.New("unsupported search result type")
	}
	if len(candidates) == 0 {
		return values, nil
	}
	keep, err := decideRelevant(ctx, provider, query, searchText, candidates)
	if err != nil {
		return nil, err
	}
	switch typed := values.(type) {
	case []models.Message:
		filtered := make([]models.Message, 0, len(typed))
		for i, value := range typed {
			if keep[i] {
				filtered = append(filtered, value)
			}
		}
		return filtered, nil
	case []models.Annotation:
		filtered := make([]models.Annotation, 0, len(typed))
		for i, value := range typed {
			if keep[i] {
				filtered = append(filtered, value)
			}
		}
		return filtered, nil
	default:
		return nil, errors.New("unsupported search result type")
	}
}

func decideRelevant(ctx context.Context, provider agents.LLMProvider, query, searchText string, candidates []relevanceCandidate) ([]bool, error) {
	keep := make([]bool, len(candidates))
	if len(candidates) == 0 {
		return keep, nil
	}
	if provider == nil {
		return nil, errors.New("LLM provider is required for search relevance")
	}
	questions := make(map[string]agents.DecisionQuestion, len(candidates))
	stateCandidates := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		questions[candidate.Key] = agents.DecisionQuestion{Type: "noul", Instructions: "Is this search result relevant to the user's query?", Criteria: map[string]any{"true": "The result directly helps answer the query.", "false": "The result is irrelevant or does not help answer the query."}}
		stateCandidates = append(stateCandidates, map[string]any{"id": candidate.Key, "result": candidate.Value})
	}
	response, err := provider.GetDecisions(ctx, agents.DecisionsRequest{Model: decisionsModel, State: map[string]any{"query": query, "searchText": searchText, "candidates": stateCandidates}, Questions: questions})
	if err != nil {
		return nil, err
	}
	for i, candidate := range candidates {
		answer, ok := response.Answers[candidate.Key]
		if !ok || answer.Type != "noul" && answer.Type != "" {
			return nil, fmt.Errorf("missing or invalid relevance decision for %s", candidate.Key)
		}
		keep[i] = answer.IsTrue()
	}
	return keep, nil
}

func FilterPageSearchResult(ctx context.Context, provider agents.LLMProvider, query string, output any) (any, error) {
	var result PageSearchResult
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal page search result: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode page search result: %w", err)
	}
	candidates := make([]relevanceCandidate, 0, len(result.Matches))
	for i, match := range result.Matches {
		candidates = append(candidates, relevanceCandidate{Key: fmt.Sprintf("match_%d", i), Value: match})
	}
	keep, err := decideRelevant(ctx, provider, query, "", candidates)
	if err != nil {
		return nil, err
	}
	filtered := result.Matches[:0]
	for i, match := range result.Matches {
		if keep[i] {
			filtered = append(filtered, match)
		}
	}
	result.Matches = filtered
	return result, nil
}
