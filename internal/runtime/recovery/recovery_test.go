package recovery

import (
	"claw-code-go/internal/runtime/worker"
	"encoding/json"
	"fmt"
	"testing"
)

func TestRecipeForAllScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scenario        FailureScenario
		wantStepNames   []string
		wantMaxAttempts uint32
		wantEscalation  EscalationPolicy
	}{
		{
			ScenarioTrustPromptUnresolved,
			[]string{"accept_trust_prompt"},
			1,
			PolicyAlertHuman,
		},
		{
			ScenarioPromptMisdelivery,
			[]string{"redirect_prompt_to_agent"},
			1,
			PolicyAlertHuman,
		},
		{
			ScenarioStaleBranch,
			[]string{"rebase_branch", "clean_build"},
			1,
			PolicyAlertHuman,
		},
		{
			ScenarioCompileRedCrossCrate,
			[]string{"clean_build"},
			1,
			PolicyAlertHuman,
		},
		{
			ScenarioMcpHandshakeFailure,
			[]string{"retry_mcp_handshake"},
			1,
			PolicyAbort,
		},
		{
			ScenarioPartialPluginStartup,
			[]string{"restart_plugin", "retry_mcp_handshake"},
			1,
			PolicyLogAndContinue,
		},
		{
			ScenarioProviderFailure,
			[]string{"restart_worker"},
			1,
			PolicyAlertHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.scenario.String(), func(t *testing.T) {
			t.Parallel()
			recipe := RecipeFor(tt.scenario)

			if recipe.Scenario != tt.scenario {
				t.Errorf("Scenario = %v, want %v", recipe.Scenario, tt.scenario)
			}
			if recipe.MaxAttempts != tt.wantMaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", recipe.MaxAttempts, tt.wantMaxAttempts)
			}
			if recipe.EscalationPolicy != tt.wantEscalation {
				t.Errorf("EscalationPolicy = %v, want %v", recipe.EscalationPolicy, tt.wantEscalation)
			}
			if len(recipe.Steps) != len(tt.wantStepNames) {
				t.Fatalf("len(Steps) = %d, want %d", len(recipe.Steps), len(tt.wantStepNames))
			}
			for i, step := range recipe.Steps {
				if step.StepName() != tt.wantStepNames[i] {
					t.Errorf("Steps[%d].StepName() = %q, want %q", i, step.StepName(), tt.wantStepNames[i])
				}
			}
		})
	}
}

func TestAllScenariosMaxAttemptsOne(t *testing.T) {
	t.Parallel()
	for _, s := range AllScenarios() {
		recipe := RecipeFor(s)
		if recipe.MaxAttempts != 1 {
			t.Errorf("RecipeFor(%v).MaxAttempts = %d, want 1", s, recipe.MaxAttempts)
		}
	}
}

func TestAttemptRecoveryFreshRecovered(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	result := AttemptRecovery(ScenarioTrustPromptUnresolved, ctx)

	rec, ok := result.(Recovered)
	if !ok {
		t.Fatalf("expected Recovered, got %T", result)
	}
	if rec.StepsTaken != 1 {
		t.Errorf("StepsTaken = %d, want 1", rec.StepsTaken)
	}
	if rec.ResultKind() != "recovered" {
		t.Errorf("ResultKind = %q, want %q", rec.ResultKind(), "recovered")
	}

	// Check events: RecoveryAttempted + RecoverySucceeded
	events := ctx.Events()
	if len(events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(events))
	}
	if _, ok := events[0].(RecoveryAttempted); !ok {
		t.Errorf("events[0] type = %T, want RecoveryAttempted", events[0])
	}
	if _, ok := events[1].(RecoverySucceeded); !ok {
		t.Errorf("events[1] type = %T, want RecoverySucceeded", events[1])
	}
}

