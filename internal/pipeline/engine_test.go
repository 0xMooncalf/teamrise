package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}
func step(t *testing.T, e *Engine, n int) {
	t.Helper()
	for range n {
		if err := e.Control("step"); err != nil {
			t.Fatal(err)
		}
	}
}
func finish(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.Control("start"); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 800 && e.Snapshot().Running; n++ {
		if err := e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if e.Snapshot().Running {
		t.Fatal("demo did not finish")
	}
}
func requestFor(t *testing.T, e *Engine, id string) AdminRequest {
	t.Helper()
	for _, r := range e.Snapshot().Requests {
		if r.CandidateID == id && r.Status == "pending" {
			return r
		}
	}
	t.Fatalf("no pending request for %s", id)
	return AdminRequest{}
}
func confirm(t *testing.T, e *Engine, id string) {
	t.Helper()
	r := requestFor(t, e, id)
	if err := e.AdminRequest(r.ID, "confirm", "admin-maya"); err != nil {
		t.Fatal(err)
	}
}

func TestParallelInterviewsAndHumanGate(t *testing.T) {
	e := newTestEngine(t)
	if err := e.Decision("candidate-01", "hire"); !errors.Is(err, ErrConflict) {
		t.Fatal("early hire accepted")
	}
	step(t, e, 1)
	active := 0
	for _, c := range e.Snapshot().Candidates {
		if c.Runs[0].Status == "active" {
			active++
		}
	}
	if active != 4 {
		t.Fatalf("expected four concurrent candidates, got %d", active)
	}
	step(t, e, 8)
	s := e.Snapshot()
	if s.Stats.ActiveInterviews < 2 {
		t.Fatalf("interviews are not concurrent: %+v", s.Stats)
	}
	rooms := map[string]bool{}
	for _, c := range s.Candidates {
		if c.Room.BotPresent {
			rooms[c.Room.ID] = true
		}
	}
	if len(rooms) < 2 {
		t.Fatal("candidates did not get separate rooms")
	}
	finish(t, e)
	s = e.Snapshot()
	final, review := 0, 0
	for _, c := range s.Candidates {
		switch c.Stage {
		case "Final":
			final++
		case "Review":
			review++
			if c.Phase != 4 {
				t.Fatal("a flagged candidate advanced automatically")
			}
		default:
			t.Fatalf("unexpected final stage %s", c.Stage)
		}
		if c.Assessment == nil || len(c.Scorecard) == 0 {
			t.Fatal("missing narrative assessment")
		}
	}
	if final == 0 || review == 0 || len(s.Requests) != 40 {
		t.Fatalf("final=%d review=%d requests=%d", final, review, len(s.Requests))
	}
	if s.Stats.InterviewSeconds <= s.ClockSeconds {
		t.Fatal("aggregate interview time should demonstrate parallel overlap")
	}
	r := requestFor(t, e, "candidate-02")
	if err := e.AdminRequest(r.ID, "open", "admin-maya"); err != nil {
		t.Fatal(err)
	}
	if err := e.Decision("candidate-02", "hire"); !errors.Is(err, ErrConflict) {
		t.Fatal("opening a link must not count as admin presence")
	}
	confirm(t, e, "candidate-02")
	if err := e.Decision("candidate-02", "hire"); err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate("candidate-02")
	if !c.Room.BotPresent || c.Stage != "Hired" {
		t.Fatal("bot left when a human recorded a decision")
	}
	if err := e.Decision(c.ID, "hire"); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate decision accepted")
	}
	confirm(t, e, "candidate-01")
	if err := e.Decision("candidate-01", "resolve"); err != nil {
		t.Fatal(err)
	}
	finish(t, e)
	c, _ = e.Candidate("candidate-01")
	if c.Stage != "Final" {
		t.Fatal("resolved candidate did not reach final")
	}
}
func TestSharedAdminEntryIsAtomic(t *testing.T) {
	e := newTestEngine(t)
	finish(t, e)
	r := requestFor(t, e, "candidate-02")
	if err := e.AdminRequest(r.ID, "open", "admin-maya"); err != nil {
		t.Fatal(err)
	}
	if requestFor(t, e, "candidate-02").Status != "pending" {
		t.Fatal("link opening hid the shared request")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, admin := range []string{"admin-maya", "admin-theo"} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); results <- e.AdminRequest(r.ID, "confirm", id) }(admin)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("entry race: success=%d conflict=%d", successes, conflicts)
	}
	c, _ := e.Candidate("candidate-02")
	admins := 0
	for _, p := range c.Room.Participants {
		if p.Kind == "admin" {
			admins++
		}
	}
	if admins != 1 {
		t.Fatal("more than one admin claimed the entry")
	}
}
func TestRoomReconnectWindowAndBotPresence(t *testing.T) {
	e := newTestEngine(t)
	finish(t, e)
	confirm(t, e, "candidate-02")
	if err := e.RoomAction("candidate-02", "candidate-leave", "admin-maya"); err != nil {
		t.Fatal(err)
	}
	if err := e.AdvanceTime(299); err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate("candidate-02")
	if !c.Room.BotPresent || c.Room.ReconnectDeadline == nil {
		t.Fatal("candidate absence did not start five-minute timer with reviewer present")
	}
	if err := e.RoomAction(c.ID, "candidate-rejoin", "admin-maya"); err != nil {
		t.Fatal(err)
	}
	_ = e.AdvanceTime(600)
	c, _ = e.Candidate(c.ID)
	if !c.Room.BotPresent || c.Room.ReconnectDeadline != nil {
		t.Fatal("reconnection did not cancel the timeout")
	}
	_ = e.RoomAction(c.ID, "candidate-leave", "admin-maya")
	_ = e.AdvanceTime(299)
	c, _ = e.Candidate(c.ID)
	if !c.Room.BotPresent {
		t.Fatal("room closed early")
	}
	_ = e.AdvanceTime(1)
	c, _ = e.Candidate(c.ID)
	if c.Room.BotPresent || !c.Room.Expired {
		t.Fatal("bot did not leave at the timeout while reviewer remained")
	}
	if err := e.RoomAction(c.ID, "candidate-rejoin", "admin-maya"); !errors.Is(err, ErrConflict) {
		t.Fatal("expired session silently reopened")
	}
}
func TestProfilesAreEditablePinnedAndPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := DefaultProfiles()[2]
	p.ID = "warehouse"
	p.Name = "Warehouse cleaner"
	p.Questions = []string{"Explain your safe procedure for an unlabeled chemical."}
	p.SkillPrompt = "You are a warehouse cleaning specialist. Discuss safety, equipment, and practical work."
	if err := e.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err := e.AssignProfile("candidate-01", p.ID); err != nil {
		t.Fatal(err)
	}
	step(t, e, 1)
	p.SkillPrompt = "Changed prompt for future warehouse interviews only."
	p.Questions = []string{"A changed question for future interviews."}
	if err := e.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err := e.AssignProfile("candidate-01", "backend"); !errors.Is(err, ErrConflict) {
		t.Fatal("changed a running interview's skill")
	}
	step(t, e, 16)
	c, _ := e.Candidate("candidate-01")
	if len(c.Transcript) == 0 || !strings.Contains(c.Transcript[0].Text, "unlabeled chemical") {
		t.Fatal("custom question was not used")
	}
	if c.Profile.SkillPrompt == p.SkillPrompt {
		t.Fatal("live interview prompt was not pinned")
	}
	if err := e.SaveSettings(Settings{Concurrent: 2, HandoffScore: 75}); err != nil {
		t.Fatal(err)
	}
	before := e.Snapshot()
	_ = e.Close()
	e, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	after := e.Snapshot()
	if after.Revision != before.Revision || after.Settings.Concurrent != 2 {
		t.Fatal("lost shared settings")
	}
	c, _ = e.Candidate("candidate-01")
	if c.Profile.ID != "warehouse" {
		t.Fatal("lost candidate profile snapshot")
	}
	_ = e.Control("reset")
	s := e.Snapshot()
	if len(s.Requests) != 0 || s.ClockSeconds != 0 || s.Settings.Concurrent != 2 || len(s.Profiles) != 5 {
		t.Fatal("reset did not preserve configuration while clearing the run")
	}
}
func TestSnapshotIsolationAndFailedPersistence(t *testing.T) {
	e := newTestEngine(t)
	finish(t, e)
	s := e.Snapshot()
	s.Candidates[0].Transcript[0].Text = "changed"
	s.Candidates[0].Profile.Questions[0] = "changed"
	s.Requests[0].Status = "closed"
	c, _ := e.Candidate("candidate-01")
	if c.Transcript[0].Text == "changed" || c.Profile.Questions[0] == "changed" {
		t.Fatal("snapshot aliases engine state")
	}
	before := e.Snapshot()
	_ = e.journal.Close()
	if err := e.Control("step"); err == nil {
		t.Fatal("closed journal accepted a write")
	}
	if e.Snapshot().Revision != before.Revision {
		t.Fatal("failed persistence advanced the state")
	}
}
func TestInvalidConfigurationAndMalformedJournal(t *testing.T) {
	e := newTestEngine(t)
	if err := e.SaveSettings(Settings{Concurrent: 0, HandoffScore: 80}); !errors.Is(err, ErrValidation) {
		t.Fatal("invalid concurrency accepted")
	}
	p := DefaultProfiles()[0]
	p.SkillPrompt = ""
	if err := e.SaveProfile(p); !errors.Is(err, ErrValidation) {
		t.Fatal("empty prompt accepted")
	}
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	_ = os.WriteFile(path, []byte("{broken\n"), 0600)
	if _, err := New(path); err == nil {
		t.Fatal("malformed journal accepted")
	}
}

func TestV01JournalRemainsReadableWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	original := []byte("{\"event\":{\"id\":1,\"agent\":\"Chief\",\"kind\":\"progress\",\"text\":\"Legacy final\"},\"candidate\":{\"id\":\"candidate-01\",\"name\":\"Legacy Candidate\",\"job\":\"Senior Go Engineer\",\"stage\":\"Final\",\"phase\":6,\"score\":82,\"transcript\":[{\"speaker\":\"Interviewer\",\"text\":\"A legacy question\"},{\"speaker\":\"Legacy Candidate\",\"text\":\"A legacy answer\"}]}}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate("candidate-01")
	_ = e.Close()
	if c.Phase != 7 || c.Stage != "Final" || len(c.Runs) != 7 || c.Assessment == nil {
		t.Fatal("legacy state was not normalized")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("reading a legacy journal rewrote it")
	}
}
