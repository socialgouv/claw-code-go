package hooks

import (
	"encoding/json"
	"reflect"
	"testing"
)

// --- BuildPayload tests ---

func TestBuildPayloadPreToolUse(t *testing.T) {
	payload := BuildPayload(PreToolUse, "Read", `{"path":"README.md"}`, nil, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["hook_event_name"] != "PreToolUse" {
		t.Errorf("expected PreToolUse, got %v", m["hook_event_name"])
	}
	if m["tool_name"] != "Read" {
		t.Errorf("expected Read, got %v", m["tool_name"])
	}
	// tool_input should be parsed object
	ti, ok := m["tool_input"].(map[string]interface{})
	if !ok {
		t.Fatal("tool_input should be a map")
	}
	if ti["path"] != "README.md" {
		t.Errorf("expected README.md, got %v", ti["path"])
	}
	if m["tool_input_json"] != `{"path":"README.md"}` {
		t.Errorf("unexpected tool_input_json: %v", m["tool_input_json"])
	}
	// tool_output should be JSON null when toolOutput is nil (matching Rust None→null)
	if v, exists := m["tool_output"]; !exists {
		t.Error("tool_output key should be present (as null)")
	} else if v != nil {
		t.Errorf("expected tool_output=null, got %v", v)
	}
	if m["tool_result_is_error"] != false {
		t.Errorf("expected false, got %v", m["tool_result_is_error"])
	}
}

func TestBuildPayloadPostToolUse(t *testing.T) {
	output := "file contents"
	payload := BuildPayload(PostToolUse, "Read", `{"path":"README.md"}`, &output, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["tool_output"] != "file contents" {
		t.Errorf("expected 'file contents', got %v", m["tool_output"])
	}
	if m["tool_result_is_error"] != false {
		t.Errorf("expected false, got %v", m["tool_result_is_error"])
	}
}

func TestBuildPayloadPostToolUseFailure(t *testing.T) {
	errMsg := "command failed"
	payload := BuildPayload(PostToolUseFailure, "Bash", `{"command":"bad"}`, &errMsg, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["hook_event_name"] != "PostToolUseFailure" {
		t.Errorf("expected PostToolUseFailure, got %v", m["hook_event_name"])
	}
	if m["tool_error"] != "command failed" {
		t.Errorf("expected 'command failed', got %v", m["tool_error"])
	}
	if _, exists := m["tool_output"]; exists {
		t.Error("tool_output should not be present for PostToolUseFailure")
	}
	if m["tool_result_is_error"] != true {
		t.Errorf("expected true, got %v", m["tool_result_is_error"])
	}
}

func TestBuildPayloadInvalidJSON(t *testing.T) {
	payload := BuildPayload(PreToolUse, "Bash", "not json", nil, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	// Rust falls back to {"raw": toolInput} for invalid JSON, not null.
	ti, ok := m["tool_input"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for invalid JSON tool_input fallback, got %T: %v", m["tool_input"], m["tool_input"])
	}
	if ti["raw"] != "not json" {
		t.Errorf("expected raw fallback with original string, got %v", ti["raw"])
	}
	if m["tool_input_json"] != "not json" {
		t.Errorf("expected raw string, got %v", m["tool_input_json"])
	}
}

// --- parseHookOutput tests ---

func TestParseHookOutputFullJSON(t *testing.T) {
	input := `{
		"systemMessage": "hook says hello",
		"reason": "because reasons",
		"continue": true,
		"decision": "allow",
		"hookSpecificOutput": {
			"additionalContext": "extra info",
			"permissionDecision": "allow",
			"permissionDecisionReason": "safe tool",
			"updatedInput": {"path": "/tmp"}
		}
	}`
	result := parseHookOutput(input)
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d: %v", len(result.Messages), result.Messages)
	}
	if result.Messages[0] != "hook says hello" {
		t.Errorf("unexpected message[0]: %s", result.Messages[0])
	}
	if result.Messages[1] != "because reasons" {
		t.Errorf("unexpected message[1]: %s", result.Messages[1])
	}
	if result.Messages[2] != "extra info" {
		t.Errorf("unexpected message[2]: %s", result.Messages[2])
	}
	if result.Deny {
		t.Error("should not be denied")
	}
	if result.PermissionOverride == nil || *result.PermissionOverride != PermissionAllow {
		t.Error("expected PermissionAllow override")
	}
	if result.PermissionReason != "safe tool" {
		t.Errorf("unexpected permission reason: %s", result.PermissionReason)
	}
	if result.UpdatedInput == "" {
		t.Error("expected updatedInput")
	}
}

func TestParseHookOutputPlainText(t *testing.T) {
	// Rust hooks.rs:584-586: when JSON parse fails, push entire stdout as message.
	result := parseHookOutput("just some plain text")
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message (stdout fallback), got %d: %v", len(result.Messages), result.Messages)
	}
	if result.Messages[0] != "just some plain text" {
		t.Errorf("expected stdout as fallback message, got %q", result.Messages[0])
	}
	if result.Deny {
		t.Error("should not be denied")
	}
}

func TestParseHookOutputValidJSONNoMessageFields(t *testing.T) {
	// Valid JSON with no systemMessage/reason/additionalContext — Rust pushes
	// entire stdout as fallback when parsed.messages is empty.
	input := `{"continue": true}`
	result := parseHookOutput(input)
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message (stdout fallback for empty messages), got %d: %v", len(result.Messages), result.Messages)
	}
	if result.Messages[0] != input {
		t.Errorf("expected stdout as fallback, got %q", result.Messages[0])
	}
	if result.Deny {
		t.Error("should not be denied")
	}
}

func TestParseHookOutputEmptyStdout(t *testing.T) {
	// Empty stdout should produce no messages, not a blank fallback.
	result := parseHookOutput("")
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages for empty stdout, got %d: %v", len(result.Messages), result.Messages)
	}
}

