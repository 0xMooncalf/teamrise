package connections

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionCredentialNeverPersistsOrExports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	u := Update{AI: s.Public().AI, APIKey: "private-test-key", Bots: []Binding{{Agent: "Interviewer", GuildID: "12345678901234567", ChannelID: "23456789012345678", Enabled: true}}}
	u.AI.Model = "my-service-model"
	if err = s.Save(u); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	public, _ := json.Marshal(s.Public())
	if strings.Contains(string(data), u.APIKey) || strings.Contains(string(public), u.APIKey) || !s.Public().HasKey {
		t.Fatal("credential disclosure or missing in-memory key")
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_, key := reloaded.Credentials()
	if key != "" || reloaded.Public().HasKey {
		t.Fatal("credential survived process restart")
	}
	u.APIKey = ""
	if err = s.Save(u); err != nil {
		t.Fatal(err)
	}
	_, key = s.Credentials()
	if key != "private-test-key" {
		t.Fatal("blank input should preserve existing credential")
	}
	u.AI.BaseURL = "https://other-service.example/v1"
	if err = s.Save(u); err != nil {
		t.Fatal(err)
	}
	_, key = s.Credentials()
	if key != "" {
		t.Fatal("old credential would be forwarded to another service")
	}
}
func TestRejectInvalidConnectionSettings(t *testing.T) {
	for _, base := range []string{"http://external.example/v1", "https://secret@example.com/v1", "https://example.com/v1?key=secret", "file:///tmp/model"} {
		s, _ := New("")
		u := Update{AI: s.Public().AI}
		u.AI.BaseURL = base
		if s.Save(u) == nil {
			t.Fatalf("accepted unsafe URL %s", base)
		}
	}
	s, _ := New("")
	u := Update{AI: s.Public().AI, Bots: []Binding{{Agent: "Interviewer", Enabled: true, GuildID: "bad", ChannelID: "bad"}}}
	if s.Save(u) == nil {
		t.Fatal("invalid Discord assignment accepted")
	}
}

func TestLocalCodexProviderPersistsWithoutCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	s, _ := New(path)
	u := Update{AI: s.Public().AI}
	u.AI.Provider, u.AI.Model = "codex", "gpt-6-sol"
	if err := s.Save(u); err != nil {
		t.Fatal(err)
	}
	restored, err := New(path)
	if err != nil || restored.Public().AI.Provider != "codex" || restored.Public().HasKey {
		t.Fatal("subscription settings did not persist safely")
	}
	u.AI.Provider = "arbitrary-shell"
	if s.Save(u) == nil {
		t.Fatal("unsupported execution provider accepted")
	}
}

func TestSaveDiscordPreservesAIAndSessionKey(t *testing.T) {
	s, _ := New(filepath.Join(t.TempDir(), "connections.json"))
	u := Update{AI: s.Public().AI, APIKey: "private-ai-session-key"}
	u.AI.Provider = "codex"
	u.AI.Model = "gpt-6-sol"
	if err := s.Save(u); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDiscord(Discord{DisplayName: "Echo", AutoCreateRooms: true}); err != nil {
		t.Fatal(err)
	}
	ai, key := s.Credentials()
	if ai.Provider != "codex" || ai.Model != "gpt-6-sol" || key != u.APIKey {
		t.Fatal("Discord setup changed AI settings or credential")
	}
}
