// Package providers defines the boundaries for real LLM, voice and Discord
// integrations. compatible.go implements actual Chat Completions requests;
// Scripted remains available only as an explicit deterministic demo provider.
package providers

import "context"

type Turn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type InterviewRequest struct {
	Job     string
	History []Turn
}

type LLMProvider interface {
	Reply(context.Context, InterviewRequest) (string, error)
}

type VoiceProvider interface {
	Transcribe(context.Context, []byte, string) (string, error)
	Speak(context.Context, string) ([]byte, string, error)
}

type RoleChange struct {
	CandidateID string
	Role        string
	EventID     int
}

type Channel interface {
	ChangeRole(context.Context, RoleChange) error
	PostStatus(context.Context, string, int) error
}

// Scripted is deterministic: no network calls, credentials or claims of live AI.
type Scripted struct{}

func (Scripted) Reply(_ context.Context, request InterviewRequest) (string, error) {
	if request.Job == "Community Manager" {
		return "Scripted follow-up: describe a specific moderation decision you would escalate, and explain what evidence you would preserve.", nil
	}
	return "Scripted follow-up: how would you test that a retried operation cannot advance a candidate twice? Walk me through a failure case.", nil
}
