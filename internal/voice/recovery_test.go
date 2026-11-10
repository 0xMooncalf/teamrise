package voice

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ladder/internal/connections"
	"ladder/internal/discordrooms"
	"ladder/internal/pipeline"
	"ladder/internal/providers"
)

type voiceCommandSink func([]byte) (int, error)

func (w voiceCommandSink) Write(p []byte) (int, error) { return w(p) }
func (w voiceCommandSink) Close() error                { return nil }

func TestWatchdogKeepsTheCurrentTurnAndSkipsWrittenTasks(t *testing.T) {
	for _, tt := range []struct {
		name, channel string
		silence       time.Duration
		connected     bool
		want          []string
	}{
		{"silence expires", "voice", 5 * time.Minute, true, []string{"health", "chime", "leave", "join", "speak"}},
		{"transport lost", "voice", time.Minute, false, []string{"health", "chime", "leave", "join", "chime", "speak"}},
		{"still listening", "voice", 4 * time.Minute, true, []string{"health"}},
		{"written exercise", "chat", 20 * time.Minute, false, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			e, err := pipeline.New(filepath.Join(t.TempDir(), "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			profile := e.Snapshot().Profiles[0]
			const question = "Explain the next step you would take."
			profile.Cards = []pipeline.InterviewCard{
				{ID: "q1", Channel: "voice", Prompt: "How would you start the diagnosis?"},
				{ID: "q2", Channel: tt.channel, Prompt: question},
			}
			if err := e.SaveProfile(profile); err != nil {
				t.Fatal(err)
			}
			id, err := e.CreateInterview("Fixture candidate", profile.ID, "", &discordrooms.Plan{
				GuildID: "guild", MemberID: "member", ChannelID: "room",
			})
			if err != nil {
				t.Fatal(err)
			}
			model := providers.Compatible{Voice: true, Config: connections.AI{Model: "fixture", MaxQuestions: 2}}
			for _, turn := range []struct{ action, answer string }{{"start", ""}, {"answer", "I would inspect the logs."}} {
				c, _ := e.Candidate(id)
				if err := e.RunAI(ctx, id, turn.action, turn.answer, c.Live.Version, model); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := e.Candidate(id)
			store, _ := connections.New("")
			manager := connectedTestManager(t, &testBackend{candidateChannel: "room"})
			a := &session{id: id, guild: "guild", channel: "room", member: "member", ctx: ctx, cancel: cancel,
				people: map[string]pipeline.Participant{"member": {ID: "member"}},
				ready:  tt.connected, presenceReady: true, epoch: 2, intro: true, limitAt: before.Live.Deadline,
				state: "listening", silenceSince: time.Now().Add(-tt.silence)}
			if tt.channel == "chat" {
				a.state = "awaiting_chat"
			}
			b := &bridge{pending: map[uint64]chan workerEvent{}, done: make(chan struct{})}
			commands := make(chan map[string]any, 12)
			connected := tt.connected
			b.in = voiceCommandSink(func(data []byte) (int, error) {
				var cmd map[string]any
				if err := json.Unmarshal(data, &cmd); err != nil {
					return 0, err
				}
				commands <- cmd
				response := workerEvent{Connected: connected}
				switch cmd["type"] {
				case "chime":
					if !connected {
						response.Error = "voice is not connected"
					}
				case "join":
					connected = true
				}
				b.mu.Lock()
				ack := b.pending[uint64(cmd["id"].(float64))]
				b.mu.Unlock()
				if ack == nil {
					return 0, errors.New("command has no acknowledgement")
				}
				ack <- response
				return len(data), nil
			})
			s := &Service{ctx: ctx, active: a, bridge: b, engine: e, store: store, bot: manager}
			s.checkVoice(a, 2)
			for _, want := range tt.want {
				select {
				case cmd := <-commands:
					if cmd["type"] != want {
						t.Fatalf("wanted %q, got %v", want, cmd["type"])
					}
					if want == "join" && (cmd["channel_id"] != "room" || cmd["member_id"] != "member") {
						t.Fatal("recovery changed room or candidate")
					}
					if want == "speak" && (cmd["text"] != question || cmd["listen"] != true) {
						t.Fatal("recovery did not repeat the pending question")
					}
				case <-ctx.Done():
					t.Fatalf("missing command %q", want)
				}
			}
			if len(tt.want) > 1 {
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					s.mu.Lock()
					listening := a.state == "listening" && !a.busy && !a.recovering && a.epoch > 2
					s.mu.Unlock()
					if listening {
						break
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatal("voice did not resume after reconnection")
					}
				}
			}
			if len(commands) != 0 {
				t.Fatal("unexpected voice command")
			}
			after, _ := e.Candidate(id)
			if !reflect.DeepEqual(before.Transcript, after.Transcript) ||
				before.Live.Version != after.Live.Version || !before.Live.Deadline.Equal(*after.Live.Deadline) {
				t.Fatal("recovery changed answers, question, or interview deadline")
			}
		})
	}
}
