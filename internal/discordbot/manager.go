// Package discordbot owns a session-only bot credential and the Discord control
// connection. Voice transport and interviewing are deliberately separate states.
package discordbot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"ladder/internal/connections"
)

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Status struct {
	Revision   uint64   `json:"revision"`
	Connected  bool     `json:"connected"`
	HasToken   bool     `json:"has_token"`
	Gateway    string   `json:"gateway"`
	BotID      string   `json:"bot_id,omitempty"`
	BotName    string   `json:"bot_name,omitempty"`
	Guilds     []Option `json:"guilds"`
	Message    string   `json:"message"`
	VoiceReady bool     `json:"voice_ready"`
}
type PermissionCheck struct {
	Name    string `json:"name"`
	Granted bool   `json:"granted"`
}
type Details struct {
	GuildID    string            `json:"guild_id"`
	Categories []Option          `json:"categories"`
	Roles      []Option          `json:"roles"`
	Checks     []PermissionCheck `json:"checks"`
}

// Backend keeps SDK/network behavior replaceable in tests without fake online
// flags in the production application.
type Backend interface {
	Identity(context.Context) (*discordgo.User, error)
	Guilds(context.Context) ([]*discordgo.UserGuild, error)
	Channels(context.Context, string) ([]*discordgo.Channel, error)
	Roles(context.Context, string) ([]*discordgo.Role, error)
	Member(context.Context, string, string) (*discordgo.Member, error)
	Guild(context.Context, string) (*discordgo.Guild, error)
	Open(context.Context) error
	Close()
	OnState(func(string))
}
type Manager struct {
	op         sync.Mutex
	mu         sync.Mutex
	backend    Backend
	generation uint64
	revision   uint64
	status     Status
	factory    func(string) (Backend, error)
	events     Events
}

func New() *Manager { return NewWithFactory(newSDK) }
func NewWithFactory(factory func(string) (Backend, error)) *Manager {
	return &Manager{factory: factory, status: Status{Gateway: "disconnected", Guilds: []Option{}, Message: "Connect a bot using its token. Voice is not connected yet."}}
}
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Revision = m.revision
	s.Guilds = append([]Option{}, s.Guilds...)
	return s
}

var tokenChars = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)

func normalizeToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "Bot ") {
		token = strings.TrimSpace(strings.TrimPrefix(token, "Bot "))
	}
	if len(token) < 32 || len(token) > 512 || !tokenChars.MatchString(token) {
		return "", errors.New("paste the bot token from Developer Portal → Bot; do not use an application ID or public key")
	}
	return token, nil
}
func safeError(err error) error {
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest.Response != nil {
		switch rest.Response.StatusCode {
		case 401:
			return errors.New("Discord rejected the bot token. Generate a current token in Developer Portal → Bot and paste it locally")
		case 403:
			return errors.New("Discord denied access. Check that this bot is installed on the server and has the required permissions")
		case 404:
			return errors.New("Discord could not find that server, category or role for this bot")
		case 429:
			return errors.New("Discord rate limit reached. Wait briefly before retrying")
		}
	}
	return errors.New("Discord could not complete the connection. Check internet access and the bot token, then retry")
}
func (m *Manager) Connect(ctx context.Context, token string) error {
	m.op.Lock()
	defer m.op.Unlock()
	token, err := normalizeToken(token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, err := m.factory(token)
	if err != nil {
		return safeError(err)
	}
	keep := false
	defer func() {
		if !keep {
			b.Close()
		}
	}()
	user, err := b.Identity(ctx)
	if err != nil {
		return safeError(err)
	}
	if user == nil || !user.Bot || !snowflake.MatchString(user.ID) {
		return errors.New("only an official Discord bot token is supported; user accounts are not supported")
	}
	guilds, err := b.Guilds(ctx)
	if err != nil {
		return safeError(err)
	}
	options := []Option{}
	for _, g := range guilds {
		if g != nil {
			options = append(options, Option{g.ID, g.Name})
		}
	}
	sortOptions(options)
	m.mu.Lock()
	old := m.backend
	m.generation++
	m.revision++
	generation := m.generation
	m.backend = b
	m.status = Status{HasToken: true, Gateway: "connecting", BotID: user.ID, BotName: user.Username, Guilds: options, Message: "Opening Discord Gateway…"}
	m.mu.Unlock()
	if old != nil {
		old.Close()
	}
	m.mu.Lock()
	hooks := m.events
	m.mu.Unlock()
	if v, ok := b.(VoiceAPI); ok {
		v.SetEvents(hooks)
	}
	b.OnState(func(state string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.generation != generation {
			return
		}
		m.revision++
		m.status.Gateway = state
		m.status.Connected = state == "ready"
		if state == "ready" {
			m.status.Message = "Bot connected to Discord. Enable local voice in Teamrise to start interviews."
		} else {
			m.status.Message = "Discord connection interrupted. Reconnect the bot using the token held in memory."
		}
	})
	if err = b.Open(ctx); err != nil {
		m.mu.Lock()
		if m.generation == generation {
			m.generation++
			m.revision++
			m.backend = nil
			m.status = Status{Gateway: "error", Guilds: []Option{}, Message: safeError(err).Error()}
		}
		m.mu.Unlock()
		return safeError(err)
	}
	m.mu.Lock()
	if m.generation == generation && m.status.Gateway == "connecting" {
		m.revision++
		m.status.Connected = true
		m.status.Gateway = "ready"
		m.status.Message = "Bot connected to Discord. Enable local voice in Teamrise to start interviews."
	}
	m.mu.Unlock()
	keep = true
	return nil
}
func (m *Manager) Reconnect(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	b, connected := m.backend, m.status.Connected
	m.mu.Unlock()
	if b == nil {
		return errors.New("enter the bot token first")
	}
	if connected {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if err := b.Open(ctx); err != nil {
		m.mu.Lock()
		m.generation++
		m.revision++
		m.backend = nil
		m.status = Status{Gateway: "error", Guilds: []Option{}, Message: "Reconnection failed. Enter the bot token again."}
		m.mu.Unlock()
		b.Close()
		return safeError(err)
	}
	return nil
}
func (m *Manager) Close() {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	old := m.backend
	m.backend = nil
	m.generation++
	m.revision++
	m.status = Status{Gateway: "disconnected", Guilds: []Option{}, Message: "Bot disconnected. Its token was removed from memory."}
	m.mu.Unlock()
	if old != nil {
		old.Close()
	}
}
func sortOptions(options []Option) {
	sort.Slice(options, func(i, j int) bool { return strings.ToLower(options[i].Name) < strings.ToLower(options[j].Name) })
}
func (m *Manager) Details(ctx context.Context, guildID, categoryID string) (Details, error) {
	m.op.Lock()
	defer m.op.Unlock()
	d := Details{GuildID: guildID, Categories: []Option{}, Roles: []Option{}, Checks: []PermissionCheck{}}
	m.mu.Lock()
	b, status := m.backend, m.status
	m.mu.Unlock()
	if b == nil || !status.Connected {
		return d, errors.New("connect the Discord bot first")
	}
	allowed := false
	for _, g := range status.Guilds {
		if g.ID == guildID {
			allowed = true
		}
	}
	if !allowed || !snowflake.MatchString(guildID) {
		return d, errors.New("choose a server where the connected bot is installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	channels, err := b.Channels(ctx, guildID)
	if err != nil {
		return d, safeError(err)
	}
	roles, err := b.Roles(ctx, guildID)
	if err != nil {
		return d, safeError(err)
	}
	member, err := b.Member(ctx, guildID, status.BotID)
	if err != nil {
		return d, safeError(err)
	}
	guild, err := b.Guild(ctx, guildID)
	if err != nil {
		return d, safeError(err)
	}
	if member == nil || guild == nil {
		return d, errors.New("Discord returned incomplete server data")
	}
	var category *discordgo.Channel
	for _, ch := range channels {
		if ch != nil && ch.Type == discordgo.ChannelTypeGuildCategory {
			d.Categories = append(d.Categories, Option{ch.ID, ch.Name})
			if ch.ID == categoryID {
				category = ch
			}
		}
	}
	for _, role := range roles {
		if role != nil && role.ID != guildID && !role.Managed {
			d.Roles = append(d.Roles, Option{role.ID, role.Name})
		}
	}
	sortOptions(d.Categories)
	sortOptions(d.Roles)
	if categoryID != "" && category == nil {
		return d, errors.New("the chosen interview category does not belong to this server")
	}
	base, effective := permissions(guild, status.BotID, member.Roles, roles, category)
	for _, check := range []struct {
		name  string
		bit   int64
		guild bool
	}{{"Manage Channels", discordgo.PermissionManageChannels, true}, {"View Channels", discordgo.PermissionViewChannel, false}, {"Connect", discordgo.PermissionVoiceConnect, false}, {"Speak", discordgo.PermissionVoiceSpeak, false}} {
		bits := effective
		if check.guild {
			bits = base
		}
		d.Checks = append(d.Checks, PermissionCheck{check.name, bits&check.bit == check.bit})
	}
	return d, nil
}
func (m *Manager) Verify(ctx context.Context, cfg connections.Discord) error {
	s := m.Status()
	if cfg.BotUserID != s.BotID {
		return errors.New("the interviewer ID must match the connected bot")
	}
	if cfg.CategoryID == "" || cfg.ReviewerRoleID == "" {
		return errors.New("choose the Interviews category and Reviewer role")
	}
	d, err := m.Details(ctx, cfg.GuildID, cfg.CategoryID)
	if err != nil {
		return err
	}
	found := false
	for _, role := range d.Roles {
		if role.ID == cfg.ReviewerRoleID {
			found = true
		}
	}
	if !found {
		return errors.New("choose a Reviewer role belonging to the selected server")
	}
	missing := []string{}
	for _, check := range d.Checks {
		if !check.Granted {
			missing = append(missing, check.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the bot needs these permissions: %s", strings.Join(missing, ", "))
	}
	return nil
}
func permissions(guild *discordgo.Guild, userID string, memberRoles []string, roles []*discordgo.Role, channel *discordgo.Channel) (int64, int64) {
	var base int64
	ids := map[string]bool{guild.ID: true}
	for _, id := range memberRoles {
		ids[id] = true
	}
	for _, r := range roles {
		if r != nil && ids[r.ID] {
			base |= r.Permissions
		}
	}
	if userID == guild.OwnerID || base&discordgo.PermissionAdministrator != 0 {
		return discordgo.PermissionAll, discordgo.PermissionAll
	}
	effective := base
	if channel == nil {
		return base, effective
	}
	for _, o := range channel.PermissionOverwrites {
		if o.Type == discordgo.PermissionOverwriteTypeRole && o.ID == guild.ID {
			effective = (effective &^ o.Deny) | o.Allow
		}
	}
	var deny, allow int64
	for _, o := range channel.PermissionOverwrites {
		if o.Type == discordgo.PermissionOverwriteTypeRole && o.ID != guild.ID && ids[o.ID] {
			deny |= o.Deny
			allow |= o.Allow
		}
	}
	effective = (effective &^ deny) | allow
	for _, o := range channel.PermissionOverwrites {
		if o.Type == discordgo.PermissionOverwriteTypeMember && o.ID == userID {
			effective = (effective &^ o.Deny) | o.Allow
		}
	}
	return base, effective
}
