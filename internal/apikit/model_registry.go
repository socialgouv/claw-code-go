package apikit

import (
	"strings"
	"sync"
)

// ModelEntry stores metadata for a registered model.
type ModelEntry struct {
	Canonical     string       // canonical model name (e.g., "claude-opus-4-6")
	Provider      ProviderKind // provider for this model
	MaxOutput     uint32       // max output tokens
	ContextWindow uint32       // context window tokens
	Aliases       []string     // short aliases (e.g., "opus")
	Metadata      *ProviderMetadata
}

// ModelRegistry is a dynamic, lazy-initialized registry of known models.
// It replaces the hardcoded switches in preflight.go and provider_routing.go
// with a map-based lookup that supports runtime extension.
//
// Thread-safe: initialization happens once via sync.Once, and the registry
// map is read-only after initialization.
type ModelRegistry struct {
	once    sync.Once
	models  map[string]*ModelEntry // keyed by canonical name
	aliases map[string]string      // alias → canonical name
}

// defaultRegistry is the package-level singleton.
var defaultRegistry = &ModelRegistry{}

// initRegistry populates the registry with all known models.
// This consolidates the data previously scattered across ResolveModelAlias,
// ModelTokenLimitForModel, and MetadataForModel.
func (r *ModelRegistry) initRegistry() {
	r.once.Do(func() {
		r.models = make(map[string]*ModelEntry)
		r.aliases = make(map[string]string)

		entries := []ModelEntry{
			{
				Canonical:     "claude-opus-4-6",
				Provider:      ProviderAnthropic,
				MaxOutput:     32_000,
				ContextWindow: 200_000,
				Aliases:       []string{"opus"},
				Metadata: &ProviderMetadata{
					Provider:       ProviderAnthropic,
					AuthEnvVar:     "ANTHROPIC_API_KEY",
					BaseURLEnvVar:  "ANTHROPIC_BASE_URL",
					DefaultBaseURL: "https://api.anthropic.com",
				},
			},
			{
				Canonical:     "claude-sonnet-4-6",
				Provider:      ProviderAnthropic,
				MaxOutput:     64_000,
				ContextWindow: 200_000,
				Aliases:       []string{"sonnet"},
				Metadata: &ProviderMetadata{
					Provider:       ProviderAnthropic,
					AuthEnvVar:     "ANTHROPIC_API_KEY",
					BaseURLEnvVar:  "ANTHROPIC_BASE_URL",
					DefaultBaseURL: "https://api.anthropic.com",
				},
			},
			{
				Canonical:     "claude-haiku-4-5-20251213",
				Provider:      ProviderAnthropic,
				MaxOutput:     64_000,
				ContextWindow: 200_000,
				Aliases:       []string{"haiku"},
				Metadata: &ProviderMetadata{
					Provider:       ProviderAnthropic,
					AuthEnvVar:     "ANTHROPIC_API_KEY",
					BaseURLEnvVar:  "ANTHROPIC_BASE_URL",
					DefaultBaseURL: "https://api.anthropic.com",
				},
			},
			{
				Canonical:     "grok-3",
				Provider:      ProviderXai,
				MaxOutput:     64_000,
				ContextWindow: 131_072,
				Aliases:       []string{"grok"},
				Metadata: &ProviderMetadata{
					Provider:       ProviderXai,
					AuthEnvVar:     "XAI_API_KEY",
					BaseURLEnvVar:  "XAI_BASE_URL",
					DefaultBaseURL: "https://api.x.ai/v1",
				},
			},
			{
				Canonical:     "grok-3-mini",
				Provider:      ProviderXai,
				MaxOutput:     64_000,
				ContextWindow: 131_072,
				Aliases:       []string{"grok-mini"},
				Metadata: &ProviderMetadata{
					Provider:       ProviderXai,
					AuthEnvVar:     "XAI_API_KEY",
					BaseURLEnvVar:  "XAI_BASE_URL",
					DefaultBaseURL: "https://api.x.ai/v1",
				},
			},
			{
				Canonical: "grok-2",
				Provider:  ProviderXai,
				Aliases:   []string{},
				Metadata: &ProviderMetadata{
					Provider:       ProviderXai,
					AuthEnvVar:     "XAI_API_KEY",
					BaseURLEnvVar:  "XAI_BASE_URL",
					DefaultBaseURL: "https://api.x.ai/v1",
				},
			},
		}

		for i := range entries {
			entry := &entries[i]
			r.models[entry.Canonical] = entry
			for _, alias := range entry.Aliases {
				r.aliases[strings.ToLower(alias)] = entry.Canonical
			}
			// Also register the canonical name as an alias for lookup.
			r.aliases[strings.ToLower(entry.Canonical)] = entry.Canonical
		}
	})
}

// LookupModel returns the model entry for a given name or alias.
// Returns nil if the model is not found in the registry.
func (r *ModelRegistry) LookupModel(nameOrAlias string) *ModelEntry {
	r.initRegistry()
	lower := strings.ToLower(strings.TrimSpace(nameOrAlias))
	if canonical, ok := r.aliases[lower]; ok {
		return r.models[canonical]
	}
	return nil
}

// ResolveAlias resolves a model alias to its canonical name.
// Returns the input unchanged if no alias match is found.
func (r *ModelRegistry) ResolveAlias(nameOrAlias string) string {
	r.initRegistry()
	lower := strings.ToLower(strings.TrimSpace(nameOrAlias))
	if canonical, ok := r.aliases[lower]; ok {
		return canonical
	}
	return strings.TrimSpace(nameOrAlias)
}

// RegisterModel adds or replaces a model entry in the registry.
// This is intended for runtime extension (e.g., plugin-provided models).
func (r *ModelRegistry) RegisterModel(entry ModelEntry) {
	r.initRegistry()
	r.models[entry.Canonical] = &entry
	for _, alias := range entry.Aliases {
		r.aliases[strings.ToLower(alias)] = entry.Canonical
	}
	r.aliases[strings.ToLower(entry.Canonical)] = entry.Canonical
}

// DefaultModelRegistry returns the package-level singleton registry.
func DefaultModelRegistry() *ModelRegistry {
	defaultRegistry.initRegistry()
	return defaultRegistry
}
