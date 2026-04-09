package permissions

// ToolPermissionLevel represents the granularity of permission required for a tool.
// Levels are ordered: ReadOnly < WorkspaceWrite < DangerFullAccess.
type ToolPermissionLevel int

const (
	// ReadOnly allows read-only operations (file reads, searches).
	ReadOnly ToolPermissionLevel = iota
	// WorkspaceWrite allows writing within the workspace directory.
	WorkspaceWrite
	// DangerFullAccess allows arbitrary command execution and system access.
	DangerFullAccess
)

// String returns the CLI string representation of the level.
func (l ToolPermissionLevel) String() string {
	switch l {
	case ReadOnly:
		return "read-only"
	case WorkspaceWrite:
		return "workspace-write"
	case DangerFullAccess:
		return "danger-full-access"
	default:
		return "unknown"
	}
}

// ParseToolPermissionLevel converts a string to a ToolPermissionLevel.
func ParseToolPermissionLevel(s string) (ToolPermissionLevel, bool) {
	switch s {
	case "read-only":
		return ReadOnly, true
	case "workspace-write":
		return WorkspaceWrite, true
	case "danger-full-access":
		return DangerFullAccess, true
	default:
		return ReadOnly, false
	}
}

// PermissionRequest describes a tool requesting permission.
type PermissionRequest struct {
	ToolName     string
	Input        string
	CurrentMode  PermissionMode
	RequiredMode ToolPermissionLevel
	Reason       string
}

// PermissionPromptDecision is the outcome of prompting the user.
type PermissionPromptDecision struct {
	Allowed bool
	Reason  string // populated when denied
}

// PermissionPrompter is the interface for interactively asking the user
// for permission decisions. Implementations should be synchronous;
// callers wrap in goroutines if async behavior is needed.
type PermissionPrompter interface {
	// Decide presents a permission request to the user and returns their decision.
	Decide(req PermissionRequest) PermissionPromptDecision
}

// HookPermissionOverride represents a permission decision made by a hook,
// which can override the normal permission flow.
type HookPermissionOverride struct {
	Decision Decision // Allow, Deny, or Ask
	Reason   string
}
