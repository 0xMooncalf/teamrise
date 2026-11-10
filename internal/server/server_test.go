package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"ladder/internal/connections"
	"ladder/internal/pipeline"
)

func TestAPIValidationAndTransitions(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	h := New(e, fstest.MapFS{"index.html": {Data: []byte("dashboard")}})
	cases := []struct {
		method, path, body, origin string
		status                     int
	}{
		{"GET", "/api/state", "", "", 200},
		{"GET", "/api/candidates/missing", "", "", 404},
		{"POST", "/api/demo/start", "{broken", "", 400},
		{"POST", "/api/demo/start", "{\"unexpected\":true}", "", 400},
		{"POST", "/api/demo/start", "{} {}", "", 400},
		{"POST", "/api/demo/start", "{}", "https://another.example", 403},
		{"POST", "/api/candidates/candidate-01/decision", "{\"action\":\"hire\"}", "", 409},
		{"POST", "/api/candidates/candidate-01/decision", "{\"action\":\"invalid\"}", "", 400},
		{"POST", "/api/demo/step", "{}", "", 200},
		{"POST", "/api/demo/start", "{}", "", 200},
		{"POST", "/api/demo/step", "{}", "", 409},
		{"POST", "/api/demo/pause", "{}", "", 200},
		{"GET", "/api/missing", "", "", 404},
		{"GET", "/missing.js", "", "", 404},
		{"GET", "/", "", "", 200},
	}
	for _, tc := range cases {
		req := localRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d: %s", tc.method, tc.path, w.Code, tc.status, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, localRequest("GET", "/api/state", nil))
	var state pipeline.State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Candidates[0].Stage != "Applicant" {
		t.Fatal("successful step did not persist")
	}
}

func TestConnectionsAPIAndExportsRedactCredentials(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	store, _ := connections.New("")
	h := New(e, fstest.MapFS{"index.html": {Data: []byte("dashboard")}}, store)
	update := connections.Update{AI: store.Public().AI, APIKey: "secret-for-api-test", Bots: []connections.Binding{}}
	update.AI.Model = "user-selected-model"
	b, _ := json.Marshal(update)
	req := localRequest("POST", "/api/connections", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), update.APIKey) || !strings.Contains(w.Body.String(), `"has_key":true`) {
		t.Fatal("settings response exposed or lost credential")
	}
	for _, path := range []string{"/api/state", "/api/export", "/api/connections"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, localRequest("GET", path, nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), update.APIKey) {
			t.Fatalf("unsafe response from %s", path)
		}
	}
	body := `{"name":"Web Candidate","profile_id":"` + e.Snapshot().Profiles[0].ID + `","binding_agent":""}`
	req = localRequest("POST", "/api/interviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"created_candidate_id"`) || !strings.Contains(w.Body.String(), `"mode":"live"`) {
		t.Fatal("real interview was not created through API")
	}
}

func localRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = "127.0.0.1:8083"
	return r
}

func TestDiscordSecretInputAndLocalHostBoundary(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	h := New(e, fstest.MapFS{"index.html": {Data: []byte("dashboard")}})
	for _, host := range []string{"attacker.example:8083", "127.0.0.1.attacker.example", "example.com"} {
		r := localRequest("POST", "/api/discord/connect", strings.NewReader(`{"token":"secret"}`))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("nonlocal Host accepted", host)
		}
	}
	r := localRequest("POST", "/api/discord/connect", strings.NewReader(`{"token":"short-secret"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 502 || strings.Contains(w.Body.String(), "short-secret") {
		t.Fatal("invalid credential leaked")
	}
	for _, path := range []string{"/api/discord/status", "/api/state", "/api/export", "/api/connections"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, localRequest("GET", path, nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), "short-secret") {
			t.Fatal("secret leaked from", path)
		}
	}
}
