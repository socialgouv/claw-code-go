package tools

import (
	"claw-code-go/internal/api"
	"encoding/json"
	"fmt"
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

func ExecuteEnterPlanMode(planModeActive *bool) (string, error) {
	if planModeActive == nil {
		return "", fmt.Errorf("enter_plan_mode: plan mode state not available")
	}
	if *planModeActive {
		return planModeResult(true, false, "Plan mode is already active")
	}
	*planModeActive = true
	return planModeResult(true, true, "Plan mode activated. Tools will be described but not executed.")
}

func ExecuteExitPlanMode(planModeActive *bool) (string, error) {
	if planModeActive == nil {
		return "", fmt.Errorf("exit_plan_mode: plan mode state not available")
	}
	if !*planModeActive {
		return planModeResult(false, false, "Plan mode is not active")
	}
	*planModeActive = false
	return planModeResult(false, true, "Plan mode deactivated. Normal tool execution resumed.")
}

func planModeResult(active, changed bool, message string) (string, error) {
	result := map[string]any{
		"success": true,
		"changed": changed,
		"active":  active,
		"message": message,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}
