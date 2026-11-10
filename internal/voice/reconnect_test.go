package voice

import (
	"context"
	"encoding/json"
	"ladder/internal/connections"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCandidateGraceIsFiveMinutesAndOnlyTheirReturnResetsIt(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	d := nextDeadline(now, false, nil)
	if d.Sub(now) != 5*time.Minute {
		t.Fatal("wrong reconnect window")
	}
	if nextDeadline(now.Add(2*time.Minute), false, d) != d {
		t.Fatal("absence restarted the five-minute window")
	}
	if nextDeadline(now.Add(3*time.Minute), true, d) != nil {
		t.Fatal("candidate return did not cancel expiry")
	}
}

func TestRestoreStartsInterviewWhenCandidateStayedInRoomAndGatewayCacheIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"message":"Describe your approach to a difficult task.","completed":false}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	id, err := e.CreateInterview("Stayed Candidate", e.Snapshot().Profiles[0].ID, "", &discordrooms.Plan{GuildID: "guild", MemberID: "member", ChannelID: "room", Payload: discordrooms.Payload{Name: "interview #1"}})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := connections.New("")
	if err := store.Save(connections.Update{AI: connections.AI{Provider: "compatible", BaseURL: server.URL, Model: "fixture", TokenField: "max_tokens", MaxQuestions: 6}}); err != nil {
		t.Fatal(err)
	}
	// No Gateway presence events are sent. REST alone confirms they never left.
	backend := &testBackend{candidateChannel: "room"}
	m := connectedTestManager(t, backend)
	s := New(context.Background(), e, store, m)
	defer s.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	a := &session{id: id, guild: "guild", channel: "room", member: "member", ctx: ctx, cancel: cancel, ready: true, people: map[string]pipeline.Participant{}}
	s.active = a
	s.refreshPresence(a)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := e.Candidate(id)
		s.mu.Lock()
		busy := a.busy
		ready := a.presenceReady
		s.mu.Unlock()
		if len(c.Transcript) == 2 && c.Transcript[1].Speaker == "Interviewer" && !busy {
			if !ready || len(c.Room.Participants) != 1 || c.Live.Voice.EmptyDeadline != nil {
				t.Fatal("restored presence was not saved")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("AI did not resume without requiring the candidate to leave/rejoin")
}

func TestExpiredRoomIsRemovedWhenCandidateIsAbsentEvenIfReviewerRemains(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	id, err := e.CreateInterview("Test Candidate", e.Snapshot().Profiles[0].ID, "", &discordrooms.Plan{GuildID: "guild", MemberID: "member", ChannelID: "room", Payload: discordrooms.Payload{Name: "interview-owned"}})
	if err != nil {
		t.Fatal(err)
	}
	backend := &testBackend{present: []string{"reviewer"}}
	m := connectedTestManager(t, backend)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &session{id: id, guild: "guild", channel: "room", member: "member", ctx: ctx, cancel: cancel, people: map[string]pipeline.Participant{}, closing: true}
	s := &Service{ctx: ctx, active: a, bot: m, engine: e}
	s.cleanup(a)
	c, _ := e.Candidate(id)
	if backend.deleted != 1 || s.active != nil || !c.Room.Expired {
		t.Fatal("candidate absence policy did not close the room")
	}
}
