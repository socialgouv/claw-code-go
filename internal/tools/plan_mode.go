package tools

import (
	"claw-code-go/internal/api"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func EnterPlanModeTool() api.Tool {
	return api.Tool{
		Name:        "enter_plan_mode",
		Description: "Enter plan mode. Tools will be described but not executed.",
		InputSchema: api.InputSchema{
			Type:       "object",
			Properties: map[string]api.Property{},
		},
	}
}

func ExitPlanModeTool() api.Tool {
	return api.Tool{
		Name:        "exit_plan_mode",
		Description: "Exit plan mode. Resume normal tool execution.",
		InputSchema: api.InputSchema{
			Type:       "object",
			Properties: map[string]api.Property{},
		},
	}
}

func ExecuteEnterPlanMode(planModeActive *bool, stateDir string) (string, error) {
	if planModeActive == nil {
		return "", fmt.Errorf("enter_plan_mode: plan mode state not available")
	}
	if *planModeActive {
		return planModeResult("enter", true, true, false, "Plan mode is already active")
	}
	*planModeActive = true
	if stateDir != "" {
		if err := persistPlanMode(stateDir, true); err != nil {
			fmt.Fprintf(os.Stderr, "[plan_mode] warning: failed to persist plan mode: %v\n", err)
		}
	}
	return planModeResult("enter", true, true, true, "Plan mode activated. Tools will be described but not executed.")
}

func ExecuteExitPlanMode(planModeActive *bool, stateDir string) (string, error) {
	if planModeActive == nil {
		return "", fmt.Errorf("exit_plan_mode: plan mode state not available")
	}
	if !*planModeActive {
		return planModeResult("exit", false, false, false, "Plan mode is not active")
	}
	*planModeActive = false
	// managed reflects whether a state file existed (i.e. plan mode was persisted).
	managed := false
	if stateDir != "" {
		_, statErr := os.Stat(planModeStatePath(stateDir))
		managed = statErr == nil
		if err := clearPlanModeState(stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "[plan_mode] warning: failed to clear plan mode state: %v\n", err)
		}
	}
	return planModeResult("exit", managed, false, true, "Plan mode deactivated. Normal tool execution resumed.")
}

// planModeResult builds the JSON response for plan mode operations.
//
// Go intentionally uses simplified plan-mode.json persistence ({active: bool})
// rather than Rust's settings.local.json mutation with {hadLocalOverride,
// previousLocalMode}. The 4 path/mode fields (settingsPath, statePath,
// previousLocalMode, currentLocalMode) are omitted because Go's simplified
// persistence does not track settings.local.json paths. This avoids coupling
// to the settings file format and simplifies the restore path.
func planModeResult(operation string, managed, active, changed bool, message string) (string, error) {
	result := map[string]any{
		"success":   true,
		"operation": operation,
		"changed":   changed,
		"active":    active,
		"managed":   managed,
		"message":   message,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

// planModeStatePath returns the path to the plan mode state file.
func planModeStatePath(stateDir string) string {
	return filepath.Join(stateDir, "tool-state", "plan-mode.json")
}

// persistPlanMode writes the plan mode state to disk.
func persistPlanMode(stateDir string, active bool) error {
	path := planModeStatePath(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state := map[string]any{
		"active": active,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// clearPlanModeState removes the persisted plan mode state file.
func clearPlanModeState(stateDir string) error {
	path := planModeStatePath(stateDir)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadPersistedPlanMode reads the persisted plan mode state from disk.
// Returns false if stateDir is empty, the file doesn't exist, or can't be read.
func LoadPersistedPlanMode(stateDir string) bool {
	if stateDir == "" {
		return false
	}
	data, err := os.ReadFile(planModeStatePath(stateDir))
	if err != nil {
		return false
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return false
	}
	active, _ := state["active"].(bool)
	return active
}
