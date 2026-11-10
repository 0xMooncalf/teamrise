package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ladder/internal/connections"
	"ladder/internal/providers"
)

func TestRealInterviewRetryReportReviewAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	profile := e.Snapshot().Profiles[0]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer fake-key" {
			t.Error("wrong provider request")
		}
		var b struct {
			Model    string                  `json:"model"`
			Messages []providers.ChatMessage `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Model != "selected-model" || !strings.Contains(b.Messages[0].Content, profile.SkillPrompt[:20]) {
			t.Error("model or pinned skill missing")
		}
		count := calls.Add(1)
		if count == 2 {
			w.WriteHeader(503)
			return
		}
		content := `{"message":"Explain how you approach a difficult task.","completed":false}`
		if strings.HasPrefix(b.Messages[0].Content, "Analyze") {
			score := 0
			card := []providers.Criterion{}
			for _, criterion := range profile.Criteria {
				card = append(card, providers.Criterion{Label: criterion, Value: &score, Evidence: []string{"I ask for requirements"}})
			}
			report := providers.Report{Score: &score, Summary: "Needs further evidence", Characterization: "Gave a brief answer", Recommendation: "Ask a human to review", Limitations: []string{"Only one answer"}, Scorecard: card}
			raw, _ := json.Marshal(report)
			content = string(raw)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer srv.Close()
	p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "selected-model", TokenField: "max_tokens", MaxQuestions: 3}, Key: "fake-key"}
	id, err := e.CreateInterview("Real Person", profile.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	run := func(action, answer string) {
		t.Helper()
		c, _ := e.Candidate(id)
		if err := e.RunAI(context.Background(), id, action, answer, c.Live.Version, p); err != nil {
			t.Fatal(err)
		}
	}
	run("start", "")
	c, _ := e.Candidate(id)
	if len(c.Transcript) != 1 || c.Live.Status != "awaiting_answer" {
		t.Fatal("no actual AI question")
	}
	oldVersion := c.Live.Version
	run("answer", "I ask for requirements")
	c, _ = e.Candidate(id)
	if c.Live.Status != "error" || answers(c) != 1 {
		t.Fatal("answer lost after provider error")
	}
	if err = e.RunAI(context.Background(), id, "answer", "duplicate", oldVersion, p); err != ErrConflict {
		t.Fatal("stale answer accepted")
	}
	run("retry", "")
	c, _ = e.Candidate(id)
	if answers(c) != 1 || len(c.Transcript) != 3 {
		t.Fatal("retry duplicated candidate answer")
	}
	run("analyze", "")
	c, _ = e.Candidate(id)
	if c.Assessment == nil || !c.Live.Scored || c.Score != 0 || c.Stage != "Review" || c.Room.BotPresent {
		t.Fatal("incorrect real report or fake Discord presence")
	}
	if err = e.Decision(id, "reject"); err != ErrConflict {
		t.Fatal("decision did not require human review")
	}
	var request AdminRequest
	for _, r := range e.Snapshot().Requests {
		if r.CandidateID == id {
			request = r
		}
	}
	if request.CreatedTimestamp == nil {
		t.Fatal("real review missing wall-clock timestamp")
	}
	if err = e.Control("reset"); err != nil {
		t.Fatal(err)
	}
	c, err = e.Candidate(id)
	if err != nil || c.Assessment == nil {
		t.Fatal("demo reset removed real interview")
	}
	if err = e.AdminRequest(request.ID, "acknowledge", "admin-maya"); err != nil {
		t.Fatal(err)
	}
	if err = e.AdminRequest(request.ID, "acknowledge", "admin-theo"); err != ErrConflict {
		t.Fatal("review was claimed twice")
	}
	if err = e.Decision(id, "resolve"); err != nil {
		t.Fatal(err)
	}
	if err = e.Decision(id, "hire"); err != nil {
		t.Fatal(err)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	c, _ = e.Candidate(id)
	if c.Stage != "Hired" || c.Assessment == nil || c.Live.Reviewer == "" || answers(c) != 1 {
		t.Fatal("real interview did not replay correctly")
	}
}
func TestSeparateInterviewsProceedWhileAnotherModelCallWaits(t *testing.T) {
	e, err := New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"message":"Describe your experience.","completed":false}`}}}})
	}))
	defer srv.Close()
	p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens", MaxQuestions: 2}}
	id, _ := e.CreateInterview("Candidate One", e.Snapshot().Profiles[0].ID, "")
	other, _ := e.CreateInterview("Candidate Two", e.Snapshot().Profiles[0].ID, "")
	done := make(chan error, 1)
	go func() { done <- e.RunAI(context.Background(), id, "start", "", 1, p) }()
	select {
	case <-entered:
	case err := <-done:
		close(release)
		t.Fatalf("local provider could not receive request: %v", err)
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("local provider did not receive request")
	}
	if err = e.RunAI(context.Background(), id, "start", "", 1, p); err != ErrConflict {
		t.Error("concurrent call on one session accepted")
	}
	fast := make(chan error, 1)
	go func() { fast <- e.RunAI(context.Background(), other, "start", "", 1, p) }()
	select {
	case err := <-fast:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("one AI call blocked all other interviews")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate(id)
	before := c.Live.Version
	if err = e.Control("step"); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Candidate(id)
	if after.Live.Version != before || len(after.Transcript) != len(c.Transcript) {
		t.Fatal("demo scheduler advanced a real candidate")
	}
}
func TestRecoverInterruptedAIRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	e, _ := New(path)
	id, _ := e.CreateInterview("Candidate One", e.Snapshot().Profiles[0].ID, "")
	p := providers.Compatible{Config: connections.AI{Model: "test", MaxQuestions: 2}}
	if _, err := e.reserveAI(id, "start", "", 1, p); err != nil {
		t.Fatal(err)
	}
	e.Close()
	restored, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	c, _ := restored.Candidate(id)
	if c.Live.Status != "error" || c.Live.Resume != "turn" {
		t.Fatal("interrupted session cannot be retried")
	}
}

