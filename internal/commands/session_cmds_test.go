package commands

import (
	"strings"
	"testing"
)

// mockSessionManager implements SessionManager for testing.
type mockSessionManager struct {
	sessions []string
	forked   string
	loaded   string
	deleted  string
}

func (m *mockSessionManager) ListSessions() ([]string, error) {
	return m.sessions, nil
}

func (m *mockSessionManager) ForkSession(name string) (string, error) {
	m.forked = name
	return "fork-123", nil
}

func (m *mockSessionManager) LoadSession(id string) error {
	m.loaded = id
	return nil
}

func (m *mockSessionManager) DeleteSession(id string) error {
	m.deleted = id
	return nil
}

func TestSessionListCommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{
		sessions: []string{"session-1", "session-2", "session-3"},
	}

	handled, err := r.Execute("/session list", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSessionForkCommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{}

	handled, err := r.Execute("/session fork my-branch", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mock.forked != "my-branch" {
		t.Errorf("expected forked='my-branch', got %q", mock.forked)
	}
}

func TestSessionDeleteCommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{}

	handled, err := r.Execute("/session delete old-session", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mock.deleted != "old-session" {
		t.Errorf("expected deleted='old-session', got %q", mock.deleted)
	}
}

func TestSessionSwitchCommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{}

	handled, err := r.Execute("/session switch target-id", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mock.loaded != "target-id" {
		t.Errorf("expected loaded='target-id', got %q", mock.loaded)
	}
}

func TestSessionDefaultSubcommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{
		sessions: []string{"s1"},
	}

	// No args should default to "list"
	handled, err := r.Execute("/session", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSessionWithoutManager(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	// Pass a non-SessionManager — should output fallback message
	handled, err := r.Execute("/session list", "not a session manager")
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResumeCommand(t *testing.T) {
	r := NewRegistry()
	RegisterSessionCommands(r)

	mock := &mockSessionManager{}
	handled, err := r.Execute("/resume path/to/session.json", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(mock.loaded, "path/to/session.json") {
		t.Errorf("expected loaded path, got %q", mock.loaded)
	}
}
