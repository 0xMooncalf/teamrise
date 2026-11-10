package voice

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type workerEvent struct {
	generation  uint64
	Event       string  `json:"event"`
	ID          uint64  `json:"id"`
	SessionID   string  `json:"session_id"`
	Error       string  `json:"error"`
	Epoch       int     `json:"epoch"`
	Text        string  `json:"text"`
	Seconds     float64 `json:"seconds"`
	Characters  int     `json:"characters"`
	DurationMS  int64   `json:"duration_ms"`
	Interrupted bool    `json:"interrupted"`
	Connected   bool    `json:"connected"`
	Payload     struct {
		Op   int `json:"op"`
		Data struct {
			GuildID   string  `json:"guild_id"`
			ChannelID *string `json:"channel_id"`
		} `json:"d"`
	} `json:"payload"`
}
type bridge struct {
	mu        sync.Mutex
	write     sync.Mutex
	closeOnce sync.Once
	cmd       *exec.Cmd
	in        io.WriteCloser
	pending   map[uint64]chan workerEvent
	next      uint64
	ready     chan error
	done      chan struct{}
	onEvent   func(workerEvent)
}

func voiceRoot() string {
	if root := os.Getenv("TEAMRISE_VOICE_ROOT"); filepath.IsAbs(root) {
		return root
	}
	exe, _ := os.Executable()
	root := filepath.Join(filepath.Dir(exe), "voice")
	if _, err := os.Stat(filepath.Join(root, "worker.mjs")); err == nil {
		return root
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "voice")
}
func startBridge(ctx context.Context, onEvent func(workerEvent)) (*bridge, error) {
	root := voiceRoot()
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("Node 22.12+ is required for Discord voice")
	}
	if _, err = os.Stat(filepath.Join(root, "models", "manifest.json")); err != nil {
		return nil, errors.New("run Install-Voice.ps1 to install local speech models")
	}
	cmd := exec.Command(node, filepath.Join(root, "worker.mjs"))
	cmd.Dir = root
	cmd.Stderr = io.Discard
	hideWindow(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	b := &bridge{cmd: cmd, in: in, pending: map[uint64]chan workerEvent{}, ready: make(chan error, 1), done: make(chan struct{}), onEvent: onEvent}
	if err = cmd.Start(); err != nil {
		in.Close()
		return nil, errors.New("voice worker could not start")
	}
	go func() {
		defer close(b.done)
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var event workerEvent
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			if event.Event == "ready" {
				select {
				case b.ready <- nil:
				default:
				}
				continue
			}
			if event.Event == "fatal" {
				select {
				case b.ready <- errors.New(event.Error):
				default:
				}
			}
			if event.Event == "ack" {
				b.mu.Lock()
				ch := b.pending[event.ID]
				delete(b.pending, event.ID)
				b.mu.Unlock()
				if ch != nil {
					ch <- event
				}
				continue
			}
			if b.onEvent != nil {
				b.onEvent(event)
			}
		}
		_ = cmd.Wait()
		select {
		case b.ready <- errors.New("voice worker stopped; check the local speech installation"):
		default:
		}
		if b.onEvent != nil {
			b.onEvent(workerEvent{Event: "fatal", Error: "voice worker stopped"})
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			b.Close()
		case <-b.done:
		}
	}()
	return b, nil
}
func (b *bridge) Send(value any) error {
	return b.send(context.Background(), value)
}
func (b *bridge) send(ctx context.Context, value any) error {
	b.write.Lock()
	defer b.write.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return errors.New("voice worker is offline")
	default:
	}
	return json.NewEncoder(b.in).Encode(value)
}
func (b *bridge) Call(ctx context.Context, typ string, params map[string]any) (workerEvent, error) {
	b.mu.Lock()
	b.next++
	id := b.next
	ch := make(chan workerEvent, 1)
	b.pending[id] = ch
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, id); b.mu.Unlock() }()
	params["type"] = typ
	params["id"] = id
	sent := make(chan error, 1)
	go func() { sent <- b.send(ctx, params) }()
	select {
	case err := <-sent:
		if err != nil {
			return workerEvent{}, err
		}
	case <-ctx.Done():
		return workerEvent{}, ctx.Err()
	case <-b.done:
		return workerEvent{}, errors.New("voice worker stopped")
	}
	select {
	case <-ctx.Done():
		return workerEvent{}, ctx.Err()
	case <-b.done:
		return workerEvent{}, errors.New("voice worker stopped")
	case event := <-ch:
		if event.Error != "" {
			return event, errors.New(event.Error)
		}
		return event, nil
	}
}
func (b *bridge) Close() {
	b.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		go func() { _ = b.send(ctx, map[string]string{"type": "shutdown"}) }()
		select {
		case <-b.done:
		case <-ctx.Done():
			_ = b.cmd.Process.Kill()
		}
		_ = b.in.Close()
	})
}
