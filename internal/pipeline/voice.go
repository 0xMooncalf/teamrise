package pipeline

import (
	"fmt"
	"strings"
	"time"

	"ladder/internal/connections"
)

type SpeechCall struct {
	Kind         string    `json:"kind"`
	Model        string    `json:"model"`
	AudioSeconds float64   `json:"audio_seconds"`
	Characters   int       `json:"characters"`
	DurationMS   int64     `json:"duration_ms"`
	At           time.Time `json:"at"`
}
type VoiceSession struct {
	ReconnectGraceSeconds int          `json:"reconnect_grace_seconds,omitempty"`
	GuildID               string       `json:"guild_id"`
	MemberID              string       `json:"member_id"`
	State                 string       `json:"state"`
	Stopped               bool         `json:"stopped"`
	Error                 string       `json:"error,omitempty"`
	EmptyDeadline         *time.Time   `json:"empty_deadline,omitempty"`
	Calls                 []SpeechCall `json:"calls"`
}

func VoiceIntroduction(skill string, limit, minutes int, greeting string) string {
	count := fmt.Sprintf("up to %d questions", limit)
	if limit == 1 {
		count = "one question"
	}
	format := " Answer in English after each question."
	if limit > 1 {
		format += " Written exercises appear in this room's text chat."
	}
	if strings.TrimSpace(greeting) == "" {
		greeting = connections.DefaultGreeting
	}
	return fmt.Sprintf("%s This is the %s interview. I'll ask %s. You have %d minutes.%s You can request a human reviewer from the room chat at any time.", greeting, skill, count, minutes, format)
}

func (c Candidate) WaitingForChat() bool {
	return c.Live != nil && c.Live.Status == "awaiting_answer" && len(c.Transcript) > 0 &&
		c.Transcript[len(c.Transcript)-1].Speaker == "Interviewer" && c.Transcript[len(c.Transcript)-1].Channel == "chat"
}

func (e *Engine) RequestVoiceReviewer(id string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if c.Live == nil || c.Live.Voice == nil || c.Room.Expired {
			return false, ErrConflict
		}
		if hasAdmin(c) {
			return false, nil
		}
		requests := clone(e.requests)
		for i := range requests {
			r := &requests[i]
			if r.CandidateID == id && r.Status == "pending" {
				if r.RequestedByCandidate {
					return false, nil
				}
				r.RequestedByCandidate = true
				r.Reason = "Candidate requested a human reviewer"
				now := time.Now().UTC()
				if r.CreatedTimestamp == nil {
					r.CreatedTimestamp = &now
				}
				return true, e.commit(record{Candidate: &c, Requests: &requests, Event: event("Ops", "reviewer-requested", r.Reason)})
			}
		}
		e.ensureRequest(&c, &requests, "Candidate requested a human reviewer", e.clock)
		now := time.Now().UTC()
		requests[len(requests)-1].RequestedByCandidate = true
		requests[len(requests)-1].CreatedTimestamp = &now
		return true, e.commit(record{Candidate: &c, Requests: &requests, Event: event("Ops", "reviewer-requested", "Candidate requested a human reviewer from the Discord room.")})
	}
	return false, ErrNotFound
}

func (e *Engine) NextVoiceRoomName() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	next := e.voiceRoomSequence + 1
	if err := e.commit(record{VoiceRoomSequence: &next, Event: event("Ops", "voice-room-number", "Private interview room number reserved.")}); err != nil {
		return "", err
	}
	return fmt.Sprintf("interview #%d", next), nil
}
func (e *Engine) RenameVoiceRoom(id, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID == id {
			c := clone(original)
			if c.Live == nil || c.Live.Discord == nil {
				return ErrConflict
			}
			c.Room.Name = name
			c.Live.Discord.Payload.Name = name
			return e.commit(record{Candidate: &c, Event: event("Ops", "voice-room-renamed", "Private interview room display name updated.")})
		}
	}
	return ErrNotFound
}

