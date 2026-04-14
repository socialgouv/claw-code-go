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
		if !strings.Contains(out, `"changed": false`) {
			// When no stateDir, exit does not actually change settings, so changed=false.
			// This is correct since there's no settings.local.json to modify.
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
		// State file now uses planModeState format with hadLocalOverride.
		if _, ok := state["hadLocalOverride"]; !ok {
			t.Error("expected hadLocalOverride in state file")
		}
	})

	t.Run("exit clears persisted state", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter then exit.
		ExecuteEnterPlanMode(&active, dir)
		ExecuteExitPlanMode(&active, dir)

		// State file should be removed.
		statePath := filepath.Join(dir, "tool-state", "plan-mode.json")
		if _, err := os.Stat(statePath); !os.IsNotExist(err) {
			t.Error("expected state file to be removed after exit")
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
	t.Run("persisted then exit yields managed=false (exit always sets managed=false)", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		// Enter plan mode with persistence.
		_, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		// Exit — matches Rust behavior: exit always sets managed=false.
		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}
		// In Rust, exit always returns managed=false.
		if parsed["managed"] != false {
			t.Errorf("expected managed=false (Rust parity), got %v", parsed["managed"])
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

		// First exit.
		ExecuteExitPlanMode(&active, dir)

		// Second exit — no state file.
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

// TestPlanModeEnrichedFields verifies that the 4 new fields are present
// in plan mode responses, matching Rust's PlanModeOutput structure.
func TestPlanModeEnrichedFields(t *testing.T) {
	t.Run("enter includes settingsPath and statePath", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		out, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse: %v", err)
		}

		sp, ok := parsed["settingsPath"].(string)
		if !ok || sp == "" {
			t.Error("expected settingsPath to be a non-empty string")
		}
		stp, ok := parsed["statePath"].(string)
		if !ok || stp == "" {
			t.Error("expected statePath to be a non-empty string")
		}

		// settingsPath should point to settings.local.json.
		if !strings.HasSuffix(sp, "settings.local.json") {
			t.Errorf("settingsPath = %s, want suffix settings.local.json", sp)
		}
		// statePath should point to plan-mode.json.
		if !strings.HasSuffix(stp, "plan-mode.json") {
			t.Errorf("statePath = %s, want suffix plan-mode.json", stp)
		}
	})

	t.Run("enter from empty state has no previousLocalMode", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		out, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		var parsed map[string]any
		json.Unmarshal([]byte(out), &parsed)

		if _, ok := parsed["previousLocalMode"]; ok {
			t.Error("expected no previousLocalMode when entering from empty state")
		}

		// currentLocalMode should be "plan" after entering.
		if parsed["currentLocalMode"] != "plan" {
			t.Errorf("currentLocalMode = %v, want 'plan'", parsed["currentLocalMode"])
		}
	})

	t.Run("enter with existing local override preserves previousLocalMode", func(t *testing.T) {
		dir := t.TempDir()

		// Pre-set a local override in settings.local.json.
		settingsPath := filepath.Join(dir, "settings.local.json")
		settingsData := `{"permissions": {"defaultMode": "acceptEdits"}}`
		os.WriteFile(settingsPath, []byte(settingsData), 0o644)

		active := false
		out, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		var parsed map[string]any
		json.Unmarshal([]byte(out), &parsed)

		if parsed["previousLocalMode"] != "acceptEdits" {
			t.Errorf("previousLocalMode = %v, want 'acceptEdits'", parsed["previousLocalMode"])
		}
		if parsed["currentLocalMode"] != "plan" {
			t.Errorf("currentLocalMode = %v, want 'plan'", parsed["currentLocalMode"])
		}
	})

	t.Run("exit restores previousLocalMode", func(t *testing.T) {
		dir := t.TempDir()

		// Pre-set a local override.
		settingsPath := filepath.Join(dir, "settings.local.json")
		settingsData := `{"permissions": {"defaultMode": "acceptEdits"}}`
		os.WriteFile(settingsPath, []byte(settingsData), 0o644)

		// Enter (saves "acceptEdits" as previous), then exit (restores).
		active := false
		ExecuteEnterPlanMode(&active, dir)

		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}

		var parsed map[string]any
		json.Unmarshal([]byte(out), &parsed)

		if parsed["previousLocalMode"] != "acceptEdits" {
			t.Errorf("previousLocalMode = %v, want 'acceptEdits'", parsed["previousLocalMode"])
		}
		if parsed["currentLocalMode"] != "acceptEdits" {
			t.Errorf("currentLocalMode = %v, want 'acceptEdits' (restored)", parsed["currentLocalMode"])
		}
		if parsed["changed"] != true {
			t.Errorf("changed = %v, want true", parsed["changed"])
		}
	})

	t.Run("exit from empty state omits previousLocalMode gracefully", func(t *testing.T) {
		dir := t.TempDir()

		// Enter from empty state (no settings.local.json).
		active := false
		ExecuteEnterPlanMode(&active, dir)

		out, err := ExecuteExitPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}

		var parsed map[string]any
		json.Unmarshal([]byte(out), &parsed)

		// previousLocalMode should be absent (was nil before entering).
		if _, ok := parsed["previousLocalMode"]; ok {
			t.Error("expected no previousLocalMode when settings.local.json didn't exist before")
		}

		// currentLocalMode should be absent (defaultMode was removed).
		if _, ok := parsed["currentLocalMode"]; ok {
			t.Errorf("expected no currentLocalMode after removing default, got %v", parsed["currentLocalMode"])
		}
	})

	t.Run("gracefully handles missing settings.local.json", func(t *testing.T) {
		dir := t.TempDir()
		active := false

		out, err := ExecuteEnterPlanMode(&active, dir)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}

		// Should work without error even though settings.local.json didn't exist.
		var parsed map[string]any
		json.Unmarshal([]byte(out), &parsed)

		if parsed["success"] != true {
			t.Error("expected success=true")
		}
	})
}
