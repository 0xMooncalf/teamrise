package pipeline

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"ladder/internal/connections"
	"ladder/internal/discordrooms"
	"ladder/internal/providers"
)

func TestOrderedCardsKeepTheirImageAndStopAtThePinnedDeadline(t *testing.T) {
	e := newTestEngine(t)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	asset, err := e.SaveScreenshot(pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	p := e.Snapshot().Profiles[0]
	p.Cards = []InterviewCard{
		{ID: "q1", Channel: "voice", Prompt: "Say how you diagnose the problem."},
		{ID: "q2", Channel: "chat", Prompt: "What does this screenshot show?", Image: asset, Notes: "Look for a clear description."},
	}
	if err := e.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	id, err := e.CreateInterview("Card candidate", p.ID, "", &discordrooms.Plan{GuildID: "guild", MemberID: "member", ChannelID: "channel"})
	if err != nil {
		t.Fatal(err)
	}
	model := providers.Compatible{Voice: true, Config: connections.AI{Model: "fixture", MaxQuestions: 1},
		Interview: connections.Interview{Minutes: 75, Greeting: "Welcome from Teamrise!"}}
	run := func(action, answer string) error {
		c, _ := e.Candidate(id)
		return e.RunAI(context.Background(), id, action, answer, c.Live.Version, model)
	}
	if err := run("start", ""); err != nil {
		t.Fatal(err)
	}
	c, _ := e.Candidate(id)
	if c.Live.Deadline.Sub(c.Live.StartedAt) != 75*time.Minute || c.Live.MaxQuestions != 2 ||
		!strings.Contains(c.Transcript[0].Text, "Welcome from Teamrise!") || c.Transcript[1].Text != p.Cards[0].Prompt {
		t.Fatal("start did not pin settings and first question")
	}
	if err := run("answer", "I would read the logs first."); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	last := c.Transcript[len(c.Transcript)-1]
	if !c.WaitingForChat() || last.Image != asset || last.Text != p.Cards[1].Prompt || strings.Contains(last.Text, p.Cards[1].Notes) {
		t.Fatal("practical card did not retain its prompt and screenshot independently of the notes")
	}
	if err := run("answer", "Wrong channel."); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected a chat answer, got %v", err)
	}
	e.mu.Lock()
	err = e.expireInterviewsLocked(*c.Live.Deadline)
	e.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	c, _ = e.Candidate(id)
	if c.Live.Status != "failed" || !c.Live.Voice.Stopped || answers(c) != 1 || c.InterviewSeconds != 4500 {
		t.Fatal("expiry must stop the interview and preserve existing answers")
	}
	if err := run("answer_chat", "Too late."); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired chat accepted an answer: %v", err)
	}
}
