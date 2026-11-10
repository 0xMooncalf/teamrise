package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"ladder/internal/connections"
)

const testGuild = "12345678901234567"
const testBot = "23456789012345678"
const testCategory = "34567890123456789"
const testRole = "45678901234567890"
const testToken = "private-discord-test-token-never-export-this"

type fakeBackend struct {
	state          func(string)
	closed         bool
	fail           error
	human          bool
	denied         bool
	categoryDenied bool
}

func (f *fakeBackend) Identity(context.Context) (*discordgo.User, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	return &discordgo.User{ID: testBot, Username: "Echo", Bot: !f.human}, nil
}
func (f *fakeBackend) Guilds(context.Context) ([]*discordgo.UserGuild, error) {
	return []*discordgo.UserGuild{{ID: testGuild, Name: "Teamrise test"}}, nil
}
func (f *fakeBackend) Channels(context.Context, string) ([]*discordgo.Channel, error) {
	c := &discordgo.Channel{ID: testCategory, Type: discordgo.ChannelTypeGuildCategory, Name: "Interviews"}
	if f.categoryDenied {
		c.PermissionOverwrites = []*discordgo.PermissionOverwrite{{ID: testGuild, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionVoiceConnect}}
	}
	return []*discordgo.Channel{c}, nil
}
func (f *fakeBackend) Roles(context.Context, string) ([]*discordgo.Role, error) {
	p := int64(discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak)
	if !f.denied {
		p |= discordgo.PermissionManageChannels
	}
	return []*discordgo.Role{{ID: testGuild, Name: "@everyone", Permissions: p}, {ID: testRole, Name: "Reviewer"}}, nil
}
func (f *fakeBackend) Member(context.Context, string, string) (*discordgo.Member, error) {
	return &discordgo.Member{}, nil
}
func (f *fakeBackend) Guild(context.Context, string) (*discordgo.Guild, error) {
	return &discordgo.Guild{ID: testGuild, OwnerID: "56789012345678901"}, nil
}
func (f *fakeBackend) Open(context.Context) error {
	if f.state != nil {
		f.state("ready")
	}
	return nil
}
func (f *fakeBackend) Close() {
	f.closed = true
	if f.state != nil {
		f.state("disconnected")
	}
}
func (f *fakeBackend) OnState(cb func(string)) { f.state = cb }

func TestConnectDiscoverVerifyAndForgetToken(t *testing.T) {
	f := &fakeBackend{}
	m := NewWithFactory(func(token string) (Backend, error) {
		if token != testToken {
			t.Fatal("token changed")
		}
		return f, nil
	})
	if err := m.Connect(context.Background(), "Bot "+testToken); err != nil {
		t.Fatal(err)
	}
	s := m.Status()
	if !s.Connected || !s.HasToken || s.VoiceReady || len(s.Guilds) != 1 {
		t.Fatalf("wrong connection state %+v", s)
	}
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), testToken) {
		t.Fatal("token in public status")
	}
	cfg := connections.Discord{Enabled: true, AutoCreateRooms: true, GuildID: testGuild, CategoryID: testCategory, ReviewerRoleID: testRole, BotUserID: testBot}
	if err := m.Verify(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	d, err := m.Details(context.Background(), testGuild, testCategory)
	if err != nil || len(d.Roles) != 1 || len(d.Categories) != 1 {
		t.Fatal("discovery failed", err)
	}
	if _, err = m.Details(context.Background(), "99999999999999999", ""); err == nil {
		t.Fatal("foreign guild accepted")
	}
	f.denied = true
	if m.Verify(context.Background(), cfg) == nil {
		t.Fatal("missing management permission accepted")
	}
	f.denied = false
	f.categoryDenied = true
	if m.Verify(context.Background(), cfg) == nil {
		t.Fatal("category permission deny ignored")
	}
	f.categoryDenied = false
	f.state("disconnected")
	if m.Status().Connected {
		t.Fatal("disconnect not reflected")
	}
	if err = m.Reconnect(context.Background()); err != nil || !m.Status().Connected {
		t.Fatal("session reconnect failed", err)
	}
	m.Close()
	if m.Status().HasToken || m.Status().Connected || !f.closed {
		t.Fatal("disconnect retained credential or socket")
	}
	f.state("ready")
	if m.Status().Connected {
		t.Fatal("stale gateway event resurrected disconnected bot")
	}
}

func TestCredentialFailuresAreRedactedAndKeepExistingConnection(t *testing.T) {
	for _, code := range []int{401, 403, 404, 429, 500} {
		f := &fakeBackend{fail: &discordgo.RESTError{Response: &http.Response{StatusCode: code}, ResponseBody: []byte(testToken)}}
		m := NewWithFactory(func(string) (Backend, error) { return f, nil })
		err := m.Connect(context.Background(), testToken)
		if err == nil || strings.Contains(err.Error(), testToken) || m.Status().HasToken || !f.closed {
			t.Fatal("unsafe authentication failure")
		}
	}
	good := &fakeBackend{}
	count := 0
	m := NewWithFactory(func(string) (Backend, error) {
		count++
		if count == 1 {
			return good, nil
		}
		return &fakeBackend{fail: errors.New(testToken)}, nil
	})
	if m.Connect(context.Background(), testToken) != nil {
		t.Fatal("initial connection failed")
	}
	if m.Connect(context.Background(), testToken) == nil || !m.Status().Connected || good.closed {
		t.Fatal("bad replacement destroyed working connection")
	}
	m.Close()
	human := &fakeBackend{human: true}
	m = NewWithFactory(func(string) (Backend, error) { return human, nil })
	if m.Connect(context.Background(), testToken) == nil {
		t.Fatal("user account accepted as bot")
	}
}

func TestAdministratorBypassesCategoryOverwrites(t *testing.T) {
	g := &discordgo.Guild{ID: testGuild}
	roles := []*discordgo.Role{{ID: testGuild, Permissions: discordgo.PermissionAdministrator}}
	_, effective := permissions(g, testBot, nil, roles, &discordgo.Channel{PermissionOverwrites: []*discordgo.PermissionOverwrite{{ID: testBot, Type: discordgo.PermissionOverwriteTypeMember, Deny: discordgo.PermissionAllChannel}}})
	if effective&discordgo.PermissionVoiceConnect == 0 {
		t.Fatal("Administrator bypass incorrectly denied")
	}
}
