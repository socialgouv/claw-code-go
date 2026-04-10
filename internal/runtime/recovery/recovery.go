package recovery

import (
	"encoding/json"
	"fmt"

	"claw-code-go/internal/runtime/worker"
)

// ---------------------------------------------------------------------------
// FailureScenario
// ---------------------------------------------------------------------------

type FailureScenario int

const (
	ScenarioTrustPromptUnresolved FailureScenario = iota
	ScenarioPromptMisdelivery
	ScenarioStaleBranch
	ScenarioCompileRedCrossCrate
	ScenarioMcpHandshakeFailure
	ScenarioPartialPluginStartup
	ScenarioProviderFailure
)

var scenarioStrings = [...]string{
	"trust_prompt_unresolved",
	"prompt_misdelivery",
	"stale_branch",
	"compile_red_cross_crate",
	"mcp_handshake_failure",
	"partial_plugin_startup",
	"provider_failure",
}

func (s FailureScenario) String() string {
	if int(s) < len(scenarioStrings) {
		return scenarioStrings[s]
	}
	return "unknown"
}

func (s FailureScenario) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *FailureScenario) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range scenarioStrings {
		if name == str {
			*s = FailureScenario(i)
			return nil
		}
	}
	return &json.UnmarshalTypeError{Value: str, Type: nil}
}

// AllScenarios returns all 7 failure scenarios.
func AllScenarios() []FailureScenario {
	return []FailureScenario{
		ScenarioTrustPromptUnresolved,
		ScenarioPromptMisdelivery,
		ScenarioStaleBranch,
		ScenarioCompileRedCrossCrate,
		ScenarioMcpHandshakeFailure,
		ScenarioPartialPluginStartup,
		ScenarioProviderFailure,
	}
}

// ---------------------------------------------------------------------------
// EscalationPolicy
// ---------------------------------------------------------------------------

type EscalationPolicy int

const (
	PolicyAlertHuman EscalationPolicy = iota
	PolicyLogAndContinue
	PolicyAbort
)

var escalationPolicyStrings = [...]string{
	"alert_human",
	"log_and_continue",
	"abort",
}

func (p EscalationPolicy) String() string {
	if int(p) < len(escalationPolicyStrings) {
		return escalationPolicyStrings[p]
	}
	return "unknown"
}

func (p EscalationPolicy) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p *EscalationPolicy) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range escalationPolicyStrings {
		if name == str {
			*p = EscalationPolicy(i)
			return nil
		}
	}
	return &json.UnmarshalTypeError{Value: str, Type: nil}
}

// ---------------------------------------------------------------------------
// RecoveryStep (sealed interface)
// ---------------------------------------------------------------------------

// RecoveryStep is a sealed interface representing a single recovery action.
type RecoveryStep interface {
	recoveryStep() // unexported marker – seals the interface
	StepName() string
}

type AcceptTrustPrompt struct{}

func (AcceptTrustPrompt) recoveryStep()    {}
func (AcceptTrustPrompt) StepName() string { return "accept_trust_prompt" }

type RedirectPromptToAgent struct{}

func (RedirectPromptToAgent) recoveryStep()    {}
func (RedirectPromptToAgent) StepName() string { return "redirect_prompt_to_agent" }

type RebaseBranch struct{}

func (RebaseBranch) recoveryStep()    {}
func (RebaseBranch) StepName() string { return "rebase_branch" }

type CleanBuild struct{}

func (CleanBuild) recoveryStep()    {}
func (CleanBuild) StepName() string { return "clean_build" }

type RetryMcpHandshake struct{ Timeout uint64 }

func (RetryMcpHandshake) recoveryStep()    {}
func (RetryMcpHandshake) StepName() string { return "retry_mcp_handshake" }

type RestartPlugin struct{ Name string }

func (RestartPlugin) recoveryStep()    {}
func (RestartPlugin) StepName() string { return "restart_plugin" }

type RestartWorker struct{}

func (RestartWorker) recoveryStep()    {}
func (RestartWorker) StepName() string { return "restart_worker" }

