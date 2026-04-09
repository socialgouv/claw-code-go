package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PluginTool is a runnable tool from a plugin.
type PluginTool struct {
	PluginID           string
	PluginName         string
	Definition         PluginToolDefinition
	Command            string
	Args               []string
	RequiredPermission PluginToolPermission
	Root               string
}

// Execute runs the plugin tool with the given JSON input.
// Sets env vars: ITERION_PLUGIN_ID, ITERION_PLUGIN_NAME, ITERION_TOOL_NAME,
// ITERION_TOOL_INPUT (JSON), ITERION_PLUGIN_ROOT (if non-empty).
// Passes input as JSON on stdin.
// Returns stdout on success, error with stderr on non-zero exit.
func (t *PluginTool) Execute(input json.RawMessage) (string, error) {
	cmd := exec.Command(t.Command, t.Args...)

	// Set environment variables (ITERION_ prefix for iterion, not CLAWD_).
	cmd.Env = append(cmd.Environ(),
		"ITERION_PLUGIN_ID="+t.PluginID,
		"ITERION_PLUGIN_NAME="+t.PluginName,
		"ITERION_TOOL_NAME="+t.Definition.Name,
		"ITERION_TOOL_INPUT="+string(input),
	)
	if t.Root != "" {
		cmd.Env = append(cmd.Env, "ITERION_PLUGIN_ROOT="+t.Root)
		cmd.Dir = t.Root
	}

	// Pass input on stdin
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Match Rust error format: "plugin tool `name` from `plugin_id` failed for `command`: stderr_or_status"
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr == "" {
			if exitErr, ok := err.(*exec.ExitError); ok {
				stderrStr = fmt.Sprintf("exit status %d", exitErr.ExitCode())
			} else {
				stderrStr = err.Error()
			}
		}
		return "", &PluginError{
			Kind:    ErrCommandFailed,
			Message: fmt.Sprintf("plugin tool `%s` from `%s` failed for `%s`: %s", t.Definition.Name, t.PluginID, t.Command, stderrStr),
			Cause:   err,
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}
