package voice

import (
	"context"
	"errors"
	"github.com/bwmarrin/discordgo"
	"ladder/internal/connections"
	"ladder/internal/discordbot"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testBackend struct {
	discordbot.Backend
	discordbot.VoiceAPI
	responses        []*discordgo.InteractionResponse
	present          []string
	deleted          int
	deleteErr        error
	candidateChannel string
	voiceErr         error
	chatPosts        int
	chatChannel      string
	chatFiles        []*discordgo.File
}

const testBot = "12345678901234567"

func (b *testBackend) Guild(context.Context, string) (*discordgo.Guild, error) {
	return nil, errors.New("unused fixture method")
}
func (b *testBackend) Member(context.Context, string, string) (*discordgo.Member, error) {
	return nil, errors.New("unused fixture method")
}
func (b *testBackend) Roles(context.Context, string) ([]*discordgo.Role, error) {
	return nil, errors.New("unused fixture method")
}

func (b *testBackend) Identity(context.Context) (*discordgo.User, error) {
	return &discordgo.User{ID: testBot, Bot: true}, nil
}
func (b *testBackend) Guilds(context.Context) ([]*discordgo.UserGuild, error) {
	return []*discordgo.UserGuild{}, nil
}
func (b *testBackend) Open(context.Context) error  { return nil }
func (b *testBackend) Close()                      {}
func (b *testBackend) OnState(func(string))        {}
func (b *testBackend) SetEvents(discordbot.Events) {}
func (b *testBackend) Respond(_ context.Context, _ *discordgo.Interaction, r *discordgo.InteractionResponse) error {
	b.responses = append(b.responses, r)
	if r.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
		return errors.New("fixture: acknowledgement fails before room creation")
	}
	return nil
}
func (b *testBackend) Present(string, string) []string     { return b.present }
func (b *testBackend) SendVoiceState(string, string) error { return nil }
func (b *testBackend) UserVoiceChannel(_ context.Context, _, user string) (string, error) {
	if b.voiceErr != nil {
		return "", b.voiceErr
	}
	if b.candidateChannel != "" {
		return b.candidateChannel, nil
	}
	for _, id := range b.present {
		if id == user {
			return "room", nil
		}
	}
	return "", nil
}
func (b *testBackend) SendInterviewMessage(_ context.Context, channel, _, _ string, files ...*discordgo.File) error {
	b.chatPosts++
	b.chatChannel = channel
	b.chatFiles = files
	return nil
}
func (b *testBackend) RenameRoom(context.Context, string, string) error { return nil }
func (b *testBackend) Channel(context.Context, string) (*discordgo.Channel, error) {
	return &discordgo.Channel{GuildID: "guild", Name: "interview-owned"}, nil
}
func (b *testBackend) DeleteRoom(context.Context, string) error { b.deleted++; return b.deleteErr }
func connectedTestManager(t *testing.T, b *testBackend) *discordbot.Manager {
	t.Helper()
	m := discordbot.NewWithFactory(func(string) (discordbot.Backend, error) { return b, nil })
	if err := m.Connect(context.Background(), strings.Repeat("fixture", 8)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
func TestDiscordConsentIsMemberBoundExpiresAndCannotReplay(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	store, _ := connections.New("")
	cfg := connections.Discord{Enabled: true, AutoCreateRooms: true, GuildID: "22345678901234567", CategoryID: "32345678901234567", BotUserID: testBot, ReviewerRoleID: "42345678901234567"}
	if err := store.SaveDiscord(cfg); err != nil {
		t.Fatal(err)
	}
	b := &testBackend{}
	m := connectedTestManager(t, b)
	s := New(context.Background(), e, store, m)
	defer s.Close()
	s.status.Ready = true
	user := &discordgo.User{ID: "52345678901234567", Username: "Test"}
	command := &discordgo.Interaction{GuildID: cfg.GuildID, Member: &discordgo.Member{User: user}, Type: discordgo.InteractionApplicationCommand, Data: discordgo.ApplicationCommandInteractionData{Name: "interview", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "skill", Type: discordgo.ApplicationCommandOptionString, Value: e.Snapshot().Profiles[0].ID}}}}
	s.interaction(&discordgo.InteractionCreate{Interaction: command})
	if len(b.responses) != 1 || b.responses[0].Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Fatal("consent was not private")
	}
	button := b.responses[0].Data.Components[0].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	key := strings.TrimPrefix(button.CustomID, "teamrise:start:")
	click := func(u *discordgo.User) {
		s.interaction(&discordgo.InteractionCreate{Interaction: &discordgo.Interaction{GuildID: cfg.GuildID, Member: &discordgo.Member{User: u}, Type: discordgo.InteractionMessageComponent, Data: discordgo.MessageComponentInteractionData{CustomID: button.CustomID}}})
	}
	click(&discordgo.User{ID: "62345678901234567"})
	s.mu.Lock()
	_, exists := s.pending[key]
	active := s.active
	s.mu.Unlock()
	if !exists || active != nil {
		t.Fatal("another member consumed consent")
	}
	s.mu.Lock()
	p := s.pending[key]
	p.expires = time.Now().Add(-time.Minute)
	s.pending[key] = p
	s.mu.Unlock()
	click(user)
	s.mu.Lock()
	active = s.active
	p = s.pending[key]
	s.mu.Unlock()
	if active != nil {
		t.Fatal("expired consent started interview")
	}
	s.mu.Lock()
	p.expires = time.Now().Add(time.Minute)
	s.pending[key] = p
	s.mu.Unlock()
	click(user)
	s.mu.Lock()
	_, exists = s.pending[key]
	s.mu.Unlock()
	if exists {
		t.Fatal("consent was not consumed once")
	}
	click(user)
	s.mu.Lock()
	active = s.active
	s.mu.Unlock()
	if active != nil {
		t.Fatal("consent replay started interview")
	}
}
func TestCleanupRetainsOccupiedOrUnconfirmedRooms(t *testing.T) {
	for _, scenario := range []string{"occupied", "delete fails", "disconnected"} {
		t.Run(scenario, func(t *testing.T) {
			e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			id, err := e.CreateInterview("Test Candidate", e.Snapshot().Profiles[0].ID, "", &discordrooms.Plan{GuildID: "guild", MemberID: "member", ChannelID: "room", Payload: discordrooms.Payload{Name: "interview-owned"}})
			if err != nil {
				t.Fatal(err)
			}
			b := &testBackend{}
			m := connectedTestManager(t, b)
			if scenario == "occupied" {
				b.present = []string{"member"}
			}
			if scenario == "delete fails" {
				b.deleteErr = errors.New("unreachable")
			}
			if scenario == "disconnected" {
				m.Close()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &session{id: id, guild: "guild", channel: "room", member: "member", ctx: ctx, cancel: cancel, people: map[string]pipeline.Participant{}, closing: true}
			s := &Service{ctx: ctx, active: a, bot: m, engine: e}
			s.cleanup(a)
			if s.active != a || a.closing {
				t.Fatal("cleanup forgot a retained room")
			}
			c, _ := e.Candidate(id)
			if c.Room.Expired || c.Live.Voice.State == "closed" {
				t.Fatal("unconfirmed deletion reported as closed")
			}
			if scenario != "delete fails" && b.deleted != 0 {
				t.Fatal("occupied or unverified room was deleted")
			}
		})
	}
}
