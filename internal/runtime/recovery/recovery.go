package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/runtime/worker"
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
// RecoveryDeps
// ---------------------------------------------------------------------------

// RecoveryDeps is the interface for executing real recovery actions.
// When nil is passed to AttemptRecovery, the recovery runs in simulation mode
// (the existing failAtStep-based testing path). When non-nil, each step
// dispatches to the corresponding method.
type RecoveryDeps interface {
	AcceptTrust() error
	RedirectPrompt() error
	RebaseBranch() error
	CleanBuild() error
	RetryMcpHandshake(timeoutMs uint64) error
	RestartPlugin(name string) error
	RestartWorker() error
}

// ---------------------------------------------------------------------------
// AttemptRecovery
// ---------------------------------------------------------------------------

// AttemptRecovery executes the recovery recipe for the given scenario within
// the provided context. It tracks attempts, honours failAtStep for testing,
// and emits appropriate events.
//
// If deps is non-nil, each step is executed via the corresponding RecoveryDeps
// method. If deps is nil, the simulation path (failAtStep) is used.
func AttemptRecovery(scenario FailureScenario, ctx *RecoveryContext) RecoveryResult {
	return AttemptRecoveryWithDeps(scenario, ctx, nil)
}

// AttemptRecoveryWithDeps executes recovery with optional real dependencies.
func AttemptRecoveryWithDeps(scenario FailureScenario, ctx *RecoveryContext, deps RecoveryDeps) RecoveryResult {
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

	// Execute steps.
	for i, step := range recipe.Steps {
		// Simulation path: test-injected failure at a specific step.
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

		// Real execution path: dispatch to deps if available.
		if deps != nil {
			if err := executeStep(step, deps); err != nil {
				var result RecoveryResult
				if i == 0 {
					result = EscalationRequired{Reason: fmt.Sprintf("recovery failed at step %q: %s", step.StepName(), err)}
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

// executeStep dispatches a single recovery step to the deps interface.
func executeStep(step RecoveryStep, deps RecoveryDeps) error {
	switch s := step.(type) {
	case AcceptTrustPrompt:
		return deps.AcceptTrust()
	case RedirectPromptToAgent:
		return deps.RedirectPrompt()
	case RebaseBranch:
		return deps.RebaseBranch()
	case CleanBuild:
		return deps.CleanBuild()
	case RetryMcpHandshake:
		return deps.RetryMcpHandshake(s.Timeout)
	case RestartPlugin:
		return deps.RestartPlugin(s.Name)
	case RestartWorker:
		return deps.RestartWorker()
	case EscalateToHuman:
		return nil // escalation is handled by the caller
	default:
		return fmt.Errorf("unknown recovery step: %T", step)
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

// ---------------------------------------------------------------------------
// Dependency interfaces (avoid import cycles with concrete packages)
// ---------------------------------------------------------------------------

// WorkerRegistryInterface is the subset of worker.WorkerRegistry needed for recovery.
type WorkerRegistryInterface interface {
	ResolveTrust(workerID string) error
	Restart(workerID string) error
	SendPrompt(workerID string, prompt string) error
}

// MCPRegistryInterface is the subset of mcp.Registry needed for recovery.
type MCPRegistryInterface interface {
	Reconnect(serverName string) error
}

// PluginManagerInterface is the subset of plugin.PluginManager needed for recovery.
type PluginManagerInterface interface {
	DisablePlugin(name string) error
	EnablePlugin(name string) error
}

// ---------------------------------------------------------------------------
// ProductionRecoveryDeps
// ---------------------------------------------------------------------------

// ProductionRecoveryDeps implements RecoveryDeps with real system dependencies.
type ProductionRecoveryDeps struct {
	Workers   WorkerRegistryInterface
	MCP       MCPRegistryInterface
	Plugins   PluginManagerInterface
	EventSink func(RecoveryEvent)
	WorkDir   string
}

var _ RecoveryDeps = (*ProductionRecoveryDeps)(nil)

func (d *ProductionRecoveryDeps) AcceptTrust() error {
	if d.Workers == nil {
		return fmt.Errorf("recovery: worker registry not available (nil dependency)")
	}
	return d.Workers.ResolveTrust("default")
}

func (d *ProductionRecoveryDeps) RedirectPrompt() error {
	if d.Workers == nil {
		return fmt.Errorf("recovery: worker registry not available (nil dependency)")
	}
	return d.Workers.SendPrompt("default", "")
}

// RebaseBranch runs `git rebase --autostash` inside d.WorkDir. WorkDir MUST
// be non-empty: rebasing in the process CWD is never what a caller wants
// (it would mutate whatever directory the iterion / claw process was
// launched from, which can be arbitrary).
func (d *ProductionRecoveryDeps) RebaseBranch() error {
	if d.WorkDir == "" {
		return fmt.Errorf("recovery: WorkDir is required for RebaseBranch (refusing to operate on process CWD)")
	}
	cmd := exec.Command("git", "rebase", "--autostash")
	cmd.Dir = d.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git rebase: %s: %w", string(out), err)
	}
	return nil
}

// CleanBuild runs `git clean -fdx` inside d.WorkDir. WorkDir MUST be
// non-empty: `git clean -fdx` deletes ALL untracked files (including
// those in .gitignore), so silently falling back to the process CWD
// would destroy arbitrary user files. Refuse rather than risk it.
func (d *ProductionRecoveryDeps) CleanBuild() error {
	if d.WorkDir == "" {
		return fmt.Errorf("recovery: WorkDir is required for CleanBuild (refusing to run `git clean -fdx` on process CWD)")
	}
	cmd := exec.Command("git", "clean", "-fdx")
	cmd.Dir = d.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clean: %s: %w", string(out), err)
	}
	return nil
}

func (d *ProductionRecoveryDeps) RetryMcpHandshake(timeoutMs uint64) error {
	if d.MCP == nil {
		return fmt.Errorf("recovery: MCP registry not available (nil dependency)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- d.MCP.Reconnect("default")
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("MCP handshake timeout after %dms", timeoutMs)
	}
}

func (d *ProductionRecoveryDeps) RestartPlugin(name string) error {
	if d.Plugins == nil {
		return fmt.Errorf("recovery: plugin manager not available (nil dependency)")
	}
	if err := d.Plugins.DisablePlugin(name); err != nil {
		return fmt.Errorf("disable plugin %q: %w", name, err)
	}
	if err := d.Plugins.EnablePlugin(name); err != nil {
		return fmt.Errorf("enable plugin %q: %w", name, err)
	}
	return nil
}

func (d *ProductionRecoveryDeps) RestartWorker() error {
	if d.Workers == nil {
		return fmt.Errorf("recovery: worker registry not available (nil dependency)")
	}
	return d.Workers.Restart("default")
}

// ---------------------------------------------------------------------------
// FromLaneEvent
// ---------------------------------------------------------------------------

// FromLaneEvent maps a lane event type string to a FailureScenario.
// Returns the scenario and true if a mapping exists, or (0, false) otherwise.
func FromLaneEvent(eventType string) (FailureScenario, bool) {
	switch eventType {
	case "trust_prompt_unresolved":
		return ScenarioTrustPromptUnresolved, true
	case "prompt_misdelivery":
		return ScenarioPromptMisdelivery, true
	case "stale_branch":
		return ScenarioStaleBranch, true
	case "compile_failure":
		return ScenarioCompileRedCrossCrate, true
	case "mcp_handshake_failure":
		return ScenarioMcpHandshakeFailure, true
	case "plugin_startup_failure":
		return ScenarioPartialPluginStartup, true
	case "provider_failure":
		return ScenarioProviderFailure, true
	default:
		return 0, false
	}
}
