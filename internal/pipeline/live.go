package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ladder/internal/discordrooms"
	"ladder/internal/providers"
)

type LiveSession struct {
	Failure      string             `json:"failure,omitempty"`
	Voice        *VoiceSession      `json:"voice,omitempty"`
	Calls        []UsageCall        `json:"calls"`
	Discord      *discordrooms.Plan `json:"discord,omitempty"`
	Status       string             `json:"status"`
	Error        string             `json:"error,omitempty"`
	Resume       string             `json:"resume,omitempty"`
	Version      int                `json:"version"`
	Model        string             `json:"model"`
	BindingAgent string             `json:"binding_agent"`
	StartedAt    time.Time          `json:"started_at"`
	QuestionAt   *time.Time         `json:"question_at"`
	CompletedAt  *time.Time         `json:"completed_at"`
	Scored       bool               `json:"scored"`
	Reviewer     string             `json:"reviewer"`
	Limitations  []string           `json:"limitations"`
	MaxQuestions int                `json:"max_questions"`
	Deadline     *time.Time         `json:"deadline,omitempty"`
}

func (e *Engine) CreateInterview(name, profileID, bindingAgent string, plans ...*discordrooms.Plan) (string, error) {
	name = strings.TrimSpace(name)
	minimumNameLength := 2
	if len(plans) > 0 && plans[0] != nil && plans[0].ChannelID != "" {
		minimumNameLength = 1
	}
	if size := len([]rune(name)); size < minimumNameLength || size > 100 {
		return "", fmt.Errorf("%w: candidate name must contain %d–100 characters", ErrValidation, minimumNameLength)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	found := false
	for _, p := range e.profiles {
		if p.ID == profileID {
			found = true
		}
	}
	if !found {
		return "", ErrNotFound
	}
	if bindingAgent != "" {
		valid := false
		for _, a := range Agents {
			if a.Name == bindingAgent {
				valid = true
			}
		}
		if !valid {
			return "", ErrValidation
		}
	}
	random := make([]byte, 10)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	id := "candidate-live-" + hex.EncodeToString(random)
	profile := e.profile(profileID)
	c := Candidate{ID: id, Mode: "live", Name: name, Initials: string([]rune(name)[:1]), Job: profile.Name, Source: "Web · real AI", Stage: "Applicant", Role: "Applicant", Color: "#5d5a92", ProfileID: profileID, Profile: profile, Phase: 1, Live: &LiveSession{Status: "ready", Version: 1, BindingAgent: bindingAgent, StartedAt: time.Now().UTC(), Limitations: []string{}}, Room: Room{ID: "room-" + id, Name: "Web interview", Participants: []Participant{}}}
	e.normalize(&c)
	c.Live.Calls = []UsageCall{}
	if len(plans) > 0 && plans[0] != nil {
		c.Live.Discord = clone(plans[0])
		if c.Live.Discord.ChannelID == "" {
			c.Live.Discord.Payload.Name = "interview-" + strings.TrimPrefix(id, "candidate-live-")[:8]
		} else {
			c.Source = "Discord · real AI"
			c.AvatarURL = c.Live.Discord.AvatarURL
			c.Room.ID = c.Live.Discord.ChannelID
			c.Room.Name = c.Live.Discord.Payload.Name
			c.Live.Voice = &VoiceSession{ReconnectGraceSeconds: 300, GuildID: c.Live.Discord.GuildID, MemberID: c.Live.Discord.MemberID, State: "joining", Calls: []SpeechCall{}}
		}
	}
	c.Runs[0] = AgentRun{Agent: "Scout", Status: "done", Summary: "Saved skill and prompt pinned for this interview."}
	c.Runs[1].Status = "idle"
	return id, e.commit(record{Event: event("Scout", "live-created", "Real AI interview created. Candidate responses must be supplied by a person; no demo answers are used."), Candidate: &c})
}
func historyFor(c Candidate) []providers.ChatMessage {
	messages := []providers.ChatMessage{}
	for _, m := range c.Transcript {
		if m.Speaker == "System" {
			continue
		}
		role := "user"
		if m.Speaker == "Interviewer" {
			role = "assistant"
		}
		messages = append(messages, providers.ChatMessage{Role: role, Content: m.Text})
	}
	return messages
}
func answers(c Candidate) int {
	n := 0
	for _, m := range c.Transcript {
		if m.Speaker == "Candidate" {
			n++
		}
	}
	return n
}

// Reserve and persist the user turn before the network call. No engine lock is
// held during inference, so separate interviews can run concurrently.
func (e *Engine) reserveAI(id, action, answer string, version int, p providers.Compatible) (Candidate, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, original := range e.candidates {
		if original.ID != id {
			continue
		}
		c := clone(original)
		l := c.Live
		if l != nil && l.Voice != nil && (l.Voice.Stopped || l.Voice.State == "closed") {
			return c, ErrConflict
		}
		if c.Mode != "live" || l == nil || l.Version != version || l.Status == "generating" || l.Status == "analyzing" || l.Status == "complete" || l.Status == "failed" {
			return c, ErrConflict
		}
		if l.timeLimitReached(time.Now()) {
			return c, fmt.Errorf("%w: interview time limit reached", ErrConflict)
		}
		switch action {
		case "start":
			if l.Status != "ready" {
				return c, ErrConflict
			}
			l.Resume = "turn"
			l.StartedAt = time.Now().UTC()
			l.MaxQuestions = max(p.Config.MaxQuestions, len(c.Profile.Cards))
			limit := InterviewMinutes(p.Interview.Minutes)
			deadline := l.StartedAt.Add(time.Duration(limit) * time.Minute)
			l.Deadline = &deadline
			if l.Voice != nil {
				c.Transcript = append(c.Transcript, Message{Speaker: "System", Text: VoiceIntroduction(c.Profile.Name, l.MaxQuestions, limit, p.Interview.Greeting)})
			}
		case "answer", "answer_chat":
			if l.Status != "awaiting_answer" {
				return c, ErrConflict
			}
			answer = strings.TrimSpace(answer)
			if answer == "" || len(answer) > 16000 {
				return c, fmt.Errorf("%w: answer must contain 1–16000 bytes", ErrValidation)
			}
			seconds := 0
			if l.QuestionAt != nil {
				seconds = int(time.Since(*l.QuestionAt).Seconds())
				if seconds < 0 {
					seconds = 0
				}
			}
			channel := ""
			if l.Voice != nil {
				written := c.WaitingForChat()
				if written != (action == "answer_chat") {
					return c, ErrConflict
				}
				if written {
					channel = "chat"
				}
			} else if action == "answer_chat" {
				return c, ErrConflict
			}
			c.Transcript = append(c.Transcript, Message{Speaker: "Candidate", Text: answer, ResponseSeconds: seconds, Channel: channel})
			l.Resume = "turn"
			if answers(c) >= l.MaxQuestions {
				l.Resume = "analysis"
			}
		case "analyze":
			if answers(c) == 0 || l.Status != "awaiting_answer" {
				return c, ErrConflict
			}
			l.Resume = "analysis"
		case "retry":
			if l.Status != "error" {
				return c, ErrConflict
			}
		default:
			return c, ErrValidation
		}
		if p.Config.Model == "" {
			return c, fmt.Errorf("%w: configure an AI model first", ErrValidation)
		}
		l.Status = "generating"
		c.Runs[1].Status = "active"
		if l.Resume == "analysis" {
			l.Status = "analyzing"
			c.Phase = 2
			c.Runs[1].Status = "done"
			c.Runs[2].Status = "active"
		}
		l.Model = p.Config.Model
		l.Version++
		l.Error = ""
		if err := e.commit(record{Event: event("Interviewer", "ai-request", "Requesting "+l.Model+" for real interview "+l.Resume+"."), Candidate: &c}); err != nil {
			return c, err
		}
		return c, nil
	}
	return Candidate{}, ErrNotFound
}
func (e *Engine) finishAI(c Candidate, err error) error {
	c = clone(c)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mergeVoiceLocked(&c)
	for _, current := range e.candidates {
		if current.ID == c.ID && (current.Live == nil || current.Live.Version != c.Live.Version || current.Live.timeLimitReached(time.Now())) {
			return ErrConflict
		}
	}
	text := "AI interview updated."
	if err != nil {
		c.Live.Status = "error"
		c.Live.Error = err.Error()
		text = "AI request failed. The saved answer is retained; retry will not duplicate it."
	}
	c.Live.Version++
	return e.commit(record{Event: event("Interviewer", "ai-result", text), Candidate: &c})
}
func (e *Engine) RunAI(ctx context.Context, id, action, answer string, version int, p providers.Compatible) error {
	p.SessionID = id
	c, err := e.reserveAI(id, action, answer, version, p)
	if err != nil {
		return err
	}
	agent := "Interviewer"
	if c.Live.Resume == "analysis" {
		agent = "Analyst"
	}
	p.OnUsage = func(usage providers.Usage) error { return e.recordUsage(&c, agent, usage, p.Config) }
	profileBytes, _ := json.Marshal(c.Profile)
	profile := string(profileBytes)
	if c.Live.Resume == "turn" {
		answered := answers(c)
		if answered < len(c.Profile.Cards) {
			card := c.Profile.Cards[answered]
			channel := ""
			if card.Channel == "chat" {
				channel = "chat"
			}
			c.Transcript = append(c.Transcript, Message{Speaker: "Interviewer", Text: strings.TrimSpace(card.Prompt), Channel: channel, Image: card.Image})
			now := time.Now().UTC()
			c.Live.QuestionAt = &now
			c.Live.Status = "awaiting_answer"
			return e.finishAI(c, nil)
		}
		p.Planned = len(c.Profile.Cards) > 0
		questionCtx := ctx
		if c.Live.Deadline != nil {
			var cancel context.CancelFunc
			questionCtx, cancel = context.WithDeadline(ctx, *c.Live.Deadline)
			defer cancel()
		}
		q, callErr := p.Next(questionCtx, profile, historyFor(c))
		if callErr != nil {
			return e.finishAI(c, callErr)
		}
		written := p.Voice && !p.Planned && answered == 1
		if q.Completed && written {
			return e.finishAI(c, fmt.Errorf("AI skipped the written question; retry"))
		}
		if q.Completed && answers(c) == 0 {
			return e.finishAI(c, fmt.Errorf("AI ended before receiving any candidate answers; retry"))
		}
		if !q.Completed {
			channel := ""
			if written {
				channel = "chat"
			}
			c.Transcript = append(c.Transcript, Message{Speaker: "Interviewer", Text: q.Message, Channel: channel})
			now := time.Now().UTC()
			c.Live.QuestionAt = &now
			c.Live.Status = "awaiting_answer"
			return e.finishAI(c, nil)
		}
		// Persist the analysis stage, including retry intent, before calling the model.
		c.Transcript = append(c.Transcript, Message{Speaker: "Interviewer", Text: q.Message})
		c.Live.Resume = "analysis"
		c.Live.Status = "analyzing"
		c.Phase = 2
		c.Runs[1].Status = "done"
		c.Runs[2].Status = "active"
		if err = e.finishAI(c, nil); err != nil {
			return err
		}
		c.Live.Version++
	}
	agent = "Analyst"
	report, callErr := p.Analyze(ctx, profile, c.Profile.Criteria, historyFor(c))
	if callErr != nil {
		return e.finishAI(c, callErr)
	}
	c.Score = 0
	c.Live.Scored = report.Score != nil
	if report.Score != nil {
		c.Score = *report.Score
	}
	c.Assessment = &Assessment{Summary: report.Summary, Characterization: report.Characterization, Recommendation: report.Recommendation, Strengths: report.Strengths, Followups: report.Followups}
	if c.Assessment.Strengths == nil {
		c.Assessment.Strengths = []string{}
	}
	if c.Assessment.Followups == nil {
		c.Assessment.Followups = []string{}
	}
	total := 0
	for _, m := range c.Transcript {
		if m.Speaker == "Candidate" {
			total += m.ResponseSeconds
		}
	}
	c.Assessment.AverageResponseSeconds = total / answers(c)
	c.Scorecard = []Score{}
	for _, s := range report.Scorecard {
		value := 0
		if s.Value != nil {
			value = *s.Value
		}
		c.Scorecard = append(c.Scorecard, Score{Label: s.Label, Value: value, NotScored: s.Value == nil, Evidence: strings.Join(s.Evidence, " · ")})
	}
	now := time.Now().UTC()
	c.Live.CompletedAt = &now
	c.InterviewSeconds = int(now.Sub(c.Live.StartedAt).Seconds())
	c.Live.Limitations = report.Limitations
	c.Live.Status = "complete"
	c.Live.Version++
	c.Live.Error = ""
	c.Phase = 7
	c.Runs[1].Status = "done"
	c.Runs[1].Summary = "Actual candidate responses collected using " + c.Live.Model
	c.Runs[2].Status = "done"
	c.Runs[2].Summary = "AI report produced from candidate answers."
	c.Runs[3] = AgentRun{Agent: "Fairness Auditor", Status: "done", Summary: "Criterion labels, score ranges and quoted evidence validated by code. This is not a separate AI fairness audit."}
	c.Runs[4] = AgentRun{Agent: "Ops", Status: "idle", Summary: "Discord gateway is not connected. Web transcript is saved locally."}
	c.Runs[5] = AgentRun{Agent: "Concierge", Status: "done", Summary: "Prepared report for a human reviewer."}
	c.Runs[6] = AgentRun{Agent: "Chief", Status: "done", Summary: "Shared human review request created; final decision belongs to a person."}
	e.mu.Lock()
	defer e.mu.Unlock()
	c.Stage = "Review"
	c.Role = "Review"
	c.Flag = "Human review of real AI evidence required"
	if c.Live.Scored && c.Score >= e.settings.HandoffScore {
		c.Stage = "Final"
		c.Role = "Final"
	}
	requests := clone(e.requests)
	e.mergeVoiceLocked(&c)
	reason := "Real AI interview ready · web review"
	if c.Live.Voice != nil {
		reason = "Discord interview ready · join the private room"
		c.Runs[4] = AgentRun{Agent: "Ops", Status: "active", Summary: "Real Discord room retained for human review. Removed after the candidate is absent for five minutes."}
	}
	e.ensureRequest(&c, &requests, reason, e.clock)
	for i := range requests {
		if requests[i].CandidateID == c.ID {
			requests[i].CreatedTimestamp = &now
			if c.Live.Voice != nil && c.Live.Reviewer != "" && requests[i].Status == "pending" {
				requests[i].Status = "attended"
				requests[i].AttendedBy = c.Live.Reviewer
				t := e.clock
				requests[i].AttendedAt = &t
			}
		}
	}
	return e.commit(record{Event: event("Chief", "ai-report", "Real AI report saved. Human review requested; no automatic hiring or rejection."), Candidate: &c, Requests: &requests})
}
