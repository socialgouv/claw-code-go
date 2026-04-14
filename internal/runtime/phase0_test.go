package runtime

import (
	"claw-code-go/hooks"
	"claw-code-go/internal/api"
	"encoding/json"
	"strings"
	"testing"
)

// --- BUG-TOOL-CALL-COUNT: Tool call count increment tests ---

func TestConversationLoop_ToolCallCount_Empty(t *testing.T) {
	loop := &ConversationLoop{
		Permissions: DefaultPermissions(),
	}
	if got := loop.ToolCallCount(); got != 0 {
		t.Errorf("ToolCallCount() on fresh loop = %d, want 0", got)
	}
}

func TestConversationLoop_ToolCallCount_Increment(t *testing.T) {
	loop := &ConversationLoop{
		Permissions: DefaultPermissions(),
		Config:      &Config{},
	}

	// Execute 3 tools (use unknown tools — they fail but still increment the counter).
	// The counter should track all execution attempts.
	for range 3 {
		loop.ExecuteTool("bash", map[string]any{"command": "echo hi"})
	}

	if got := loop.ToolCallCount(); got != 3 {
		t.Errorf("ToolCallCount() after 3 calls = %d, want 3", got)
	}
}

func TestLoopAdapter_ToolCallCount_ReadsFromLoop(t *testing.T) {
	loop := &ConversationLoop{
		Permissions: DefaultPermissions(),
		Config:      &Config{},
	}

	// Execute 2 tools.
	loop.ExecuteTool("bash", map[string]any{"command": "echo 1"})
	loop.ExecuteTool("bash", map[string]any{"command": "echo 2"})

	adapter := NewLoopAdapter(loop)
	if got := adapter.ToolCallCount(); got != 2 {
		t.Errorf("adapter.ToolCallCount() = %d, want 2", got)
	}
}

func TestLoopAdapter_ToolCallCount_NilLoop(t *testing.T) {
	adapter := NewLoopAdapter(nil)
	if got := adapter.ToolCallCount(); got != 0 {
		t.Errorf("adapter.ToolCallCount() on nil loop = %d, want 0", got)
	}
}

// --- BUG-INJECT-PROMPT-FLAG: IsInjected flag tests ---

func TestMessage_IsInjected_OmitEmpty(t *testing.T) {
	// Non-injected message should omit the field from JSON.
	msg := api.Message{
		Role:    "user",
		Content: []api.ContentBlock{{Type: "text", Text: "hello"}},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "is_injected") {
		t.Errorf("non-injected message JSON should omit is_injected, got: %s", s)
	}
}

func TestMessage_IsInjected_Present(t *testing.T) {
	// Injected message should include the field in JSON.
	msg := api.Message{
		Role:       "user",
		Content:    []api.ContentBlock{{Type: "text", Text: "injected"}},
		IsInjected: true,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"is_injected":true`) {
		t.Errorf("injected message JSON should include is_injected:true, got: %s", s)
	}
}

func TestMessage_IsInjected_Deserialize_Missing(t *testing.T) {
	// Old sessions without is_injected should deserialize cleanly.
	raw := `{"role":"user","content":[{"type":"text","text":"hello"}]}`
	var msg api.Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.IsInjected {
		t.Error("missing is_injected should default to false")
	}
}

func TestMessage_IsInjected_Deserialize_True(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"text","text":"injected"}],"is_injected":true}`
	var msg api.Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if !msg.IsInjected {
		t.Error("is_injected:true should deserialize correctly")
	}
}

func TestInjectPrompt_SetsIsInjected(t *testing.T) {
	loop := &ConversationLoop{
		Permissions: DefaultPermissions(),
		Config:      &Config{},
		Session:     NewSession(),
	}
	adapter := NewLoopAdapter(loop)

	if err := adapter.InjectPrompt("test injection"); err != nil {
		t.Fatal(err)
	}

	if len(loop.Session.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loop.Session.Messages))
	}
	if !loop.Session.Messages[0].IsInjected {
		t.Error("injected message should have IsInjected=true")
	}
}

func TestCountRealUserTurns_ExcludesInjected(t *testing.T) {
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "real 1"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "injected"}}, IsInjected: true},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "reply"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "real 2"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "injected 2"}}, IsInjected: true},
	}

	got := CountRealUserTurns(messages)
	if got != 2 {
		t.Errorf("CountRealUserTurns = %d, want 2", got)
	}
}

func TestCountRealUserTurns_Empty(t *testing.T) {
	if got := CountRealUserTurns(nil); got != 0 {
		t.Errorf("CountRealUserTurns(nil) = %d, want 0", got)
	}
}

// --- WIRE-BUILD-VARS: Build version/commit wiring tests ---

func TestLoopAdapter_Version_Default(t *testing.T) {
	adapter := NewLoopAdapter(nil)
	if got := adapter.Version(); got != "dev" {
		t.Errorf("Version() default = %q, want %q", got, "dev")
	}
}

func TestLoopAdapter_Version_Wired(t *testing.T) {
	adapter := NewLoopAdapter(nil)
	adapter.SetBuildInfo("v1.2.3", "abc1234")
	if got := adapter.Version(); got != "v1.2.3" {
		t.Errorf("Version() = %q, want %q", got, "v1.2.3")
	}
	if got := adapter.Commit(); got != "abc1234" {
		t.Errorf("Commit() = %q, want %q", got, "abc1234")
	}
}

func TestLoopAdapter_BuildInfo_ViaLoop(t *testing.T) {
	loop := &ConversationLoop{
		Permissions:  DefaultPermissions(),
		Config:       &Config{},
		Session:      NewSession(),
		BuildVersion: "v2.0.0",
		BuildCommit:  "deadbeef",
	}
	adapter := NewLoopAdapter(loop)
	adapter.SetBuildInfo(loop.BuildVersion, loop.BuildCommit)
	if got := adapter.Version(); got != "v2.0.0" {
		t.Errorf("Version() from loop = %q, want %q", got, "v2.0.0")
	}
	if got := adapter.Commit(); got != "deadbeef" {
		t.Errorf("Commit() from loop = %q, want %q", got, "deadbeef")
	}
}

// --- WIRE-HOOK-PERM-OVERRIDE: Hook permission override test ---

func TestExecuteTool_HookPermissionDeny(t *testing.T) {
	deny := hooks.PermissionDeny
	loop := &ConversationLoop{
		Permissions: DefaultPermissions(),
		Config:      &Config{},
		HookRunner:  hooks.NewHookRunnerWithOverride(&deny, "policy says no"),
	}

	result := loop.ExecuteTool("bash", map[string]any{"command": "echo hi"})
	if !result.IsError {
		t.Error("hook deny should produce error result")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "policy says no") {
		t.Errorf("deny reason not in result: %+v", result)
	}
}
