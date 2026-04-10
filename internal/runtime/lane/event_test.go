package lane

import (
	"encoding/json"
	"testing"
)

func TestLaneEventNamesSerialization(t *testing.T) {
	cases := []struct {
		name     LaneEventName
		expected string
	}{
		{EventStarted, "lane.started"},
		{EventReady, "lane.ready"},
		{EventPromptMisdelivery, "lane.prompt_misdelivery"},
		{EventBlocked, "lane.blocked"},
		{EventRed, "lane.red"},
		{EventGreen, "lane.green"},
		{EventCommitCreated, "lane.commit.created"},
		{EventPrOpened, "lane.pr.opened"},
		{EventMergeReady, "lane.merge.ready"},
		{EventFinished, "lane.finished"},
		{EventFailed, "lane.failed"},
		{EventReconciled, "lane.reconciled"},
		{EventMerged, "lane.merged"},
		{EventSuperseded, "lane.superseded"},
		{EventClosed, "lane.closed"},
		{EventBranchStaleAgainstMain, "branch.stale_against_main"},
	}

	for _, tc := range cases {
		data, err := json.Marshal(tc.name)
		if err != nil {
			t.Fatalf("marshal %s: %v", tc.name, err)
		}
		var got string
		json.Unmarshal(data, &got)
		if got != tc.expected {
			t.Errorf("LaneEventName %s serialized to %q, want %q", tc.name, got, tc.expected)
		}
	}
}

func TestFailureClassesSerialization(t *testing.T) {
	cases := []struct {
		fc       LaneFailureClass
		expected string
	}{
		{FailurePromptDelivery, "prompt_delivery"},
		{FailureTrustGate, "trust_gate"},
		{FailureBranchDivergence, "branch_divergence"},
		{FailureCompile, "compile"},
		{FailureTest, "test"},
		{FailurePluginStartup, "plugin_startup"},
		{FailureMcpStartup, "mcp_startup"},
		{FailureMcpHandshake, "mcp_handshake"},
		{FailureGatewayRouting, "gateway_routing"},
		{FailureToolRuntime, "tool_runtime"},
		{FailureInfra, "infra"},
	}

	for _, tc := range cases {
		data, err := json.Marshal(tc.fc)
		if err != nil {
			t.Fatalf("marshal %s: %v", tc.fc, err)
		}
		var got string
		json.Unmarshal(data, &got)
		if got != tc.expected {
			t.Errorf("LaneFailureClass %s serialized to %q, want %q", tc.fc, got, tc.expected)
		}
	}
}

func TestBlockedAndFailedEventsReuseBlockerDetails(t *testing.T) {
	blocker := &LaneEventBlocker{
		FailureClass: FailureMcpStartup,
		Detail:       "broken server",
	}

	blocked := Blocked("2026-04-04T00:00:00Z", blocker)
	failed := Failed("2026-04-04T00:00:01Z", blocker)

	if blocked.Event != EventBlocked {
		t.Errorf("blocked.Event = %s", blocked.Event)
	}
	if blocked.Status != StatusBlocked {
		t.Errorf("blocked.Status = %s", blocked.Status)
	}
	if blocked.FailureClass == nil || *blocked.FailureClass != FailureMcpStartup {
		t.Errorf("blocked.FailureClass = %v", blocked.FailureClass)
	}
	if failed.Event != EventFailed {
		t.Errorf("failed.Event = %s", failed.Event)
	}
	if failed.Status != StatusFailed {
		t.Errorf("failed.Status = %s", failed.Status)
	}
	if failed.Detail == nil || *failed.Detail != "broken server" {
		t.Errorf("failed.Detail = %v", failed.Detail)
	}
}

func TestCommitEventsCarryWorktreeAndSupersessionMetadata(t *testing.T) {
	wt := "wt-a"
	cc := "abc123"
	detail := "commit created"
	event := CommitCreated(
		"2026-04-04T00:00:00Z",
		&detail,
		LaneCommitProvenance{
			Commit:          "abc123",
			Branch:          "feature/provenance",
			Worktree:        &wt,
			CanonicalCommit: &cc,
			Lineage:         []string{"abc123"},
		},
	)

	var eventJSON map[string]interface{}
	data, _ := json.Marshal(event)
	json.Unmarshal(data, &eventJSON)

	if eventJSON["event"] != "lane.commit.created" {
		t.Errorf("event = %v", eventJSON["event"])
	}
	dataField, ok := eventJSON["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if dataField["branch"] != "feature/provenance" {
		t.Errorf("data.branch = %v", dataField["branch"])
	}
	if dataField["worktree"] != "wt-a" {
		t.Errorf("data.worktree = %v", dataField["worktree"])
	}
}

func TestDedupeSupersededCommitEvents(t *testing.T) {
	wta := "wt-a"
	wtb := "wt-b"
	oldDetail := "old"
	newDetail := "new"
	cc := "canon123"
	superseded := "new123"

	retained := DedupeSupersededCommitEvents([]LaneEvent{
		CommitCreated(
			"2026-04-04T00:00:00Z",
			&oldDetail,
			LaneCommitProvenance{
				Commit:          "old123",
				Branch:          "feature/provenance",
				Worktree:        &wta,
				CanonicalCommit: &cc,
				SupersededBy:    &superseded,
				Lineage:         []string{"old123", "new123"},
			},
		),
		CommitCreated(
			"2026-04-04T00:00:01Z",
			&newDetail,
			LaneCommitProvenance{
				Commit:          "new123",
				Branch:          "feature/provenance",
				Worktree:        &wtb,
				CanonicalCommit: &cc,
				Lineage:         []string{"old123", "new123"},
			},
		),
	})

	if len(retained) != 1 {
		t.Fatalf("expected 1 event, got %d", len(retained))
	}
	if retained[0].Detail == nil || *retained[0].Detail != "new" {
		t.Errorf("retained detail = %v, want 'new'", retained[0].Detail)
	}
}

func TestLaneEventJSONRoundTrip(t *testing.T) {
	e := Started("2026-04-04T00:00:00Z")
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded LaneEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Event != EventStarted {
		t.Errorf("decoded.Event = %s", decoded.Event)
	}
	if decoded.Status != StatusRunning {
		t.Errorf("decoded.Status = %s", decoded.Status)
	}
	if decoded.EmittedAt != "2026-04-04T00:00:00Z" {
		t.Errorf("decoded.EmittedAt = %s", decoded.EmittedAt)
	}
}
