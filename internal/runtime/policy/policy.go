// Package policy implements a declarative rule engine for lane-level runtime
// policy evaluation. Rules are evaluated in priority order; all matching rules
// produce actions, and Chain actions are recursively flattened.
package policy

import (
	"sort"
	"time"
)

// GreenLevel is a severity/quality level indicator (0 = lowest).
type GreenLevel = uint8

// StaleBranchThreshold is the default duration after which a branch is
// considered stale for the StaleBranch condition.
const StaleBranchThreshold = time.Hour

// --- Condition types ---

// PolicyCondition is an evaluable predicate over a LaneContext.
type PolicyCondition interface {
	Matches(ctx *LaneContext) bool
}

// ConditionAnd matches when all children match. An empty list evaluates to true.
type ConditionAnd struct {
	Conditions []PolicyCondition
}

func (c *ConditionAnd) Matches(ctx *LaneContext) bool {
	for _, child := range c.Conditions {
		if !child.Matches(ctx) {
			return false
		}
	}
	return true
}

// ConditionOr matches when any child matches. An empty list evaluates to false.
type ConditionOr struct {
	Conditions []PolicyCondition
}

func (c *ConditionOr) Matches(ctx *LaneContext) bool {
	for _, child := range c.Conditions {
		if child.Matches(ctx) {
			return true
		}
	}
	return false
}

// ConditionGreenAt matches when the context green level >= the given threshold.
type ConditionGreenAt struct {
	Level GreenLevel
}

func (c *ConditionGreenAt) Matches(ctx *LaneContext) bool {
	return ctx.GreenLevel >= c.Level
}

// ConditionStaleBranch matches when branch freshness >= StaleBranchThreshold.
type ConditionStaleBranch struct{}

func (c *ConditionStaleBranch) Matches(ctx *LaneContext) bool {
	return ctx.BranchFreshness >= StaleBranchThreshold
}

// ConditionStartupBlocked matches when the blocker is Startup.
type ConditionStartupBlocked struct{}

func (c *ConditionStartupBlocked) Matches(ctx *LaneContext) bool {
	return ctx.Blocker == BlockerStartup
}

// ConditionLaneCompleted matches when the lane is completed.
type ConditionLaneCompleted struct{}

func (c *ConditionLaneCompleted) Matches(ctx *LaneContext) bool {
	return ctx.Completed
}

// ConditionLaneReconciled matches when the lane is reconciled.
type ConditionLaneReconciled struct{}

func (c *ConditionLaneReconciled) Matches(ctx *LaneContext) bool {
	return ctx.Reconciled
}

// ConditionReviewPassed matches when review status is Approved.
type ConditionReviewPassed struct{}

func (c *ConditionReviewPassed) Matches(ctx *LaneContext) bool {
	return ctx.ReviewStatus == ReviewApproved
}

// ConditionScopedDiff matches when the diff scope is Scoped.
type ConditionScopedDiff struct{}

func (c *ConditionScopedDiff) Matches(ctx *LaneContext) bool {
	return ctx.DiffScope == DiffScoped
}

// ConditionTimedOut matches when branch freshness >= the given duration.
type ConditionTimedOut struct {
	Duration time.Duration
}

func (c *ConditionTimedOut) Matches(ctx *LaneContext) bool {
	return ctx.BranchFreshness >= c.Duration
}

// --- Context enums ---

// LaneBlocker describes what is blocking a lane.
type LaneBlocker string

const (
	BlockerNone     LaneBlocker = "none"
	BlockerStartup  LaneBlocker = "startup"
	BlockerExternal LaneBlocker = "external"
)

// ReviewStatus represents the review state of a lane.
type ReviewStatus string

const (
	ReviewPending  ReviewStatus = "pending"
	ReviewApproved ReviewStatus = "approved"
	ReviewRejected ReviewStatus = "rejected"
)

// DiffScope represents the scope of a lane's diff.
type DiffScope string

const (
	DiffFull   DiffScope = "full"
	DiffScoped DiffScope = "scoped"
)

// --- Lane context ---

// LaneContext holds all context needed for policy rule evaluation.
type LaneContext struct {
	LaneID          string
	GreenLevel      GreenLevel
	BranchFreshness time.Duration
	Blocker         LaneBlocker
	ReviewStatus    ReviewStatus
	DiffScope       DiffScope
	Completed       bool
	Reconciled      bool
}

// NewLaneContext creates a LaneContext with the given fields.
func NewLaneContext(
	laneID string,
	greenLevel GreenLevel,
	branchFreshness time.Duration,
	blocker LaneBlocker,
	reviewStatus ReviewStatus,
	diffScope DiffScope,
	completed bool,
) *LaneContext {
	return &LaneContext{
		LaneID:          laneID,
		GreenLevel:      greenLevel,
		BranchFreshness: branchFreshness,
		Blocker:         blocker,
		ReviewStatus:    reviewStatus,
		DiffScope:       diffScope,
		Completed:       completed,
		Reconciled:      false,
	}
}

