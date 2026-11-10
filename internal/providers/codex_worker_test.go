package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEAMRISE_FIXTURE_RPC") == "1" && len(os.Args) > 1 && os.Args[1] == "app-server" {
		fixtureRPC()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fixtureRPC() {
	disabled := false
	for _, a := range os.Args {
		if a == "mcp_servers.fixture.enabled=false" {
			disabled = true
		}
	}
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	thread := 0
	for scanner.Scan() {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			return
		}
		if path := os.Getenv("TEAMRISE_FIXTURE_LOG"); path != "" {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if f != nil {
				f.Write(append(append([]byte{}, scanner.Bytes()...), '\n'))
				f.Close()
			}
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialized":
			continue
		case "mcpServerStatus/list":
			tools := map[string]any{}
			if !disabled || os.Getenv("TEAMRISE_FIXTURE_UNSAFE") == "1" {
				tools["fixture_tool"] = map[string]any{}
			}
			result = map[string]any{"data": []any{map[string]any{"name": "fixture", "tools": tools, "resources": []any{}, "resourceTemplates": []any{}}}, "nextCursor": nil}
		case "thread/start":
			thread++
			result = map[string]any{"thread": map[string]string{"id": strings.Repeat("t", thread)}}
		case "turn/start":
			var p struct {
				ThreadID string `json:"threadId"`
			}
			json.Unmarshal(req.Params, &p)
			if os.Getenv("TEAMRISE_FIXTURE_WAIT") == "1" {
				continue
			}
			kind := "agentMessage"
			if os.Getenv("TEAMRISE_FIXTURE_TOOL") == "1" {
				kind = "commandExecution"
			}
			// Notifications intentionally precede the RPC acknowledgement.
			enc.Encode(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": p.ThreadID, "item": map[string]string{"type": kind, "text": `{"message":"What did you build?","completed":false}`}}})
			enc.Encode(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"threadId": p.ThreadID, "tokenUsage": map[string]any{"last": map[string]int{"inputTokens": 100, "outputTokens": 20, "cachedInputTokens": 50, "reasoningOutputTokens": 5}}}})
			enc.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": p.ThreadID, "turn": map[string]string{"status": "completed"}}})
		}
		enc.Encode(map[string]any{"id": req.ID, "result": result})
	}
}

func fixtureWorker(t *testing.T) (*codexWorker, context.Context) {
	t.Helper()
	CloseLocalCodex()
	path, _ := os.Executable()
	t.Setenv("TEAMRISE_CODEX_PATH", path)
	t.Setenv("TEAMRISE_FIXTURE_RPC", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(CloseLocalCodex)
	w, err := prepareCodexWorker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return w, ctx
}

func TestWarmCodexReusesThreadSendsOnlyNewAnswerAndPreservesEarlyEvents(t *testing.T) {
	log := filepath.Join(t.TempDir(), "rpc.jsonl")
	t.Setenv("TEAMRISE_FIXTURE_LOG", log)
	w, ctx := fixtureWorker(t)
	var usage Usage
	p := Compatible{SessionID: "interview-a", OnUsage: func(u Usage) error { usage = u; return nil }}
	messages := []ChatMessage{{"system", "Interview profile."}}
	content, err := p.rpcInterview(ctx, w, messages, true)
	if err != nil || content == "" {
		t.Fatal(err)
	}
	messages = append(messages, ChatMessage{"assistant", "What did you build?"}, ChatMessage{"user", "I built a queue."})
	if _, err = p.rpcInterview(ctx, w, messages, true); err != nil {
		t.Fatal(err)
	}
	if !usage.Reported || usage.TotalTokens != 120 || usage.CachedTokens != 50 || usage.Transport != "codex-app-server" {
		t.Fatalf("usage lost: %+v", usage)
	}
	raw, _ := os.ReadFile(log)
	starts, turns := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var r struct {
			Method string `json:"method"`
			Params struct {
				Input []struct {
					Text string `json:"text"`
				} `json:"input"`
			} `json:"params"`
		}
		json.Unmarshal([]byte(line), &r)
		if r.Method == "thread/start" {
			starts++
		}
		if r.Method == "turn/start" {
			turns++
			if turns == 2 && (!strings.Contains(r.Params.Input[0].Text, "I built a queue.") || strings.Contains(r.Params.Input[0].Text, "Interview profile.") || strings.Contains(r.Params.Input[0].Text, "What did you build?")) {
				t.Fatal("warm turn resubmitted history")
			}
		}
	}
	if starts != 1 || turns != 2 {
		t.Fatalf("not reused: starts=%d turns=%d", starts, turns)
	}
}

func TestCodexWorkerRejectsExposedToolsBeforeInference(t *testing.T) {
	CloseLocalCodex()
	t.Cleanup(CloseLocalCodex)
	path, _ := os.Executable()
	t.Setenv("TEAMRISE_CODEX_PATH", path)
	t.Setenv("TEAMRISE_FIXTURE_RPC", "1")
	t.Setenv("TEAMRISE_FIXTURE_UNSAFE", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := prepareCodexWorker(ctx); err == nil {
		t.Fatal("exposed MCP capability accepted")
	}
}

func TestCodexWorkerDiscardsToolResponseAndResetsAfterCancellation(t *testing.T) {
	for _, mode := range []string{"TOOL", "WAIT"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TEAMRISE_FIXTURE_"+mode, "1")
			w, ctx := fixtureWorker(t)
			if mode == "WAIT" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			p := Compatible{SessionID: "interview-b"}
			if content, err := p.rpcInterview(ctx, w, []ChatMessage{{"system", "Interview"}}, true); err == nil || content != "" {
				t.Fatal("unsafe or incomplete output accepted")
			}
			if sharedCodexWorker != nil {
				t.Fatal("failed worker retained")
			}
		})
	}
}
