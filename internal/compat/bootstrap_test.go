package compat

import (
	"testing"
)

func TestFromPhasesDeduplicatesPreservingOrder(t *testing.T) {
	phases := []BootstrapPhaseType{
		PhaseCliEntry,
		PhaseFastPathVersion,
		PhaseCliEntry,
		PhaseMainRuntime,
		PhaseFastPathVersion,
	}

	plan := FromPhases(phases)
	got := plan.Phases()

	expected := []BootstrapPhaseType{
		PhaseCliEntry,
		PhaseFastPathVersion,
		PhaseMainRuntime,
	}

	if len(got) != len(expected) {
		t.Fatalf("len = %d, want %d", len(got), len(expected))
	}
	for i, p := range got {
		if p != expected[i] {
			t.Errorf("phase[%d] = %v, want %v", i, p, expected[i])
		}
	}
}

func TestClaudeCodeDefaultCoversEachPhaseOnce(t *testing.T) {
	expected := []BootstrapPhaseType{
		PhaseCliEntry,
		PhaseFastPathVersion,
		PhaseStartupProfiler,
		PhaseSystemPromptFastPath,
		PhaseChromeMcpFastPath,
		PhaseDaemonWorkerFastPath,
		PhaseBridgeFastPath,
		PhaseDaemonFastPath,
		PhaseBackgroundSessionFastPath,
		PhaseTemplateFastPath,
		PhaseEnvironmentRunnerFastPath,
		PhaseMainRuntime,
	}

	plan := ClaudeCodeDefault()
	got := plan.Phases()

	if len(got) != len(expected) {
		t.Fatalf("len = %d, want %d", len(got), len(expected))
	}
	for i, p := range got {
		if p != expected[i] {
			t.Errorf("phase[%d] = %s, want %s", i, p, expected[i])
		}
	}
}
