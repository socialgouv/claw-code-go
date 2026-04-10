// Package lane implements lane-level event tracking, commit provenance,
// branch locking, and freshness analysis for multi-lane orchestration.
package lane

import "encoding/json"

// LaneEventName identifies the type of lane event on the wire.
type LaneEventName string

const (
	EventStarted                LaneEventName = "lane.started"
	EventReady                  LaneEventName = "lane.ready"
	EventPromptMisdelivery      LaneEventName = "lane.prompt_misdelivery"
	EventBlocked                LaneEventName = "lane.blocked"
	EventRed                    LaneEventName = "lane.red"
	EventGreen                  LaneEventName = "lane.green"
	EventCommitCreated          LaneEventName = "lane.commit.created"
	EventPrOpened               LaneEventName = "lane.pr.opened"
	EventMergeReady             LaneEventName = "lane.merge.ready"
	EventFinished               LaneEventName = "lane.finished"
	EventFailed                 LaneEventName = "lane.failed"
	EventReconciled             LaneEventName = "lane.reconciled"
	EventMerged                 LaneEventName = "lane.merged"
	EventSuperseded             LaneEventName = "lane.superseded"
	EventClosed                 LaneEventName = "lane.closed"
	EventBranchStaleAgainstMain LaneEventName = "branch.stale_against_main"
)

// LaneEventStatus describes the lane's current status.
type LaneEventStatus string

const (
	StatusRunning    LaneEventStatus = "running"
	StatusReady      LaneEventStatus = "ready"
	StatusBlocked    LaneEventStatus = "blocked"
	StatusRed        LaneEventStatus = "red"
	StatusGreen      LaneEventStatus = "green"
	StatusCompleted  LaneEventStatus = "completed"
	StatusFailed     LaneEventStatus = "failed"
	StatusReconciled LaneEventStatus = "reconciled"
	StatusMerged     LaneEventStatus = "merged"
	StatusSuperseded LaneEventStatus = "superseded"
	StatusClosed     LaneEventStatus = "closed"
)

// LaneFailureClass categorizes the type of failure.
type LaneFailureClass string

const (
	FailurePromptDelivery   LaneFailureClass = "prompt_delivery"
	FailureTrustGate        LaneFailureClass = "trust_gate"
	FailureBranchDivergence LaneFailureClass = "branch_divergence"
	FailureCompile          LaneFailureClass = "compile"
	FailureTest             LaneFailureClass = "test"
	FailurePluginStartup    LaneFailureClass = "plugin_startup"
	FailureMcpStartup       LaneFailureClass = "mcp_startup"
	FailureMcpHandshake     LaneFailureClass = "mcp_handshake"
	FailureGatewayRouting   LaneFailureClass = "gateway_routing"
	FailureToolRuntime      LaneFailureClass = "tool_runtime"
	FailureInfra            LaneFailureClass = "infra"
)

// LaneEventBlocker describes what is blocking a lane.
type LaneEventBlocker struct {
	FailureClass LaneFailureClass `json:"failureClass"`
	Detail       string           `json:"detail"`
}

// LaneCommitProvenance tracks the provenance of a commit within a lane.
type LaneCommitProvenance struct {
	Commit          string   `json:"commit"`
	Branch          string   `json:"branch"`
	Worktree        *string  `json:"worktree,omitempty"`
	CanonicalCommit *string  `json:"canonicalCommit,omitempty"`
	SupersededBy    *string  `json:"supersededBy,omitempty"`
	Lineage         []string `json:"lineage,omitempty"`
}

// LaneEvent is a single append-only lane event record.
type LaneEvent struct {
	Event        LaneEventName     `json:"event"`
	Status       LaneEventStatus   `json:"status"`
	EmittedAt    string            `json:"emittedAt"`
	FailureClass *LaneFailureClass `json:"failureClass,omitempty"`
	Detail       *string           `json:"detail,omitempty"`
	Data         json.RawMessage   `json:"data,omitempty"`
}

// NewLaneEvent creates a base LaneEvent.
func NewLaneEvent(event LaneEventName, status LaneEventStatus, emittedAt string) LaneEvent {
	return LaneEvent{
		Event:     event,
		Status:    status,
		EmittedAt: emittedAt,
	}
}

// Started creates a lane.started event.
func Started(emittedAt string) LaneEvent {
	return NewLaneEvent(EventStarted, StatusRunning, emittedAt)
}

// Finished creates a lane.finished event.
func Finished(emittedAt string, detail *string) LaneEvent {
	e := NewLaneEvent(EventFinished, StatusCompleted, emittedAt)
	e.Detail = detail
	return e
}

// CommitCreated creates a lane.commit.created event with provenance data.
func CommitCreated(emittedAt string, detail *string, provenance LaneCommitProvenance) LaneEvent {
	e := NewLaneEvent(EventCommitCreated, StatusCompleted, emittedAt)
	e.Detail = detail
	data, _ := json.Marshal(provenance)
	e.Data = data
	return e
}

// Superseded creates a lane.superseded event with provenance data.
func Superseded(emittedAt string, detail *string, provenance LaneCommitProvenance) LaneEvent {
	e := NewLaneEvent(EventSuperseded, StatusSuperseded, emittedAt)
	e.Detail = detail
	data, _ := json.Marshal(provenance)
	e.Data = data
	return e
}

// Blocked creates a lane.blocked event from a blocker.
func Blocked(emittedAt string, blocker *LaneEventBlocker) LaneEvent {
	e := NewLaneEvent(EventBlocked, StatusBlocked, emittedAt)
	fc := blocker.FailureClass
	e.FailureClass = &fc
	d := blocker.Detail
	e.Detail = &d
	return e
}

// Failed creates a lane.failed event from a blocker.
func Failed(emittedAt string, blocker *LaneEventBlocker) LaneEvent {
	e := NewLaneEvent(EventFailed, StatusFailed, emittedAt)
	fc := blocker.FailureClass
	e.FailureClass = &fc
	d := blocker.Detail
	e.Detail = &d
	return e
}

// DedupeSupersededCommitEvents removes superseded commit events,
// keeping only the latest event per canonical commit key.
func DedupeSupersededCommitEvents(events []LaneEvent) []LaneEvent {
	keep := make([]bool, len(events))
	for i := range keep {
		keep[i] = true
	}

	latestByKey := make(map[string]int) // canonical commit -> index of latest

	for i, event := range events {
		if event.Event != EventCommitCreated {
			continue
		}
		if event.Data == nil {
			continue
		}
		var data map[string]interface{}
		if json.Unmarshal(event.Data, &data) != nil {
			continue
		}

		// Check if superseded
		if _, ok := data["supersededBy"]; ok {
			if v, isStr := data["supersededBy"].(string); isStr && v != "" {
				keep[i] = false
				continue
			}
		}

		// Find the key (canonicalCommit or commit)
		var key string
		if cc, ok := data["canonicalCommit"].(string); ok && cc != "" {
			key = cc
		} else if c, ok := data["commit"].(string); ok && c != "" {
			key = c
		}
		if key != "" {
			if prev, exists := latestByKey[key]; exists {
				keep[prev] = false
			}
			latestByKey[key] = i
		}
	}

	var result []LaneEvent
	for i, event := range events {
		if keep[i] {
			result = append(result, event)
		}
	}
	return result
}
