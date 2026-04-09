package hooks

import "fmt"

// HookConfig holds the hook command lists per event type.
type HookConfig struct {
	PreToolUse         []string
	PostToolUse        []string
	PostToolUseFailure []string
}

// HookProgressEvent reports hook execution progress.
type HookProgressEvent struct {
	Kind     string // "started", "completed", "cancelled"
	Event    HookEvent
	ToolName string
	Command  string
}

// HookProgressReporter receives progress notifications.
type HookProgressReporter interface {
	OnEvent(event HookProgressEvent)
}

// HookRunner executes hook commands for tool lifecycle events.
type HookRunner struct {
	config HookConfig
}

// NewHookRunner creates a new HookRunner with the given config.
func NewHookRunner(config HookConfig) *HookRunner {
	return &HookRunner{config: config}
}

// RunPreToolUse runs hooks for the PreToolUse event.
func (r *HookRunner) RunPreToolUse(toolName, toolInput string) HookRunResult {
	return r.RunPreToolUseWithContext(toolName, toolInput, nil, nil)
}

// RunPostToolUse runs hooks for the PostToolUse event.
func (r *HookRunner) RunPostToolUse(toolName, toolInput, toolOutput string, isError bool) HookRunResult {
	return r.RunPostToolUseWithContext(toolName, toolInput, toolOutput, isError, nil, nil)
}

// RunPostToolUseFailure runs hooks for the PostToolUseFailure event.
func (r *HookRunner) RunPostToolUseFailure(toolName, toolInput, toolError string) HookRunResult {
	return r.RunPostToolUseFailureWithContext(toolName, toolInput, toolError, nil, nil)
}

// RunPreToolUseWithContext runs PreToolUse hooks with abort signal and progress reporter.
func (r *HookRunner) RunPreToolUseWithContext(toolName, toolInput string, abort *HookAbortSignal, reporter HookProgressReporter) HookRunResult {
	return r.runHooks(PreToolUse, toolName, toolInput, nil, false, abort, reporter)
}

// RunPostToolUseWithContext runs PostToolUse hooks with abort signal and progress reporter.
func (r *HookRunner) RunPostToolUseWithContext(toolName, toolInput, toolOutput string, isError bool, abort *HookAbortSignal, reporter HookProgressReporter) HookRunResult {
	return r.runHooks(PostToolUse, toolName, toolInput, &toolOutput, isError, abort, reporter)
}

// RunPostToolUseFailureWithContext runs PostToolUseFailure hooks with abort signal and progress reporter.
func (r *HookRunner) RunPostToolUseFailureWithContext(toolName, toolInput, toolError string, abort *HookAbortSignal, reporter HookProgressReporter) HookRunResult {
	return r.runHooks(PostToolUseFailure, toolName, toolInput, &toolError, true, abort, reporter)
}

// commandsForEvent returns the command list for the given event type.
func (r *HookRunner) commandsForEvent(event HookEvent) []string {
	switch event {
	case PreToolUse:
		return r.config.PreToolUse
	case PostToolUse:
		return r.config.PostToolUse
	case PostToolUseFailure:
		return r.config.PostToolUseFailure
	default:
		return nil
	}
}

