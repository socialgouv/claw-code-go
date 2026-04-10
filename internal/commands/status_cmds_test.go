package commands

import (
	"testing"
)

type mockUsageTracker struct {
	model      string
	turns      int
	inputToks  int
	outputToks int
	costUSD    float64
}

func (m *mockUsageTracker) ModelName() string         { return m.model }
func (m *mockUsageTracker) TurnCount() int            { return m.turns }
func (m *mockUsageTracker) InputTokens() int          { return m.inputToks }
func (m *mockUsageTracker) OutputTokens() int         { return m.outputToks }
func (m *mockUsageTracker) EstimatedCostUSD() float64 { return m.costUSD }

func TestStatusCommand(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)

	mock := &mockUsageTracker{
		model:      "claude-sonnet-4-6",
		turns:      5,
		inputToks:  1000,
		outputToks: 500,
		costUSD:    0.0123,
	}

	handled, err := r.Execute("/status", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCostCommand(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)

	mock := &mockUsageTracker{
		inputToks:  2000,
		outputToks: 800,
		costUSD:    0.045,
	}

	handled, err := r.Execute("/cost", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUsageCommand(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)

	mock := &mockUsageTracker{
		model:      "claude-sonnet-4-6",
		turns:      10,
		inputToks:  5000,
		outputToks: 2000,
		costUSD:    0.12,
	}

	handled, err := r.Execute("/usage", mock)
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVersionCommand(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)

	// Without version provider — should still work
	handled, err := r.Execute("/version", "not a version provider")
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStatusWithoutTracker(t *testing.T) {
	r := NewRegistry()
	RegisterStatusCommands(r)

	// Pass non-tracker — should output fallback
	handled, err := r.Execute("/status", "not a tracker")
	if !handled {
		t.Error("expected command to be handled")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
