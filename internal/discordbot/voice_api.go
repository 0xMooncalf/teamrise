package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bwmarrin/discordgo"
	"ladder/internal/discordrooms"
	"strconv"
)

// Voice credentials travel only from Gateway to the owned voice subprocess.
// These callbacks and API methods are never exposed as arbitrary HTTP proxies.
type Events struct {
	VoiceServer  func(*discordgo.VoiceServerUpdate)
	VoiceState   func(*discordgo.VoiceStateUpdate)
	Interaction  func(*discordgo.InteractionCreate)
	Disconnected func()
}
type VoiceAPI interface {
	SetEvents(Events)
	SendVoiceState(string, string) error
	RegisterInterviewCommands(context.Context, string, string, []*discordgo.ApplicationCommandOptionChoice) error
	Respond(context.Context, *discordgo.Interaction, *discordgo.InteractionResponse) error
	EditResponse(context.Context, *discordgo.Interaction, string) error
	SendInterviewMessage(context.Context, string, string, string, ...*discordgo.File) error
	CreatePrivateRoom(context.Context, string, discordrooms.Payload) (*discordgo.Channel, error)
	DeleteRoom(context.Context, string) error
	Channel(context.Context, string) (*discordgo.Channel, error)
	Present(string, string) []string
	UserVoiceChannel(context.Context, string, string) (string, error)
	RenameRoom(context.Context, string, string) error
	Member(context.Context, string, string) (*discordgo.Member, error)
	Guild(context.Context, string) (*discordgo.Guild, error)
	Roles(context.Context, string) ([]*discordgo.Role, error)
}

func (m *Manager) SetEvents(events Events) {
	m.mu.Lock()
	m.events = events
	b := m.backend
	m.mu.Unlock()
	if v, ok := b.(VoiceAPI); ok {
		v.SetEvents(events)
	}
}
func (m *Manager) UseVoice(fn func(VoiceAPI) error) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	b, connected := m.backend, m.status.Connected
	m.mu.Unlock()
	if !connected {
		return errors.New("connect the Discord bot first")
	}
	v, ok := b.(VoiceAPI)
	if !ok {
		return errors.New("this Discord backend does not support voice")
	}
	return fn(v)
}
func (x *sdk) SetEvents(e Events) {
	if e.VoiceServer != nil {
		x.s.AddHandler(func(_ *discordgo.Session, event *discordgo.VoiceServerUpdate) { e.VoiceServer(event) })
	}
	if e.VoiceState != nil {
		x.s.AddHandler(func(_ *discordgo.Session, event *discordgo.VoiceStateUpdate) { e.VoiceState(event) })
	}
	if e.Interaction != nil {
		x.s.AddHandler(func(_ *discordgo.Session, event *discordgo.InteractionCreate) { e.Interaction(event) })
	}
	if e.Disconnected != nil {
		x.s.AddHandler(func(*discordgo.Session, *discordgo.Disconnect) { e.Disconnected() })
	}
}
func (x *sdk) SendVoiceState(guildID, channelID string) error {
	return x.s.ChannelVoiceJoinManual(guildID, channelID, false, false)
}
func (x *sdk) RegisterInterviewCommands(ctx context.Context, botID, guildID string, choices []*discordgo.ApplicationCommandOptionChoice) error {
	for _, cmd := range []*discordgo.ApplicationCommand{
		{Name: "interview", Description: "Start a private AI voice interview", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "skill", Description: "Interview skill and prompt", Required: true, Choices: choices}}},
		{Name: "interview-help", Description: "Request a human reviewer in your interview room"},
		{Name: "interview-answer", Description: "Answer a written exercise in your interview room", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "answer", Description: "Your answer to the written exercise", Required: true, MaxLength: 1500}}},
		{Name: "interview-repeat", Description: "Repeat your current interview question or retry after an error"},
		{Name: "interview-stop", Description: "Stop your interview and keep the transcript for human review"},
	} {
		if _, err := x.s.ApplicationCommandCreate(botID, guildID, cmd, discordgo.WithContext(ctx)); err != nil {
			return safeError(err)
		}
	}
	return nil
}
func (x *sdk) Respond(ctx context.Context, i *discordgo.Interaction, r *discordgo.InteractionResponse) error {
	return x.s.InteractionRespond(i, r, discordgo.WithContext(ctx))
}
func (x *sdk) EditResponse(ctx context.Context, i *discordgo.Interaction, text string) error {
	_, err := x.s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &text}, discordgo.WithContext(ctx))
	return err
}
func (x *sdk) SendInterviewMessage(ctx context.Context, channelID, content, reviewerRole string, files ...*discordgo.File) error {
	mentions := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	if reviewerRole != "" {
		mentions.Roles = []string{reviewerRole}
	}
	_, err := x.s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{Content: content, AllowedMentions: mentions, Files: files}, discordgo.WithContext(ctx))
	return err
}
func (x *sdk) CreatePrivateRoom(ctx context.Context, guildID string, p discordrooms.Payload) (*discordgo.Channel, error) {
	overwrites := []*discordgo.PermissionOverwrite{}
	for _, o := range p.PermissionOverwrites {
		allow, err := strconv.ParseInt(o.Allow, 10, 64)
		if err != nil {
			return nil, err
		}
		deny, err := strconv.ParseInt(o.Deny, 10, 64)
		if err != nil {
			return nil, err
		}
		overwrites = append(overwrites, &discordgo.PermissionOverwrite{ID: o.ID, Type: discordgo.PermissionOverwriteType(o.Type), Allow: allow, Deny: deny})
	}
	return x.s.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{Name: p.Name, Type: discordgo.ChannelTypeGuildVoice, ParentID: p.ParentID, PermissionOverwrites: overwrites}, discordgo.WithContext(ctx))
}
func (x *sdk) DeleteRoom(ctx context.Context, id string) error {
	_, err := x.s.ChannelDelete(id, discordgo.WithContext(ctx))
	return err
}
func (x *sdk) Channel(ctx context.Context, id string) (*discordgo.Channel, error) {
	return x.s.Channel(id, discordgo.WithContext(ctx))
}
func (x *sdk) UserVoiceChannel(ctx context.Context, guildID, userID string) (string, error) {
	endpoint := discordgo.EndpointGuild(guildID) + "/voice-states/" + userID
	body, err := x.s.RequestWithBucketID("GET", endpoint, nil, discordgo.EndpointGuild(guildID)+"/voice-states/:user", discordgo.WithContext(ctx))
	if err != nil {
		var rest *discordgo.RESTError
		if errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == 404 {
			return "", nil
		}
		return "", err
	}
	var state struct {
		ChannelID json.RawMessage `json:"channel_id"`
	}
	if json.Unmarshal(body, &state) != nil || len(state.ChannelID) == 0 {
		return "", errors.New("invalid Discord voice state")
	}
	var channel string
	if json.Unmarshal(state.ChannelID, &channel) != nil {
		return "", errors.New("invalid Discord voice channel")
	}
	return channel, nil
}
func (x *sdk) RenameRoom(ctx context.Context, id, name string) error {
	_, err := x.s.ChannelEdit(id, &discordgo.ChannelEdit{Name: name}, discordgo.WithContext(ctx))
	return err
}
func (x *sdk) Present(guildID, channelID string) []string {
	x.s.State.RLock()
	defer x.s.State.RUnlock()
	ids := []string{}
	for _, g := range x.s.State.Guilds {
		if g.ID == guildID {
			for _, s := range g.VoiceStates {
				if s.ChannelID == channelID {
					ids = append(ids, s.UserID)
				}
			}
		}
	}
	return ids
}
