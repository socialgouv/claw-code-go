package permissions

import "testing"

func TestToolPermissionLevelOrdering(t *testing.T) {
	if ReadOnly >= WorkspaceWrite {
		t.Error("ReadOnly should be < WorkspaceWrite")
	}
	if WorkspaceWrite >= DangerFullAccess {
		t.Error("WorkspaceWrite should be < DangerFullAccess")
	}
}

func TestToolPermissionLevelString(t *testing.T) {
	tests := []struct {
		level ToolPermissionLevel
		want  string
	}{
		{ReadOnly, "read-only"},
		{WorkspaceWrite, "workspace-write"},
		{DangerFullAccess, "danger-full-access"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestParseToolPermissionLevel(t *testing.T) {
	tests := []struct {
		input string
		want  ToolPermissionLevel
		ok    bool
	}{
		{"read-only", ReadOnly, true},
		{"workspace-write", WorkspaceWrite, true},
		{"danger-full-access", DangerFullAccess, true},
		{"bogus", ReadOnly, false},
	}
	for _, tt := range tests {
		got, ok := ParseToolPermissionLevel(tt.input)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseToolPermissionLevel(%q) = (%v, %v), want (%v, %v)",
				tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCheckWithHookOverrideAllow(t *testing.T) {
	m := NewManager(ModeDefault, nil)
	override := &HookPermissionOverride{Decision: DecisionAllow, Reason: "hook says ok"}
	d := m.CheckWithHookOverride("bash", "rm -rf /", override)
	if d != DecisionAllow {
		t.Errorf("expected Allow with hook override, got %v", d)
	}
}

func TestCheckWithHookOverrideDeny(t *testing.T) {
	m := NewManager(ModeBypassPermissions, nil)
	override := &HookPermissionOverride{Decision: DecisionDeny, Reason: "hook blocks"}
	d := m.CheckWithHookOverride("bash", "ls", override)
	if d != DecisionDeny {
		t.Errorf("expected Deny with hook override even in bypass mode, got %v", d)
	}
}

func TestCheckWithHookOverrideNilFallsThrough(t *testing.T) {
	m := NewManager(ModeBypassPermissions, nil)
	d := m.CheckWithHookOverride("bash", "ls", nil)
	if d != DecisionAllow {
		t.Errorf("expected Allow in bypass mode with nil override, got %v", d)
	}
}

func TestCheckWithHookOverridePlanMode(t *testing.T) {
	m := NewManager(ModePlan, nil)
	// No override → Plan mode denies.
	d := m.CheckWithHookOverride("bash", "ls", nil)
	if d != DecisionDeny {
		t.Errorf("expected Deny in plan mode, got %v", d)
	}

	// Override Allow takes precedence over Plan mode.
	override := &HookPermissionOverride{Decision: DecisionAllow}
	d = m.CheckWithHookOverride("bash", "ls", override)
	if d != DecisionAllow {
		t.Errorf("expected Allow with hook override in plan mode, got %v", d)
	}
}

// mockPrompter implements PermissionPrompter for testing.
type mockPrompter struct {
	called bool
}

func (m *mockPrompter) Decide(req PermissionRequest) PermissionPromptDecision {
	m.called = true
	return PermissionPromptDecision{Allowed: true}
}

func TestPermissionPrompterMockable(t *testing.T) {
	var p PermissionPrompter = &mockPrompter{}
	result := p.Decide(PermissionRequest{
		ToolName: "bash",
		Input:    "echo hello",
	})
	if !result.Allowed {
		t.Error("mock prompter should allow")
	}
	if !p.(*mockPrompter).called {
		t.Error("mock prompter should have been called")
	}
}

func TestSetPrompter(t *testing.T) {
	m := NewManager(ModeDefault, nil)
	mp := &mockPrompter{}
	m.SetPrompter(mp)
	// Verify it was set (we can't directly check the field, but this shouldn't panic).
	if m.prompter != mp {
		t.Error("prompter should be set")
	}
}
