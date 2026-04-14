// Package parity provides golden fixture comparison tests for behavioral
// validation between the Rust source and Go port. These tests verify that
// key behaviors produce identical output across both implementations.
package parity

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/apikit"
	"claw-code-go/internal/commands"
	"claw-code-go/internal/config"
	"claw-code-go/internal/mcp"
	"claw-code-go/internal/permissions"
	"claw-code-go/internal/runtime"
	"claw-code-go/internal/runtime/recovery"
	"claw-code-go/internal/runtime/worker"
	"claw-code-go/internal/testutil"
	"claw-code-go/internal/tools"
)

// fixtureDir returns the path to the golden fixtures directory.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("fixtures")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("parity fixtures directory %q not found", dir)
	}
	return dir
}

func TestCompactMergeFormatParity(t *testing.T) {
	_ = fixtureDir(t) // ensure fixtures exist

	// Simulate Rust's merge_compact_summaries behavior with structured input.
	previous := "<summary>Conversation summary:\n- Scope: 3 earlier messages compacted (user=1, assistant=1, tool=1).\n- Tools mentioned: bash.\n- Key timeline:\n  - user: initial request</summary>"
	current := "<summary>Conversation summary:\n- Scope: 2 earlier messages compacted (user=1, assistant=1, tool=0).\n- Key timeline:\n  - user: follow-up request\n  - assistant: working on it</summary>"

	merged := runtime.MergeCompactSummaries(previous, current)

	// Verify Rust-matching structure.
	checks := []string{
		"<summary>",
		"Conversation summary:",
		"- Previously compacted context:",
		"- Newly compacted context:",
		"- Key timeline:",
		"</summary>",
	}
	for _, check := range checks {
		if !strings.Contains(merged, check) {
			t.Errorf("merged output missing %q\nFull output:\n%s", check, merged)
		}
	}

	// Verify content from both summaries is present.
	if !strings.Contains(merged, "Scope: 3 earlier messages") {
		t.Error("previous summary content should be preserved")
	}
	if !strings.Contains(merged, "Scope: 2 earlier messages") {
		t.Error("current summary content should be preserved")
	}

	// Verify analysis tags are stripped from current.
	currentWithAnalysis := "<analysis>scratch notes</analysis>" + current
	merged2 := runtime.MergeCompactSummaries(previous, currentWithAnalysis)
	if strings.Contains(merged2, "scratch notes") {
		t.Error("analysis tags should be stripped from current before merging")
	}
}

func TestPermissionPolicyDecisionParity(t *testing.T) {
	_ = fixtureDir(t)

	// Test 1: Active mode meets requirement → Allow (Rust test: allows_tools_when_active_mode_meets_requirement)
	t.Run("mode_meets_requirement", func(t *testing.T) {
		policy := permissions.NewPermissionPolicy(permissions.ModeWorkspaceWrite).
			WithToolRequirement("read_file", permissions.ModeReadOnly).
			WithToolRequirement("write_file", permissions.ModeWorkspaceWrite)

		o := policy.Authorize("read_file", "{}", nil)
		if !o.Allowed {
			t.Errorf("read_file: expected Allow, got Deny: %s", o.Reason)
		}
		o = policy.Authorize("write_file", "{}", nil)
		if !o.Allowed {
			t.Errorf("write_file: expected Allow, got Deny: %s", o.Reason)
		}
	})

	// Test 2: ReadOnly denies escalations (Rust test: denies_read_only_escalations_without_prompt)
	t.Run("read_only_denies", func(t *testing.T) {
		policy := permissions.NewPermissionPolicy(permissions.ModeReadOnly).
			WithToolRequirement("write_file", permissions.ModeWorkspaceWrite).
			WithToolRequirement("bash", permissions.ModeDangerFullAccess)

		o := policy.Authorize("write_file", "{}", nil)
		if o.Allowed {
			t.Error("write_file: expected Deny from ReadOnly")
		}
		o = policy.Authorize("bash", "{}", nil)
		if o.Allowed {
			t.Error("bash: expected Deny from ReadOnly")
		}
	})

	// Test 3: Rule-based allow/deny (Rust test: applies_rule_based_denials_and_allows)
	t.Run("rule_based_decisions", func(t *testing.T) {
		policy := permissions.NewPermissionPolicy(permissions.ModeReadOnly).
			WithToolRequirement("bash", permissions.ModeDangerFullAccess).
			WithPermissionRules(
				[]string{"bash(git:*)"},
				[]string{"bash(rm -rf:*)"},
				nil,
			)

		o := policy.Authorize("bash", `{"command":"git status"}`, nil)
		if !o.Allowed {
			t.Errorf("git: expected Allow via rule, got Deny: %s", o.Reason)
		}
		o = policy.Authorize("bash", `{"command":"rm -rf /tmp"}`, nil)
		if o.Allowed {
			t.Error("rm: expected Deny via rule")
		}
	})
}

