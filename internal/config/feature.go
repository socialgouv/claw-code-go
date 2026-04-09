package config

import "encoding/json"

// RuntimeFeatureConfig holds extracted feature-specific configuration
// from the merged settings. This provides typed access to hook commands,
// plugin settings, permission rules, and provider fallback chains.
type RuntimeFeatureConfig struct {
	Hooks             RuntimeHookConfig
	Plugins           RuntimePluginConfig
	Model             string
	Aliases           map[string]string
	PermissionMode    string
	PermissionRules   RuntimePermissionRuleConfig
	ProviderFallbacks ProviderFallbackConfig
	TrustedRoots      []string
}

// RuntimeHookConfig holds the hook command lists extracted from settings.
type RuntimeHookConfig struct {
	PreToolUse         []string
	PostToolUse        []string
	PostToolUseFailure []string
}

// RuntimePluginConfig holds plugin-related settings.
type RuntimePluginConfig struct {
	EnabledPlugins      map[string]bool
	ExternalDirectories []string
	InstallRoot         string
	RegistryPath        string
	BundledRoot         string
	MaxOutputTokens     int
}

// RuntimePermissionRuleConfig holds permission rule lists.
type RuntimePermissionRuleConfig struct {
	Allow []string
	Deny  []string
	Ask   []string
}

// ProviderFallbackConfig describes the primary model and fallback chain.
type ProviderFallbackConfig struct {
	Primary   string
	Fallbacks []string
}

// IsEmpty returns true if no primary or fallbacks are configured.
func (c ProviderFallbackConfig) IsEmpty() bool {
	return c.Primary == "" && len(c.Fallbacks) == 0
}

// ExtractFeatureConfig extracts a RuntimeFeatureConfig from raw JSON settings data.
// Missing or unparseable fields are left at zero values.
func ExtractFeatureConfig(data []byte) RuntimeFeatureConfig {
	var cfg RuntimeFeatureConfig

	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}

	// Model
	if v, ok := raw["model"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			cfg.Model = s
		}
	}

	// Aliases
	if v, ok := raw["aliases"]; ok {
		var m map[string]string
		if json.Unmarshal(v, &m) == nil {
			cfg.Aliases = m
		}
	}

	// Permission mode (from permissions.defaultMode or legacy permissionMode)
	if v, ok := raw["permissions"]; ok {
		var perms map[string]json.RawMessage
		if json.Unmarshal(v, &perms) == nil {
			if dm, ok2 := perms["defaultMode"]; ok2 {
				var s string
				if json.Unmarshal(dm, &s) == nil {
					cfg.PermissionMode = s
				}
			}
			// Permission rules
			if ar, ok2 := perms["allow"]; ok2 {
				json.Unmarshal(ar, &cfg.PermissionRules.Allow)
			}
			if dr, ok2 := perms["deny"]; ok2 {
				json.Unmarshal(dr, &cfg.PermissionRules.Deny)
			}
			if askr, ok2 := perms["ask"]; ok2 {
				json.Unmarshal(askr, &cfg.PermissionRules.Ask)
			}
		}
	}
	// Legacy permissionMode fallback
	if cfg.PermissionMode == "" {
		if v, ok := raw["permissionMode"]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				cfg.PermissionMode = s
			}
		}
	}

	// Hooks
	if v, ok := raw["hooks"]; ok {
		var hooks map[string]json.RawMessage
		if json.Unmarshal(v, &hooks) == nil {
			if pre, ok2 := hooks["PreToolUse"]; ok2 {
				json.Unmarshal(pre, &cfg.Hooks.PreToolUse)
			}
			if post, ok2 := hooks["PostToolUse"]; ok2 {
				json.Unmarshal(post, &cfg.Hooks.PostToolUse)
			}
			if fail, ok2 := hooks["PostToolUseFailure"]; ok2 {
				json.Unmarshal(fail, &cfg.Hooks.PostToolUseFailure)
			}
		}
	}

	// Plugins
	if v, ok := raw["plugins"]; ok {
		var plugins map[string]json.RawMessage
		if json.Unmarshal(v, &plugins) == nil {
			if ep, ok2 := plugins["enabled"]; ok2 {
				json.Unmarshal(ep, &cfg.Plugins.EnabledPlugins)
			}
			if ed, ok2 := plugins["externalDirectories"]; ok2 {
				json.Unmarshal(ed, &cfg.Plugins.ExternalDirectories)
			}
			if ir, ok2 := plugins["installRoot"]; ok2 {
				var s string
				if json.Unmarshal(ir, &s) == nil {
					cfg.Plugins.InstallRoot = s
				}
			}
			if rp, ok2 := plugins["registryPath"]; ok2 {
				var s string
				if json.Unmarshal(rp, &s) == nil {
					cfg.Plugins.RegistryPath = s
				}
			}
			if br, ok2 := plugins["bundledRoot"]; ok2 {
				var s string
				if json.Unmarshal(br, &s) == nil {
					cfg.Plugins.BundledRoot = s
				}
			}
			if mot, ok2 := plugins["maxOutputTokens"]; ok2 {
				var n int
				if json.Unmarshal(mot, &n) == nil {
					cfg.Plugins.MaxOutputTokens = n
				}
			}
		}
	}
	// Legacy enabledPlugins fallback
	if len(cfg.Plugins.EnabledPlugins) == 0 {
		if v, ok := raw["enabledPlugins"]; ok {
			json.Unmarshal(v, &cfg.Plugins.EnabledPlugins)
		}
	}

	// Provider fallbacks
	if v, ok := raw["providerFallbacks"]; ok {
		var fb map[string]json.RawMessage
		if json.Unmarshal(v, &fb) == nil {
			if p, ok2 := fb["primary"]; ok2 {
				var s string
				if json.Unmarshal(p, &s) == nil {
					cfg.ProviderFallbacks.Primary = s
				}
			}
			if f, ok2 := fb["fallbacks"]; ok2 {
				json.Unmarshal(f, &cfg.ProviderFallbacks.Fallbacks)
			}
		}
	}

	// Trusted roots
	if v, ok := raw["trustedRoots"]; ok {
		json.Unmarshal(v, &cfg.TrustedRoots)
	}

	return cfg
}
