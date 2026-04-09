package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateManifest checks a manifest for errors.
// Returns all validation errors found (does not stop at first).
func ValidateManifest(m *PluginManifest, root string, kind PluginKind) []ValidationError {
	var errs []ValidationError

	// Required fields
	if strings.TrimSpace(m.Name) == "" {
		errs = append(errs, ValidationError{Code: "empty_field", Message: "name is required"})
	}
	if strings.TrimSpace(m.Version) == "" {
		errs = append(errs, ValidationError{Code: "empty_field", Message: "version is required"})
	}
	if strings.TrimSpace(m.Description) == "" {
		errs = append(errs, ValidationError{Code: "empty_field", Message: "description is required"})
	}

	// Duplicate permissions
	validPerms := ValidPluginPermissions()
	seenPerms := make(map[PluginPermission]bool)
	for _, p := range m.Permissions {
		if !validPerms[p] {
			errs = append(errs, ValidationError{
				Code:    "invalid_permission",
				Message: fmt.Sprintf("invalid permission: %q", string(p)),
			})
		}
		if seenPerms[p] {
			errs = append(errs, ValidationError{
				Code:    "duplicate_permission",
				Message: fmt.Sprintf("duplicate permission: %q", string(p)),
			})
		}
		seenPerms[p] = true
	}

	// Tool validation
	validToolPerms := ValidToolPermissions()
	seenTools := make(map[string]bool)
	for _, t := range m.Tools {
		if strings.TrimSpace(t.Name) == "" {
			errs = append(errs, ValidationError{
				Code:    "empty_entry_field",
				Message: "tool name is required",
			})
		}
		if strings.TrimSpace(t.Command) == "" {
			errs = append(errs, ValidationError{
				Code:    "empty_entry_field",
				Message: fmt.Sprintf("tool %q command is required", t.Name),
			})
		}
		if t.Name != "" {
			if seenTools[t.Name] {
				errs = append(errs, ValidationError{
					Code:    "duplicate_entry",
					Message: fmt.Sprintf("duplicate tool name: %q", t.Name),
				})
			}
			seenTools[t.Name] = true
		}

		// inputSchema must be a JSON object
		if len(t.InputSchema) > 0 {
			trimmed := strings.TrimSpace(string(t.InputSchema))
			if !strings.HasPrefix(trimmed, "{") {
				errs = append(errs, ValidationError{
					Code:    "invalid_tool_input_schema",
					Message: fmt.Sprintf("tool %q inputSchema must be a JSON object", t.Name),
				})
			}
		}

		// requiredPermission validation
		if t.RequiredPermission != "" && !validToolPerms[t.RequiredPermission] {
			errs = append(errs, ValidationError{
				Code:    "invalid_tool_required_permission",
				Message: fmt.Sprintf("tool %q has invalid requiredPermission: %q", t.Name, string(t.RequiredPermission)),
			})
		}

		// Path existence check for bundled/external
		if (kind == KindBundled || kind == KindExternal) && root != "" && strings.TrimSpace(t.Command) != "" {
			cmdPath := t.Command
			if !filepath.IsAbs(cmdPath) {
				cmdPath = filepath.Join(root, cmdPath)
			}
			if _, err := os.Stat(cmdPath); err != nil {
				errs = append(errs, ValidationError{
					Code:    "missing_path",
					Message: fmt.Sprintf("tool %q command path not found: %s", t.Name, cmdPath),
				})
			}
		}
	}

	// Unsupported contracts
	if len(m.RawJSON) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(m.RawJSON, &raw); err == nil {
			for _, key := range []string{"skills", "mcpServers", "agents"} {
				if _, ok := raw[key]; ok {
					errs = append(errs, ValidationError{
						Code:    "unsupported_contract",
						Message: fmt.Sprintf("unsupported manifest key: %q", key),
					})
				}
			}
		}
	}

	return errs
}
