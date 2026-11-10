package discordbot

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

type sdk struct {
	s      *discordgo.Session
	mu     sync.Mutex
	closed bool
	conns  []net.Conn
	ready  chan struct{}
	once   sync.Once
}

func newSDK(token string) (Backend, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	s.LogLevel = -1 // Never print SDK request bodies or upstream error text.
	s.Debug = false
	s.SyncEvents = true
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates
	s.Identify.Presence = discordgo.GatewayStatusUpdate{Status: "online"}
	s.ShouldRetryOnRateLimit = false
	s.ShouldReconnectOnError = false // Explicit bounded reconnect; never retain an orphan retry loop after disconnect.
	s.Client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	x := &sdk{s: s, ready: make(chan struct{})}
	d := *websocket.DefaultDialer
	d.HandshakeTimeout = 10 * time.Second
	d.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		x.mu.Lock()
		defer x.mu.Unlock()
		if x.closed {
			conn.Close()
			return nil, errors.New("Discord connection closed")
		}
		for _, old := range x.conns {
			_ = old.Close()
		}
		x.conns = []net.Conn{conn}
		return conn, nil
	}
	s.Dialer = &d
	return x, nil
}
func (x *sdk) OnState(callback func(string)) {
	x.s.AddHandler(func(*discordgo.Session, *discordgo.Ready) { x.once.Do(func() { close(x.ready) }); callback("ready") })
	x.s.AddHandler(func(*discordgo.Session, *discordgo.Resumed) { callback("ready") })
	x.s.AddHandler(func(*discordgo.Session, *discordgo.Disconnect) { callback("disconnected") })
}
func (x *sdk) Open(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- x.s.Open() }()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		x.Close()
		return ctx.Err()
	}
	select {
	case <-x.ready:
		return nil
	case <-ctx.Done():
		x.Close()
		return ctx.Err()
	}
}
func (x *sdk) Close() {
	x.mu.Lock()
	x.closed = true
	for _, conn := range x.conns {
		_ = conn.Close()
	}
	x.conns = nil
	x.mu.Unlock()
	_ = x.s.Close()
}
func (x *sdk) Identity(ctx context.Context) (*discordgo.User, error) {
	return x.s.User("@me", discordgo.WithContext(ctx))
}
func (x *sdk) Guilds(ctx context.Context) ([]*discordgo.UserGuild, error) {
	all := []*discordgo.UserGuild{}
	after := ""
	for i := 0; i < 50; i++ {
		page, err := x.s.UserGuilds(200, "", after, false, discordgo.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < 200 {
			return all, nil
		}
		next := page[len(page)-1].ID
		if next == after {
			return nil, errors.New("invalid guild pagination")
		}
		after = next
	}
	return nil, errors.New("guild list too large")
}
func (x *sdk) Channels(ctx context.Context, id string) ([]*discordgo.Channel, error) {
	return x.s.GuildChannels(id, discordgo.WithContext(ctx))
}
func (x *sdk) Roles(ctx context.Context, id string) ([]*discordgo.Role, error) {
	return x.s.GuildRoles(id, discordgo.WithContext(ctx))
}
func (x *sdk) Member(ctx context.Context, id, user string) (*discordgo.Member, error) {
	return x.s.GuildMember(id, user, discordgo.WithContext(ctx))
}
func (x *sdk) Guild(ctx context.Context, id string) (*discordgo.Guild, error) {
	return x.s.Guild(id, discordgo.WithContext(ctx))
}