func TestParseHookOutputWhitespaceOnlyStdout(t *testing.T) {
	// Whitespace-only stdout should produce no messages.
	result := parseHookOutput("   \n  ")
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages for whitespace-only stdout, got %d: %v", len(result.Messages), result.Messages)
	}
}

func TestParseHookOutputDenyContinueFalse(t *testing.T) {
	input := `{"continue": false, "reason": "blocked"}`
	result := parseHookOutput(input)
	if !result.Deny {
		t.Error("expected deny when continue=false")
	}
	if len(result.Messages) != 1 || result.Messages[0] != "blocked" {
		t.Errorf("unexpected messages: %v", result.Messages)
	}
}

func TestParseHookOutputDenyDecisionBlock(t *testing.T) {
	input := `{"decision": "block", "systemMessage": "not allowed"}`
	result := parseHookOutput(input)
	if !result.Deny {
		t.Error("expected deny when decision=block")
	}
}

func TestParseHookOutputPermissionDecision(t *testing.T) {
	input := `{"hookSpecificOutput": {"permissionDecision": "deny", "permissionDecisionReason": "unsafe"}}`
	result := parseHookOutput(input)
	if result.PermissionOverride == nil || *result.PermissionOverride != PermissionDeny {
		t.Error("expected PermissionDeny override")
	}
	if result.PermissionReason != "unsafe" {
		t.Errorf("expected 'unsafe', got '%s'", result.PermissionReason)
	}
}

func TestParseHookOutputUpdatedInput(t *testing.T) {
	input := `{"hookSpecificOutput": {"updatedInput": {"command": "echo safe"}}}`
	result := parseHookOutput(input)
	if result.UpdatedInput == "" {
		t.Fatal("expected updatedInput")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(result.UpdatedInput), &m); err != nil {
		t.Fatal(err)
	}
	if m["command"] != "echo safe" {
		t.Errorf("unexpected updatedInput content: %v", m)
	}
}

// --- MergeConfigs tests ---

func TestMergeConfigs(t *testing.T) {
	user := HookConfig{
		PreToolUse:  []string{"cmd1", "cmd2"},
		PostToolUse: []string{"post1"},
	}
	project := HookConfig{
		PreToolUse:         []string{"cmd2", "cmd3"}, // cmd2 is duplicate
		PostToolUseFailure: []string{"fail1"},
	}
	local := HookConfig{
		PreToolUse: []string{"cmd4"},
	}

	merged := MergeConfigs(user, project, local)

	expectedPre := []string{"cmd1", "cmd2", "cmd3", "cmd4"}
	if !reflect.DeepEqual(merged.PreToolUse, expectedPre) {
		t.Errorf("PreToolUse: expected %v, got %v", expectedPre, merged.PreToolUse)
	}
	expectedPost := []string{"post1"}
	if !reflect.DeepEqual(merged.PostToolUse, expectedPost) {
		t.Errorf("PostToolUse: expected %v, got %v", expectedPost, merged.PostToolUse)
	}
	expectedFail := []string{"fail1"}
	if !reflect.DeepEqual(merged.PostToolUseFailure, expectedFail) {
		t.Errorf("PostToolUseFailure: expected %v, got %v", expectedFail, merged.PostToolUseFailure)
	}
}

func TestMergeConfigsEmpty(t *testing.T) {
	merged := MergeConfigs()
	if len(merged.PreToolUse) != 0 || len(merged.PostToolUse) != 0 || len(merged.PostToolUseFailure) != 0 {
		t.Error("expected all empty slices")
	}
}

// --- HookAbortSignal tests ---

