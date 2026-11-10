package pipeline

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrConflict = errors.New("action is not available at the current stage")
var ErrNotFound = errors.New("record not found")
var ErrValidation = errors.New("invalid configuration")

type record struct {
	VoiceRoomSequence *int               `json:"voice_room_sequence,omitempty"`
	Event             Event              `json:"event"`
	Candidate         *Candidate         `json:"candidate,omitempty"`
	Candidates        []Candidate        `json:"candidates,omitempty"`
	Reset             bool               `json:"reset,omitempty"`
	Profiles          []InterviewProfile `json:"profiles,omitempty"`
	Settings          *Settings          `json:"settings,omitempty"`
	Requests          *[]AdminRequest    `json:"requests,omitempty"`
	Clock             *int               `json:"clock_seconds,omitempty"`
	Running           *bool              `json:"running,omitempty"`
}
type Engine struct {
	voiceRoomSequence int
	mu                sync.Mutex
	candidates        []Candidate
	events            []Event
	running           bool
	revision          int
	journal           *os.File
	profiles          []InterviewProfile
	settings          Settings
	requests          []AdminRequest
	clock             int
}

func clone[T any](value T) T {
	b, _ := json.Marshal(value)
	var result T
	_ = json.Unmarshal(b, &result)
	return result
}
func candidateNumber(c Candidate) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(c.ID, "candidate-"))
	return n
}
func (e *Engine) profile(id string) InterviewProfile {
	for _, p := range e.profiles {
		if p.ID == id {
			return clone(p)
		}
	}
	return clone(e.profiles[0])
}
func (e *Engine) normalize(c *Candidate) {
	if c.ProfileID == "" {
		c.ProfileID = profileIDForJob(c.Job)
	}
	if c.Profile.ID == "" {
		c.Profile = e.profile(c.ProfileID)
	}
	if len(c.Runs) == 0 {
		if c.Phase >= 6 {
			c.Phase = 7
		}
		for i, a := range Agents {
			status := "idle"
			if i < c.Phase {
				status = "done"
			}
			c.Runs = append(c.Runs, AgentRun{Agent: a.Name, Status: status})
		}
	}
	if c.Room.ID == "" {
		c.Room = Room{ID: "room-" + c.ID, Name: fmt.Sprintf("interview-%02d", candidateNumber(*c)), Participants: []Participant{}}
		if c.Phase > 1 {
			c.Room.BotPresent = true
			c.Room.Participants = append(c.Room.Participants, Participant{ID: c.ID, Name: c.Name, Kind: "candidate"})
		}
	}
	if c.Transcript == nil {
		c.Transcript = []Message{}
	}
	if c.Live != nil && c.Live.Voice != nil && (c.Live.Voice.Stopped || c.Live.Voice.State == "closed") && c.Live.Status != "complete" && c.Live.Status != "failed" {
		c.Live.Failure = "The interview ended before all questions were answered."
		c.Live.Status = "failed"
	}
	if c.Scorecard == nil {
		c.Scorecard = []Score{}
	}
	if c.Feedback == nil {
		c.Feedback = []string{}
	}
	if c.Mode != "live" && c.Assessment == nil && c.Score > 0 {
		c.Assessment, _, _ = assessmentFor(*c)
	}
}
func New(path string) (*Engine, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	e := &Engine{candidates: Seed(), events: []Event{}, journal: f, profiles: DefaultProfiles(), settings: Settings{Concurrent: 4, HandoffScore: 80}, requests: []AdminRequest{}}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 8<<20)
	for s.Scan() {
		var r record
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			f.Close()
			return nil, fmt.Errorf("invalid journal entry: %w", err)
		}
		if r.Event.ID != e.revision+1 {
			f.Close()
			return nil, errors.New("journal sequence is invalid")
		}
		e.apply(r)
	}
	if err := s.Err(); err != nil {
		f.Close()
		return nil, err
	}
	for i := range e.candidates {
		e.normalize(&e.candidates[i])
		if l := e.candidates[i].Live; l != nil && l.Voice != nil && l.Voice.State != "closed" {
			e.candidates[i].Room.BotPresent = false
			e.candidates[i].Room.Participants = []Participant{}
			l.Discord.Status = "disconnected"
			if !l.Voice.Stopped {
				l.Voice.State = "connection_lost"
				l.Voice.Error = "Reconnect the bot and enable voice to recover this room."
			}
		}
		if l := e.candidates[i].Live; l != nil && (l.Status == "generating" || l.Status == "analyzing") {
			l.Status = "error"
			l.Error = "The app stopped during the AI request. Retry to continue."
		}
		if e.candidates[i].Stage == "Final" || e.candidates[i].Stage == "Review" {
			e.ensureRequest(&e.candidates[i], &e.requests, e.candidates[i].Stage, e.clock)
		}
	}
	e.running = false
	return e, nil
}
func (e *Engine) Close() error { e.mu.Lock(); defer e.mu.Unlock(); return e.journal.Close() }
func (e *Engine) apply(r record) {
	if r.VoiceRoomSequence != nil {
		e.voiceRoomSequence = *r.VoiceRoomSequence
	}
	if len(r.Profiles) > 0 {
		e.profiles = r.Profiles
	}
	if r.Settings != nil {
		e.settings = *r.Settings
	}
	if r.Reset {
		live := []Candidate{}
		for _, c := range e.candidates {
			if c.Mode == "live" {
				live = append(live, c)
			}
		}
		e.candidates = append(Seed(), live...)
		for i := range e.candidates {
			e.normalize(&e.candidates[i])
		}
		liveEvents := []Event{}
		for _, ev := range e.events {
			for _, c := range live {
				if ev.CandidateID == c.ID {
					liveEvents = append(liveEvents, ev)
				}
			}
		}
		e.events = liveEvents
		liveRequests := []AdminRequest{}
		for _, req := range e.requests {
			for _, c := range live {
				if req.CandidateID == c.ID {
					liveRequests = append(liveRequests, req)
				}
			}
		}
		e.requests = liveRequests
		e.clock = 0
		e.running = false
	}
	changes := r.Candidates
	if r.Candidate != nil {
		changes = append(changes, *r.Candidate)
	}
	for _, c := range changes {
		found := false
		for i := range e.candidates {
			if e.candidates[i].ID == c.ID {
				e.candidates[i] = clone(c)
				found = true
				break
			}
		}
		if !found {
			e.candidates = append(e.candidates, clone(c))
		}
	}
	if r.Requests != nil {
		e.requests = *r.Requests
	}
	if r.Clock != nil {
		e.clock = *r.Clock
	}
	if r.Running != nil {
		e.running = *r.Running
	}
	e.events = append(e.events, r.Event)
	e.revision = r.Event.ID
}
func (e *Engine) commit(r record) error {
	r.Event.ID = e.revision + 1
	r.Event.Time = time.Now().UTC()
	if r.Event.Channel == "" {
		r.Event.Channel = "pipeline-status"
	}
	if r.Candidate != nil {
		r.Event.CandidateID = r.Candidate.ID
		r.Event.CandidateName = r.Candidate.Name
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	written, err := e.journal.Write(b)
	if err != nil {
		return err
	}
	if written != len(b) {
		return io.ErrShortWrite
	}
	if err := e.journal.Sync(); err != nil {
		return err
	}
	e.apply(r)
	return nil
}
func (e *Engine) Snapshot() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	stats := Stats{}
	for _, c := range e.candidates {
		if c.Mode == "live" {
			continue
		} // Real elapsed time is displayed separately from the demo clock.
		stats.InterviewSeconds += c.InterviewSeconds
		if c.Phase > 1 {
			stats.CompletedInterviews++
		}
		if c.Room.BotPresent && len(c.Room.Participants) > 0 {
			stats.LiveRooms++
		}
		if c.Phase == 1 && c.Runs[1].Status == "active" && hasCandidate(c) {
			stats.ActiveInterviews++
		}
	}
	for _, r := range e.requests {
		if r.Status == "pending" {
			stats.PendingRequests++
		}
	}
	return clone(State{Demo: true, Running: e.running, Revision: e.revision, Candidates: e.candidates, Events: e.events, Agents: Agents, Profiles: e.profiles, Settings: e.settings, Requests: e.requests, ClockSeconds: e.clock, Stats: stats})
}
func (e *Engine) Candidate(id string) (Candidate, error) {
	for _, c := range e.Snapshot().Candidates {
		if c.ID == id {
			return c, nil
		}
	}
	return Candidate{}, ErrNotFound
}
func event(agent, kind, text string) Event { return Event{Agent: agent, Kind: kind, Text: text} }
func (e *Engine) Control(action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch action {
	case "start":
		if e.running {
			return nil
		}
		running := true
		return e.commit(record{Event: event("Chief", "control", "Parallel demo started. Rooms and admin requests are shared across all open dashboards."), Running: &running})
	case "pause":
		if !e.running {
			return nil
		}
		running := false
		return e.commit(record{Event: event("Chief", "control", "Simulation paused. All candidate progress is saved."), Running: &running})
	case "reset":
		return e.commit(record{Event: event("Chief", "control", "New demo run. Interview profiles and workspace settings were preserved."), Reset: true})
	case "step":
		if e.running {
			return ErrConflict
		}
		return e.advance(30, true)
	default:
		return fmt.Errorf("%w: unknown action", ErrValidation)
	}
}
func (e *Engine) Tick() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.expireInterviewsLocked(time.Now()); err != nil {
		return err
	}
	if !e.running {
		return nil
	}
	return e.advance(30, true)
}
func eligible(c Candidate) bool {
	return c.Mode != "live" && c.Phase < 7 && c.Stage != "Hired" && c.Stage != "Rejected" && c.Stage != "Review" && !c.Room.Expired && c.Room.EmptySince == nil
}
func hasCandidate(c Candidate) bool {
	for _, p := range c.Room.Participants {
		if p.Kind == "candidate" {
			return true
		}
	}
	return false
}
func hasAdmin(c Candidate) bool {
	if c.Mode == "live" {
		return c.Live != nil && c.Live.Reviewer != ""
	}
	for _, p := range c.Room.Participants {
		if p.Kind == "admin" {
			return true
		}
	}
	return false
}
func (e *Engine) ensureRequest(c *Candidate, requests *[]AdminRequest, reason string, clock int) {
	for _, r := range *requests {
		if r.CandidateID == c.ID && (r.Status == "pending" || r.Status == "attended" && hasAdmin(*c)) {
			return
		}
	}
	*requests = append(*requests, AdminRequest{ID: fmt.Sprintf("request-%s-%d", c.ID, e.revision+len(*requests)+1), CandidateID: c.ID, CandidateName: c.Name, RoomID: c.Room.ID, Reason: reason, Status: "pending", CreatedAt: clock, OpenedBy: []string{}})
}
func expireRoom(c *Candidate, requests []AdminRequest, clock int) bool {
	if c.Room.BotPresent && !hasCandidate(*c) && c.Room.ReconnectDeadline != nil && clock >= *c.Room.ReconnectDeadline {
		c.Room.BotPresent = false
		c.Room.Expired = true
		c.Room.EmptySince = nil
		c.Room.ReconnectDeadline = nil
		for i := range requests {
			if requests[i].CandidateID == c.ID && requests[i].Status != "closed" {
				requests[i].Status = "closed"
			}
		}
		return true
	}
	return false
}
func (e *Engine) advance(seconds int, workflow bool) error {
	// A demo tick never republishes or rewrites growing real transcripts.
	demo := []Candidate{}
	for _, c := range e.candidates {
		if c.Mode != "live" {
			demo = append(demo, c)
		}
	}
	cs := clone(demo)
	requests := clone(e.requests)
	clock := e.clock + seconds
	messages := []string{}
	actor := "Chief"
	slots := 0
	for _, c := range cs {
		if eligible(c) && c.Phase > 0 {
			slots++
		}
	}
	for i := range cs {
		c := &cs[i]
		if expireRoom(c, requests, clock) {
			messages = append(messages, c.Name+": bot left after the 5-minute candidate reconnect window.")
		}
		if !workflow || !eligible(*c) {
			continue
		}
		if c.Phase == 0 && slots >= e.settings.Concurrent {
			continue
		}
		if c.Phase == 0 {
			slots++
		}
		phase := c.Phase
		run := &c.Runs[phase]
		actor = run.Agent
		if run.Status == "idle" {
			if phase == 1 && c.NoShow && !c.Rescheduled {
				c.Rescheduled = true
				c.Stage = "Rescheduled"
				messages = append(messages, c.Name+": Concierge prepared a replacement demo slot.")
				continue
			}
			run.Status = "active"
			run.Ticks = 0
			if phase == 0 {
				c.Profile = e.profile(c.ProfileID)
				c.Stage = "Applicant"
				c.Role = "Applicant"
			}
			if phase == 1 {
				c.Stage = "Interviewing"
				c.Room.BotPresent = true
				c.Room.Participants = []Participant{{ID: c.ID, Name: c.Name, Kind: "candidate"}}
			}
			messages = append(messages, c.Name+": "+run.Agent+" started.")
			continue
		}
		if phase == 1 && !hasCandidate(*c) {
			continue
		}
		run.Ticks++
		duration := 2 + (candidateNumber(*c)+phase)%2
		if phase == 1 {
			c.InterviewSeconds += seconds
			plan := PlannedTranscript(*c)
			duration = len(plan) + 1
			if len(c.Transcript) < len(plan) {
				c.Transcript = append(c.Transcript, plan[len(c.Transcript)])
			}
		}
		if run.Ticks < duration {
			continue
		}
		run.Status = "done"
		switch phase {
		case 0:
			run.Summary = "Matched to " + c.Profile.Name + ". Skill prompt pinned for this interview."
		case 1:
			c.Stage = "Screened"
			c.Role = "Screened"
			run.Summary = fmt.Sprintf("Completed %d questions in %d simulated seconds.", len(c.Profile.Questions), c.InterviewSeconds)
		case 2:
			c.Assessment, c.Scorecard, c.Score = assessmentFor(*c)
			c.Stage = "Assessed"
			run.Summary = c.Assessment.Summary
		case 3:
			if c.Score < e.settings.HandoffScore || candidateNumber(*c)%7 == 0 {
				c.Flag = "More concrete evidence is required before a decision. Please review the transcript and role criteria."
				c.Stage = "Review"
				run.Summary = "Human evidence review requested."
				e.ensureRequest(c, &requests, "Evidence review", clock)
			} else {
				c.Stage = "Tech Passed"
				c.Role = "Tech Passed"
				run.Summary = "Demo evidence check complete; no scenario flag."
			}
		case 4:
			run.Summary = "Dedicated room is connected. Bot will stay while participants remain."
		case 5:
			run.Summary = "Handoff package prepared: transcript, observations, scorecard, and next steps."
		case 6:
			c.Stage = "Final"
			c.Role = "Final"
			run.Summary = "A human is needed for the final interview and decision."
			e.ensureRequest(c, &requests, "Final interview", clock)
		}
		c.Phase++
		messages = append(messages, c.Name+": "+run.Agent+" completed.")
		if c.Stage == "Review" || c.Phase >= 7 {
			slots--
		}
	}
	running := e.running
	remaining := false
	for _, c := range cs {
		if eligible(c) || c.Room.ReconnectDeadline != nil {
			remaining = true
			break
		}
	}
	if !remaining {
		running = false
	}
	text := "Parallel rooms are progressing. Each demo tick represents 30 seconds."
	if !workflow {
		text = fmt.Sprintf("Advanced the room clock by %d seconds without advancing interviews.", seconds)
	} else if len(messages) > 0 {
		text = strings.Join(messages, " ")
	}
	return e.commit(record{Event: event(actor, "progress", text), Candidates: cs, Requests: &requests, Clock: &clock, Running: &running})
}
func (e *Engine) AdvanceTime(seconds int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if seconds < 1 || seconds > 600 {
		return fmt.Errorf("%w: advance 1–600 seconds", ErrValidation)
	}
	return e.advance(seconds, false)
}
func (e *Engine) SaveProfile(p InterviewProfile) error {
	if len(p.Cards) > 0 {
		p.Questions = make([]string, 0, len(p.Cards))
		for _, card := range p.Cards {
			p.Questions = append(p.Questions, strings.TrimSpace(card.Prompt))
		}
	}
	if err := validateProfile(p); err != nil {
		return fmt.Errorf("%w: %s", ErrValidation, err)
	}
	for _, card := range p.Cards {
		if card.Image != "" {
			path, err := e.screenshotPath(card.Image)
			if err != nil {
				return fmt.Errorf("%w: invalid screenshot", ErrValidation)
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("%w: upload the screenshot before saving the skill", ErrValidation)
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	profiles := clone(e.profiles)
	found := false
	for i := range profiles {
		if profiles[i].ID == p.ID {
			profiles[i] = p
			found = true
		}
	}
	if !found {
		if len(profiles) >= 20 {
			return fmt.Errorf("%w: at most 20 profiles", ErrValidation)
		}
		profiles = append(profiles, p)
	}
	return e.commit(record{Event: event("Human", "configuration", "Saved interview profile: "+p.Name+". Interviews already started keep their pinned profile."), Profiles: profiles})
}
func (e *Engine) SaveSettings(s Settings) error {
	if s.Concurrent < 1 || s.Concurrent > 8 || s.HandoffScore < 0 || s.HandoffScore > 100 {
		return fmt.Errorf("%w: use 1–8 rooms and a score threshold of 0–100", ErrValidation)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.commit(record{Event: event("Human", "configuration", "Updated concurrent room limit and human-review threshold."), Settings: &s})
}
func (e *Engine) AssignProfile(id, profileID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	exists := false
	for _, p := range e.profiles {
		if p.ID == profileID {
			exists = true
		}
	}
	if !exists {
		return ErrNotFound
	}
	if id == "queued" {
		changes := []Candidate{}
		for _, original := range e.candidates {
			if original.Mode != "live" && original.Phase == 0 && original.Runs[0].Status == "idle" {
				c := clone(original)
				c.ProfileID = profileID
				c.Profile = e.profile(profileID)
				c.Job = c.Profile.Name
				changes = append(changes, c)
			}
		}
		if len(changes) == 0 {
			return ErrConflict
		}
		return e.commit(record{Event: event("Human", "configuration", fmt.Sprintf("Assigned %s to %d queued candidates.", e.profile(profileID).Name, len(changes))), Candidates: changes})
	}
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if c.Phase != 0 || c.Runs[0].Status != "idle" {
			return ErrConflict
		}
		c.ProfileID = profileID
		c.Profile = e.profile(profileID)
		c.Job = c.Profile.Name
		return e.commit(record{Event: event("Human", "configuration", "Assigned interview skill: "+c.Profile.Name), Candidate: &c})
	}
	return ErrNotFound
}

var Admins = map[string]string{"admin-maya": "Maya Chen", "admin-theo": "Theo Brooks", "admin-jules": "Jules Rivera"}

func (e *Engine) AdminRequest(id, action, adminID string) error {
	name, ok := Admins[adminID]
	if !ok {
		return fmt.Errorf("%w: choose a demo admin", ErrValidation)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	requests := clone(e.requests)
	for i := range requests {
		req := &requests[i]
		if req.ID != id {
			continue
		}
		if req.Status != "pending" {
			return ErrConflict
		}
		var c Candidate
		for _, candidate := range e.candidates {
			if candidate.ID == req.CandidateID {
				c = clone(candidate)
				break
			}
		}
		if c.ID == "" {
			return ErrNotFound
		}
		if c.Mode == "live" {
			if action == "open" {
				return nil
			}
			if action != "acknowledge" || c.Live == nil || c.Live.Voice != nil {
				return ErrConflict
			}
			req.Status = "attended"
			req.AttendedBy = name
			t := e.clock
			req.AttendedAt = &t
			c.Live.Reviewer = name
			return e.commit(record{Event: event("Human", "review-claimed", name+" claimed the web interview review. No Discord presence is implied."), Candidate: &c, Requests: &requests})
		}
		if !c.Room.BotPresent || c.Room.Expired {
			return ErrConflict
		}
		if action == "open" {
			for _, opened := range req.OpenedBy {
				if opened == adminID {
					return nil
				}
			}
			req.OpenedBy = append(req.OpenedBy, adminID)
			return e.commit(record{Event: event("Human", "request-opened", name+" opened the room link. Entry is not confirmed; the request remains visible."), Requests: &requests})
		}
		if action != "confirm" {
			return fmt.Errorf("%w: unknown request action", ErrValidation)
		}
		req.Status = "attended"
		req.AttendedBy = name
		t := e.clock
		req.AttendedAt = &t
		present := false
		for _, p := range c.Room.Participants {
			if p.ID == adminID {
				present = true
			}
		}
		if !present {
			c.Room.Participants = append(c.Room.Participants, Participant{ID: adminID, Name: name, Kind: "admin"})
		}
		if hasCandidate(c) {
			c.Room.EmptySince = nil
			c.Room.ReconnectDeadline = nil
		}
		return e.commit(record{Event: event("Ops", "admin-joined", name+" joined "+c.Room.Name+" (simulated confirmed voice-state event). Shared request is now disabled for all admins."), Candidate: &c, Requests: &requests})
	}
	return ErrNotFound
}
func (e *Engine) RoomAction(id, action, adminID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if !c.Room.BotPresent || c.Room.Expired {
			return ErrConflict
		}
		switch action {
		case "candidate-leave":
			ps := []Participant{}
			for _, p := range c.Room.Participants {
				if p.Kind != "candidate" {
					ps = append(ps, p)
				}
			}
			c.Room.Participants = ps
		case "candidate-rejoin":
			if !hasCandidate(c) {
				c.Room.Participants = append(c.Room.Participants, Participant{ID: c.ID, Name: c.Name, Kind: "candidate"})
			}
		case "admin-leave":
			if _, ok := Admins[adminID]; !ok {
				return fmt.Errorf("%w: unknown demo admin", ErrValidation)
			}
			ps := []Participant{}
			for _, p := range c.Room.Participants {
				if p.ID != adminID {
					ps = append(ps, p)
				}
			}
			c.Room.Participants = ps
		case "everyone-leave":
			c.Room.Participants = []Participant{}
		default:
			return fmt.Errorf("%w: unknown room action", ErrValidation)
		}
		if !hasCandidate(c) {
			if c.Room.EmptySince == nil {
				t := e.clock
				deadline := t + 300
				c.Room.EmptySince = &t
				c.Room.ReconnectDeadline = &deadline
			}
		} else {
			c.Room.EmptySince = nil
			c.Room.ReconnectDeadline = nil
		}
		return e.commit(record{Event: event("Ops", "room-presence", c.Room.Name+": "+action+". Bot leaves after the candidate is absent for 5 simulated minutes."), Candidate: &c})
	}
	return ErrNotFound
}
func (e *Engine) Decision(id, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if !hasAdmin(c) {
			return ErrConflict
		}
		switch action {
		case "resolve":
			if c.Stage != "Review" || c.ReviewResolved {
				return ErrConflict
			}
			c.ReviewResolved = true
			c.Stage = "Tech Passed"
			c.Role = "Tech Passed"
			if c.Mode == "live" {
				c.Stage = "Final"
				c.Role = "Final"
			}
		case "hire", "reject":
			if c.Stage != "Final" && c.Stage != "Review" {
				return ErrConflict
			}
			if action == "hire" && c.Stage != "Final" {
				return ErrConflict
			}
			if action == "hire" {
				c.Stage = "Hired"
				c.Role = "Hired"
				c.Feedback = []string{"A human recorded the final hiring decision in this synthetic demo."}
			} else {
				c.Stage = "Rejected"
				c.Role = "Rejected"
				c.Feedback = []string{"Thank you for taking part in this synthetic interview.", "Review the specific examples in the scorecard.", "A human recorded this decision; no automatic rejection was made."}
			}
		default:
			return fmt.Errorf("%w: unknown decision", ErrValidation)
		}
		requests := clone(e.requests)
		if c.Mode == "live" {
			c.Feedback = []string{"A human recorded the decision: " + action}
		}
		if action != "resolve" {
			for i := range requests {
				if requests[i].CandidateID == id {
					requests[i].Status = "closed"
				}
			}
		}
		text := "Human action: " + action + ". The bot remains until the candidate has been absent for five minutes."
		if c.Mode == "live" {
			text = "Human web review decision: " + action + ". Discord is not connected."
		}
		return e.commit(record{Event: event("Human", "decision", text), Candidate: &c, Requests: &requests})
	}
	return ErrNotFound
}