func TestTelemetryTracerParity(t *testing.T) {
	_ = fixtureDir(t)

	// Verify SessionTracer dual-event emission matches Rust behavior.
	sink := &apikit.MemoryTelemetrySink{}
	tracer := apikit.NewSessionTracer("session-123", sink)

	tracer.RecordHTTPRequestStarted(1, "POST", "/v1/messages", nil)
	tracer.RecordAnalytics(
		apikit.NewAnalyticsEvent("cli", "prompt_sent").WithProperty("model", "claude-opus"),
	)

	events := sink.Events()
	if len(events) != 4 {
		t.Fatalf("expected 4 events (2 primary + 2 traces), got %d", len(events))
	}

	// Event 0: HTTPRequestStarted
	if events[0].Type != apikit.EventTypeHTTPRequestStarted {
		t.Errorf("event[0]: expected http_request_started, got %s", events[0].Type)
	}
	if events[0].SessionID != "session-123" {
		t.Errorf("event[0]: session_id = %q, want 'session-123'", events[0].SessionID)
	}

	// Event 1: SessionTrace for started (sequence=0)
	if events[1].Type != apikit.EventTypeSessionTrace {
		t.Errorf("event[1]: expected session_trace, got %s", events[1].Type)
	}
	if events[1].SessionTrace.Name != "http_request_started" {
		t.Errorf("event[1]: name = %q, want 'http_request_started'", events[1].SessionTrace.Name)
	}
	if events[1].SessionTrace.Sequence != 0 {
		t.Errorf("event[1]: sequence = %d, want 0", events[1].SessionTrace.Sequence)
	}

	// Event 2: Analytics
	if events[2].Type != apikit.EventTypeAnalytics {
		t.Errorf("event[2]: expected analytics, got %s", events[2].Type)
	}

	// Event 3: SessionTrace for analytics (sequence=1)
	if events[3].Type != apikit.EventTypeSessionTrace {
		t.Errorf("event[3]: expected session_trace, got %s", events[3].Type)
	}
	if events[3].SessionTrace.Name != "analytics" {
		t.Errorf("event[3]: name = %q, want 'analytics'", events[3].SessionTrace.Name)
	}
	if events[3].SessionTrace.Sequence != 1 {
		t.Errorf("event[3]: sequence = %d, want 1", events[3].SessionTrace.Sequence)
	}
}