func TestAttemptRecoveryExceedsMaxAttempts(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()

	// First attempt succeeds.
	r1 := AttemptRecovery(ScenarioProviderFailure, ctx)
	if _, ok := r1.(Recovered); !ok {
		t.Fatalf("first attempt: expected Recovered, got %T", r1)
	}

	// Second attempt should escalate (max_attempts=1).
	r2 := AttemptRecovery(ScenarioProviderFailure, ctx)
	esc, ok := r2.(EscalationRequired)
	if !ok {
		t.Fatalf("second attempt: expected EscalationRequired, got %T", r2)
	}
	if esc.ResultKind() != "escalation_required" {
		t.Errorf("ResultKind = %q, want %q", esc.ResultKind(), "escalation_required")
	}

	// Should have Escalated event.
	events := ctx.Events()
	lastEvent := events[len(events)-1]
	if _, ok := lastEvent.(Escalated); !ok {
		t.Errorf("last event type = %T, want Escalated", lastEvent)
	}
}

func TestAttemptRecoveryFailAtStepZero(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContextWithFailAt(0)
	result := AttemptRecovery(ScenarioStaleBranch, ctx)

	_, ok := result.(EscalationRequired)
	if !ok {
		t.Fatalf("expected EscalationRequired, got %T", result)
	}

	// Should have Escalated event (Rust emits Escalated for all EscalationRequired results).
	events := ctx.Events()
	if len(events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(events))
	}
	if _, ok := events[1].(Escalated); !ok {
		t.Errorf("events[1] type = %T, want Escalated", events[1])
	}
}

func TestAttemptRecoveryFailAtStepOnePartial(t *testing.T) {
	t.Parallel()
	// StaleBranch has 2 steps: [RebaseBranch, CleanBuild]
	ctx := NewRecoveryContextWithFailAt(1)
	result := AttemptRecovery(ScenarioStaleBranch, ctx)

	pr, ok := result.(PartialRecovery)
	if !ok {
		t.Fatalf("expected PartialRecovery, got %T", result)
	}
	if len(pr.RecoveredSteps) != 1 {
		t.Errorf("len(RecoveredSteps) = %d, want 1", len(pr.RecoveredSteps))
	}
	if len(pr.Remaining) != 1 {
		t.Errorf("len(Remaining) = %d, want 1", len(pr.Remaining))
	}
	if pr.RecoveredSteps[0].StepName() != "rebase_branch" {
		t.Errorf("RecoveredSteps[0] = %q, want %q", pr.RecoveredSteps[0].StepName(), "rebase_branch")
	}
	if pr.Remaining[0].StepName() != "clean_build" {
		t.Errorf("Remaining[0] = %q, want %q", pr.Remaining[0].StepName(), "clean_build")
	}
	if pr.ResultKind() != "partial_recovery" {
		t.Errorf("ResultKind = %q, want %q", pr.ResultKind(), "partial_recovery")
	}
}

func TestFromWorkerFailureKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind string
		want FailureScenario
	}{
		{"trust_gate", ScenarioTrustPromptUnresolved},
		{"prompt_delivery", ScenarioPromptMisdelivery},
		{"protocol", ScenarioMcpHandshakeFailure},
		{"provider", ScenarioProviderFailure},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			got := FromWorkerFailureKind(tt.kind)
			if got != tt.want {
				t.Errorf("FromWorkerFailureKind(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestFromWorkerFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind worker.WorkerFailureKind
		want FailureScenario
	}{
		{worker.FailureTrustGate, ScenarioTrustPromptUnresolved},
		{worker.FailurePromptDelivery, ScenarioPromptMisdelivery},
		{worker.FailureProtocol, ScenarioMcpHandshakeFailure},
		{worker.FailureProvider, ScenarioProviderFailure},
	}
	for _, tt := range tests {
		got := FromWorkerFailure(tt.kind)
		if got != tt.want {
			t.Errorf("FromWorkerFailure(%v) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestEventsAccumulateAcrossScenarios(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()

	AttemptRecovery(ScenarioTrustPromptUnresolved, ctx)
	AttemptRecovery(ScenarioProviderFailure, ctx)
	AttemptRecovery(ScenarioMcpHandshakeFailure, ctx)

	// Each successful attempt emits 2 events (Attempted + Succeeded).
	events := ctx.Events()
	if len(events) != 6 {
		t.Errorf("len(Events) = %d, want 6", len(events))
	}

	// Verify attempt counts.
	if ctx.AttemptCount(ScenarioTrustPromptUnresolved) != 1 {
		t.Errorf("AttemptCount(TrustPromptUnresolved) = %d, want 1", ctx.AttemptCount(ScenarioTrustPromptUnresolved))
	}
	if ctx.AttemptCount(ScenarioProviderFailure) != 1 {
		t.Errorf("AttemptCount(ProviderFailure) = %d, want 1", ctx.AttemptCount(ScenarioProviderFailure))
	}
}

func TestCorrectEscalationPolicies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scenario FailureScenario
		want     EscalationPolicy
	}{
		{ScenarioTrustPromptUnresolved, PolicyAlertHuman},
		{ScenarioPromptMisdelivery, PolicyAlertHuman},
		{ScenarioStaleBranch, PolicyAlertHuman},
		{ScenarioCompileRedCrossCrate, PolicyAlertHuman},
		{ScenarioMcpHandshakeFailure, PolicyAbort},
		{ScenarioPartialPluginStartup, PolicyLogAndContinue},
		{ScenarioProviderFailure, PolicyAlertHuman},
	}
	for _, tt := range tests {
		t.Run(tt.scenario.String(), func(t *testing.T) {
			t.Parallel()
			recipe := RecipeFor(tt.scenario)
			if recipe.EscalationPolicy != tt.want {
				t.Errorf("EscalationPolicy = %v, want %v", recipe.EscalationPolicy, tt.want)
			}
		})
	}
}

func TestAllScenariosCount(t *testing.T) {
	t.Parallel()
	scenarios := AllScenarios()
	if len(scenarios) != 7 {
		t.Errorf("len(AllScenarios()) = %d, want 7", len(scenarios))
	}
}

func TestFailureScenarioJSONRoundTrip(t *testing.T) {
	t.Parallel()
	for _, s := range AllScenarios() {
		t.Run(s.String(), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got FailureScenario
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got != s {
				t.Errorf("round-trip: got %v, want %v", got, s)
			}
		})
	}
}

func TestEscalationPolicyJSONRoundTrip(t *testing.T) {
	t.Parallel()
	policies := []EscalationPolicy{PolicyAlertHuman, PolicyLogAndContinue, PolicyAbort}
	for _, p := range policies {
		t.Run(p.String(), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got EscalationPolicy
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got != p {
				t.Errorf("round-trip: got %v, want %v", got, p)
			}
		})
	}
}

func TestPartialPluginStartupRecipeDetails(t *testing.T) {
	t.Parallel()
	recipe := RecipeFor(ScenarioPartialPluginStartup)
	if len(recipe.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(recipe.Steps))
	}

	rp, ok := recipe.Steps[0].(RestartPlugin)
	if !ok {
		t.Fatalf("Steps[0] type = %T, want RestartPlugin", recipe.Steps[0])
	}
	if rp.Name != "stalled" {
		t.Errorf("RestartPlugin.Name = %q, want %q", rp.Name, "stalled")
	}

	mcp, ok := recipe.Steps[1].(RetryMcpHandshake)
	if !ok {
		t.Fatalf("Steps[1] type = %T, want RetryMcpHandshake", recipe.Steps[1])
	}
	if mcp.Timeout != 3000 {
		t.Errorf("RetryMcpHandshake.Timeout = %d, want 3000", mcp.Timeout)
	}
}

