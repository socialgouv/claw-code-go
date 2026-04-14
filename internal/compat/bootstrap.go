package compat

// BootstrapPhaseType is a typed enum for the 12 named bootstrap phases,
// matching the Rust BootstrapPhase enum in bootstrap.rs.
type BootstrapPhaseType int

const (
	PhaseCliEntry BootstrapPhaseType = iota
	PhaseFastPathVersion
	PhaseStartupProfiler
	PhaseSystemPromptFastPath
	PhaseChromeMcpFastPath
	PhaseDaemonWorkerFastPath
	PhaseBridgeFastPath
	PhaseDaemonFastPath
	PhaseBackgroundSessionFastPath
	PhaseTemplateFastPath
	PhaseEnvironmentRunnerFastPath
	PhaseMainRuntime
)

// String returns the human-readable name of the bootstrap phase.
func (p BootstrapPhaseType) String() string {
	switch p {
	case PhaseCliEntry:
		return "CliEntry"
	case PhaseFastPathVersion:
		return "FastPathVersion"
	case PhaseStartupProfiler:
		return "StartupProfiler"
	case PhaseSystemPromptFastPath:
		return "SystemPromptFastPath"
	case PhaseChromeMcpFastPath:
		return "ChromeMcpFastPath"
	case PhaseDaemonWorkerFastPath:
		return "DaemonWorkerFastPath"
	case PhaseBridgeFastPath:
		return "BridgeFastPath"
	case PhaseDaemonFastPath:
		return "DaemonFastPath"
	case PhaseBackgroundSessionFastPath:
		return "BackgroundSessionFastPath"
	case PhaseTemplateFastPath:
		return "TemplateFastPath"
	case PhaseEnvironmentRunnerFastPath:
		return "EnvironmentRunnerFastPath"
	case PhaseMainRuntime:
		return "MainRuntime"
	default:
		return "Unknown"
	}
}

// BootstrapPlan holds an ordered, deduplicated list of bootstrap phases.
type BootstrapPlan struct {
	phases []BootstrapPhaseType
}

// ClaudeCodeDefault returns the default bootstrap plan for Claude Code
// containing all 12 phases in canonical order.
func ClaudeCodeDefault() BootstrapPlan {
	return FromPhases([]BootstrapPhaseType{
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
	})
}

// FromPhases creates a BootstrapPlan from the given phases, deduplicating
// while preserving insertion order.
func FromPhases(phases []BootstrapPhaseType) BootstrapPlan {
	seen := make(map[BootstrapPhaseType]struct{}, len(phases))
	deduped := make([]BootstrapPhaseType, 0, len(phases))
	for _, p := range phases {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			deduped = append(deduped, p)
		}
	}
	return BootstrapPlan{phases: deduped}
}

// Phases returns the ordered phases in this plan.
func (bp BootstrapPlan) Phases() []BootstrapPhaseType {
	return bp.phases
}
