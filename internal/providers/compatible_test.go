package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ladder/internal/connections"
)

func TestReportRejectsInventedOrInterviewerEvidence(t *testing.T) {
	for _, quote := range []string{"invented experience", "interviewer example"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			report := map[string]any{"score": 80, "summary": "Summary", "characterization": "Observed answers", "recommendation": "Human review", "scorecard": []map[string]any{{"label": "Evidence", "value": 80, "evidence_quotes": []string{quote}}}}
			content, _ := json.Marshal(report)
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}})
		}))
		p := Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens"}}
		_, err := p.Analyze(context.Background(), "profile", []string{"Evidence"}, []ChatMessage{{"assistant", "interviewer example"}, {"user", "I implemented retries"}})
		srv.Close()
		if err == nil {
			t.Fatalf("accepted invalid evidence %q", quote)
		}
	}
}
func TestProviderErrorsDoNotEchoSecretsAndRedirectsAreBlocked(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	for _, status := range []int{401, 429, 500, 307} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			w.Write([]byte("private-test-key"))
		}))
		p := Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens"}, Key: "private-test-key"}
		err := p.Test(context.Background())
		srv.Close()
		if err == nil || strings.Contains(err.Error(), p.Key) {
			t.Fatal("unsafe or missing provider error")
		}
	}
	if hit {
		t.Fatal("credential-bearing request followed redirect")
	}
}
