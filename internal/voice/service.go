package voice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"ladder/internal/connections"
	"ladder/internal/discordbot"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
	"ladder/internal/providers"
)

type Status struct {
	Revision    uint64 `json:"revision"`
	Enabled     bool   `json:"enabled"`
	Ready       bool   `json:"ready"`
	State       string `json:"state"`
	Message     string `json:"message"`
	CandidateID string `json:"candidate_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	Language    string `json:"language"`
}
type consent struct {
	user, guild, profile string
	expires              time.Time
}
type presenceEvent struct {
	session *session
	state   *discordgo.VoiceStateUpdate
}
type session struct {
	id, guild, channel, member, state    string
	ctx                                  context.Context
	cancel                               context.CancelFunc
	people                               map[string]pipeline.Participant
	deadline                             *time.Time
	limitAt                              *time.Time
	ready, intro, busy, stopped, closing bool
	presenceReady, syncing               bool
	lastPresenceCheck, cleanupRetry      time.Time
	postedQuestion                       int
	epoch                                int
	pending                              *workerEvent
	recovering, restartRuntime           bool
	voiceCheckAt, silenceSince           time.Time
	recoveryChimePlayed                  bool
}
type Service struct {
	mu         sync.Mutex
	op         sync.Mutex
	presenceOp sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	engine     *pipeline.Engine
	store      *connections.Store
	bot        *discordbot.Manager
	bridge     *bridge
	status     Status
	active     *session
	pending    map[string]consent
	events     chan workerEvent
	presence   chan presenceEvent
	generation uint64
}

func New(parent context.Context, e *pipeline.Engine, store *connections.Store, bot *discordbot.Manager) *Service {
	ctx, cancel := context.WithCancel(parent)
	s := &Service{ctx: ctx, cancel: cancel, engine: e, store: store, bot: bot, pending: map[string]consent{}, events: make(chan workerEvent, 64), presence: make(chan presenceEvent, 128), status: Status{State: "disabled", Language: "en", Message: "Enable local English voice to register /interview in your server."}}
	bot.SetEvents(discordbot.Events{VoiceServer: func(v *discordgo.VoiceServerUpdate) { s.send(map[string]any{"type": "voice_server", "data": v}) }, VoiceState: s.onPresence, Interaction: func(i *discordgo.InteractionCreate) { go s.interaction(i) }, Disconnected: func() {
		s.mu.Lock()
		a := s.active
		s.status.Ready = false
		s.status.Revision++
		if a != nil {
			a.ready = false
		}
		s.mu.Unlock()
		if a != nil {
			s.state(a, "connection_lost", "Discord Gateway disconnected")
		}
	}})
	go s.loop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-s.presence:
				if v.session != nil {
					s.updatePresence(v.session, v.state.UserID, v.state.ChannelID)
				}
			}
		}
	}()
	return s
}
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.status
	if s.active != nil {
		r.CandidateID = s.active.id
		r.ChannelID = s.active.channel
	}
	return r
}
func (s *Service) state(a *session, state, message string) {
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	if a.stopped && state != "closed" {
		if state != "stopped" || message == "" {
			message = "Interview stopped. Saved transcript is available for human review."
		}
		state = "stopped"
	}
	if a.recovering && state != "recovering_voice" && state != "stopped" && state != "closed" {
		s.mu.Unlock()
		return
	}
	switch state {
	case "listening":
		if a.silenceSince.IsZero() {
			a.silenceSince = time.Now()
		}
	case "transcribing":
	default:
		a.silenceSince = time.Time{}
	}
	a.state = state
	s.status.Revision++
	s.status.State = state
	s.status.Message = message
	botPresent := a.ready && state != "closed" && state != "connection_lost"
	id := a.id
	s.mu.Unlock()
	if id != "" {
		_ = s.engine.SetVoiceState(id, state, message, botPresent)
	}
}
func (s *Service) send(data any) {
	s.mu.Lock()
	b := s.bridge
	s.mu.Unlock()
	if b != nil {
		_ = b.Send(data)
	}
}
func (s *Service) Enable(ctx context.Context) error {
	s.op.Lock()
	defer s.op.Unlock()
	cfg := s.store.Public().Discord
	if !cfg.Enabled {
		return errors.New("verify and save Discord settings before enabling voice")
	}
	if s.store.Public().AI.Model == "" {
		return errors.New("configure an AI model before enabling voice")
	}
	s.mu.Lock()
	busy := s.active != nil && (s.active.busy || s.active.closing || s.active.recovering)
	s.mu.Unlock()
	if busy {
		return errors.New("wait for the current interview turn before reconnecting voice")
	}
	ai := s.store.Public().AI
	if ai.Provider == "codex" {
		if err := providers.WarmLocalCodex(ctx, ai.Model); err != nil {
			return s.failEnable(err)
		}
	}
	if err := s.bot.Verify(ctx, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.status.State = "preparing"
	s.status.Message = "Loading local speech models…"
	s.status.Revision++
	s.mu.Unlock()
	if err := s.ensureRuntime(ctx); err != nil {
		return s.failEnable(err)
	}
	choices := []*discordgo.ApplicationCommandOptionChoice{}
	for _, p := range s.engine.Snapshot().Profiles {
		if len(choices) == 25 {
			break
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: p.Name, Value: p.ID})
	}
	err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		return api.RegisterInterviewCommands(ctx, s.bot.Status().BotID, cfg.GuildID, choices)
	})
	if err != nil {
		return s.failEnable(errors.New("could not register interview commands; re-add the bot with applications.commands scope"))
	}
	s.mu.Lock()
	s.status.Enabled = true
	s.status.Ready = true
	s.status.State = "ready"
	s.status.Message = "Local English voice ready. Use /interview in Discord and choose a skill."
	s.status.Revision++
	s.mu.Unlock()
	if err := s.recoverRoom(ctx); err != nil {
		return s.failEnable(err)
	}
	return nil
}
func (s *Service) failEnable(err error) error {
	s.mu.Lock()
	s.status.Ready = false
	s.status.State = "error"
	s.status.Message = err.Error()
	s.status.Revision++
	s.mu.Unlock()
	return err
}
func (s *Service) Close() {
	s.mu.Lock()
	a, b := s.active, s.bridge
	s.bridge = nil
	s.mu.Unlock()
	if a != nil {
		a.cancel()
		providers.ForgetCodexInterview(a.id)
	}
	if b != nil {
		b.Close()
	}
	s.cancel()
}
func nonce() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func (s *Service) reply(i *discordgo.Interaction, text string, components []discordgo.MessageComponent) {
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		return api.Respond(ctx, i, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: text, Flags: discordgo.MessageFlagsEphemeral, Components: components, AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}}})
	})
}
func (s *Service) interaction(event *discordgo.InteractionCreate) {
	i := event.Interaction
	cfg := s.store.Public().Discord
	if i == nil || i.GuildID != cfg.GuildID || i.Member == nil || i.Member.User == nil {
		return
	}
	user := i.Member.User
	if user.Bot {
		return
	}
	if i.Type == discordgo.InteractionApplicationCommand {
		data := i.ApplicationCommandData()
		if !strings.HasPrefix(data.Name, "interview") {
			return
		}
		if data.Name == "interview" {
			if !s.Status().Ready {
				s.reply(i, "Voice is not ready. Ask the server owner to enable it in Teamrise.", nil)
				return
			}
			profile := ""
			for _, o := range data.Options {
				if o.Name == "skill" {
					profile = o.StringValue()
				}
			}
			valid := false
			for _, p := range s.engine.Snapshot().Profiles {
				if p.ID == profile {
					valid = true
				}
			}
			if !valid {
				s.reply(i, "Choose an available interview skill.", nil)
				return
			}
			s.mu.Lock()
			a := s.active
			if a != nil {
				s.mu.Unlock()
				text := "The interviewer is occupied. Try again after the current room closes."
				if a.member == user.ID {
					text = "Your interview room is <#" + a.channel + ">. Use /interview-repeat or /interview-stop."
				}
				s.reply(i, text, nil)
				return
			}
			key := nonce()
			if key == "" || len(s.pending) >= 100 {
				s.mu.Unlock()
				s.reply(i, "Please try again shortly.", nil)
				return
			}
			s.pending[key] = consent{user.ID, i.GuildID, profile, time.Now().Add(5 * time.Minute)}
			s.mu.Unlock()
			s.reply(i, "This is an AI interview in English. Speech is transcribed and synthesized locally; the transcript and written answers are sent to the configured AI provider (OpenAI via the owner's Codex account when selected) and saved in Teamrise. AI provides evidence for a human, never a final hiring decision. Continue only if you agree. Use /interview-stop at any time.", []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{Label: "I agree — start interview", Style: discordgo.PrimaryButton, CustomID: "teamrise:start:" + key}}}})
			return
		}
		s.mu.Lock()
		a := s.active
		owned := a != nil && a.member == user.ID
		s.mu.Unlock()
		if !owned {
			s.reply(i, "You do not have an active interview with this bot.", nil)
			return
		}
		switch data.Name {
		case "interview-help":
			s.requestReviewer(a, i)
			return
		case "interview-answer":
			answer := ""
			for _, option := range data.Options {
				if option.Name == "answer" {
					answer = strings.TrimSpace(option.StringValue())
				}
			}
			s.acceptChat(a, i, answer)
			return
		}
		if data.Name == "interview-stop" {
			s.reply(i, "Interview stopped. Your transcript is retained for human review. The room closes after you have been out of voice for five minutes.", nil)
			s.stop(a)
			return
		}
		if data.Name == "interview-repeat" {
			s.mu.Lock()
			busy, stopped, ready, recovering := a.busy, a.stopped, a.ready, a.recovering
			s.mu.Unlock()
			if stopped {
				s.reply(i, "This interview has been stopped. Ask a reviewer to review the saved transcript.", nil)
				return
			}
			if busy {
				s.reply(i, "The interviewer is processing your last turn. Please wait for the next question.", nil)
				return
			}
			if recovering || !ready {
				c, err := s.engine.Candidate(a.id)
				if err == nil && awaitingSpokenAnswer(c) {
					s.reply(i, "Teamrise is checking voice and will repeat the current question once it reconnects.", nil)
				} else {
					s.reply(i, "The voice connection needs recovery. Ask the server owner to reconnect voice in Teamrise.", nil)
				}
				return
			}
			s.reply(i, "Repeating the question, or retrying the saved request if needed.", nil)
			s.maybeStart(a)
			return
		}
	} else if i.Type == discordgo.InteractionMessageComponent {
		custom := i.MessageComponentData().CustomID
		if !strings.HasPrefix(custom, "teamrise:start:") {
			return
		}
		key := strings.TrimPrefix(custom, "teamrise:start:")
		s.mu.Lock()
		pending, ok := s.pending[key]
		if !ok || pending.user != user.ID || pending.guild != i.GuildID || time.Now().After(pending.expires) || s.active != nil {
			s.mu.Unlock()
			s.reply(i, "This start request expired or the interviewer is busy. Run /interview again.", nil)
			return
		}
		delete(s.pending, key)
		aCtx, cancel := context.WithCancel(s.ctx)
		a := &session{guild: i.GuildID, member: user.ID, ctx: aCtx, cancel: cancel, people: map[string]pipeline.Participant{}, state: "preparing", busy: true}
		s.active = a
		s.status.Revision++
		s.mu.Unlock()
		ctx, done := context.WithTimeout(s.ctx, 3*time.Second)
		defer done()
		err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
			return api.Respond(ctx, i, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral}})
		})
		if err != nil {
			s.abandon(a)
			return
		}
		go s.createRoom(a, pending.profile, user, i)
	}
}
func (s *Service) abandon(a *session) {
	a.cancel()
	s.mu.Lock()
	if s.active == a {
		s.active = nil
		s.status.Revision++
		s.status.State = "ready"
		s.status.Message = "Use /interview in Discord to begin a new interview."
	}
	s.mu.Unlock()
}
func (s *Service) edit(i *discordgo.Interaction, text string) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.EditResponse(ctx, i, text) })
}
func (s *Service) createRoom(a *session, profile string, user *discordgo.User, i *discordgo.Interaction) {
	cfg := s.store.Public().Discord
	ctx, cancel := context.WithTimeout(a.ctx, 40*time.Second)
	defer cancel()
	fail := func() {
		s.edit(i, "Could not prepare a private voice room. Check Teamrise voice status and the bot's permissions, then retry.")
		s.abandon(a)
	}
	if s.bot.Verify(ctx, cfg) != nil {
		fail()
		return
	}
	plan, err := discordrooms.Build(cfg, user.ID)
	if err != nil {
		fail()
		return
	}
	plan.Payload.Name, err = s.engine.NextVoiceRoomName()
	if err != nil {
		fail()
		return
	}
	var channel *discordgo.Channel
	err = s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		var e error
		channel, e = api.CreatePrivateRoom(ctx, cfg.GuildID, plan.Payload)
		return e
	})
	if err != nil || channel == nil {
		fail()
		return
	}
	// Verify the returned private channel before inviting a candidate or joining.
	if !privateChannel(channel, cfg, user.ID) {
		if channel.GuildID == cfg.GuildID && channel.Name == plan.Payload.Name {
			_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.DeleteRoom(ctx, channel.ID) })
		}
		fail()
		return
	}
	plan.ChannelID = channel.ID
	plan.Status = "connected"
	name := i.Member.Nick
	if name == "" {
		name = user.GlobalName
	}
	if name == "" {
		name = user.Username
	}
	plan.AvatarURL = user.AvatarURL("128")
	if i.Member.Avatar != "" {
		member := *i.Member
		member.GuildID = i.GuildID
		plan.AvatarURL = member.AvatarURL("128")
	}
	id, err := s.engine.CreateInterview(name, profile, "", plan)
	if err != nil {
		_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.DeleteRoom(ctx, channel.ID) })
		fail()
		return
	}
	s.mu.Lock()
	a.id = id
	a.channel = channel.ID
	a.busy = false
	deadline := time.Now().UTC().Add(5 * time.Minute)
	a.deadline = &deadline
	s.mu.Unlock()
	_ = s.engine.VoicePresence(id, []pipeline.Participant{}, &deadline)
	if err = s.join(a); err != nil {
		s.state(a, "voice_error", err.Error())
		s.edit(i, "Room created: <#"+channel.ID+">. Voice could not connect. Check Teamrise and use /interview-repeat to retry.")
		return
	}
	s.edit(i, "Your private interview room is <#"+channel.ID+">. Join it to begin. Wait for each question, then speak naturally; a short pause submits your answer. Open the room chat for written tasks or /interview-help. Commands: /interview-repeat and /interview-stop.")
}
func privateChannel(ch *discordgo.Channel, cfg connections.Discord, member string) bool {
	if ch.Type != discordgo.ChannelTypeGuildVoice || ch.GuildID != cfg.GuildID || ch.ParentID != cfg.CategoryID {
		return false
	}
	found := map[string]*discordgo.PermissionOverwrite{}
	for _, o := range ch.PermissionOverwrites {
		found[o.ID] = o
	}
	required := int64(discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak)
	everyone := found[cfg.GuildID]
	hidden := int64(discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect)
	if everyone == nil || everyone.Type != discordgo.PermissionOverwriteTypeRole || everyone.Deny&hidden != hidden || everyone.Allow&hidden != 0 {
		return false
	}
	for _, id := range []string{member, cfg.BotUserID, cfg.ReviewerRoleID} {
		o := found[id]
		typ := discordgo.PermissionOverwriteTypeMember
		if id == cfg.ReviewerRoleID {
			typ = discordgo.PermissionOverwriteTypeRole
		}
		if o == nil || o.Type != typ || o.Allow&required != required || o.Deny&required != 0 {
			return false
		}
	}
	// A newly created room must not contain unexpected role/member grants.
	for id, o := range found {
		if id != cfg.GuildID && id != member && id != cfg.BotUserID && id != cfg.ReviewerRoleID && (o.Allow&discordgo.PermissionViewChannel != 0 || o.Allow&discordgo.PermissionVoiceConnect != 0) {
			return false
		}
	}
	return true
}
func (s *Service) join(a *session) error {
	return s.joinVoice(a, false)
}

func (s *Service) joinVoice(a *session, recovering bool) error {
	s.mu.Lock()
	b := s.bridge
	a.presenceReady = false
	s.mu.Unlock()
	if b == nil {
		return errors.New("voice runtime is offline")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	_, err := b.Call(ctx, "join", map[string]any{"session_id": a.id, "guild_id": a.guild, "channel_id": a.channel, "member_id": a.member, "bot_id": s.bot.Status().BotID, "quiet": recovering})
	return err
}
func (s *Service) onPresence(v *discordgo.VoiceStateUpdate) {
	s.mu.Lock()
	a := s.active
	valid := a != nil && a.channel != "" && v.GuildID == a.guild
	s.mu.Unlock()
	if !valid {
		return
	}
	if v.UserID == s.bot.Status().BotID {
		s.send(map[string]any{"type": "voice_state", "data": v})
		return
	}
	s.mu.Lock()
	relevant := v.UserID == a.member || v.ChannelID == a.channel || a.people[v.UserID].ID != "" || (v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID == a.channel)
	s.mu.Unlock()
	if !relevant {
		return
	}
	// Ordered membership updates prevent a slow member lookup resurrecting a user who left.
	select {
	case s.presence <- presenceEvent{a, v}:
	case <-s.ctx.Done():
	}
}

const reconnectGrace = 5 * time.Minute

func nextDeadline(now time.Time, present bool, previous *time.Time) *time.Time {
	if present {
		return nil
	}
	if previous != nil {
		return previous
	}
	deadline := now.UTC().Add(reconnectGrace)
	return &deadline
}
func (s *Service) participant(a *session, user string) pipeline.Participant {
	p := pipeline.Participant{ID: user, Name: user, Kind: "guest"}
	if user == a.member {
		p.Kind = "candidate"
	}
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		m, err := api.Member(ctx, a.guild, user)
		if err != nil {
			return err
		}
		if m.User != nil {
			p.Name = m.User.Username
		}
		if m.Nick != "" {
			p.Name = m.Nick
		}
		if user != a.member {
			for _, r := range m.Roles {
				if r == s.store.Public().Discord.ReviewerRoleID {
					p.Kind = "admin"
					break
				}
			}
		}
		return nil
	})
	return p
}
func (s *Service) publishPresence(a *session, resume bool) {
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	people := []pipeline.Participant{}
	for _, p := range a.people {
		people = append(people, p)
	}
	sort.Slice(people, func(i, j int) bool { return people[i].ID < people[j].ID })
	deadline := a.deadline
	paused := len(a.people) != 1 || a.people[a.member].ID == ""
	id := a.id
	s.mu.Unlock()
	if id != "" {
		_ = s.engine.VoicePresence(id, people, deadline)
	}
	if paused {
		s.send(map[string]any{"type": "listen", "session_id": id, "enabled": false})
		s.state(a, "waiting_for_people", "")
	} else if resume {
		s.maybeStart(a)
	}
}
func (s *Service) updatePresence(a *session, user, channel string) {
	s.presenceOp.Lock()
	defer s.presenceOp.Unlock()
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	before := a.people[user]
	wasPaused := len(a.people) != 1 || a.people[a.member].ID == ""
	s.mu.Unlock()
	person := pipeline.Participant{}
	if channel == a.channel {
		person = s.participant(a, user)
	}
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	if channel == a.channel {
		a.people[user] = person
	} else {
		delete(a.people, user)
	}
	if before == person {
		s.mu.Unlock()
		return
	}
	a.deadline = nextDeadline(time.Now(), a.people[a.member].ID != "", a.deadline)
	resume := wasPaused && len(a.people) == 1 && a.people[a.member].ID != ""
	s.mu.Unlock()
	s.publishPresence(a, resume)
}

// REST verifies the assigned candidate even if the initial Gateway cache has
// not populated yet. No leave/rejoin gesture is required to discover them.
func (s *Service) refreshPresence(a *session) {
	s.mu.Lock()
	if s.active != a || a.syncing || a.closing || a.recovering {
		s.mu.Unlock()
		return
	}
	a.syncing = true
	a.lastPresenceCheck = time.Now()
	s.mu.Unlock()
	s.presenceOp.Lock()
	defer s.presenceOp.Unlock()
	defer func() { s.mu.Lock(); a.syncing = false; a.lastPresenceCheck = time.Now(); s.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	var ids []string
	var candidateChannel string
	err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		ids = api.Present(a.guild, a.channel)
		var e error
		candidateChannel, e = api.UserVoiceChannel(ctx, a.guild, a.member)
		return e
	})
	if err != nil {
		s.mu.Lock()
		a.presenceReady = false
		s.mu.Unlock()
		s.send(map[string]any{"type": "listen", "session_id": a.id, "enabled": false})
		s.state(a, "presence_error", "Could not verify the participant's voice channel. Retrying automatically.")
		return
	}
	people := map[string]pipeline.Participant{}
	for _, id := range ids {
		if id != s.bot.Status().BotID && id != a.member {
			people[id] = s.participant(a, id)
		}
	}
	if candidateChannel == a.channel {
		people[a.member] = s.participant(a, a.member)
	}
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	wasPaused := len(a.people) != 1 || a.people[a.member].ID == ""
	changed := !a.presenceReady || len(a.people) != len(people)
	for id, p := range people {
		if a.people[id] != p {
			changed = true
		}
	}
	resume := !a.presenceReady || wasPaused
	a.people = people
	a.presenceReady = true
	a.deadline = nextDeadline(time.Now(), people[a.member].ID != "", a.deadline)
	s.mu.Unlock()
	if changed {
		s.publishPresence(a, resume)
	}
}
func (s *Service) maybeStart(a *session) {
	s.mu.Lock()
	if s.active != a || a.busy || a.recovering || a.stopped || !a.ready || !a.presenceReady || a.people[a.member].ID == "" || len(a.people) != 1 {
		s.mu.Unlock()
		return
	}
	a.busy = true
	s.mu.Unlock()
	go s.continueInterview(a)
}
func (s *Service) continueInterview(a *session) {
	defer s.finishBusy(a)
	c, err := s.engine.Candidate(a.id)
	if err != nil {
		return
	}
	if c.Live.Status == "ready" {
		s.turn(a, "start", "")
	} else if c.Live.Status == "error" {
		s.turn(a, "retry", "")
	} else if c.Live.Status == "awaiting_answer" {
		s.question(a, c)
	} else if c.Live.Status == "complete" {
		s.state(a, "human_review", "")
		_ = s.speak(a, "Your interview is complete. Your summary is ready for a human reviewer. I'll remain here while you wait.", false, a.epoch)
	}
}
func (s *Service) finishBusy(a *session) {
	s.mu.Lock()
	if s.active != a {
		s.mu.Unlock()
		return
	}
	a.busy = false
	pending := a.pending
	a.pending = nil
	waiting := a.state == "waiting_for_people"
	s.mu.Unlock()
	if pending != nil {
		s.accept(a, *pending)
	}
	if waiting {
		s.maybeStart(a)
	}
}
func (s *Service) turn(a *session, action, answer string) {
	s.state(a, "thinking", "")
	s.send(map[string]any{"type": "listen", "session_id": a.id, "enabled": false})
	c, err := s.engine.Candidate(a.id)
	if err != nil {
		return
	}
	cfg, key := s.store.Credentials()
	settings := s.store.Public().Interview
	if action == "start" {
		s.mu.Lock()
		intro := a.intro
		a.intro = true
		s.mu.Unlock()
		if !intro {
			count := max(cfg.MaxQuestions, len(c.Profile.Cards))
			greeting := pipeline.VoiceIntroduction(c.Profile.Name, count, settings.Minutes, settings.Greeting)
			_ = s.chatMessage(a, greeting+"\n\nUse `/interview-help` to call a reviewer, `/interview-repeat` to repeat, or `/interview-stop` to stop.", "")
			_ = s.speak(a, greeting, false, 0)
		}
	}
	err = s.engine.RunAI(a.ctx, a.id, action, answer, c.Live.Version, providers.Compatible{Config: cfg, Key: key, Voice: true, Interview: settings})
	saved, savedErr := s.engine.Candidate(a.id)
	if savedErr == nil && saved.Live != nil {
		s.mu.Lock()
		a.limitAt = saved.Live.Deadline
		s.mu.Unlock()
	}
	if savedErr != nil || saved.Live == nil || saved.Live.Status == "failed" {
		return
	}
	if err != nil {
		s.state(a, "voice_error", "Interview state could not be saved")
		return
	}
	c = saved
	if c.Live.Status == "complete" {
		providers.ForgetCodexInterview(a.id)
	}
	s.mu.Lock()
	canSpeak := s.active == a && a.ready && !a.stopped && !a.closing && a.presenceReady && len(a.people) == 1 && a.people[a.member].ID != ""
	s.mu.Unlock()
	if !canSpeak {
		return
	}
	if c.Live.Status == "error" {
		s.state(a, "voice_error", c.Live.Error)
		_ = s.speak(a, "The AI request could not finish. Your answer is saved. Use interview repeat to retry, or ask a reviewer for help.", false, a.epoch)
		return
	}
	if c.Live.Status == "complete" {
		s.state(a, "human_review", "")
		_ = s.speak(a, "Thank you. Your interview is complete, and your report is ready for a human reviewer. I'll remain in this room.", false, a.epoch)
		return
	}
	s.question(a, c)
}
func (s *Service) question(a *session, c pipeline.Candidate) {
	for i := len(c.Transcript) - 1; i >= 0; i-- {
		if c.Transcript[i].Speaker == "Interviewer" {
			if c.Transcript[i].Channel == "chat" {
				s.writtenQuestion(a, c.Transcript[i], i+1)
				return
			}
			s.mu.Lock()
			a.epoch++
			epoch := a.epoch
			s.mu.Unlock()
			s.state(a, "speaking", "")
			if err := s.speak(a, c.Transcript[i].Text, true, epoch); err != nil {
				s.interruptVoice(a, true, "Question playback interrupted. Checking the voice connection…")
			} else {
				s.mu.Lock()
				canListen := s.active == a && a.ready && a.presenceReady && !a.stopped && len(a.people) == 1 && a.people[a.member].ID != ""
				s.mu.Unlock()
				if canListen {
					s.state(a, "listening", "")
				}
			}
			return
		}
	}
}
func (s *Service) speak(a *session, text string, listen bool, epoch int) error {
	if len([]rune(text)) > 1600 {
		return errors.New("question is too long for a voice turn")
	}
	s.mu.Lock()
	b := s.bridge
	s.mu.Unlock()
	if b == nil {
		return errors.New("voice worker is offline")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	event, err := b.Call(ctx, "speak", map[string]any{"session_id": a.id, "text": text, "listen": listen, "epoch": epoch})
	if err == nil && event.Characters > 0 {
		_ = s.engine.RecordSpeech(a.id, "synthesis", event.Seconds, event.Characters, event.DurationMS)
	}
	return err
}
func (s *Service) accept(a *session, event workerEvent) {
	s.mu.Lock()
	if s.active != a || a.stopped || a.closing || a.recovering || !a.ready || !a.presenceReady || event.Epoch != a.epoch ||
		(a.limitAt != nil && !time.Now().Before(*a.limitAt)) {
		s.mu.Unlock()
		return
	}
	if a.busy {
		copy := event
		a.pending = &copy
		s.mu.Unlock()
		return
	}
	if len(a.people) != 1 || a.people[a.member].ID == "" {
		s.mu.Unlock()
		return
	}
	a.busy = true
	s.mu.Unlock()
	go func() {
		defer s.finishBusy(a)
		_ = s.engine.RecordSpeech(a.id, "recognition", event.Seconds, 0, event.DurationMS)
		text := strings.TrimSpace(event.Text)
		command := strings.Trim(strings.ToLower(text), " .!?,")
		if command == "stop interview" {
			s.stop(a)
			return
		}
		if command == "repeat question" {
			c, _ := s.engine.Candidate(a.id)
			s.question(a, c)
			return
		}
		if text == "" {
			// Breathing/noise is not an answer. Resume silently rather than
			// repeatedly speaking a clarification after each short noise burst.
			s.send(map[string]any{"type": "listen", "session_id": a.id, "enabled": true, "epoch": a.epoch})
			s.state(a, "listening", "")
			return
		}
		s.turn(a, "answer", text)
	}()
}
func (s *Service) stop(a *session) {
	s.stopInterview(a, "Interview stopped by the candidate; saved answers remain available for human review.")
}

func (s *Service) stopInterview(a *session, message string) bool {
	s.mu.Lock()
	if s.active != a || a.stopped || a.closing {
		s.mu.Unlock()
		return false
	}
	a.stopped = true
	a.cancel()
	s.mu.Unlock()
	go providers.ForgetCodexInterview(a.id)
	s.send(map[string]any{"type": "listen", "session_id": a.id, "enabled": false})
	s.state(a, "stopped", message)
	return true
}
func (s *Service) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case event := <-s.events:
			s.mu.Lock()
			a := s.active
			stale := event.generation != s.generation
			id := ""
			if a != nil {
				id = a.id
			}
			s.mu.Unlock()
			if stale {
				continue
			}
			if event.Event == "fatal" {
				s.mu.Lock()
				if event.generation != s.generation {
					s.mu.Unlock()
					continue
				}
				b := s.bridge
				s.bridge = nil
				s.generation++
				if a != nil {
					a.ready = false
				}
				s.status.Ready = false
				s.status.Message = event.Error
				s.status.State = "error"
				s.status.Revision++
				s.mu.Unlock()
				if b != nil {
					go b.Close()
				}
				if a != nil {
					s.state(a, "voice_error", event.Error)
				}
				continue
			}
			if a == nil || id == "" || event.SessionID != id {
				continue
			}
			switch event.Event {
			case "gateway.send":
				if event.Payload.Op == 4 && event.Payload.Data.GuildID == a.guild {
					channel := ""
					if event.Payload.Data.ChannelID != nil {
						channel = *event.Payload.Data.ChannelID
					}
					if channel == a.channel || channel == "" {
						_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.SendVoiceState(a.guild, channel) })
					}
				}
			case "voice.ready":
				s.mu.Lock()
				a.ready = true
				s.mu.Unlock()
				s.state(a, "waiting_for_people", "")
				go s.refreshPresence(a)
			case "voice.activity", "transcribing":
				s.mu.Lock()
				current := s.active == a && event.Epoch == a.epoch && !a.recovering
				if current && a.state == "listening" && event.Event == "voice.activity" {
					a.silenceSince = time.Now()
				}
				s.mu.Unlock()
				if current && event.Event == "transcribing" {
					s.state(a, "transcribing", "")
				}
			case "transcript", "empty":
				s.accept(a, event)
			case "voice.lost", "voice.error", "speech.error":
				s.interruptVoice(a, event.Event == "speech.error", "Voice interrupted. Checking the connection…")
			}
		case now := <-ticker.C:
			s.mu.Lock()
			for key, p := range s.pending {
				if now.After(p.expires) {
					delete(s.pending, key)
				}
			}
			a := s.active
			expire := a != nil && !a.closing && a.deadline != nil && now.After(*a.deadline) && a.people[a.member].ID == "" && now.After(a.cleanupRetry)
			if expire {
				a.closing = true
			}
			syncPresence := a != nil && !a.recovering && a.ready && !a.closing && !a.syncing && !a.busy && now.Sub(a.lastPresenceCheck) > 10*time.Second
			checkLimit := a != nil && !a.closing && !a.stopped && a.limitAt != nil && !now.Before(*a.limitAt)
			s.mu.Unlock()
			if checkLimit {
				c, err := s.engine.Candidate(a.id)
				if err == nil && c.Live != nil && c.Live.Status == "failed" && c.Live.Voice != nil && c.Live.Voice.Stopped && s.stopInterview(a, c.Live.Failure) {
					go func(a *session) {
						_ = s.chatMessage(a, "Time is up. Your answers have been saved in Teamrise.", "")
					}(a)
				} else if err == nil && c.Live != nil && c.Live.Status == "complete" {
					s.mu.Lock()
					a.limitAt = nil
					s.mu.Unlock()
				}
			}
			s.scheduleVoiceCheck(a, now)
			if syncPresence {
				go s.refreshPresence(a)
			}
			if expire {
				go s.cleanup(a)
			}
		}
	}
}
func (s *Service) cleanup(a *session) {
	s.op.Lock()
	defer s.op.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	present := false
	err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
		channel, e := api.UserVoiceChannel(ctx, a.guild, a.member)
		if e != nil {
			return e
		}
		if channel == a.channel {
			present = true
			return nil
		}
		ch, e := api.Channel(ctx, a.channel)
		if e != nil {
			var rest *discordgo.RESTError
			if errors.As(e, &rest) && rest.Response != nil && rest.Response.StatusCode == 404 {
				return nil
			}
			return e
		}
		c, e := s.engine.Candidate(a.id)
		if e != nil || c.Live == nil || c.Live.Discord == nil || ch.GuildID != a.guild || ch.Name != c.Live.Discord.Payload.Name {
			return errors.New("room ownership mismatch")
		}
		channel, e = api.UserVoiceChannel(ctx, a.guild, a.member)
		if e != nil {
			return e
		}
		if channel == a.channel {
			present = true
			return nil
		}
		return api.DeleteRoom(ctx, a.channel)
	})
	if present {
		s.mu.Lock()
		a.closing = false
		a.deadline = nil
		s.mu.Unlock()
		s.refreshPresence(a)
		return
	}
	if err != nil {
		s.mu.Lock()
		a.closing = false
		a.cleanupRetry = time.Now().Add(time.Minute)
		s.mu.Unlock()
		s.state(a, "cleanup_pending", "Could not remove the expired room. Cleanup will retry.")
		return
	}
	a.cancel()
	providers.ForgetCodexInterview(a.id)
	s.mu.Lock()
	b := s.bridge
	a.ready = false
	s.mu.Unlock()
	if b != nil {
		_, _ = b.Call(ctx, "leave", map[string]any{"session_id": a.id})
	}
	_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.SendVoiceState(a.guild, "") })
	_ = s.engine.SetVoiceState(a.id, "closed", "", false)
	s.abandon(a)
}
func (s *Service) recoverRoom(ctx context.Context) error {
	s.mu.Lock()
	current := s.active
	if current != nil {
		if current.busy {
			s.mu.Unlock()
			return errors.New("wait for the current interview turn before reconnecting voice")
		}
		current.ready = false
		s.mu.Unlock()
		if err := s.verifySavedRoom(ctx, current.id, current.guild, current.channel); err != nil {
			var rest *discordgo.RESTError
			if errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == 404 {
				s.mu.Lock()
				b := s.bridge
				s.mu.Unlock()
				if b != nil {
					_, _ = b.Call(ctx, "leave", map[string]any{"session_id": current.id})
				}
				_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.SendVoiceState(current.guild, "") })
				_ = s.engine.SetVoiceState(current.id, "closed", "The previous room was deleted.", false)
				s.abandon(current)
				return nil
			}
			return errors.New("could not verify the saved interview room")
		}
		s.send(map[string]any{"type": "leave", "session_id": current.id})
		_ = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.SendVoiceState(current.guild, "") })
		// Acknowledge leave before starting a new transport.
		s.mu.Lock()
		b := s.bridge
		s.mu.Unlock()
		if b != nil {
			if _, err := b.Call(ctx, "leave", map[string]any{"session_id": current.id}); err != nil {
				return err
			}
		}
		return s.join(current)
	}
	s.mu.Unlock()
	cfg := s.store.Public().Discord
	cs := s.engine.Snapshot().Candidates
	for i := len(cs) - 1; i >= 0; i-- {
		c := cs[i]
		if c.Live == nil || c.Live.Voice == nil || c.Live.Voice.State == "closed" || c.Live.Discord == nil || c.Live.Voice.GuildID != cfg.GuildID {
			continue
		}
		channelID := c.Live.Discord.ChannelID
		exists := false
		err := s.bot.UseVoice(func(api discordbot.VoiceAPI) error {
			ch, err := api.Channel(ctx, channelID)
			if err != nil {
				return err
			}
			exists = ch != nil && ch.Name == c.Live.Discord.Payload.Name && privateChannel(ch, cfg, c.Live.Voice.MemberID)
			return nil
		})
		if err != nil {
			var rest *discordgo.RESTError
			if !errors.As(err, &rest) || rest.Response == nil || rest.Response.StatusCode != 404 {
				return errors.New("could not verify the previous interview room; reconnect voice when Discord is reachable")
			}
		}
		if !exists {
			_ = s.engine.SetVoiceState(c.ID, "closed", "Previous room no longer exists", false)
			continue
		}
		if err := s.verifySavedRoom(ctx, c.ID, c.Live.Voice.GuildID, channelID); err != nil {
			return errors.New("could not prepare the saved interview room")
		}
		aCtx, cancel := context.WithCancel(s.ctx)
		a := &session{id: c.ID, guild: cfg.GuildID, channel: channelID, member: c.Live.Voice.MemberID, ctx: aCtx, cancel: cancel, people: map[string]pipeline.Participant{}, intro: len(c.Transcript) > 0, stopped: c.Live.Voice.Stopped}
		a.deadline = c.Live.Voice.EmptyDeadline
		a.limitAt = c.Live.Deadline
		if a.deadline != nil && c.Live.Voice.ReconnectGraceSeconds == 0 {
			shortened := a.deadline.Add(-5 * time.Minute)
			a.deadline = &shortened
		}
		a.deadline = nextDeadline(time.Now(), false, a.deadline)
		s.mu.Lock()
		s.active = a
		s.status.Revision++
		s.mu.Unlock()
		if err := s.join(a); err != nil {
			s.state(a, "voice_error", err.Error())
			return nil
		}
		return nil
	}
	return nil
}

func (s *Service) verifySavedRoom(ctx context.Context, id, guild, channel string) error {
	c, err := s.engine.Candidate(id)
	if err != nil || c.Live == nil || c.Live.Discord == nil {
		return errors.New("saved interview is unavailable")
	}
	var ch *discordgo.Channel
	err = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { var e error; ch, e = api.Channel(ctx, channel); return e })
	if err != nil {
		return err
	}
	if ch == nil || ch.GuildID != guild || ch.Name != c.Live.Discord.Payload.Name || !privateChannel(ch, s.store.Public().Discord, c.Live.Voice.MemberID) {
		return errors.New("room ownership mismatch")
	}
	if strings.HasPrefix(ch.Name, "interview-") {
		name, e := s.engine.NextVoiceRoomName()
		if e != nil {
			return e
		}
		e = s.bot.UseVoice(func(api discordbot.VoiceAPI) error { return api.RenameRoom(ctx, channel, name) })
		if e != nil {
			return e
		}
		return s.engine.RenameVoiceRoom(id, name)
	}
	return nil
}
