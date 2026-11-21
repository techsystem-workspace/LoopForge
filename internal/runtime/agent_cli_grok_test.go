package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Lines recorded from `grok -p --output-format streaming-json` (grok 1.0.92),
// trimmed to the fields the translator reads.
var grokStreamFixture = []string{
	`{"type":"available_commands","tools":["run_terminal_command"],"commands":["compact"]}`,
	`{"type":"thought","data":"The user wants "}`,
	`{"type":"thought","data":"a file."}`,
	`{"type":"text","data":"I'll create "}`,
	`{"type":"text","data":"a.txt."}`,
	`{"type":"usage","usage":{"input_tokens":40509,"output_tokens":772}}`,
	`{"type":"tool_call","toolCallId":"call-1","title":"run_terminal_command","kind":"execute","status":"pending","toolName":"run_terminal_command","rawInput":{"command":"cat a.txt"},"content":[],"locations":[]}`,
	`{"type":"tool_call_update","toolCallId":"call-1","status":"in_progress","content":[{"type":"content","content":{"type":"text","text":"hi\n"}}]}`,
	`{"type":"tool_call_update","toolCallId":"call-1","status":"completed","content":[{"type":"content","content":{"type":"text","text":"hi\n"}}],"rawOutput":{"type":"Bash","output_for_prompt":"exit: 0\nhi\n","exit_code":0}}`,
	`{"type":"text","data":"DONE"}`,
	`{"type":"end","stopReason":"end_turn","sessionId":"01a1feae-724d-7cf0","usage":{"input_tokens":83174,"cache_read_input_tokens":44672,"cache_creation_input_tokens":0,"output_tokens":817,"reasoning_tokens":708},"num_turns":3}`,
}

func translateGrok(lines ...string) []map[string]any {
	translate := NewAgentCLITranslator(AgentCLIGrok)
	var out []map[string]any
	for _, line := range lines {
		for _, rendered := range translate(line) {
			var event map[string]any
			_ = json.Unmarshal([]byte(rendered), &event)
			out = append(out, event)
		}
	}
	return out
}

func TestGrokStreamTranslatesTurn(t *testing.T) {
	events := translateGrok(grokStreamFixture...)
	var blocks []string
	var result map[string]any
	for _, event := range events {
		switch event["type"] {
		case "assistant":
			message, _ := event["message"].(map[string]any)
			block, _ := sliceValue(message["content"])[0].(map[string]any)
			blocks = append(blocks, stringValue(block["type"])+":"+stringValue(block["text"])+stringValue(block["thinking"])+stringValue(block["name"]))
		case "user":
			message, _ := event["message"].(map[string]any)
			block, _ := sliceValue(message["content"])[0].(map[string]any)
			blocks = append(blocks, "tool_result:"+stringValue(block["content"]))
		case "result":
			result = event
		}
	}
	want := []string{"thinking:The user wants a file.", "text:I'll create a.txt.", "tool_use:run_terminal_command", "tool_result:exit: 0\nhi\n", "text:DONE"}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %q, want %q", blocks, want)
	}
	if result == nil || result["is_error"] != false || result["result"] != "DONE" || result["session_id"] != "01a1feae-724d-7cf0" {
		t.Fatalf("result = %v", result)
	}
	usage, _ := result["usage"].(map[string]any)
	if usage["input_tokens"] != float64(83174) || usage["output_tokens"] != float64(817) || usage["cache_read_input_tokens"] != float64(44672) {
		t.Fatalf("usage = %v", usage)
	}
}

func TestGrokStreamErrorEndsTurn(t *testing.T) {
	events := translateGrok(`{"type":"error","message":"--effort: unknown effort level 'bogus'"}`, `{"type":"end","stopReason":"end_turn"}`)
	var results []map[string]any
	for _, event := range events {
		if event["type"] == "result" {
			results = append(results, event)
		}
	}
	if len(results) != 1 || results[0]["is_error"] != true || !strings.Contains(stringValue(results[0]["result"]), "unknown effort") {
		t.Fatalf("results = %v", results)
	}
}

