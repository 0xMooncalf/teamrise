package providers

import (
	"context"
	"ladder/internal/connections"
	"os"
	"testing"
	"time"
)

func TestInstalledCodexQuestionLatency(t *testing.T) {
	if os.Getenv("TEAMRISE_TEST_CODEX_LATENCY") != "1" {
		t.Skip("opt-in real Codex latency test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer CloseLocalCodex()
	if err := WarmLocalCodex(ctx, "gpt-6-sol"); err != nil {
		t.Fatal(err)
	}
	history := []ChatMessage{}
	for i := 0; i < 2; i++ {
		var usage Usage
		p := Compatible{SessionID: "latency-fixture", Config: connections.AI{Provider: "codex", Model: "gpt-6-sol"}, Voice: true, OnUsage: func(u Usage) error { usage = u; return nil }}
		start := time.Now()
		q, err := p.Next(ctx, "Backend engineer. Assess practical concurrency, API reliability and debugging using actual examples.", history)
		if err != nil {
			t.Fatal(err)
		}
		if !usage.Reported || usage.Billing != "subscription" || usage.TotalTokens <= 0 {
			t.Fatal("lost completed usage during early exit")
		}
		t.Logf("Question %d: transport=%s, wall=%dms, usage=%dms, input=%d, output=%d, text=%q", i+1, usage.Transport, time.Since(start).Milliseconds(), usage.DurationMS, usage.InputTokens, usage.OutputTokens, q.Message)
		history = append(history, ChatMessage{"assistant", q.Message}, ChatMessage{"user", "I use idempotency keys, database transactions and a unique constraint to prevent duplicate writes."})
	}
}
