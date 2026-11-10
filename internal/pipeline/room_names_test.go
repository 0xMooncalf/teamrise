package pipeline

import (
	"path/filepath"
	"testing"
)

func TestReadableVoiceRoomNumbersSurviveRestartAndDemoReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.NextVoiceRoomName()
	if err != nil || first != "interview #1" {
		t.Fatalf("first room: %q %v", first, err)
	}
	_ = e.Control("reset")
	second, err := e.NextVoiceRoomName()
	if err != nil || second != "interview #2" {
		t.Fatal("demo reset reused a voice room number")
	}
	e.Close()
	e, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	third, err := e.NextVoiceRoomName()
	if err != nil || third != "interview #3" {
		t.Fatal("restart reused a voice room number")
	}
}
