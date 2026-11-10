package voice

import (
	"context"
	"errors"
	"time"

	"ladder/internal/discordbot"
	"ladder/internal/pipeline"
)

const oralSilenceLimit = 5 * time.Minute
const voiceCheckInterval = 15 * time.Second

// ensureRuntime is called with op held. It does not touch the interview or
// re-register Discord commands when replacing the audio processes.
func (s *Service) ensureRuntime(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.bridge != nil {
		s.mu.Unlock()
		return nil
	}
	s.generation++
	generation := s.generation
	s.mu.Unlock()
	b, err := startBridge(s.ctx, func(event workerEvent) {
		event.generation = generation
		select {
		case s.events <- event:
		case <-s.ctx.Done():
		default:
		}
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.bridge = b
	s.mu.Unlock()
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	select {
	case err = <-b.ready:
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = errors.New("local speech startup timed out")
	}
	if err != nil {
		s.discardRuntime(b)
	}
	return err
}

func (s *Service) discardRuntime(b *bridge) {
	s.mu.Lock()
	if s.bridge == b {
		s.bridge = nil
		s.generation++
	}
	s.mu.Unlock()
	if b != nil {
		b.Close()
	}
}

func (s *Service) interruptVoice(a *session, restartRuntime bool, message string) {
	s.mu.Lock()
	if s.active != a || a.recovering || a.stopped || a.closing {
		s.mu.Unlock()
		return
	}
	a.ready = false
	a.restartRuntime = a.restartRuntime || restartRuntime
	a.voiceCheckAt = time.Time{}
	s.mu.Unlock()
	s.state(a, "voice_error", message)
}

func awaitingSpokenAnswer(c pipeline.Candidate) bool {
	if c.Live == nil || c.Live.Status != "awaiting_answer" || len(c.Transcript) == 0 {
		return false
	}
	last := c.Transcript[len(c.Transcript)-1]
	return last.Speaker == "Interviewer" && last.Channel != "chat"
}

// canCheckVoice requires mu. Room expiry and the interview deadline run
// independently of the spoken-answer watchdog.
func (s *Service) canCheckVoice(a *session, now time.Time) bool {
	return a != nil && s.active == a && !a.busy && !a.syncing && !a.recovering && !a.stopped && !a.closing &&
		len(a.people) == 1 && a.people[a.member].ID != "" &&
		(a.limitAt == nil || now.Before(*a.limitAt))
}

func silenceExpired(a *session, now time.Time) bool {
	return a.state == "listening" && !a.silenceSince.IsZero() && now.Sub(a.silenceSince) >= oralSilenceLimit
}

func (s *Service) scheduleVoiceCheck(a *session, now time.Time) {
	s.mu.Lock()
	if !s.canCheckVoice(a, now) || now.Before(a.voiceCheckAt) {
		s.mu.Unlock()
		return
	}
	a.voiceCheckAt = now.Add(voiceCheckInterval)
	epoch := a.epoch
	s.mu.Unlock()
	go s.checkVoice(a, epoch)
}

func (s *Service) checkVoice(a *session, epoch int) {
	c, err := s.engine.Candidate(a.id)
	if err != nil || !awaitingSpokenAnswer(c) {
		return
	}
	s.mu.Lock()
	b, restartRuntime := s.bridge, a.restartRuntime
	s.mu.Unlock()
	var connected bool
	if b != nil {
		ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
		result, err := b.Call(ctx, "health", map[string]any{"session_id": a.id})
		cancel()
		restartRuntime = restartRuntime || err != nil
		connected = err == nil && result.Connected
	}
	s.op.Lock()
	defer s.op.Unlock()
	c, err = s.engine.Candidate(a.id)
	if err != nil || !awaitingSpokenAnswer(c) {
		return
	}
	s.mu.Lock()
	now := time.Now()
	if !s.canCheckVoice(a, now) || a.epoch != epoch || s.bridge != b {
		s.mu.Unlock()
		return
	}
	silent := silenceExpired(a, now)
	restartRuntime = restartRuntime || a.restartRuntime
	if connected && a.ready && !silent && !restartRuntime {
		if a.state == "listening" && !a.silenceSince.IsZero() {
			next := a.silenceSince.Add(oralSilenceLimit)
			if next.Before(a.voiceCheckAt) {
				a.voiceCheckAt = next
			}
		}
		s.mu.Unlock()
		return
	}
	a.recovering = true
	a.ready = false
	a.epoch++
	a.pending = nil
	s.mu.Unlock()

	message := "Voice interrupted. Reconnecting to the same room. Your answers are saved."
	if silent && connected && !restartRuntime {
		message = "Five minutes without speech. Reconnecting, then repeating the current question."
	}
	s.state(a, "recovering_voice", message)
	err = s.reconnectVoice(a, b, restartRuntime)
	s.mu.Lock()
	a.recovering = false
	a.voiceCheckAt = time.Now().Add(voiceCheckInterval)
	if err == nil && s.bridge != nil && !a.stopped && !a.closing {
		a.ready = true
		a.restartRuntime = false
		s.status.Ready = true
	} else if err == nil {
		err = errors.New("voice runtime interrupted")
	}
	stopped := a.stopped || a.closing
	s.mu.Unlock()
	if stopped {
		return
	}
	if err != nil {
		s.state(a, "voice_error", "Could not reconnect voice yet. Teamrise will try again in this room.")
		return
	}
	s.state(a, "waiting_for_people", "")
	s.refreshPresence(a)
}

func (s *Service) reconnectVoice(a *session, b *bridge, restartRuntime bool) error {
	// If transport still plays audio, give the signal before leaving. On a
	// hard outage, carry it over until the connection can play again.
	s.mu.Lock()
	chimePlayed := a.recoveryChimePlayed
	s.mu.Unlock()
	if !chimePlayed && b != nil {
		chimePlayed = s.playRecoveryChime(a, b)
		s.mu.Lock()
		a.recoveryChimePlayed = chimePlayed
		s.mu.Unlock()
	}

	if b != nil && !restartRuntime {
		ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
		_, err := b.Call(ctx, "leave", map[string]any{"session_id": a.id})
		cancel()
		restartRuntime = err != nil
	}
	if restartRuntime {
		s.discardRuntime(b)
	}
	// The worker's leave suppresses adapter events, so explicitly clear the
	// old Discord voice state before joining again.
	if err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.SendVoiceState(a.guild, "") }); err != nil {
		return err
	}
	if err := s.ensureRuntime(a.ctx); err != nil {
		return err
	}
	if err := s.joinVoice(a, true); err != nil {
		return err
	}
	s.mu.Lock()
	b = s.bridge
	s.mu.Unlock()
	if !chimePlayed && b != nil {
		s.playRecoveryChime(a, b)
	}
	s.mu.Lock()
	a.recoveryChimePlayed = false
	s.mu.Unlock()
	return nil
}

func (s *Service) playRecoveryChime(a *session, b *bridge) bool {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	_, err := b.Call(ctx, "chime", map[string]any{"session_id": a.id})
	return err == nil
}
