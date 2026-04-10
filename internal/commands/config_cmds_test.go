package commands

import (
	"testing"
)

type mockConfigSwitcher struct {
	model          string
	permissionMode string
}

func (m *mockConfigSwitcher) CurrentModel() string          { return m.model }
func (m *mockConfigSwitcher) SetModel(model string) error   { m.model = model; return nil }
func (m *mockConfigSwitcher) CurrentPermissionMode() string { return m.permissionMode }
func (m *mockConfigSwitcher) SetPermissionMode(mode string) error {
	m.permissionMode = mode
	return nil
}

func TestConfigEnvSubcommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	handled, err := r.Execute("/config env", nil)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigModelSubcommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	mock := &mockConfigSwitcher{model: "claude-sonnet-4-6"}

	handled, err := r.Execute("/config model", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigUnknownSubcommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	handled, err := r.Execute("/config unknown", nil)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigHelpSubcommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	handled, err := r.Execute("/config", nil)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModelCommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	mock := &mockConfigSwitcher{model: "claude-sonnet-4-6"}

	// Show current model
	handled, err := r.Execute("/model", mock)
	if !handled || err != nil {
		t.Errorf("show model: handled=%v, err=%v", handled, err)
	}

	// Set new model
	handled, err = r.Execute("/model claude-opus-4", mock)
	if !handled || err != nil {
		t.Errorf("set model: handled=%v, err=%v", handled, err)
	}
	if mock.model != "claude-opus-4" {
		t.Errorf("expected model='claude-opus-4', got %q", mock.model)
	}
}

func TestPermissionsCommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	mock := &mockConfigSwitcher{permissionMode: "default"}

	handled, err := r.Execute("/permissions workspace-write", mock)
	if !handled || err != nil {
		t.Errorf("set permissions: handled=%v, err=%v", handled, err)
	}
	if mock.permissionMode != "workspace-write" {
		t.Errorf("expected mode='workspace-write', got %q", mock.permissionMode)
	}
}

func TestCompactCommand(t *testing.T) {
	r := NewRegistry()
	RegisterConfigCommands(r)

	// Without compactor — should output fallback
	handled, err := r.Execute("/compact", "not a compactor")
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