func TestGrokCommandArgv(t *testing.T) {
	spec := AgentCLICommand(AgentCLIGrok, AgentCLICommandOptions{Model: "grok-4.7", Effort: "high", Resume: "sess-1", Prompt: "do it"})
	want := []string{"grok", "--output-format", "streaming-json", "--permission-mode", "bypassPermissions", "--no-memory", "-m", "grok-4.7", "--reasoning-effort", "high", "-r", "sess-1", "-p", "do it"}
	if !reflect.DeepEqual(spec.Argv, want) {
		t.Fatalf("argv = %q", spec.Argv)
	}
	chat := AgentCLICommand(AgentCLIGrok, AgentCLICommandOptions{Prompt: "hi", Chat: true})
	if !strings.Contains(strings.Join(chat.Argv, " "), "--disallowed-tools "+grokChatDisallowedTools) {
		t.Fatalf("chat argv = %q", chat.Argv)
	}
}

func TestWriteAgentCLIMCPConfigGrokTOML(t *testing.T) {
	home := t.TempDir()
	entries := map[string]any{
		"agentflow-gate":    map[string]any{"command": ".agentflow/config/x/gate-mcp", "args": []string{}, "env": map[string]string{"AGENTFLOW_GATE_RESULT_PATH": `a "b".json`}},
		"agentflow-browser": map[string]any{"type": "http", "url": "http://127.0.0.1:8931/mcp", "headers": map[string]string{"Authorization": "Bearer t"}},
	}
	if err := WriteAgentCLIMCPConfig(AgentCLIGrok, HostAgentCLIHome(home), entries, AgentCLIMCPOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".grok", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `[mcp_servers."agentflow-browser"]
"enabled" = true
"url" = "http://127.0.0.1:8931/mcp"

[mcp_servers."agentflow-browser".headers]
"Authorization" = "Bearer t"

[mcp_servers."agentflow-gate"]
"args" = []
"command" = ".agentflow/config/x/gate-mcp"
"enabled" = true

[mcp_servers."agentflow-gate".env]
"AGENTFLOW_GATE_RESULT_PATH" = "a \"b\".json"

`
	if string(data) != want {
		t.Fatalf("config.toml =\n%s", data)
	}
}

func TestSameGrokLoginOnlyAcceptsRefreshOfSameAccount(t *testing.T) {
	entry := func(user, key, refresh string) string {
		return `{"https://auth.x.ai::c":{"key":"` + key + `","refresh_token":"` + refresh + `","expires_at":"t","user_id":"` + user + `","email":"e"}}`
	}
	original := []byte(entry("u1", "k1", "r1"))
	if !sameGrokLogin(original, []byte(entry("u1", "k2", "r2"))) {
		t.Fatal("token refresh of the same account must be accepted")
	}
	if sameGrokLogin(original, []byte(entry("u2", "k2", "r2"))) {
		t.Fatal("another account must be rejected")
	}
	if sameGrokLogin(original, []byte(`{"https://auth.x.ai::c":{"key":"k2","refresh_token":"r2","expires_at":"t","user_id":"u1","email":"e"},"other":{"refresh_token":"x","user_id":"u1"}}`)) {
		t.Fatal("an added entry must be rejected")
	}
}

func TestGrokStreamNamesMCPToolCalls(t *testing.T) {
	events := translateGrok(`{"type":"tool_call","toolCallId":"c2","toolName":"use_tool","rawInput":{"tool_name":"agentflow-gate__submit_gate_result","tool_input":{"decision":"pass","reason":"ok"}}}`)
	for _, event := range events {
		if event["type"] != "assistant" {
			continue
		}
		message, _ := event["message"].(map[string]any)
		block, _ := sliceValue(message["content"])[0].(map[string]any)
		input, _ := block["input"].(map[string]any)
		if block["name"] != "mcp__agentflow-gate__submit_gate_result" || input["decision"] != "pass" {
			t.Fatalf("tool_use = %v", block)
		}
		return
	}
	t.Fatal("no tool_use emitted")
}

func TestGrokConversationApprovalHook(t *testing.T) {
	root := t.TempDir()
	if _, err := InstallConversationApproval(root, AgentCLIGrok, CommandSpec{Argv: []string{"grok", "-p", "task"}, Provider: AgentCLIGrok}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "home", ".grok", "hooks", "loopforge-approval.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"PreToolUse"`) || !strings.Contains(string(data), "native-approval.py") {
		t.Fatalf("hook = %s", data)
	}
}
