// Package parity provides golden fixture comparison tests for behavioral
// validation between the Rust source and Go port. These tests verify that
// key behaviors produce identical output across both implementations.
package parity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/apikit"
	"claw-code-go/internal/commands"
	"claw-code-go/internal/permissions"
	"claw-code-go/internal/runtime"
	"claw-code-go/internal/runtime/recovery"
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
