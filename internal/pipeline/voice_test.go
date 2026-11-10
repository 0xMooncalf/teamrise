package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"ladder/internal/connections"
	"ladder/internal/discordrooms"
	"ladder/internal/providers"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVoiceWrittenTurnRequiresChatAndIncompleteClosurePreservesAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message":       map[string]string{"content": `{"message":"What does this code do?","completed":false}`},
			"finish_reason": "stop",
		}}})
	}))
	defer srv.Close()
	e := newTestEngine(t)
	id := voiceInterview(t, e)
	p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens", MaxQuestions: 3}, Key: "test", Voice: true}
	run := func(action, text string) error {
		c, _ := e.Candidate(id)
		return e.RunAI(context.Background(), id, action, text, c.Live.Version, p)
	}
	if err := run("start", ""); err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate(id)
	if c.Transcript[0].Speaker != "System" || len(historyFor(c)) != 1 || answers(c) != 0 {
		t.Fatal("greeting must not count as an answer or be sent to the model")
	}
	if err := run("answer", "My spoken answer."); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if !c.WaitingForChat() {
		t.Fatal("second question was not routed to chat")
	}
	if err := run("answer", "Wrong input mode."); !errors.Is(err, ErrConflict) {
		t.Fatalf("voice input accepted for a written exercise: %v", err)
	}
	if err := run("answer_chat", "My written answer."); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if c.WaitingForChat() || answers(c) != 2 || c.Transcript[len(c.Transcript)-2].Channel != "chat" {
		t.Fatal("chat answer did not resume the voice interview")
	}
	if err := e.SetVoiceState(id, "closed", "", false); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if c.Live.Status != "failed" || c.Live.Failure == "" || answers(c) != 2 || c.Assessment != nil {
		t.Fatal("incomplete closure lost the transcript or fabricated a result")
	}
}

func voiceInterview(t *testing.T, e *Engine) string {
	t.Helper()
	id, err := e.CreateInterview("Voice Candidate", e.Snapshot().Profiles[0].ID, "", &discordrooms.Plan{GuildID: "guild", MemberID: "member", ChannelID: "channel", Payload: discordrooms.Payload{Name: "interview-owned"}})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVoicePresenceAndSpeechSurviveInflightModelResult(t *testing.T) {
	e := newTestEngine(t)
	id := voiceInterview(t, e)
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"message":"Describe a project.","completed":false}`}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}})
	}))
	defer srv.Close()
	p := providers.Compatible{Config: connections.AI{BaseURL: srv.URL, Model: "test", TokenField: "max_tokens", MaxQuestions: 3}, Key: "test", Voice: true}
	c, _ := e.Candidate(id)
	done := make(chan error, 1)
	go func() { done <- e.RunAI(context.Background(), id, "start", "", c.Live.Version, p) }()
	<-started
	if err := e.SetVoiceState(id, "listening", "", true); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordSpeech(id, "synthesis", 3.5, 40, 160); err != nil {
		t.Fatal(err)
	}
	people := []Participant{{ID: "member", Name: "Candidate", Kind: "candidate"}, {ID: "reviewer", Name: "Actual Reviewer", Kind: "admin"}}
	if err := e.VoicePresence(id, people, nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if len(c.Room.Participants) != 2 || !c.Room.BotPresent || c.Live.Reviewer != "Actual Reviewer" || len(c.Live.Voice.Calls) != 1 || c.Live.Voice.State != "listening" {
		t.Fatalf("voice runtime lost during inference: %+v", c.Live)
	}
	if len(c.Live.Calls) != 1 || c.Live.Calls[0].Usage.TotalTokens != 18 {
		t.Fatal("text usage lost or speech counted as model tokens")
	}
}

func TestVoiceStopRequiresActualReviewerEntryAndSurvivesRestart(t *testing.T) {
	e := newTestEngine(t)
	id := voiceInterview(t, e)
	if err := e.SetVoiceState(id, "stopped", "Candidate stopped", true); err != nil {
		t.Fatal(err)
	}
	r := requestFor(t, e, id)
	if err := e.AdminRequest(r.ID, "acknowledge", "admin-maya"); err != ErrConflict {
		t.Fatal("web testing identity claimed real voice attendance")
	}
	if err := e.VoicePresence(id, []Participant{{ID: "stranger", Name: "Stranger", Kind: "guest"}}, nil); err != nil {
		t.Fatal(err)
	}
	if requestFor(t, e, id).Status != "pending" {
		t.Fatal("guest confirmed attendance")
	}
	if err := e.VoicePresence(id, []Participant{{ID: "reviewer", Name: "Real Reviewer", Kind: "admin"}}, nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range e.Snapshot().Requests {
		if r.CandidateID == id {
			found = r.Status == "attended" && r.AttendedBy == "Real Reviewer"
		}
	}
	if !found {
		t.Fatal("actual reviewer did not dim the shared request")
	}
	c, _ := e.Candidate(id)
	if !c.Live.Voice.Stopped || c.Assessment != nil || c.Live.Scored {
		t.Fatal("stopped session fabricated assessment")
	}
	if _, err := e.reserveAI(id, "start", "", c.Live.Version, providers.Compatible{}); err != ErrConflict {
		t.Fatal("stopped interview resumed")
	}
	deadline := time.Now().Add(10 * time.Minute)
	_ = e.VoicePresence(id, []Participant{}, &deadline)
	reopened, err := New(e.journal.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	c, _ = reopened.Candidate(id)
	if c.Room.BotPresent || !c.Live.Voice.Stopped || len(c.Room.Participants) != 0 {
		t.Fatal("restart invented live presence or lost stop")
	}
}