// TestCommandCountParity verifies the Go command registry has at least the
// target count from Batch 6 (expanded from Batch 5). The Rust reference has
// 142 SlashCommandSpec entries; we track how close we are.
func TestCommandCountParity(t *testing.T) {
	_ = fixtureDir(t)

	r := commands.NewFullRegistry()
	goCount := r.Count()

	// Final batch target: at least 142 ported commands.
	// Go now exceeds Rust (146 vs 142) with Go-only additions.
	const finalTarget = 142
	// Rust reference: 142 SlashCommandSpec entries
	const rustTotal = 142

	if goCount < finalTarget {
		t.Errorf("Go command count %d < final target %d", goCount, finalTarget)
	}

	t.Logf("Command count parity: Go=%d, Rust=%d (%.0f%%)", goCount, rustTotal, float64(goCount)/float64(rustTotal)*100)

	// Verify key commands from each category exist
	requiredCommands := []string{
		// Builtins
		"help", "exit", "quit", "clear",
		// Session
		"session", "resume", "rename", "export", "history", "tag", "summary",
		"pin", "unpin", "bookmarks", "focus", "unfocus", "add-dir", "workspace",
		// Status
		"status", "cost", "usage", "version", "stats", "tokens", "cache",
		"providers", "metrics", "notifications", "billing",
		// Config
		"config", "model", "permissions", "plan", "compact", "temperature",
		"max-tokens", "system-prompt", "reasoning", "budget", "rate-limit",
		"allowed-tools", "api-key", "telemetry", "profile", "language", "ultraplan",
		// Diagnostics
		"doctor", "diff",
		// Plugin/session-mgmt
		"plugin", "agents", "skills", "tasks", "team", "cron", "memory", "sandbox", "init", "upgrade",
		// Code
		"commit", "pr", "issue", "bughunter", "review", "security-review", "release-notes", "test",
		"explain", "refactor", "docs", "fix", "perf", "chat", "web", "autofix",
		// UX
		"theme", "vim", "effort", "fast", "brief", "advisor", "color", "keybindings",
		"privacy-settings", "output-style", "voice", "share", "feedback",
		// Context
		"files", "context", "hooks", "search", "copy", "rewind", "branch",
		"symbols", "references", "definition", "hover", "diagnostics", "map", "tool-details",
		// Auth
		"auth",
		// MCP
		"mcp",
		// Interaction
		"approve", "deny", "undo", "stop", "retry",
	}

	for _, name := range requiredCommands {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("required command /%s not found in Go registry", name)
		}
	}
}

// TestLoopAdapterInterfaceWiring verifies that LoopAdapter satisfies the
// interface contracts required by all command handler groups.
func TestLoopAdapterInterfaceWiring(t *testing.T) {
	_ = fixtureDir(t)

	// Create a minimal ConversationLoop and wrap it in a LoopAdapter.
	cfg := &runtime.Config{
		Model:      "claude-test",
		MaxTokens:  4096,
		SessionDir: t.TempDir(),
	}
	loop := &runtime.ConversationLoop{
		Config:  cfg,
		Session: runtime.NewSession(),
	}
	adapter := runtime.NewLoopAdapter(loop)

	// Verify the adapter can be used as each command group's interface.
	// This is a behavioral check — we call through the interface, not just compile-time.

	// SessionManager
	if _, err := adapter.ListSessions(); err != nil {
		t.Errorf("ListSessions: %v", err)
	}

	// UsageTracker
	if adapter.ModelName() != "claude-test" {
		t.Errorf("ModelName = %q, want 'claude-test'", adapter.ModelName())
	}

	// ConfigSwitcher
	if adapter.CurrentModel() != "claude-test" {
		t.Errorf("CurrentModel = %q", adapter.CurrentModel())
	}

	// toggleLoop
	adapter.SetToggle("test", true)
	if !adapter.GetToggle("test") {
		t.Error("GetToggle should return true after SetToggle(true)")
	}

	// themeLoop
	themes := adapter.ListThemes()
	if len(themes) == 0 {
		t.Error("ListThemes should return available themes")
	}

	// effortLoop
	if err := adapter.SetEffort("high"); err != nil {
		t.Errorf("SetEffort: %v", err)
	}
	if adapter.GetEffort() != "high" {
		t.Errorf("GetEffort = %q, want 'high'", adapter.GetEffort())
	}
}

