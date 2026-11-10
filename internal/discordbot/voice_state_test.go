package discordbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVoiceStateRESTDistinguishesAbsenceFromAccessFailure(t *testing.T) {
	status := 200
	body := `{"channel_id":"room","user_id":"member"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bot "+testToken || !strings.HasSuffix(r.URL.Path, "/guilds/guild/voice-states/member") {
			t.Error("wrong voice state request")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == 200 {
			w.Write([]byte(body))
		} else {
			w.Write([]byte(`{"message":"fixture","code":10065}`))
		}
	}))
	defer srv.Close()
	raw, err := newSDK(testToken)
	if err != nil {
		t.Fatal(err)
	}
	sdk := raw.(*sdk)
	defer sdk.Close()
	target, _ := url.Parse(srv.URL)
	sdk.s.Client.Transport = fixtureTransport{target}
	channel, err := sdk.UserVoiceChannel(context.Background(), "guild", "member")
	if err != nil || channel != "room" {
		t.Fatal("could not recover existing voice membership")
	}
	body = `{}`
	if _, err = sdk.UserVoiceChannel(context.Background(), "guild", "member"); err == nil {
		t.Fatal("invalid response was treated as absence")
	}
	status = 404
	channel, err = sdk.UserVoiceChannel(context.Background(), "guild", "member")
	if err != nil || channel != "" {
		t.Fatal("confirmed absence was not handled")
	}
	status = 403
	if _, err = sdk.UserVoiceChannel(context.Background(), "guild", "member"); err == nil {
		t.Fatal("access failure was treated as absence")
	}
}
