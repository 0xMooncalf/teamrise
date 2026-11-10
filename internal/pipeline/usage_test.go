package pipeline

import (
	"context"
	"encoding/json"
	"ladder/internal/connections"
	"ladder/internal/providers"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUsageSurvivesAutomaticAnalysisInvalidReportRetryAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := e.Snapshot().Profiles[0]
	var analyzed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []providers.ChatMessage `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		content := `{"message":"Describe a practical example.","completed":false}`
		if strings.HasPrefix(body.Messages[0].Content, "Analyze") {
			if analyzed.Add(1) == 1 {
				content = `{"score":999}`
			} else {
				score := 80
				card := []providers.Criterion{}
				for _, label := range profile.Criteria {
					card = append(card, providers.Criterion{Label: label, Value: &score, Evidence: []string{"I ask for requirements"}})
				}
				raw, _ := json.Marshal(providers.Report{Score: &score, Summary: "Summary", Characterization: "Observed response", Recommendation: "Human review", Scorecard: card})
				content = string(raw)
			}
		} else if len(body.Messages) > 1 {
			content = `{"message":"Thank you.","completed":true}`
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "actual-model", "choices": []any{map[string]any{"message": map[string]string{"content": content}}}, "usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 100, "prompt_tokens_details": map[string]int{"cached_tokens": 200}, "completion_tokens_details": map[string]int{"reasoning_tokens": 30}}})
	}))
	defer srv.Close()
	input, cached, output := 2.0, 0.5, 8.0
	p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "configured-model", TokenField: "max_tokens", MaxQuestions: 3, InputUSDPerMillion: &input, CachedInputUSDPerMillion: &cached, OutputUSDPerMillion: &output}}
	id, _ := e.CreateInterview("Usage Person", profile.ID, "")
	run := func(action, answer string) {
		t.Helper()
		c, _ := e.Candidate(id)
		if err := e.RunAI(context.Background(), id, action, answer, c.Live.Version, p); err != nil {
			t.Fatal(err)
		}
	}
	run("start", "")
	run("answer", "I ask for requirements")
	c, _ := e.Candidate(id)
	if c.Live.Status != "error" || c.Live.Resume != "analysis" || len(c.Live.Calls) != 3 {
		t.Fatalf("usage lost before invalid report retry: %+v", c.Live)
	}
	// A later price change must affect only the retry, not historical calls.
	input = 4 // set a fresh pointer: recorded rate snapshots must be independent.
	updatedInput := 4.0
	p.Config.InputUSDPerMillion = &updatedInput
	run("retry", "")
	c, _ = e.Candidate(id)
	if len(c.Live.Calls) != 4 || c.Live.Calls[0].Agent != "Interviewer" || c.Live.Calls[2].Agent != "Analyst" || c.Live.Status != "complete" {
		t.Fatal("automatic analysis lost usage or attributed the wrong role")
	}
	for i, call := range c.Live.Calls {
		if !call.Usage.Reported || call.Usage.TotalTokens != 1100 || call.Usage.ReasoningTokens != 30 || call.Usage.Model != "actual-model" || call.EstimatedUSD == nil {
			t.Fatal("usage incorrectly parsed")
		}
		expected := .0025
		if i == 3 {
			expected = .0041
		}
		if math.Abs(*call.EstimatedUSD-expected) > 1e-10 {
			t.Fatalf("wrong estimate %g", *call.EstimatedUSD)
		}
	}
	if *c.Live.Calls[0].InputRate != 2 {
		t.Fatal("past price snapshot mutated")
	}
	e.Close()
	restored, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	c, _ = restored.Candidate(id)
	if len(c.Live.Calls) != 4 || c.Live.Calls[2].EstimatedUSD == nil {
		t.Fatal("usage ledger did not survive restart")
	}
}
func TestUnknownUsageAndExplicitFreeRates(t *testing.T) {
	e, err := New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, withUsage := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"message":"A question?","completed":false}`}}}}
			if withUsage {
				body["usage"] = map[string]int{"prompt_tokens": 50, "completion_tokens": 10}
			}
			json.NewEncoder(w).Encode(body)
		}))
		zero := 0.0
		p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "free", TokenField: "max_tokens", MaxQuestions: 2, InputUSDPerMillion: &zero, OutputUSDPerMillion: &zero}}
		id, _ := e.CreateInterview("No Usage Person", e.Snapshot().Profiles[0].ID, "")
		if err = e.RunAI(context.Background(), id, "start", "", 1, p); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		c, _ := e.Candidate(id)
		call := c.Live.Calls[0]
		if withUsage {
			if call.EstimatedUSD == nil || *call.EstimatedUSD != 0 {
				t.Fatal("explicit free rate was not respected")
			}
		} else if call.Usage.Reported || call.EstimatedUSD != nil {
			t.Fatal("missing usage treated as free")
		}
	}
}

func TestSubscriptionUsageIsNeverPricedAsAPIBilling(t *testing.T) {
	e, err := New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	id, err := e.CreateInterview("Subscription QA", e.Snapshot().Profiles[0].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate(id)
	rate := 100.0
	cfg := connections.AI{Provider: "codex", InputUSDPerMillion: &rate, OutputUSDPerMillion: &rate}
	u := providers.Usage{Provider: "codex", Billing: "subscription", Reported: true, InputTokens: 1000, OutputTokens: 50, TotalTokens: 1050}
	if err = e.recordUsage(&c, "Interviewer", u, cfg); err != nil {
		t.Fatal(err)
	}
	restored, _ := e.Candidate(id)
	if len(restored.Live.Calls) != 1 || restored.Live.Calls[0].EstimatedUSD != nil || restored.Live.Calls[0].Usage.TotalTokens != 1050 {
		t.Fatal("subscription incorrectly priced or usage lost")
	}
}
