package policy

import (
	"testing"
	"time"
)

func defaultContext() *LaneContext {
	return NewLaneContext(
		"lane-7",
		0,
		0,
		BlockerNone,
		ReviewPending,
		DiffFull,
		false,
	)
}

func TestMergeToDevRuleFiresForGreenScopedReviewedLane(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule(
			"merge-to-dev",
			&ConditionAnd{Conditions: []PolicyCondition{
				&ConditionGreenAt{Level: 2},
				&ConditionScopedDiff{},
				&ConditionReviewPassed{},
			}},
			MergeToDev(),
			20,
		),
	})
	ctx := NewLaneContext("lane-7", 3, 5*time.Second, BlockerNone, ReviewApproved, DiffScoped, false)

	actions := engine.Evaluate(ctx)

	if len(actions) != 1 || actions[0].Kind != ActionMergeToDev {
		t.Fatalf("expected [MergeToDev], got %+v", actions)
	}
}

func TestStaleBranchRuleFiresAtThreshold(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule("merge-forward", &ConditionStaleBranch{}, MergeForward(), 10),
	})
	ctx := NewLaneContext("lane-7", 1, StaleBranchThreshold, BlockerNone, ReviewPending, DiffFull, false)

	actions := engine.Evaluate(ctx)

	if len(actions) != 1 || actions[0].Kind != ActionMergeForward {
		t.Fatalf("expected [MergeForward], got %+v", actions)
	}
}

func TestStartupBlockedRuleRecoversAndEscalates(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule(
			"startup-recovery",
			&ConditionStartupBlocked{},
			Chain(RecoverOnce(), Escalate("startup remained blocked")),
			15,
		),
	})
	ctx := NewLaneContext("lane-7", 0, 0, BlockerStartup, ReviewPending, DiffFull, false)

	actions := engine.Evaluate(ctx)

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d: %+v", len(actions), actions)
	}
	if actions[0].Kind != ActionRecoverOnce {
		t.Errorf("actions[0] = %s, want recover_once", actions[0].Kind)
	}
	if actions[1].Kind != ActionEscalate || actions[1].Reason != "startup remained blocked" {
		t.Errorf("actions[1] = %+v, want escalate", actions[1])
	}
}

func TestCompletedLaneCloseoutAndCleanup(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule(
			"lane-closeout",
			&ConditionLaneCompleted{},
			Chain(CloseoutLane(), CleanupSession()),
			30,
		),
	})
	ctx := NewLaneContext("lane-7", 0, 0, BlockerNone, ReviewPending, DiffFull, true)

	actions := engine.Evaluate(ctx)

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Kind != ActionCloseoutLane {
		t.Errorf("actions[0] = %s, want closeout_lane", actions[0].Kind)
	}
	if actions[1].Kind != ActionCleanupSession {
		t.Errorf("actions[1] = %s, want cleanup_session", actions[1].Kind)
	}
}

func TestPriorityOrderWithStableTies(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule("late-cleanup", &ConditionAnd{}, CleanupSession(), 30),
		NewRule("first-notify", &ConditionAnd{}, Notify("ops"), 10),
		NewRule("second-notify", &ConditionAnd{}, Notify("review"), 10),
		NewRule("merge", &ConditionAnd{}, MergeToDev(), 20),
	})
	ctx := defaultContext()

	actions := Evaluate(engine, ctx)

	expected := []struct {
		kind   PolicyActionKind
		reason string
	}{
		{ActionNotify, "ops"},
		{ActionNotify, "review"},
		{ActionMergeToDev, ""},
		{ActionCleanupSession, ""},
	}
	if len(actions) != len(expected) {
		t.Fatalf("expected %d actions, got %d: %+v", len(expected), len(actions), actions)
	}
	for i, exp := range expected {
		if actions[i].Kind != exp.kind {
			t.Errorf("actions[%d].Kind = %s, want %s", i, actions[i].Kind, exp.kind)
		}
		if exp.reason != "" && actions[i].Reason != exp.reason {
			t.Errorf("actions[%d].Reason = %q, want %q", i, actions[i].Reason, exp.reason)
		}
	}
}