type EscalateToHuman struct{ Reason string }

func (EscalateToHuman) recoveryStep()    {}
func (EscalateToHuman) StepName() string { return "escalate_to_human" }

// ---------------------------------------------------------------------------
// RecoveryResult (sealed interface)
// ---------------------------------------------------------------------------

// RecoveryResult represents the outcome of a recovery attempt.
type RecoveryResult interface {
	recoveryResult() // unexported marker
	ResultKind() string
}

type Recovered struct{ StepsTaken uint32 }

func (Recovered) recoveryResult()    {}
func (Recovered) ResultKind() string { return "recovered" }

type PartialRecovery struct {
	RecoveredSteps []RecoveryStep
	Remaining      []RecoveryStep
}

func (PartialRecovery) recoveryResult()    {}
func (PartialRecovery) ResultKind() string { return "partial_recovery" }

type EscalationRequired struct{ Reason string }

func (EscalationRequired) recoveryResult()    {}
func (EscalationRequired) ResultKind() string { return "escalation_required" }

// ---------------------------------------------------------------------------
// RecoveryEvent (sealed interface)
// ---------------------------------------------------------------------------

// RecoveryEvent represents an observable event during recovery.
type RecoveryEvent interface {
	recoveryEvent() // unexported marker
	EventKind() string
}

type RecoveryAttempted struct {
	Scenario FailureScenario
	Recipe   RecoveryRecipe
	Result   RecoveryResult
}

func (RecoveryAttempted) recoveryEvent()    {}
func (RecoveryAttempted) EventKind() string { return "recovery_attempted" }

type RecoverySucceeded struct{}

func (RecoverySucceeded) recoveryEvent()    {}
func (RecoverySucceeded) EventKind() string { return "recovery_succeeded" }

type RecoveryFailed struct{}

func (RecoveryFailed) recoveryEvent()    {}
func (RecoveryFailed) EventKind() string { return "recovery_failed" }

type Escalated struct{}

func (Escalated) recoveryEvent()    {}
func (Escalated) EventKind() string { return "escalated" }

// ---------------------------------------------------------------------------
// RecoveryRecipe
// ---------------------------------------------------------------------------

// RecoveryRecipe describes the steps and policy for recovering from a scenario.
type RecoveryRecipe struct {
	Scenario         FailureScenario
	Steps            []RecoveryStep
	MaxAttempts      uint32
	EscalationPolicy EscalationPolicy
}

// ---------------------------------------------------------------------------
// RecoveryContext
// ---------------------------------------------------------------------------

// RecoveryContext tracks attempt counts and accumulated events.
type RecoveryContext struct {
	attempts   map[FailureScenario]uint32
	events     []RecoveryEvent
	failAtStep *int // for testing: simulate failure at this step index
}

// NewRecoveryContext creates a fresh context with no prior attempts.
func NewRecoveryContext() *RecoveryContext {
	return &RecoveryContext{
		attempts: make(map[FailureScenario]uint32),
	}
}

// NewRecoveryContextWithFailAt creates a context that simulates failure at the
// given step index during recovery execution.
func NewRecoveryContextWithFailAt(step int) *RecoveryContext {
	return &RecoveryContext{
		attempts:   make(map[FailureScenario]uint32),
		failAtStep: &step,
	}
}

// Events returns all accumulated recovery events.
func (c *RecoveryContext) Events() []RecoveryEvent {
	return c.events
}

// AttemptCount returns how many times recovery has been attempted for a scenario.
func (c *RecoveryContext) AttemptCount(scenario FailureScenario) uint32 {
	return c.attempts[scenario]
}

// ---------------------------------------------------------------------------
// RecipeFor
// ---------------------------------------------------------------------------

