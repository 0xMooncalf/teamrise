package pipeline

import (
	"fmt"
	"time"

	"ladder/internal/connections"
)

func InterviewMinutes(minutes int) int {
	if minutes < 1 || minutes > 480 {
		return connections.DefaultInterviewMinutes
	}
	return minutes
}

func (l *LiveSession) timeLimitReached(now time.Time) bool {
	if l.Deadline == nil || now.Before(*l.Deadline) || l.Resume == "analysis" {
		return false
	}
	switch l.Status {
	case "awaiting_answer", "generating", "error":
		return true
	default:
		return false
	}
}

func (e *Engine) expireInterviewsLocked(now time.Time) error {
	for _, current := range e.candidates {
		if current.Live == nil || !current.Live.timeLimitReached(now) {
			continue
		}
		c := clone(current)
		c.Live.Status = "failed"
		c.Live.Version++
		c.Live.Failure = "The interview time limit was reached."
		c.Live.Error = ""
		c.InterviewSeconds = int(c.Live.Deadline.Sub(c.Live.StartedAt).Seconds())
		if c.Live.Voice != nil {
			c.Live.Voice.Stopped = true
			c.Live.Voice.State = "stopped"
			c.Live.Voice.Error = c.Live.Failure
		}
		c.Runs[1].Status = "idle"
		requests := clone(e.requests)
		if answers(c) > 0 {
			c.Stage, c.Role = "Review", "Review"
			e.ensureRequest(&c, &requests, "Time limit reached · review saved answers", e.clock)
		}
		message := fmt.Sprintf("Interview time limit reached after %d minutes.", c.InterviewSeconds/60)
		if err := e.commit(record{Candidate: &c, Requests: &requests, Event: event("Interviewer", "time-limit", message)}); err != nil {
			return err
		}
	}
	return nil
}
