package discordbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
	"ladder/internal/connections"
)

type fixtureTransport struct{ target *url.URL }

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = f.target.Scheme
	u.Host = f.target.Host
	copy.URL = &u
	return http.DefaultTransport.RoundTrip(copy)
}

func TestSDKUsesBotAuthenticationAndReceivesGatewayReady(t *testing.T) {
	var gatewayURL string
	var identified atomic.Bool
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/socket" {
			ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer ws.Close()
			_ = ws.WriteJSON(map[string]any{"op": 10, "d": map[string]int{"heartbeat_interval": 60000}})
			var identify struct {
				Op   int `json:"op"`
				Data struct {
					Token   string `json:"token"`
					Intents int    `json:"intents"`
				} `json:"d"`
			}
			if ws.ReadJSON(&identify) != nil {
				return
			}
			if identify.Op != 2 || identify.Data.Token != "Bot "+testToken || identify.Data.Intents != int(discordgo.IntentsGuilds|discordgo.IntentsGuildVoiceStates) {
				t.Error("wrong gateway identification")
				return
			}
			identified.Store(true)
			_ = ws.WriteJSON(map[string]any{"op": 0, "t": "READY", "s": 1, "d": map[string]any{"v": 9, "session_id": "local-fixture", "resume_gateway_url": gatewayURL, "user": map[string]any{"id": testBot, "username": "Echo", "bot": true}, "guilds": []any{}, "application": map[string]any{"id": testBot, "flags": 0}}})
			for {
				var msg struct {
					Op int `json:"op"`
				}
				if ws.ReadJSON(&msg) != nil {
					return
				}
				if msg.Op == 1 {
					_ = ws.WriteJSON(map[string]any{"op": 11, "d": nil})
				}
			}
		}
		if !strings.HasSuffix(r.URL.Path, "/gateway") && r.Header.Get("Authorization") != "Bot "+testToken {
			t.Errorf("missing bot auth for %s (header length %d)", r.URL.Path, len(r.Header.Get("Authorization")))
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var body any
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/@me/guilds"):
			body = []any{map[string]string{"id": testGuild, "name": "Teamrise test"}}
		case strings.HasSuffix(r.URL.Path, "/users/@me"):
			body = map[string]any{"id": testBot, "username": "Echo", "bot": true}
		case strings.HasSuffix(r.URL.Path, "/gateway"):
			body = map[string]string{"url": gatewayURL}
		case strings.HasSuffix(r.URL.Path, "/channels"):
			body = []any{map[string]any{"id": testCategory, "name": "Interviews", "type": 4}}
		case strings.HasSuffix(r.URL.Path, "/roles"):
			body = []any{map[string]any{"id": testGuild, "name": "@everyone", "permissions": "3146768"}, map[string]any{"id": testRole, "name": "Reviewer", "permissions": "0"}}
		case strings.Contains(r.URL.Path, "/members/"):
			body = map[string]any{"user": map[string]string{"id": testBot}, "roles": []string{}}
		case strings.HasSuffix(r.URL.Path, "/guilds/"+testGuild):
			body = map[string]string{"id": testGuild, "owner_id": "56789012345678901"}
		default:
			t.Error("unexpected API route", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer fixture.Close()
	gatewayURL = "ws" + strings.TrimPrefix(fixture.URL, "http") + "/socket"
	target, _ := url.Parse(fixture.URL)
	m := NewWithFactory(func(token string) (Backend, error) {
		b, err := newSDK(token)
		if err != nil {
			return nil, err
		}
		x := b.(*sdk)
		x.s.Client.Transport = fixtureTransport{target}
		return x, nil
	})
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := m.Connect(ctx, testToken); err != nil {
		t.Fatal(err)
	}
	if !identified.Load() || !m.Status().Connected || m.Status().VoiceReady {
		t.Fatal("Gateway READY not distinguished from voice readiness")
	}
	cfg := connections.Discord{GuildID: testGuild, CategoryID: testCategory, ReviewerRoleID: testRole, BotUserID: testBot}
	if err := m.Verify(ctx, cfg); err != nil {
		t.Fatal(err)
	}
}
