package hooks

// NewHookRunnerWithOverride creates a HookRunner for testing that will return
// the given permission override on the first PreToolUse call. This is used
// to test that the permission wiring in the conversation loop correctly
// handles hook permission decisions.
//
// This works by configuring a PreToolUse hook command that echoes the
// appropriate JSON on stdout.
func NewHookRunnerWithOverride(decision *PermissionDecision, reason string) *HookRunner {
	if decision == nil {
		return NewHookRunner(HookConfig{})
	}

	// Build a command that echoes the JSON permission override to stdout.
	// The hook output parser expects permissionDecision nested under hookSpecificOutput.
	cmd := `echo '{"hookSpecificOutput":{"permissionDecision":"` + string(*decision) + `","permissionDecisionReason":"` + reason + `"}}'`

	return NewHookRunner(HookConfig{
		PreToolUse: []string{cmd},
	})
}
