package agents

import (
	"encoding/json"
	"testing"
)

func TestDecisionAnswerUnmarshalsNumericNoul(t *testing.T) {
	var response DecisionsResponse
	if err := json.Unmarshal([]byte(`{"answers":{"match_0":{"type":"noul","noul":1},"match_1":{"type":"noul","noul":0}}}`), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Answers["match_0"].IsTrue() {
		t.Fatal("expected numeric 1 to be true")
	}
	if response.Answers["match_1"].IsTrue() {
		t.Fatal("expected numeric 0 to be false")
	}
}

func TestDecisionAnswerUnmarshalsBooleanValue(t *testing.T) {
	var answer DecisionAnswer
	if err := json.Unmarshal([]byte(`{"type":"bool","value":true}`), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.IsTrue() {
		t.Fatal("expected boolean value to be true")
	}
}
