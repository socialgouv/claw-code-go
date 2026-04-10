package plugin

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestExecuteSetsEnvVars(t *testing.T) {
	t.Parallel()

	// Use a shell command to print env vars
	var cmd, arg string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		arg = "/c set"
	} else {
		cmd = "env"
		arg = ""
	}

	var args []string
	if arg != "" {
		args = []string{arg}
	}

	tool := &PluginTool{
		PluginID:   "test-plugin",
		PluginName: "TestPlugin",
		Definition: PluginToolDefinition{Name: "echo_tool"},
		Command:    cmd,
		Args:       args,
		Root:       t.TempDir(),
	}

	input := json.RawMessage(`{"key":"value"}`)
	output, err := tool.Execute(input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify primary CLAWD_ env vars are present
	clawd := []string{
		"CLAWD_PLUGIN_ID=test-plugin",
		"CLAWD_PLUGIN_NAME=TestPlugin",
		"CLAWD_TOOL_NAME=echo_tool",
		`CLAWD_TOOL_INPUT={"key":"value"}`,
	}
	for _, want := range clawd {
		if !strings.Contains(output, want) {
			t.Errorf("output missing primary env var %q", want)
		}
	}

	// Verify backward-compat ITERION_ env vars are present
	iterion := []string{
		"ITERION_PLUGIN_ID=test-plugin",
		"ITERION_PLUGIN_NAME=TestPlugin",
		"ITERION_TOOL_NAME=echo_tool",
		`ITERION_TOOL_INPUT={"key":"value"}`,
	}
	for _, want := range iterion {
		if !strings.Contains(output, want) {
			t.Errorf("output missing backward-compat env var %q", want)
		}
	}

	// Verify CLAWD_PLUGIN_ROOT and ITERION_PLUGIN_ROOT when Root is set
	if !strings.Contains(output, "CLAWD_PLUGIN_ROOT=") {
		t.Error("output missing CLAWD_PLUGIN_ROOT")
	}
	if !strings.Contains(output, "ITERION_PLUGIN_ROOT=") {
		t.Error("output missing ITERION_PLUGIN_ROOT")
	}
}
