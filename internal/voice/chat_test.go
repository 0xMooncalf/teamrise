package voice

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"testing"

	"github.com/bwmarrin/discordgo"

	"ladder/internal/connections"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
)

func TestPracticalScreenshotPostsOnceToVoiceRoomChat(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	asset, err := e.SaveScreenshot(pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	b := &testBackend{}
	store, _ := connections.New("")
	s := New(context.Background(), e, store, connectedTestManager(t, b))
	defer s.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	a := &session{ctx: ctx, cancel: cancel, channel: "private-voice-room", member: "candidate", ready: true,
		presenceReady: true, people: map[string]pipeline.Participant{"candidate": {ID: "candidate"}}}
	s.active = a
	message := pipeline.Message{Text: "Explain the screenshot.", Image: asset, Channel: "chat"}
	s.writtenQuestion(a, message, 2)
	s.writtenQuestion(a, message, 2)
	if b.chatChannel != a.channel || b.chatPosts != 1 || len(b.chatFiles) != 1 || a.state != "awaiting_chat" {
		t.Fatalf("expected one attachment in the voice room, got %d posts and %d files", b.chatPosts, len(b.chatFiles))
	}
	got, err := io.ReadAll(b.chatFiles[0].Reader)
	if err != nil || !bytes.Equal(got, pngData.Bytes()) || b.chatFiles[0].ContentType != "image/png" {
		t.Fatal("Discord transport did not receive the screenshot")
	}
}

func TestReviewerCallIsBoundToCandidateAndRoomAndDeduplicated(t *testing.T) {
	e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	const guild, room, candidate, reviewer = "22345678901234567", "32345678901234567", "42345678901234567", "52345678901234567"
	id, err := e.CreateInterview("Same nickname", e.Snapshot().Profiles[0].ID, "", &discordrooms.Plan{GuildID: guild, MemberID: candidate, ChannelID: room})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := connections.New("")
	if err := store.SaveDiscord(connections.Discord{GuildID: guild, ReviewerRoleID: reviewer}); err != nil {
		t.Fatal(err)
	}
	b := &testBackend{}
	s := New(context.Background(), e, store, connectedTestManager(t, b))
	defer s.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	s.active = &session{id: id, guild: guild, channel: room, member: candidate, ctx: ctx, cancel: cancel, people: map[string]pipeline.Participant{candidate: {ID: candidate, Kind: "candidate"}}}
	call := func(member, channel string) {
		s.interaction(&discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand, GuildID: guild, ChannelID: channel,
			Member: &discordgo.Member{Nick: "Same nickname", User: &discordgo.User{ID: member}},
			Data:   discordgo.ApplicationCommandInteractionData{Name: "interview-help"},
		}})
	}
	call("another-member", room)
	call(candidate, "another-room")
	if len(e.Snapshot().Requests) != 0 || b.chatPosts != 0 {
		t.Fatal("another member or room triggered a reviewer notification")
	}
	call(candidate, room)
	call(candidate, room)
	if requests := e.Snapshot().Requests; len(requests) != 1 || !requests[0].RequestedByCandidate || b.chatPosts != 1 {
		t.Fatalf("expected one dashboard request and Discord post, got %d and %d", len(requests), b.chatPosts)
	}
}