func TestCombinatorsHandleEmptyCasesAndNestedChains(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule("empty-and", &ConditionAnd{}, Notify("orchestrator"), 5),
		NewRule("empty-or", &ConditionOr{}, Block("should not fire"), 10),
		NewRule("nested",
			&ConditionOr{Conditions: []PolicyCondition{
				&ConditionStartupBlocked{},
				&ConditionAnd{Conditions: []PolicyCondition{
					&ConditionGreenAt{Level: 2},
					&ConditionTimedOut{Duration: 5 * time.Second},
				}},
			}},
			Chain(Notify("alerts"), Chain(MergeForward(), CleanupSession())),
			15,
		),
	})
	ctx := NewLaneContext("lane-7", 2, 10*time.Second, BlockerExternal, ReviewPending, DiffFull, false)

	actions := engine.Evaluate(ctx)

	expected := []PolicyActionKind{ActionNotify, ActionNotify, ActionMergeForward, ActionCleanupSession}
	if len(actions) != len(expected) {
		t.Fatalf("expected %d actions, got %d: %+v", len(expected), len(actions), actions)
	}
	for i, kind := range expected {
		if actions[i].Kind != kind {
			t.Errorf("actions[%d].Kind = %s, want %s", i, actions[i].Kind, kind)
		}
	}
	if actions[0].Reason != "orchestrator" {
		t.Errorf("actions[0] channel = %q, want orchestrator", actions[0].Reason)
	}
	if actions[1].Reason != "alerts" {
		t.Errorf("actions[1] channel = %q, want alerts", actions[1].Reason)
	}
}

func TestReconciledLaneEmitsReconcileAndCleanup(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule(
			"reconcile-closeout",
			&ConditionLaneReconciled{},
			Chain(Reconcile(ReconcileAlreadyMerged), CloseoutLane(), CleanupSession()),
			5,
		),
		NewRule(
			"generic-closeout",
			&ConditionAnd{Conditions: []PolicyCondition{
				&ConditionLaneCompleted{},
				&ConditionAnd{},
			}},
			CloseoutLane(),
			30,
		),
	})
	ctx := ReconciledContext("lane-9411")

	actions := engine.Evaluate(ctx)

	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d: %+v", len(actions), actions)
	}
	if actions[0].Kind != ActionReconcile || actions[0].ReconcileReason != ReconcileAlreadyMerged {
		t.Errorf("actions[0] = %+v, want reconcile(already_merged)", actions[0])
	}
	if actions[1].Kind != ActionCloseoutLane {
		t.Errorf("actions[1] = %s, want closeout_lane", actions[1].Kind)
	}
	if actions[2].Kind != ActionCleanupSession {
		t.Errorf("actions[2] = %s, want cleanup_session", actions[2].Kind)
	}
	if actions[3].Kind != ActionCloseoutLane {
		t.Errorf("actions[3] = %s, want closeout_lane", actions[3].Kind)
	}
}

func TestReconciledContextDefaults(t *testing.T) {
	ctx := ReconciledContext("test-lane")
	if ctx.LaneID != "test-lane" {
		t.Errorf("LaneID = %q", ctx.LaneID)
	}
	if !ctx.Completed {
		t.Error("expected Completed = true")
	}
	if !ctx.Reconciled {
		t.Error("expected Reconciled = true")
	}
	if ctx.Blocker != BlockerNone {
		t.Errorf("Blocker = %s", ctx.Blocker)
	}
	if ctx.GreenLevel != 0 {
		t.Errorf("GreenLevel = %d", ctx.GreenLevel)
	}
}

func TestNonReconciledLaneDoesNotTriggerReconcileRule(t *testing.T) {
	engine := NewEngine([]PolicyRule{
		NewRule(
			"reconcile-closeout",
			&ConditionLaneReconciled{},
			Reconcile(ReconcileEmptyDiff),
			5,
		),
	})
	ctx := NewLaneContext("lane-7", 0, 0, BlockerNone, ReviewPending, DiffFull, true)

	actions := engine.Evaluate(ctx)

	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %+v", actions)
	}
}

func TestReconcileReasonVariantsDistinct(t *testing.T) {
	if ReconcileAlreadyMerged == ReconcileSuperseded {
		t.Error("AlreadyMerged should not equal Superseded")
	}
	if ReconcileEmptyDiff == ReconcileManualClose {
		t.Error("EmptyDiff should not equal ManualClose")
	}
}
