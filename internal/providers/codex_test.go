package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexEventsKeepTokensSeparateFromInvoice(t *testing.T) {
	var content string
	var usage Usage
	failed := false
	for _, line := range []string{
		`{"type":"item.completed","item":{"type":"error","message":"nonfatal setup warning"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"{\"message\":\"What did you build?\",\"completed\":false}"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":80,"reasoning_output_tokens":30}}`,
	} {
		if parseCodexEvent([]byte(line), &content, &usage, &failed) {
			t.Fatal("ordinary response treated as a tool")
		}
	}
	if failed || !json.Valid([]byte(content)) || !usage.Reported || usage.TotalTokens != 1080 || usage.CachedTokens != 200 || usage.ReasoningTokens != 30 {
		t.Fatalf("wrong event conversion: %+v", usage)
	}
}

func TestCodexRejectsToolEventsAndInvalidTokenCounts(t *testing.T) {
	for _, kind := range []string{"command_execution", "mcp_tool_call", "file_change", "web_search"} {
		var content string
		var usage Usage
		failed := false
		if !parseCodexEvent([]byte(`{"type":"item.started","item":{"type":"`+kind+`"}}`), &content, &usage, &failed) {
			t.Fatal("tool event was not stopped", kind)
		}
	}
	var content string
	var usage Usage
	failed := false
	parseCodexEvent([]byte(`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":20,"cached_input_tokens":101}}`), &content, &usage, &failed)
	if usage.Reported {
		t.Fatal("invalid cached subset accepted")
	}
	parseCodexEvent([]byte(`{"type":"turn.failed","error":{"message":"secret"}}`), &content, &usage, &failed)
	if !failed || strings.Contains(content, "secret") {
		t.Fatal("unsafe failure conversion")
	}
}

func TestCodexEnvironmentAndExecutableAreNotBrowserCommands(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "private-test-key")
	t.Setenv("CODEX_API_KEY", "private-test-key")
	t.Setenv("TEAMRISE_CODEX_PATH", "codex.exe --run-anything")
	if _, err := codexExecutable(); err == nil {
		t.Fatal("relative command accepted")
	}
	for _, entry := range codexEnvironment() {
		if strings.Contains(entry, "private-test-key") {
			t.Fatal("API credential inherited by subscription process")
		}
	}
	args := strings.Join(codexOptions(), " ")
	for _, flag := range []string{`forced_login_method="chatgpt"`, "features.shell_tool=false", "features.hooks=false", "features.apps=false", "features.multi_agent=false", `web_search="disabled"`} {
		if !strings.Contains(args, flag) {
			t.Fatal("missing constraint", flag)
		}
	}
}

func TestOnlyCompletedTurnAllowsEarlyClientExit(t *testing.T) {
	for _, line := range []string{`{"type":"turn.failed"}`, `{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}`, `invalid`} {
		if completedCodexTurn([]byte(line)) {
			t.Fatal("unfinished response treated as complete")
		}
	}
	if !completedCodexTurn([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":3}}`)) {
		t.Fatal("completed response ignored")
	}
}