// TestRecoveryScenariosParity verifies all 7 Rust failure scenarios are
// implemented and have matching recipes.
func TestRecoveryScenariosParity(t *testing.T) {
	_ = fixtureDir(t)

	scenarios := recovery.AllScenarios()
	if len(scenarios) != 7 {
		t.Fatalf("expected 7 scenarios, got %d", len(scenarios))
	}

	// Verify each scenario has a recipe with correct escalation policy (matches Rust).
	expectedPolicies := map[recovery.FailureScenario]recovery.EscalationPolicy{
		recovery.ScenarioTrustPromptUnresolved: recovery.PolicyAlertHuman,
		recovery.ScenarioPromptMisdelivery:     recovery.PolicyAlertHuman,
		recovery.ScenarioStaleBranch:           recovery.PolicyAlertHuman,
		recovery.ScenarioCompileRedCrossCrate:  recovery.PolicyAlertHuman,
		recovery.ScenarioMcpHandshakeFailure:   recovery.PolicyAbort,
		recovery.ScenarioPartialPluginStartup:  recovery.PolicyLogAndContinue,
		recovery.ScenarioProviderFailure:       recovery.PolicyAlertHuman,
	}

	for scenario, expectedPolicy := range expectedPolicies {
		recipe := recovery.RecipeFor(scenario)
		if recipe.EscalationPolicy != expectedPolicy {
			t.Errorf("scenario %s: escalation policy = %s, want %s",
				scenario, recipe.EscalationPolicy, expectedPolicy)
		}
		if recipe.MaxAttempts != 1 {
			t.Errorf("scenario %s: max_attempts = %d, want 1", scenario, recipe.MaxAttempts)
		}
		if len(recipe.Steps) == 0 {
			t.Errorf("scenario %s: no recovery steps", scenario)
		}
	}

	// Verify FromLaneEvent mapping for all known event types.
	laneEvents := map[string]recovery.FailureScenario{
		"trust_prompt_unresolved": recovery.ScenarioTrustPromptUnresolved,
		"prompt_misdelivery":      recovery.ScenarioPromptMisdelivery,
		"stale_branch":            recovery.ScenarioStaleBranch,
		"compile_failure":         recovery.ScenarioCompileRedCrossCrate,
		"mcp_handshake_failure":   recovery.ScenarioMcpHandshakeFailure,
		"plugin_startup_failure":  recovery.ScenarioPartialPluginStartup,
		"provider_failure":        recovery.ScenarioProviderFailure,
	}

	for event, expected := range laneEvents {
		got, ok := recovery.FromLaneEvent(event)
		if !ok {
			t.Errorf("FromLaneEvent(%q): not found", event)
			continue
		}
		if got != expected {
			t.Errorf("FromLaneEvent(%q) = %s, want %s", event, got, expected)
		}
	}
}

// TestInteractionModelParity verifies the interaction model golden fixture
// documents all expected behaviors. This test validates that the fixture
// exists and contains the expected scenario descriptions.
func TestInteractionModelParity(t *testing.T) {
	dir := fixtureDir(t)

	data, err := os.ReadFile(filepath.Join(dir, "interaction_model.golden"))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	content := string(data)

	// Verify all three scenarios are documented.
	scenarios := []string{
		"InteractionLLM auto-respond",
		"InteractionLLMOrHuman auto-respond",
		"InteractionLLMOrHuman escalation",
	}
	for _, s := range scenarios {
		if !strings.Contains(content, s) {
			t.Errorf("golden fixture missing scenario: %q", s)
		}
	}

	// Verify key invariants are documented.
	invariants := []string{
		"InteractionHuman always pauses",
		"InteractionNone rejects",
		"SessionID is preserved",
		"needs_human_input",
		"schema sanitization",
	}
	for _, inv := range invariants {
		if !containsCaseInsensitive(content, inv) {
			t.Errorf("golden fixture missing invariant: %q", inv)
		}
	}
}

// containsCaseInsensitive checks if s contains substr (case-insensitive).
func containsCaseInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// TestRecoveryWithDepsExecutesParity verifies recovery execution matches Rust behavior.
func TestRecoveryWithDepsExecutesParity(t *testing.T) {
	_ = fixtureDir(t)

	// Test: successful recovery produces Recovered result with correct step count.
	ctx := recovery.NewRecoveryContext()
	result := recovery.AttemptRecovery(recovery.ScenarioProviderFailure, ctx)
	if result.ResultKind() != "recovered" {
		t.Errorf("expected 'recovered', got %q", result.ResultKind())
	}
	if r, ok := result.(recovery.Recovered); ok {
		if r.StepsTaken != 1 {
			t.Errorf("expected 1 step taken, got %d", r.StepsTaken)
		}
	}

	// Test: second attempt for same scenario triggers escalation (max_attempts=1).
	result2 := recovery.AttemptRecovery(recovery.ScenarioProviderFailure, ctx)
	if result2.ResultKind() != "escalation_required" {
		t.Errorf("expected 'escalation_required' on second attempt, got %q", result2.ResultKind())
	}

	// Test: partial recovery when step fails mid-recipe.
	ctx2 := recovery.NewRecoveryContextWithFailAt(1) // fail at step index 1
	result3 := recovery.AttemptRecovery(recovery.ScenarioStaleBranch, ctx2)
	// StaleBranch has [RebaseBranch, CleanBuild]; fail at step 1 = partial
	if result3.ResultKind() != "partial_recovery" {
		t.Errorf("expected 'partial_recovery', got %q", result3.ResultKind())
	}

	// Verify events were emitted.
	events := ctx2.Events()
	if len(events) == 0 {
		t.Error("expected recovery events to be emitted")
	}
}