// ReconciledContext creates a lane context that is already reconciled.
func ReconciledContext(laneID string) *LaneContext {
	return &LaneContext{
		LaneID:       laneID,
		GreenLevel:   0,
		Blocker:      BlockerNone,
		ReviewStatus: ReviewPending,
		DiffScope:    DiffFull,
		Completed:    true,
		Reconciled:   true,
	}
}

// --- Actions ---

// ReconcileReason describes why a lane was reconciled.
type ReconcileReason string

const (
	ReconcileAlreadyMerged ReconcileReason = "already_merged"
	ReconcileSuperseded    ReconcileReason = "superseded"
	ReconcileEmptyDiff     ReconcileReason = "empty_diff"
	ReconcileManualClose   ReconcileReason = "manual_close"
)

// PolicyAction is an action produced by a matching rule.
type PolicyAction struct {
	Kind            PolicyActionKind
	Reason          string          // for Escalate, Block, Notify (channel), Reconcile
	ReconcileReason ReconcileReason // for Reconcile
	Children        []PolicyAction  // for Chain
}

// PolicyActionKind identifies the type of action.
type PolicyActionKind string

const (
	ActionMergeToDev     PolicyActionKind = "merge_to_dev"
	ActionMergeForward   PolicyActionKind = "merge_forward"
	ActionRecoverOnce    PolicyActionKind = "recover_once"
	ActionEscalate       PolicyActionKind = "escalate"
	ActionCloseoutLane   PolicyActionKind = "closeout_lane"
	ActionCleanupSession PolicyActionKind = "cleanup_session"
	ActionReconcile      PolicyActionKind = "reconcile"
	ActionNotify         PolicyActionKind = "notify"
	ActionBlock          PolicyActionKind = "block"
	ActionChain          PolicyActionKind = "chain"
)

// Action constructors for convenience.

func MergeToDev() PolicyAction     { return PolicyAction{Kind: ActionMergeToDev} }
func MergeForward() PolicyAction   { return PolicyAction{Kind: ActionMergeForward} }
func RecoverOnce() PolicyAction    { return PolicyAction{Kind: ActionRecoverOnce} }
func CloseoutLane() PolicyAction   { return PolicyAction{Kind: ActionCloseoutLane} }
func CleanupSession() PolicyAction { return PolicyAction{Kind: ActionCleanupSession} }

func Escalate(reason string) PolicyAction {
	return PolicyAction{Kind: ActionEscalate, Reason: reason}
}

func Notify(channel string) PolicyAction {
	return PolicyAction{Kind: ActionNotify, Reason: channel}
}

func Block(reason string) PolicyAction {
	return PolicyAction{Kind: ActionBlock, Reason: reason}
}

func Reconcile(reason ReconcileReason) PolicyAction {
	return PolicyAction{Kind: ActionReconcile, ReconcileReason: reason}
}

func Chain(actions ...PolicyAction) PolicyAction {
	return PolicyAction{Kind: ActionChain, Children: actions}
}

// flattenInto recursively flattens Chain actions into a flat list.
func (a *PolicyAction) flattenInto(out *[]PolicyAction) {
	if a.Kind == ActionChain {
		for i := range a.Children {
			a.Children[i].flattenInto(out)
		}
	} else {
		*out = append(*out, *a)
	}
}

// --- Rule ---

// PolicyRule is a named rule with a condition, action, and priority.
type PolicyRule struct {
	Name      string
	Condition PolicyCondition
	Action    PolicyAction
	Priority  int
}

// NewRule creates a PolicyRule.
func NewRule(name string, condition PolicyCondition, action PolicyAction, priority int) PolicyRule {
	return PolicyRule{
		Name:      name,
		Condition: condition,
		Action:    action,
		Priority:  priority,
	}
}

// Matches returns true if the rule's condition matches the context.
func (r *PolicyRule) Matches(ctx *LaneContext) bool {
	return r.Condition.Matches(ctx)
}

// --- Engine ---

// PolicyEngine holds a priority-sorted list of rules and evaluates them
// against a LaneContext. All matching rules produce actions (collected in
// priority order). Chain actions are recursively flattened.
type PolicyEngine struct {
	rules []PolicyRule
}

// NewEngine creates a PolicyEngine, sorting rules by priority (ascending).
func NewEngine(rules []PolicyRule) *PolicyEngine {
	sorted := make([]PolicyRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return &PolicyEngine{rules: sorted}
}

// Rules returns the sorted rule list.
func (e *PolicyEngine) Rules() []PolicyRule {
	return e.rules
}

// Evaluate returns all actions from matching rules, in priority order,
// with Chain actions recursively flattened.
func (e *PolicyEngine) Evaluate(ctx *LaneContext) []PolicyAction {
	return Evaluate(e, ctx)
}

// Evaluate is the standalone evaluation function.
func Evaluate(engine *PolicyEngine, ctx *LaneContext) []PolicyAction {
	var actions []PolicyAction
	for i := range engine.rules {
		if engine.rules[i].Matches(ctx) {
			engine.rules[i].Action.flattenInto(&actions)
		}
	}
	return actions
}