// RecipeFor returns the canonical recovery recipe for a given failure scenario.
func RecipeFor(scenario FailureScenario) RecoveryRecipe {
	switch scenario {
	case ScenarioTrustPromptUnresolved:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{AcceptTrustPrompt{}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	case ScenarioPromptMisdelivery:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{RedirectPromptToAgent{}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	case ScenarioStaleBranch:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{RebaseBranch{}, CleanBuild{}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	case ScenarioCompileRedCrossCrate:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{CleanBuild{}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	case ScenarioMcpHandshakeFailure:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{RetryMcpHandshake{Timeout: 5000}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAbort,
		}
	case ScenarioPartialPluginStartup:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{RestartPlugin{Name: "stalled"}, RetryMcpHandshake{Timeout: 3000}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyLogAndContinue,
		}
	case ScenarioProviderFailure:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{RestartWorker{}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	default:
		return RecoveryRecipe{
			Scenario:         scenario,
			Steps:            []RecoveryStep{EscalateToHuman{Reason: "unknown scenario"}},
			MaxAttempts:      1,
			EscalationPolicy: PolicyAlertHuman,
		}
	}
}

// ---------------------------------------------------------------------------
// AttemptRecovery
// ---------------------------------------------------------------------------

// AttemptRecovery executes the recovery recipe for the given scenario within
// the provided context. It tracks attempts, honours failAtStep for testing,
// and emits appropriate events.
func AttemptRecovery(scenario FailureScenario, ctx *RecoveryContext) RecoveryResult {
	recipe := RecipeFor(scenario)

	// Check if max attempts already reached.
	if ctx.attempts[scenario] >= recipe.MaxAttempts {
		result := EscalationRequired{Reason: fmt.Sprintf("max recovery attempts (%d) exceeded for %s", recipe.MaxAttempts, scenario)}
		ctx.events = append(ctx.events,
			RecoveryAttempted{Scenario: scenario, Recipe: recipe, Result: result},
			Escalated{},
		)
		return result
	}

	// Increment attempt counter.
	ctx.attempts[scenario]++

	// Execute steps (simulated).
	for i := range recipe.Steps {
		if ctx.failAtStep != nil && i == *ctx.failAtStep {
			var result RecoveryResult
			if i == 0 {
				result = EscalationRequired{Reason: fmt.Sprintf("recovery failed at first step for %s", scenario)}
				ctx.events = append(ctx.events,
					RecoveryAttempted{Scenario: scenario, Recipe: recipe, Result: result},
					Escalated{},
				)
			} else {
				result = PartialRecovery{
					RecoveredSteps: recipe.Steps[:i],
					Remaining:      recipe.Steps[i:],
				}
				ctx.events = append(ctx.events,
					RecoveryAttempted{Scenario: scenario, Recipe: recipe, Result: result},
					RecoveryFailed{},
				)
			}
			return result
		}
	}

	// All steps succeeded.
	result := Recovered{StepsTaken: uint32(len(recipe.Steps))}
	ctx.events = append(ctx.events,
		RecoveryAttempted{Scenario: scenario, Recipe: recipe, Result: result},
		RecoverySucceeded{},
	)
	return result
}

// ---------------------------------------------------------------------------
// FromWorkerFailureKind
// ---------------------------------------------------------------------------

// FromWorkerFailureKind maps a worker failure kind string to a FailureScenario.
func FromWorkerFailureKind(kind string) FailureScenario {
	switch kind {
	case "trust_gate":
		return ScenarioTrustPromptUnresolved
	case "prompt_delivery":
		return ScenarioPromptMisdelivery
	case "protocol":
		return ScenarioMcpHandshakeFailure
	case "provider":
		return ScenarioProviderFailure
	default:
		return ScenarioProviderFailure
	}
}

// FromWorkerFailure maps a typed worker.WorkerFailureKind to a FailureScenario.
func FromWorkerFailure(kind worker.WorkerFailureKind) FailureScenario {
	switch kind {
	case worker.FailureTrustGate:
		return ScenarioTrustPromptUnresolved
	case worker.FailurePromptDelivery:
		return ScenarioPromptMisdelivery
	case worker.FailureProtocol:
		return ScenarioMcpHandshakeFailure
	case worker.FailureProvider:
		return ScenarioProviderFailure
	default:
		return ScenarioProviderFailure
	}
}
