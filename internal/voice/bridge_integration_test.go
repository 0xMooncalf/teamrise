package voice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledVoiceWorkerStartsAndShutsDown(t *testing.T) {
	if os.Getenv("TEAMRISE_TEST_VOICE") != "1" {
		t.Skip("opt-in installed speech runtime test")
	}
	root, err := filepath.Abs("../../voice")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEAMRISE_VOICE_ROOT", root)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b, err := startBridge(ctx, func(workerEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	select {
	case err = <-b.ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("speech startup timed out")
	}
	health, err := b.Call(ctx, "health", map[string]any{"session_id": "not-active"})
	if err != nil || health.Connected {
		t.Fatalf("healthy local speech runtime should answer even without a Discord room: %+v, %v", health, err)
	}
	if _, err = b.Call(ctx, "speak", map[string]any{"session_id": "not-active", "text": "Test", "epoch": 1}); err == nil {
		t.Fatal("speech accepted without active Discord session")
	}
	b.Close()
	select {
	case <-b.done:
	case <-ctx.Done():
		t.Fatal("voice child did not stop")
	}
}
