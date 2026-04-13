package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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
// Sets env vars with CLAWD_ prefix (primary) and ITERION_ prefix (backward compat).
// Passes input as JSON on stdin.
// Returns stdout on success, error with stderr on non-zero exit.
func (t *PluginTool) Execute(input json.RawMessage) (string, error) {
	cmd := exec.Command(t.Command, t.Args...)

	// Set environment variables: CLAWD_ (primary) and ITERION_ (backward compat).
	cmd.Env = append(cmd.Environ(),
		// Primary CLAWD_ prefix
		"CLAWD_PLUGIN_ID="+t.PluginID,
		"CLAWD_PLUGIN_NAME="+t.PluginName,
		"CLAWD_TOOL_NAME="+t.Definition.Name,
		"CLAWD_TOOL_INPUT="+string(input),
		// Backward-compat ITERION_ prefix
		"ITERION_PLUGIN_ID="+t.PluginID,
		"ITERION_PLUGIN_NAME="+t.PluginName,
		"ITERION_TOOL_NAME="+t.Definition.Name,
		"ITERION_TOOL_INPUT="+string(input),
	)
	if t.Root != "" {
		cmd.Env = append(cmd.Env,
			"CLAWD_PLUGIN_ROOT="+t.Root,
			"ITERION_PLUGIN_ROOT="+t.Root,
		)
		cmd.Dir = t.Root
	}

	// Emit deprecation warning for ITERION_ prefix on first use only.
	warnIterionDeprecation()

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

var warnIterionDeprecation = sync.OnceFunc(func() {
	fmt.Fprintln(os.Stderr, "DEPRECATION WARNING: ITERION_* env vars are deprecated, use CLAWD_* instead")
})
