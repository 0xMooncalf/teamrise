package voice

import (
	"context"
	"github.com/bwmarrin/discordgo"
	"ladder/internal/connections"
	"ladder/internal/pipeline"
	"path/filepath"
	"testing"
)

func privateFixture() (*discordgo.Channel, connections.Discord) {
	cfg := connections.Discord{GuildID: "guild", CategoryID: "category", BotUserID: "bot", ReviewerRoleID: "reviewer"}
	bits := int64(discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak)
	return &discordgo.Channel{GuildID: "guild", ParentID: "category", Type: discordgo.ChannelTypeGuildVoice, PermissionOverwrites: []*discordgo.PermissionOverwrite{
		{ID: "guild", Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect},
		{ID: "member", Type: discordgo.PermissionOverwriteTypeMember, Allow: bits},
		{ID: "bot", Type: discordgo.PermissionOverwriteTypeMember, Allow: bits},
		{ID: "reviewer", Type: discordgo.PermissionOverwriteTypeRole, Allow: bits},
	}}, cfg
}
func TestReturnedRoomMustBePrivateAndOwned(t *testing.T) {
	ch, cfg := privateFixture()
	if !privateChannel(ch, cfg, "member") {
		t.Fatal("valid private room rejected")
	}
	tests := []struct {
		name   string
		modify func(*discordgo.Channel)
	}{
		{"wrong server", func(c *discordgo.Channel) { c.GuildID = "other" }},
		{"wrong category", func(c *discordgo.Channel) { c.ParentID = "other" }},
		{"wrong type", func(c *discordgo.Channel) { c.Type = discordgo.ChannelTypeGuildText }},
		{"everyone allowed", func(c *discordgo.Channel) { c.PermissionOverwrites[0].Allow = discordgo.PermissionViewChannel }},
		{"role instead of member", func(c *discordgo.Channel) { c.PermissionOverwrites[1].Type = discordgo.PermissionOverwriteTypeRole }},
		{"candidate denied", func(c *discordgo.Channel) { c.PermissionOverwrites[1].Deny = discordgo.PermissionVoiceConnect }},
		{"extra visible role", func(c *discordgo.Channel) {
			c.PermissionOverwrites = append(c.PermissionOverwrites, &discordgo.PermissionOverwrite{ID: "strangers", Allow: discordgo.PermissionViewChannel})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, cfg := privateFixture()
			tt.modify(c)
			if privateChannel(c, cfg, "member") {
				t.Fatal("unsafe room accepted")
			}
		})
	}
}
func TestStaleSpeechCannotStartTurnAndBusyOnlyQueuesCurrentEpoch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	a := &session{ctx: ctx, cancel: cancel, epoch: 3, busy: true, ready: true, presenceReady: true, people: map[string]pipeline.Participant{}}
	s := &Service{engine: e, active: a}
	s.accept(a, workerEvent{Epoch: 2, Text: "stale answer"})
	if a.pending != nil {
		t.Fatal("stale recognition queued")
	}
	s.accept(a, workerEvent{Epoch: 3, Text: "current answer"})
	if a.pending == nil || a.pending.Text != "current answer" {
		t.Fatal("barge-in answer lost while playback completing")
	}
	a.pending = nil
	a.stopped = true
	s.accept(a, workerEvent{Epoch: 3, Text: "after stop"})
	if a.pending != nil {
		t.Fatal("stopped answer accepted")
	}
}