// runHooks is the core execution loop for hook commands.
func (r *HookRunner) runHooks(event HookEvent, toolName, toolInput string, toolOutput *string, isError bool, abort *HookAbortSignal, reporter HookProgressReporter) HookRunResult {
	commands := r.commandsForEvent(event)
	if len(commands) == 0 {
		return Allow(nil)
	}

	var allMessages []string
	var permOverride *PermissionDecision
	var permReason string
	var updatedInput string

	for _, command := range commands {
		// Check abort signal.
		if abort != nil && abort.IsAborted() {
			return HookRunResult{
				Cancelled: true,
				Messages:  append(allMessages, fmt.Sprintf("%s hook cancelled before execution", event)),
			}
		}

		// Report started.
		if reporter != nil {
			reporter.OnEvent(HookProgressEvent{
				Kind:     "started",
				Event:    event,
				ToolName: toolName,
				Command:  command,
			})
		}

		// Build env vars.
		env := map[string]string{
			"HOOK_EVENT":      string(event),
			"HOOK_TOOL_NAME":  toolName,
			"HOOK_TOOL_INPUT": toolInput,
		}
		if toolOutput != nil {
			env["HOOK_TOOL_OUTPUT"] = *toolOutput
		}
		if isError {
			env["HOOK_TOOL_IS_ERROR"] = "1"
		} else {
			env["HOOK_TOOL_IS_ERROR"] = "0"
		}

		// Build payload.
		payload := BuildPayload(event, toolName, toolInput, toolOutput, isError)

		// Execute command.
		result := runShellCommand(command, env, payload, abort)

		// Handle cancellation.
		if result.Cancelled {
			if reporter != nil {
				reporter.OnEvent(HookProgressEvent{
					Kind:     "cancelled",
					Event:    event,
					ToolName: toolName,
					Command:  command,
				})
			}
			return HookRunResult{
				Cancelled: true,
				Messages:  append(allMessages, fmt.Sprintf("%s hook %q cancelled", event, command)),
			}
		}

		// Parse response.
		parsed := parseHookOutput(result.Stdout)
		allMessages = append(allMessages, parsed.Messages...)

		if parsed.PermissionOverride != nil {
			permOverride = parsed.PermissionOverride
		}
		if parsed.PermissionReason != "" {
			permReason = parsed.PermissionReason
		}
		if parsed.UpdatedInput != "" {
			updatedInput = parsed.UpdatedInput
		}

		// Interpret exit code.
		switch {
		case result.ExitCode == 0 && parsed.Deny:
			// JSON response requested denial.
			if reporter != nil {
				reporter.OnEvent(HookProgressEvent{
					Kind:     "completed",
					Event:    event,
					ToolName: toolName,
					Command:  command,
				})
			}
			return HookRunResult{
				Denied:             true,
				Messages:           allMessages,
				PermissionOverride: permOverride,
				PermissionReason:   permReason,
				UpdatedInput:       updatedInput,
			}
		case result.ExitCode == 0:
			// Success, continue chain.
		case result.ExitCode == 2:
			// Explicit denial.
			if reporter != nil {
				reporter.OnEvent(HookProgressEvent{
					Kind:     "completed",
					Event:    event,
					ToolName: toolName,
					Command:  command,
				})
			}
			// Fallback message when no hook output provided (matching Rust with_fallback_message).
			if len(allMessages) == 0 {
				allMessages = append(allMessages, fmt.Sprintf("%s hook denied tool %q", event, toolName))
			}
			return HookRunResult{
				Denied:             true,
				Messages:           allMessages,
				PermissionOverride: permOverride,
				PermissionReason:   permReason,
				UpdatedInput:       updatedInput,
			}
		default:
			// Non-zero (other than 2): failure, stop chain.
			if reporter != nil {
				reporter.OnEvent(HookProgressEvent{
					Kind:     "completed",
					Event:    event,
					ToolName: toolName,
					Command:  command,
				})
			}
			// Fallback message when no hook output provided (matching Rust format_hook_failure).
			if len(allMessages) == 0 {
				allMessages = append(allMessages, fmt.Sprintf("%s hook %q failed with exit code %d", event, command, result.ExitCode))
			}
			return HookRunResult{
				Failed:             true,
				Messages:           allMessages,
				PermissionOverride: permOverride,
				PermissionReason:   permReason,
				UpdatedInput:       updatedInput,
			}
		}

		// Report completed.
		if reporter != nil {
			reporter.OnEvent(HookProgressEvent{
				Kind:     "completed",
				Event:    event,
				ToolName: toolName,
				Command:  command,
			})
		}
	}

	return HookRunResult{
		Messages:           allMessages,
		PermissionOverride: permOverride,
		PermissionReason:   permReason,
		UpdatedInput:       updatedInput,
	}
}
