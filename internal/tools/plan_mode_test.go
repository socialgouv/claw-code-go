package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanMode(t *testing.T) {
	t.Run("enter plan mode sets active true", func(t *testing.T) {
		active := false
		out, err := ExecuteEnterPlanMode(&active, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !active {
			t.Fatal("expected active to be true")
		}
		if !strings.Contains(out, `"changed": true`) {
			t.Fatalf("expected changed=true in output, got %s", out)
		}
	})

	t.Run("enter plan mode when already active", func(t *testing.T) {
		active := true
		out, err := ExecuteEnterPlanMode(&active, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"changed": false`) {
			t.Fatalf("expected changed=false in output, got %s", out)
		}
	})

	t.Run("exit plan mode sets active false", func(t *testing.T) {
		active := true
		out, err := ExecuteExitPlanMode(&active, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if active {
			t.Fatal("expected active to be false")
		}
		if !strings.Contains(out, `"changed": true`) {
			t.Fatalf("expected changed=true in output, got %s", out)
		}
	})

	t.Run("exit plan mode when not active", func(t *testing.T) {
		active := false
		out, err := ExecuteExitPlanMode(&active, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"changed": false`) {
			t.Fatalf("expected changed=false in output, got %s", out)
		}
	})

	t.Run("enter nil pointer returns error", func(t *testing.T) {
		_, err := ExecuteEnterPlanMode(nil, "")
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("expected nil pointer error, got %v", err)
		}
	})

	t.Run("exit nil pointer returns error", func(t *testing.T) {
		_, err := ExecuteExitPlanMode(nil, "")
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("expected nil pointer error, got %v", err)
		}
	})
}

func TestPlanModePersistence(t *testing.T) {
	t.Run("persist and load round-trip", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter plan mode with persistence.
		_, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}
		if !active {
			t.Fatal("expected active to be true")
		}

		// Verify state file exists.
		statePath := filepath.Join(dir, "tool-state", "plan-mode.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("failed to read state file: %v", err)
		}
		var state map[string]any
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatalf("failed to parse state: %v", err)
		}
		if state["active"] != true {
			t.Errorf("expected active=true in persisted state, got %v", state["active"])
		}

		// Load persisted state.
		loaded := LoadPersistedPlanMode(dir)
		if !loaded {
			t.Error("expected loaded plan mode to be true")
		}
	})

	t.Run("exit clears persisted state", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter then exit.
		ExecuteEnterPlanMode(&active, dir)
		ExecuteExitPlanMode(&active, dir)

		// Load should return false.
		loaded := LoadPersistedPlanMode(dir)
		if loaded {
			t.Error("expected loaded plan mode to be false after exit")
		}
	})

	t.Run("load from empty dir returns false", func(t *testing.T) {
		dir := t.TempDir()
		loaded := LoadPersistedPlanMode(dir)
		if loaded {
			t.Error("expected false from empty dir")
		}
	})

	t.Run("load from empty string returns false", func(t *testing.T) {
		loaded := LoadPersistedPlanMode("")
		if loaded {
			t.Error("expected false from empty string")
		}
	})
}

func TestExitPlanMode_ManagedFlag_DerivedFromRemove(t *testing.T) {
	t.Run("persisted then exit yields managed=true", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter plan mode with persistence.
		_, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		// Exit — file existed, so managed should be true.
		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed["managed"] != true {
			t.Errorf("expected managed=true, got %v", parsed["managed"])
		}
	})

	t.Run("exit without persisting yields managed=false", func(t *testing.T) {
		dir := t.TempDir()
		active := true // Set active without persisting.

		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed["managed"] != false {
			t.Errorf("expected managed=false, got %v", parsed["managed"])
		}
	})

	t.Run("double exit yields managed=false on second call", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter with persistence, then exit twice.
		ExecuteEnterPlanMode(&active, dir)

		// First exit removes the file.
		active = true // re-arm since first exit cleared it
		ExecuteExitPlanMode(&active, dir)

		// Second exit — file already gone, clearPlanModeState returns (false, nil).
		active = true
		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("second exit: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed["managed"] != false {
			t.Errorf("expected managed=false on second exit, got %v", parsed["managed"])
		}
	})
}