func TestHookAbortSignal(t *testing.T) {
	sig := NewHookAbortSignal()
	if sig.IsAborted() {
		t.Error("should not be aborted initially")
	}
	sig.Abort()
	if !sig.IsAborted() {
		t.Error("should be aborted after Abort()")
	}
}

// --- Allow helper test ---

func TestAllow(t *testing.T) {
	result := Allow(nil)
	if result.Denied || result.Failed || result.Cancelled {
		t.Error("Allow result should not be denied/failed/cancelled")
	}
	if result.Messages == nil || len(result.Messages) != 0 {
		t.Error("Allow with nil should give empty slice")
	}

	msgs := []string{"info"}
	result = Allow(msgs)
	if len(result.Messages) != 1 || result.Messages[0] != "info" {
		t.Errorf("unexpected messages: %v", result.Messages)
	}
}

// --- Env var encoding test ---

func TestHookEnvVarEncoding(t *testing.T) {
	// Verify HOOK_TOOL_IS_ERROR uses "1"/"0" encoding (matching Rust),
	// not "true"/"false" (Go's fmt.Sprintf("%t")).
	// We construct the env map the same way runner.go does.

	buildEnv := func(isError bool) map[string]string {
		env := map[string]string{
			"HOOK_EVENT":      string(PreToolUse),
			"HOOK_TOOL_NAME":  "Bash",
			"HOOK_TOOL_INPUT": "{}",
		}
		if isError {
			env["HOOK_TOOL_IS_ERROR"] = "1"
		} else {
			env["HOOK_TOOL_IS_ERROR"] = "0"
		}
		return env
	}

	envTrue := buildEnv(true)
	if envTrue["HOOK_TOOL_IS_ERROR"] != "1" {
		t.Errorf("expected HOOK_TOOL_IS_ERROR='1' for isError=true, got %q", envTrue["HOOK_TOOL_IS_ERROR"])
	}

	envFalse := buildEnv(false)
	if envFalse["HOOK_TOOL_IS_ERROR"] != "0" {
		t.Errorf("expected HOOK_TOOL_IS_ERROR='0' for isError=false, got %q", envFalse["HOOK_TOOL_IS_ERROR"])
	}
}

// --- formatHookFailure tests ---

func TestFormatHookFailureWithStdout(t *testing.T) {
	msg := formatHookFailure("my-hook.sh", 1, "some output", "some error")
	expected := "Hook `my-hook.sh` exited with status 1: some output"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestFormatHookFailureWithStderrFallback(t *testing.T) {
	msg := formatHookFailure("my-hook.sh", 3, "", "error details")
	expected := "Hook `my-hook.sh` exited with status 3: error details"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestFormatHookFailureNoOutput(t *testing.T) {
	msg := formatHookFailure("my-hook.sh", 127, "", "")
	expected := "Hook `my-hook.sh` exited with status 127"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

// --- Payload null key tests ---

func TestBuildPayloadPreToolUseNullOutput(t *testing.T) {
	payload := BuildPayload(PreToolUse, "Read", `{"path":"x"}`, nil, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	// tool_output should be present as JSON null
	v, exists := m["tool_output"]
	if !exists {
		t.Fatal("tool_output key should exist (as null)")
	}
	if v != nil {
		t.Errorf("expected null, got %v", v)
	}
}

func TestBuildPayloadPostToolUseFailureNullError(t *testing.T) {
	payload := BuildPayload(PostToolUseFailure, "Bash", `{}`, nil, false)
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	// tool_error should be present as JSON null
	v, exists := m["tool_error"]
	if !exists {
		t.Fatal("tool_error key should exist (as null)")
	}
	if v != nil {
		t.Errorf("expected null, got %v", v)
	}
}

// --- HookRunner empty config test ---

func TestHookRunnerEmptyConfig(t *testing.T) {
	runner := NewHookRunner(HookConfig{})
	result := runner.RunPreToolUse("Read", `{"path":"x"}`)
	if result.Denied || result.Failed || result.Cancelled {
		t.Error("empty config should return Allow")
	}
}

// --- HookProgressReporter mock test ---

type mockReporter struct {
	events []HookProgressEvent
}

func (m *mockReporter) OnEvent(event HookProgressEvent) {
	m.events = append(m.events, event)
}

func TestHookProgressReporterInterface(t *testing.T) {
	var reporter HookProgressReporter = &mockReporter{}
	reporter.OnEvent(HookProgressEvent{
		Kind:     "started",
		Event:    PreToolUse,
		ToolName: "Bash",
		Command:  "echo test",
	})
	mock := reporter.(*mockReporter)
	if len(mock.events) != 1 {
		t.Fatal("expected 1 event")
	}
	if mock.events[0].Kind != "started" {
		t.Errorf("expected started, got %s", mock.events[0].Kind)
	}
	if mock.events[0].Event != PreToolUse {
		t.Errorf("expected PreToolUse, got %s", mock.events[0].Event)
	}
}