func TestMcpHandshakeRecipeTimeout(t *testing.T) {
	t.Parallel()
	recipe := RecipeFor(ScenarioMcpHandshakeFailure)
	if len(recipe.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(recipe.Steps))
	}
	mcp, ok := recipe.Steps[0].(RetryMcpHandshake)
	if !ok {
		t.Fatalf("Steps[0] type = %T, want RetryMcpHandshake", recipe.Steps[0])
	}
	if mcp.Timeout != 5000 {
		t.Errorf("RetryMcpHandshake.Timeout = %d, want 5000", mcp.Timeout)
	}
}

func TestRecoveryEventKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		event RecoveryEvent
		want  string
	}{
		{RecoveryAttempted{}, "recovery_attempted"},
		{RecoverySucceeded{}, "recovery_succeeded"},
		{RecoveryFailed{}, "recovery_failed"},
		{Escalated{}, "escalated"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.event.EventKind(); got != tt.want {
				t.Errorf("EventKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RecoveryDeps tests
// ---------------------------------------------------------------------------

// mockDeps implements RecoveryDeps and records calls.
type mockDeps struct {
	calls     []string
	failAfter int // fail after this many calls (-1 = never fail)
}

func newMockDeps(failAfter int) *mockDeps {
	return &mockDeps{failAfter: failAfter}
}

func (m *mockDeps) maybeErr(step string) error {
	m.calls = append(m.calls, step)
	if m.failAfter >= 0 && len(m.calls) > m.failAfter {
		return fmt.Errorf("simulated failure at step %s", step)
	}
	return nil
}

func (m *mockDeps) AcceptTrust() error    { return m.maybeErr("accept_trust") }
func (m *mockDeps) RedirectPrompt() error { return m.maybeErr("redirect_prompt") }
func (m *mockDeps) RebaseBranch() error   { return m.maybeErr("rebase_branch") }
func (m *mockDeps) CleanBuild() error     { return m.maybeErr("clean_build") }
func (m *mockDeps) RetryMcpHandshake(timeoutMs uint64) error {
	return m.maybeErr("retry_mcp_handshake")
}
func (m *mockDeps) RestartPlugin(name string) error { return m.maybeErr("restart_plugin") }
func (m *mockDeps) RestartWorker() error            { return m.maybeErr("restart_worker") }

func TestAttemptRecoveryWithDepsSucceeds(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	deps := newMockDeps(-1) // never fail
	result := AttemptRecoveryWithDeps(ScenarioTrustPromptUnresolved, ctx, deps)

	rec, ok := result.(Recovered)
	if !ok {
		t.Fatalf("expected Recovered, got %T", result)
	}
	if rec.StepsTaken != 1 {
		t.Errorf("StepsTaken = %d, want 1", rec.StepsTaken)
	}
	if len(deps.calls) != 1 {
		t.Errorf("len(calls) = %d, want 1", len(deps.calls))
	}
	if deps.calls[0] != "accept_trust" {
		t.Errorf("calls[0] = %q, want accept_trust", deps.calls[0])
	}
}

func TestAttemptRecoveryWithDepsMultiStep(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	deps := newMockDeps(-1)
	result := AttemptRecoveryWithDeps(ScenarioStaleBranch, ctx, deps)

	rec, ok := result.(Recovered)
	if !ok {
		t.Fatalf("expected Recovered, got %T", result)
	}
	if rec.StepsTaken != 2 {
		t.Errorf("StepsTaken = %d, want 2", rec.StepsTaken)
	}
	if len(deps.calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(deps.calls))
	}
	if deps.calls[0] != "rebase_branch" {
		t.Errorf("calls[0] = %q, want rebase_branch", deps.calls[0])
	}
	if deps.calls[1] != "clean_build" {
		t.Errorf("calls[1] = %q, want clean_build", deps.calls[1])
	}
}

func TestAttemptRecoveryWithDepsFailsAtFirstStep(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	deps := newMockDeps(0) // fail on first call
	result := AttemptRecoveryWithDeps(ScenarioStaleBranch, ctx, deps)

	_, ok := result.(EscalationRequired)
	if !ok {
		t.Fatalf("expected EscalationRequired, got %T", result)
	}

	events := ctx.Events()
	if len(events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(events))
	}
	if _, ok := events[1].(Escalated); !ok {
		t.Errorf("events[1] type = %T, want Escalated", events[1])
	}
}

func TestAttemptRecoveryWithDepsPartialFailure(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	deps := newMockDeps(1) // fail after first call
	result := AttemptRecoveryWithDeps(ScenarioStaleBranch, ctx, deps)

	pr, ok := result.(PartialRecovery)
	if !ok {
		t.Fatalf("expected PartialRecovery, got %T", result)
	}
	if len(pr.RecoveredSteps) != 1 {
		t.Errorf("len(RecoveredSteps) = %d, want 1", len(pr.RecoveredSteps))
	}
	if len(pr.Remaining) != 1 {
		t.Errorf("len(Remaining) = %d, want 1", len(pr.Remaining))
	}
}

func TestAttemptRecoveryNilDepsSimulationPath(t *testing.T) {
	t.Parallel()
	// nil deps should use the simulation path (backward compat)
	ctx := NewRecoveryContext()
	result := AttemptRecoveryWithDeps(ScenarioProviderFailure, ctx, nil)

	_, ok := result.(Recovered)
	if !ok {
		t.Fatalf("expected Recovered with nil deps, got %T", result)
	}
}

func TestAttemptRecoveryWithDepsPluginStartup(t *testing.T) {
	t.Parallel()
	ctx := NewRecoveryContext()
	deps := newMockDeps(-1)
	result := AttemptRecoveryWithDeps(ScenarioPartialPluginStartup, ctx, deps)

	rec, ok := result.(Recovered)
	if !ok {
		t.Fatalf("expected Recovered, got %T", result)
	}
	if rec.StepsTaken != 2 {
		t.Errorf("StepsTaken = %d, want 2", rec.StepsTaken)
	}
	if len(deps.calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(deps.calls))
	}
	if deps.calls[0] != "restart_plugin" {
		t.Errorf("calls[0] = %q, want restart_plugin", deps.calls[0])
	}
	if deps.calls[1] != "retry_mcp_handshake" {
		t.Errorf("calls[1] = %q, want retry_mcp_handshake", deps.calls[1])
	}
}

// ---------------------------------------------------------------------------
// ProductionRecoveryDeps tests
// ---------------------------------------------------------------------------

type prodMockWorkerRegistry struct {
	trustResolved bool
	restarted     bool
	failOn        string
}

func (m *prodMockWorkerRegistry) ResolveTrust(id string) error {
	if m.failOn == "resolve_trust" {
		return fmt.Errorf("mock: failed")
	}
	m.trustResolved = true
	return nil
}
func (m *prodMockWorkerRegistry) Restart(id string) error {
	if m.failOn == "restart" {
		return fmt.Errorf("mock: failed")
	}
	m.restarted = true
	return nil
}
func (m *prodMockWorkerRegistry) SendPrompt(id, p string) error { return nil }

type prodMockMCPRegistry struct{ reconnected bool }

func (m *prodMockMCPRegistry) Reconnect(n string) error { m.reconnected = true; return nil }

type prodMockPluginManager struct{ disabled, enabled []string }

func (m *prodMockPluginManager) DisablePlugin(n string) error {
	m.disabled = append(m.disabled, n)
	return nil
}
func (m *prodMockPluginManager) EnablePlugin(n string) error {
	m.enabled = append(m.enabled, n)
	return nil
}

func TestProductionDepsAcceptTrust(t *testing.T) {
	t.Parallel()
	w := &prodMockWorkerRegistry{}
	d := &ProductionRecoveryDeps{Workers: w}
	if err := d.AcceptTrust(); err != nil {
		t.Fatal(err)
	}
	if !w.trustResolved {
		t.Error("not resolved")
	}
}

func TestProductionDepsNilGuards(t *testing.T) {
	t.Parallel()
	d := &ProductionRecoveryDeps{}
	fns := []func() error{
		d.AcceptTrust,
		d.RedirectPrompt,
		d.RestartWorker,
		func() error { return d.RetryMcpHandshake(100) },
		func() error { return d.RestartPlugin("x") },
	}
	for _, fn := range fns {
		if err := fn(); err == nil {
			t.Error("expected nil-guard error")
		}
	}
}

func TestProductionDepsAllScenarios(t *testing.T) {
	t.Parallel()
	for _, s := range AllScenarios() {
		t.Run(s.String(), func(t *testing.T) {
			d := &ProductionRecoveryDeps{
				Workers: &prodMockWorkerRegistry{},
				MCP:     &prodMockMCPRegistry{},
				Plugins: &prodMockPluginManager{},
			}
			ctx := NewRecoveryContext()
			r := AttemptRecoveryWithDeps(s, ctx, d)
			if r.ResultKind() != "recovered" {
				t.Errorf("got %q", r.ResultKind())
			}
		})
	}
}

func TestProductionDepsEscalation(t *testing.T) {
	t.Parallel()
	d := &ProductionRecoveryDeps{Workers: &prodMockWorkerRegistry{}}
	ctx := NewRecoveryContext()
	AttemptRecoveryWithDeps(ScenarioProviderFailure, ctx, d)
	r := AttemptRecoveryWithDeps(ScenarioProviderFailure, ctx, d)
	if r.ResultKind() != "escalation_required" {
		t.Errorf("got %q", r.ResultKind())
	}
}

func TestProductionDepsRestartPlugin(t *testing.T) {
	t.Parallel()
	p := &prodMockPluginManager{}
	d := &ProductionRecoveryDeps{Plugins: p}
	if err := d.RestartPlugin("test"); err != nil {
		t.Fatal(err)
	}
	if len(p.disabled) != 1 || p.disabled[0] != "test" {
		t.Errorf("disabled: %v", p.disabled)
	}
	if len(p.enabled) != 1 || p.enabled[0] != "test" {
		t.Errorf("enabled: %v", p.enabled)
	}
}

func TestProductionDepsFailingStep(t *testing.T) {
	t.Parallel()
	d := &ProductionRecoveryDeps{Workers: &prodMockWorkerRegistry{failOn: "resolve_trust"}}
	ctx := NewRecoveryContext()
	r := AttemptRecoveryWithDeps(ScenarioTrustPromptUnresolved, ctx, d)
	if r.ResultKind() != "escalation_required" {
		t.Errorf("got %q", r.ResultKind())
	}
}

func TestProductionDepsWorkDir(t *testing.T) {
	t.Parallel()
	d := &ProductionRecoveryDeps{
		WorkDir: "/tmp/test-workdir",
	}
	// We can't run actual git commands, but we verify the field is set.
	if d.WorkDir != "/tmp/test-workdir" {
		t.Errorf("WorkDir = %q, want /tmp/test-workdir", d.WorkDir)
	}
}

func TestFromLaneEventMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ev string
		sc FailureScenario
		ok bool
	}{
		{"trust_prompt_unresolved", ScenarioTrustPromptUnresolved, true},
		{"prompt_misdelivery", ScenarioPromptMisdelivery, true},
		{"stale_branch", ScenarioStaleBranch, true},
		{"compile_failure", ScenarioCompileRedCrossCrate, true},
		{"mcp_handshake_failure", ScenarioMcpHandshakeFailure, true},
		{"plugin_startup_failure", ScenarioPartialPluginStartup, true},
		{"provider_failure", ScenarioProviderFailure, true},
		{"unknown", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := FromLaneEvent(c.ev)
		if ok != c.ok {
			t.Errorf("FromLaneEvent(%q): ok=%v, want %v", c.ev, ok, c.ok)
		}
		if ok && got != c.sc {
			t.Errorf("FromLaneEvent(%q): got %s, want %s", c.ev, got, c.sc)
		}
	}
}