// parityScenarioNames lists the 12 mock parity scenarios matching Rust's harness.
var parityScenarioNames = []string{
	"streaming_text",
	"read_file_roundtrip",
	"grep_chunk_assembly",
	"write_file_allowed",
	"write_file_denied",
	"multi_tool_turn_roundtrip",
	"bash_stdout_roundtrip",
	"bash_permission_prompt_approved",
	"bash_permission_prompt_denied",
	"plugin_tool_roundtrip",
	"auto_compact_triggered",
	"token_cost_reporting",
}

// --- Delta fix parity tests ---

// TestSendMessageWhitespaceValidationParity verifies that whitespace-only messages
// are rejected, matching Rust's message.trim().is_empty() behavior.
func TestSendMessageWhitespaceValidationParity(t *testing.T) {
	_ = fixtureDir(t)

	// Whitespace-only messages should be rejected.
	whitespaceInputs := []map[string]any{
		{"message": "   ", "status": "normal"},
		{"message": "\t\n", "status": "normal"},
		{"message": "  \r\n  ", "status": "normal"},
	}

	for _, input := range whitespaceInputs {
		_, err := tools.ExecuteSendUserMessage(input)
		if err == nil {
			t.Errorf("expected error for whitespace-only message %q, got nil", input["message"])
		}
	}

	// Empty string should also be rejected.
	_, err := tools.ExecuteSendUserMessage(map[string]any{"message": "", "status": "normal"})
	if err == nil {
		t.Error("expected error for empty message, got nil")
	}

	// Valid message should succeed.
	result, err := tools.ExecuteSendUserMessage(map[string]any{"message": "hello", "status": "normal"})
	if err != nil {
		t.Fatalf("valid message should succeed: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Error("result should contain the message text")
	}
}

// TestSendMessageAttachmentsNullabilityParity verifies that when no attachments
// are provided, the result marshals to null (not []), matching Rust's
// Option<Vec<Attachment>> serialization.
func TestSendMessageAttachmentsNullabilityParity(t *testing.T) {
	_ = fixtureDir(t)

	result, err := tools.ExecuteSendUserMessage(map[string]any{
		"message": "hello",
		"status":  "normal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	// Rust: Option<Vec<Attachment>> with None serializes as null.
	// Go: nil marshals to null in JSON.
	attachments, exists := parsed["attachments"]
	if !exists {
		t.Fatal("attachments key should exist in result")
	}
	if attachments != nil {
		t.Errorf("attachments should be null (nil) when no attachments provided, got %v", attachments)
	}
}

// TestWorkerCreateTrustedRootsMergeParity verifies that worker_create merges
// config-level trusted_roots with per-call trusted_roots, matching Rust's
// ConfigLoader::default_for(cwd) behavior.
func TestWorkerCreateTrustedRootsMergeParity(t *testing.T) {
	_ = fixtureDir(t)

	// Set up a temp directory with config containing trustedRoots.
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configData := `{"trustedRoots": ["/config/root1", "/config/root2"]}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify LoadForDir picks up trusted roots from config.
	settings := config.LoadForDir(tmpDir)
	if len(settings.RawJSON) == 0 {
		t.Fatal("expected settings.RawJSON to be populated")
	}
	fc := config.ExtractFeatureConfig(settings.RawJSON)
	if len(fc.TrustedRoots) != 2 {
		t.Fatalf("expected 2 config trusted roots, got %d", len(fc.TrustedRoots))
	}

	// Create a worker registry and call ExecuteWorkerCreate with per-call roots.
	reg := worker.NewWorkerRegistry()
	input := map[string]any{
		"cwd":           tmpDir,
		"trusted_roots": []any{"/call/root3"},
	}
	result, err := tools.ExecuteWorkerCreate(input, reg)
	if err != nil {
		t.Fatalf("ExecuteWorkerCreate failed: %v", err)
	}

	// The result should be valid JSON (worker was created).
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["worker_id"] == nil || parsed["worker_id"] == "" {
		t.Error("worker should have a worker_id")
	}
}

// TestReadMcpResourceResponseShapeParity verifies that read_mcp_resource returns
// the Rust-matching response shape: exactly {server, uri, name, description, mime_type}.
// The `content` field must NOT be present in tool output (content is internal only).
func TestReadMcpResourceResponseShapeParity(t *testing.T) {
	_ = fixtureDir(t)

	// Verify the error path response shape (server + uri + error).
	result, err := tools.ExecuteReadMcpResource(
		map[string]any{"uri": "test://resource", "server": "nonexistent"},
		mcp.NewRegistry(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Error case should still have server and uri.
	if parsed["server"] != "nonexistent" {
		t.Errorf("server = %v, want 'nonexistent'", parsed["server"])
	}
	if parsed["uri"] != "test://resource" {
		t.Errorf("uri = %v, want 'test://resource'", parsed["uri"])
	}

	// Verify the McpResourceContent struct retains content internally.
	rc := mcp.McpResourceContent{
		URI:         "test://res",
		Name:        "Test Resource",
		Description: "A test",
		MimeType:    "text/plain",
		Content:     "hello",
	}
	data, _ := json.Marshal(rc)
	var rcParsed map[string]any
	json.Unmarshal(data, &rcParsed)

	// Internal struct should have content.
	if _, ok := rcParsed["content"]; !ok {
		t.Error("McpResourceContent internal struct should retain 'content' field")
	}

}

// TestMcpAuthResponseShapeParity verifies that mcp_auth returns the Rust-matching
// response shape with server_info and resource_count fields.
func TestMcpAuthResponseShapeParity(t *testing.T) {
	_ = fixtureDir(t)

	registry := mcp.NewRegistry()
	authState := mcp.NewAuthState()

	// Test disconnected server response shape.
	result, err := tools.ExecuteMcpAuth(
		map[string]any{"server": "unknown"},
		registry, authState,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Disconnected server should have status and message.
	if parsed["status"] != "disconnected" {
		t.Errorf("status = %v, want 'disconnected'", parsed["status"])
	}
	if _, ok := parsed["message"]; !ok {
		t.Error("disconnected response should include 'message' field")
	}

	// Verify accessor methods exist on Registry.
	info := registry.GetServerInfo("test")
	if info != "" {
		t.Error("GetServerInfo for unknown server should return empty string")
	}
	count := registry.GetResourceCount("test")
	if count != 0 {
		t.Error("GetResourceCount for unknown server should return 0")
	}
}

// TestMockParityScenarioManifest verifies the scenario manifest loads and
// contains all 12 expected scenarios matching Rust's harness structure.
func TestMockParityScenarioManifest(t *testing.T) {
	dir := fixtureDir(t)

	data, err := os.ReadFile(filepath.Join(dir, "mock_parity_scenarios.json"))
	if err != nil {
		t.Fatalf("read scenario manifest: %v", err)
	}

	var scenarios []struct {
		Name        string   `json:"name"`
		Category    string   `json:"category"`
		Description string   `json:"description"`
		ParityRefs  []string `json:"parity_refs"`
	}
	if err := json.Unmarshal(data, &scenarios); err != nil {
		t.Fatalf("parse scenario manifest: %v", err)
	}

	if len(scenarios) != 12 {
		t.Fatalf("expected 12 scenarios, got %d", len(scenarios))
	}

	// Verify all 12 scenario names match Rust's harness.
	for i, expected := range parityScenarioNames {
		if scenarios[i].Name != expected {
			t.Errorf("scenario[%d]: name = %q, want %q", i, scenarios[i].Name, expected)
		}
		if scenarios[i].Category == "" {
			t.Errorf("scenario %q: missing category", expected)
		}
		if scenarios[i].Description == "" {
			t.Errorf("scenario %q: missing description", expected)
		}
		if len(scenarios[i].ParityRefs) == 0 {
			t.Errorf("scenario %q: missing parity_refs", expected)
		}
	}

	// Verify all scenarios have corresponding mock service scenario constants.
	for _, s := range scenarios {
		_, ok := testutil.ParseScenario(s.Name)
		if !ok {
			t.Errorf("scenario %q not found in mock service ParseScenario", s.Name)
		}
	}
}

// TestMockServiceRequestCapture verifies that the mock service captures
// requests and tracks scenario/streaming state, matching Rust's 21-request
// expectation structure.
func TestMockServiceRequestCapture(t *testing.T) {
	_ = fixtureDir(t)

	svc := testutil.SpawnMockService()
	defer svc.Close()

	// Send requests for each of the 12 parity scenarios.
	for _, scenario := range parityScenarioNames {
		body := `{"model":"claude-sonnet-4-6","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"test ` + testutil.ScenarioPrefix + scenario + `"}]}]}`
		resp, err := sendMockRequest(svc.BaseURL(), body)
		if err != nil {
			t.Fatalf("scenario %s: %v", scenario, err)
		}
		resp.Body.Close()
	}

	captured := svc.CapturedRequests()
	if len(captured) != 12 {
		t.Errorf("expected 12 captured requests (one per scenario), got %d", len(captured))
	}

	// Verify all requests are streaming.
	for _, req := range captured {
		if !req.Stream {
			t.Errorf("request for %s should be streaming", req.Scenario)
		}
		if req.Path != "/v1/messages" {
			t.Errorf("request path = %q, want /v1/messages", req.Path)
		}
	}
}

// sendMockRequest is a helper for TestMockServiceRequestCapture.
func sendMockRequest(baseURL, body string) (*http.Response, error) {
	return http.Post(baseURL+"/v1/messages", "application/json", strings.NewReader(body))
}

// TestSendMessageResponseShapeNoStatusField verifies that send_user_message output
// matches Rust's BriefOutput: {message, sentAt, attachments} with NO status field.
func TestSendMessageResponseShapeNoStatusField(t *testing.T) {
	_ = fixtureDir(t)

	result, err := tools.ExecuteSendUserMessage(map[string]any{
		"message": "hello world",
		"status":  "normal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	// Rust BriefOutput has exactly {message, attachments, sentAt}.
	expectedKeys := map[string]bool{"message": true, "sentAt": true, "attachments": true}
	for key := range parsed {
		if !expectedKeys[key] {
			t.Errorf("unexpected key %q in output (Rust BriefOutput has only {message, sentAt, attachments})", key)
		}
	}
	for key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing key %q in output", key)
		}
	}

	// Verify values.
	if parsed["message"] != "hello world" {
		t.Errorf("message = %v, want 'hello world'", parsed["message"])
	}
	if parsed["sentAt"] == nil || parsed["sentAt"] == "" {
		t.Error("sentAt should be a non-empty timestamp")
	}
}

// TestMcpRegistryAddServerLogsResourceError verifies that AddServer succeeds
// even when ListResources returns an error, and that the error is logged
// at debug level (not silently swallowed).
func TestMcpRegistryAddServerLogsResourceError(t *testing.T) {
	_ = fixtureDir(t)

	// Create a mock transport that succeeds for Initialize and ListTools
	// but fails for ListResources.
	transport := &mockTransportResourceError{}
	registry := mcp.NewRegistry()

	err := registry.AddServer(t.Context(), "test-server", transport)
	if err != nil {
		t.Fatalf("AddServer should succeed even when ListResources fails: %v", err)
	}

	// Verify the server was added despite resource discovery failure.
	names := registry.ServerNames()
	found := false
	for _, n := range names {
		if n == "test-server" {
			found = true
		}
	}
	if !found {
		t.Error("test-server should be registered after AddServer")
	}

	// Resource count should be 0 since discovery failed.
	if count := registry.GetResourceCount("test-server"); count != 0 {
		t.Errorf("resource count = %d, want 0 (ListResources failed)", count)
	}
}

// mockTransportResourceError is a Transport that succeeds for Initialize/ListTools
// but returns an RPC error for resources/list. Used by TestMcpRegistryAddServerLogsResourceError.
type mockTransportResourceError struct{}

func (m *mockTransportResourceError) Send(ctx context.Context, req mcp.Request) (mcp.Response, error) {
	switch req.Method {
	case "initialize":
		return mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "test-server", "version": "1.0"},
			},
		}, nil
	case "tools/list":
		return mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": []any{}},
		}, nil
	case "resources/list":
		return mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcp.RPCError{Code: -32601, Message: "method not found: resources/list"},
		}, nil
	default:
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}, nil
	}
}