func TestAutomaticAnalysisRetriesTheAnalysisStep(t *testing.T) {
	for _, limit := range []int{1, 3} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			e, err := New(filepath.Join(t.TempDir(), "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			profile := e.Snapshot().Profiles[0]
			var analyzed atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []providers.ChatMessage `json:"messages"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				content := `{"message":"Describe your work.","completed":false}`
				if strings.HasPrefix(body.Messages[0].Content, "Analyze") {
					if analyzed.Add(1) == 1 {
						w.WriteHeader(502)
						return
					}
					card := []providers.Criterion{}
					for _, label := range profile.Criteria {
						card = append(card, providers.Criterion{Label: label, Evidence: []string{}})
					}
					raw, _ := json.Marshal(providers.Report{Summary: "Insufficient evidence", Characterization: "The answer was brief", Recommendation: "Gather more evidence with a human", Scorecard: card, Limitations: []string{"One brief answer"}})
					content = string(raw)
				} else if len(body.Messages) > 1 {
					content = `{"message":"Thank you for your answer.","completed":true}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
			}))
			defer srv.Close()
			p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens", MaxQuestions: limit}}
			id, _ := e.CreateInterview("QA Person", profile.ID, "")
			for _, step := range []struct{ action, answer string }{{"start", ""}, {"answer", "I am still learning"}} {
				c, _ := e.Candidate(id)
				if err = e.RunAI(context.Background(), id, step.action, step.answer, c.Live.Version, p); err != nil {
					t.Fatal(err)
				}
			}
			c, _ := e.Candidate(id)
			if c.Live.Status != "error" || c.Live.Resume != "analysis" {
				t.Fatal("failed automatic analysis lost its retry stage")
			}
			messages := len(c.Transcript)
			if err = e.RunAI(context.Background(), id, "retry", "", c.Live.Version, p); err != nil {
				t.Fatal(err)
			}
			c, _ = e.Candidate(id)
			if c.Live.Status != "complete" || c.Live.Scored || len(c.Transcript) != messages || answers(c) != 1 || c.Assessment.Strengths == nil || !c.Scorecard[0].NotScored {
				t.Fatal("analysis retry fabricated score, omitted arrays, or duplicated dialogue")
			}
		})
	}
}
