package voice

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"

	"ladder/internal/discordbot"
	"ladder/internal/pipeline"
)

func (s *Service) chatMessage(a *session, text, role string, files ...*discordgo.File) error {
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	return s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		return api.SendInterviewMessage(ctx, a.channel, text, role, files...)
	})
}

func (s *Service) requestReviewer(a *session, i *discordgo.Interaction) {
	s.mu.Lock()
	allowed := s.active == a && !a.closing && i.ChannelID == a.channel &&
		i.Member.User.ID == a.member && a.people[a.member].ID != ""
	s.mu.Unlock()
	if !allowed {
		s.reply(i, "Join your assigned voice room and run this command in its chat.", nil)
		return
	}
	created, err := s.engine.RequestVoiceReviewer(a.id)
	if err != nil {
		s.reply(i, "Could not save the reviewer request. Try again shortly.", nil)
		return
	}
	if !created {
		s.reply(i, "A reviewer has already been requested or is in the room.", nil)
		return
	}
	s.reply(i, "Your request is in the Teamrise review queue.", nil)
	role := s.store.Public().Discord.ReviewerRoleID
	if role != "" {
		_ = s.chatMessage(a, "<@&"+role+"> The candidate requested a human reviewer in this interview.", role)
	}
}

func (s *Service) writtenQuestion(a *session, question pipeline.Message, number int) {
	s.mu.Lock()
	a.epoch++ // Invalidate recognition events from the previous spoken question.
	posted := a.postedQuestion == number
	s.mu.Unlock()
	s.send(map[string]any{"type": "listen", "session_id": a.id, "enabled": false})
	if !posted {
		content := fmt.Sprintf("**Practical exercise**\n\n%s\n\nSubmit your answer with **/interview-answer** in this chat.", question.Text)
		var files []*discordgo.File
		if question.Image != "" {
			data, contentType, err := s.engine.Screenshot(question.Image)
			if err != nil {
				s.state(a, "voice_error", "The screenshot for this exercise is unavailable. Ask a reviewer for help.")
				return
			}
			name := "exercise.png"
			if contentType == "image/jpeg" {
				name = "exercise.jpg"
			}
			files = append(files, &discordgo.File{Name: name, ContentType: contentType, Reader: bytes.NewReader(data)})
		}
		if err := s.chatMessage(a, content, "", files...); err != nil {
			s.state(a, "voice_error", "Could not post the exercise. Check Send Messages and Attach Files in the room and use /interview-repeat.")
			return
		}
		s.mu.Lock()
		a.postedQuestion = number
		epoch := a.epoch
		s.mu.Unlock()
		_ = s.speak(a, "I've posted your next question in this room's text chat. Please answer it there using the interview answer command.", false, epoch)
	}
	s.mu.Lock()
	present := s.active == a && a.ready && a.presenceReady && !a.stopped && !a.closing &&
		len(a.people) == 1 && a.people[a.member].ID != ""
	s.mu.Unlock()
	if present {
		s.state(a, "awaiting_chat", "Waiting for a written answer in the Discord room chat.")
	}
}

func (s *Service) acceptChat(a *session, i *discordgo.Interaction, text string) {
	if text == "" || len([]rune(text)) > 1500 {
		s.reply(i, "Enter an answer of 1 to 1,500 characters.", nil)
		return
	}
	s.mu.Lock()
	if s.active != a || a.stopped || a.closing || !a.ready || !a.presenceReady || a.busy ||
		(a.limitAt != nil && !time.Now().Before(*a.limitAt)) ||
		a.state != "awaiting_chat" || i.ChannelID != a.channel || i.Member.User.ID != a.member ||
		len(a.people) != 1 || a.people[a.member].ID == "" {
		s.mu.Unlock()
		s.reply(i, "A written exercise must be open in your assigned room. Join voice and use its chat after the question appears.", nil)
		return
	}
	a.busy = true
	s.mu.Unlock()
	s.reply(i, "Answer received. The interviewer will continue shortly.", nil)
	defer s.finishBusy(a)
	s.turn(a, "answer_chat", text)
}
