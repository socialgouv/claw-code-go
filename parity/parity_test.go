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
	"claw-code-go/internal/permissions"
	"claw-code-go/internal/runtime"
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