// Voice presence and speech metrics do not invalidate an in-flight model turn.
// Model results merge the latest runtime fields before committing their output.
func (e *Engine) mergeVoiceLocked(c *Candidate) {
	if c.Live == nil || c.Live.Voice == nil {
		return
	}
	for _, current := range e.candidates {
		if current.ID == c.ID && current.Live != nil {
			c.Room = clone(current.Room)
			c.Live.Voice = clone(current.Live.Voice)
			c.Live.Reviewer = current.Live.Reviewer
			return
		}
	}
}
func (e *Engine) SetVoiceState(id, state, message string, botPresent bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if c.Live == nil || c.Live.Voice == nil {
			return ErrConflict
		}
		if c.Live.Voice.State == "closed" && state != "closed" {
			return ErrConflict
		}
		c.Live.Voice.State = state
		c.Live.Voice.Error = message
		if state == "stopped" {
			c.Live.Voice.Stopped = true
		}
		c.Room.BotPresent = botPresent
		if state == "closed" {
			c.Room.Expired = true
			c.Live.Discord.Status = "closed"
		} else if botPresent {
			c.Live.Discord.Status = "connected"
		} else {
			c.Live.Discord.Status = "disconnected"
		}
		requests := clone(e.requests)
		if (state == "stopped" || state == "closed") && c.Live.Status != "complete" && c.Live.Status != "failed" {
			neverStarted := c.Live.Status == "ready"
			c.Live.Status = "failed"
			c.Live.Version++
			c.Live.Failure = "The candidate did not return before the five-minute room deadline."
			if state == "stopped" {
				c.Live.Failure = "The candidate stopped before completing the interview."
			} else if message != "" {
				c.Live.Failure = message
			} else if neverStarted {
				c.Live.Failure = "The room expired before the interview began."
			}
			for i := range c.Runs {
				if c.Runs[i].Status == "active" {
					c.Runs[i].Status = "idle"
				}
			}
		}
		if state == "stopped" && c.Live.Status != "complete" && !c.ReviewResolved && c.Stage != "Hired" && c.Stage != "Rejected" {
			c.Stage = "Review"
			c.Role = "Review"
			c.Flag = "Interview stopped · human review required"
			e.ensureRequest(&c, &requests, "Discord interview stopped · review the saved transcript", e.clock)
		}
		return e.commit(record{Event: event("Ops", "voice-state", "Discord voice state: "+state+"."), Candidate: &c, Requests: &requests})
	}
	return ErrNotFound
}
func (e *Engine) RecordSpeech(id, kind string, seconds float64, characters int, latency int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if c.Live == nil || c.Live.Voice == nil {
			return ErrConflict
		}
		model := "faster-whisper-base.en"
		if kind == "synthesis" {
			model = "piper-ljspeech-medium"
		}
		c.Live.Voice.Calls = append(c.Live.Voice.Calls, SpeechCall{Kind: kind, Model: model, AudioSeconds: seconds, Characters: characters, DurationMS: latency, At: time.Now().UTC()})
		return e.commit(record{Event: event("Interviewer", "speech-usage", "Local speech processing recorded. No speech API charge or invented token count."), Candidate: &c})
	}
	return ErrNotFound
}
func (e *Engine) VoicePresence(id string, people []Participant, deadline *time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		if c.Live == nil || c.Live.Voice == nil {
			return ErrConflict
		}
		c.Room.Participants = clone(people)
		c.Live.Voice.EmptyDeadline = deadline
		c.Live.Voice.ReconnectGraceSeconds = 300
		requests := clone(e.requests)
		for _, person := range people {
			if person.Kind == "admin" {
				c.Live.Reviewer = person.Name
				for i := range requests {
					if requests[i].CandidateID == id && requests[i].Status == "pending" {
						requests[i].Status = "attended"
						requests[i].AttendedBy = person.Name
						t := e.clock
						requests[i].AttendedAt = &t
					}
				}
				break
			}
		}
		return e.commit(record{Event: event("Ops", "discord-presence", "Actual Discord voice membership updated. Authorized reviewer entry confirms shared handoff attendance."), Candidate: &c, Requests: &requests})
	}
	return ErrNotFound
}
