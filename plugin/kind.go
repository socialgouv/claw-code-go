package plugin

import (
	"encoding/json"
	"fmt"
)

// PluginKind represents the source type of a plugin.
type PluginKind int

const (
	KindBuiltin  PluginKind = iota // Hardcoded plugins
	KindBundled                    // Pre-packaged with application
	KindExternal                   // User-installed
)

var kindNames = map[PluginKind]string{
	KindBuiltin:  "builtin",
	KindBundled:  "bundled",
	KindExternal: "external",
}

var kindFromString = map[string]PluginKind{
	"builtin":  KindBuiltin,
	"bundled":  KindBundled,
	"external": KindExternal,
}

func (k PluginKind) String() string {
	if s, ok := kindNames[k]; ok {
		return s
	}
	return fmt.Sprintf("PluginKind(%d)", int(k))
}

// Marketplace returns the marketplace string used in plugin IDs.
// Matches Rust's PluginKind::marketplace() → "builtin"/"bundled"/"external".
func (k PluginKind) Marketplace() string {
	return k.String()
}

func (k PluginKind) MarshalJSON() ([]byte, error) {
	s, ok := kindNames[k]
	if !ok {
		return nil, fmt.Errorf("unknown PluginKind: %d", int(k))
	}
	return json.Marshal(s)
}

func (k *PluginKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := kindFromString[s]
	if !ok {
		return fmt.Errorf("unknown PluginKind: %q", s)
	}
	*k = v
	return nil
}

// PluginPermission is a manifest-level permission.
type PluginPermission string

const (
	PermissionRead    PluginPermission = "read"
	PermissionWrite   PluginPermission = "write"
	PermissionExecute PluginPermission = "execute"
)

// ValidPluginPermissions returns the set of valid permission strings.
func ValidPluginPermissions() map[PluginPermission]bool {
	return map[PluginPermission]bool{
		PermissionRead:    true,
		PermissionWrite:   true,
		PermissionExecute: true,
	}
}

// PluginToolPermission is the required permission level for a tool.
type PluginToolPermission string

const (
	ToolPermReadOnly         PluginToolPermission = "read-only"
	ToolPermWorkspaceWrite   PluginToolPermission = "workspace-write"
	ToolPermDangerFullAccess PluginToolPermission = "danger-full-access"
)

// ValidToolPermissions returns the set of valid tool permission strings.
func ValidToolPermissions() map[PluginToolPermission]bool {
	return map[PluginToolPermission]bool{
		ToolPermReadOnly:         true,
		ToolPermWorkspaceWrite:   true,
		ToolPermDangerFullAccess: true,
	}
}