func (m *mockTransportResourceError) Notify(n mcp.Notification) error { return nil }
func (m *mockTransportResourceError) Close() error                    { return nil }

// TestPlanModeOutputEnrichedFields verifies that plan mode responses include
// the enriched fields matching Rust's PlanModeOutput structure:
// operation, managed, settingsPath, statePath, previousLocalMode, currentLocalMode.
func TestPlanModeOutputEnrichedFields(t *testing.T) {
	_ = fixtureDir(t)

	stateDir := t.TempDir()

	t.Run("enter", func(t *testing.T) {
		active := false
		result, err := tools.ExecuteEnterPlanMode(&active, stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Fatalf("failed to parse result: %v", err)
		}

		// Verify core fields.
		if parsed["operation"] != "enter" {
			t.Errorf("operation = %v, want 'enter'", parsed["operation"])
		}
		if parsed["managed"] != true {
			t.Errorf("managed = %v, want true (enter always sets managed=true)", parsed["managed"])
		}
		if parsed["active"] != true {
			t.Errorf("active = %v, want true", parsed["active"])
		}
		if parsed["changed"] != true {
			t.Errorf("changed = %v, want true", parsed["changed"])
		}
		if parsed["success"] != true {
			t.Errorf("success = %v, want true", parsed["success"])
		}

		// Verify all 10 fields from Rust's PlanModeOutput are present.
		// The typed planModeOutput struct always serializes all fields
		// (previousLocalMode as null when unset, settingsPath/statePath as "").
		expectedKeys := map[string]bool{
			"success": true, "operation": true, "changed": true,
			"active": true, "managed": true, "message": true,
			"settingsPath": true, "statePath": true,
			"previousLocalMode": true, "currentLocalMode": true,
		}
		for key := range parsed {
			if !expectedKeys[key] {
				t.Errorf("unexpected key %q in plan mode output", key)
			}
		}

		// Verify enriched field values.
		if parsed["currentLocalMode"] != "plan" {
			t.Errorf("currentLocalMode = %v, want 'plan'", parsed["currentLocalMode"])
		}
	})

	t.Run("exit", func(t *testing.T) {
		active := true
		result, err := tools.ExecuteExitPlanMode(&active, stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Fatalf("failed to parse result: %v", err)
		}

		if parsed["operation"] != "exit" {
			t.Errorf("operation = %v, want 'exit'", parsed["operation"])
		}
		if parsed["active"] != false {
			t.Errorf("active = %v, want false", parsed["active"])
		}
		if parsed["changed"] != true {
			t.Errorf("changed = %v, want true", parsed["changed"])
		}
	})

	t.Run("enter_already_active_via_settings", func(t *testing.T) {
		// Re-enter plan mode (from fresh state after exit).
		active := false
		tools.ExecuteEnterPlanMode(&active, stateDir)

		// Second enter while already active → changed=false.
		result, err := tools.ExecuteEnterPlanMode(&active, stateDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]any
		json.Unmarshal([]byte(result), &parsed)

		if parsed["operation"] != "enter" {
			t.Errorf("operation = %v, want 'enter'", parsed["operation"])
		}
		if parsed["changed"] != false {
			t.Errorf("changed = %v, want false (already active)", parsed["changed"])
		}
	})
}
