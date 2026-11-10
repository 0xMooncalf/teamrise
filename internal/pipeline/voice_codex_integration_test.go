package pipeline

import (
	"context"
	"ladder/internal/connections"
	"ladder/internal/providers"
	"os"
	"testing"
	"time"
)

func TestInstalledCodexVoiceQuestionAndReport(t *testing.T) {
	if os.Getenv("TEAMRISE_TEST_CODEX_VOICE") != "1" {
		t.Skip("opt-in real Codex subscription calls")
	}
	e := newTestEngine(t)
	id := voiceInterview(t, e)
	p := providers.Compatible{Config: connections.AI{Provider: "codex", BaseURL: "https://api.openai.com/v1", Model: "gpt-6-sol", TokenField: "max_tokens", MaxQuestions: 1}, Voice: true}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	defer providers.CloseLocalCodex()
	c, _ := e.Candidate(id)
	if err := e.RunAI(ctx, id, "start", "", c.Live.Version, p); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if c.Live.Status != "awaiting_answer" {
		t.Fatalf("question failed: %s", c.Live.Error)
	}
	t.Logf("Spoken-style question: %s", c.Transcript[0].Text)
	// This exact answer was recognized from generated test audio by test_speech.py.
	answer := "I ask for requirements, break the task into steps and check my work before submitting it."
	if err := e.RunAI(ctx, id, "answer", answer, c.Live.Version, p); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if c.Live.Status != "complete" || c.Assessment == nil {
		t.Fatalf("report failed: %s", c.Live.Error)
	}
	if len(c.Live.Calls) != 2 || c.Live.Calls[0].Usage.Billing != "subscription" || c.Live.Calls[1].Usage.Billing != "subscription" {
		t.Fatal("subscription usage missing")
	}
	if c.Live.Calls[0].EstimatedUSD != nil || c.Live.Calls[1].EstimatedUSD != nil {
		t.Fatal("subscription invoice fabricated")
	}
	requestFor(t, e, id)
	t.Logf("Report complete: scored=%t, tokens=%d + %d, human review pending", c.Live.Scored, c.Live.Calls[0].Usage.TotalTokens, c.Live.Calls[1].Usage.TotalTokens)
}
