// Package apikit re-exports selected symbols from internal/apikit via type
// aliases. This is the public surface for external consumers (e.g., iterion).
// Internal consumers continue importing internal/apikit unchanged.
package apikit

import (
	"github.com/SocialGouv/claw-code-go/internal/apikit"
)

// SessionTracer records telemetry events scoped to a session.
type SessionTracer = apikit.SessionTracer

// PromptCache manages completion caching and cache break detection.
type PromptCache = apikit.PromptCache

// ModelTokenLimit holds the token limits for a known model.
type ModelTokenLimit = apikit.ModelTokenLimit

// PreflightMessageRequest checks whether a request would exceed the model's
// context window before sending it.
var PreflightMessageRequest = apikit.PreflightMessageRequest

// ResolveModelAlias resolves a model alias to its canonical name.
var ResolveModelAlias = apikit.ResolveModelAlias

// ModelTokenLimitForModel returns the token limits for a known model.
var ModelTokenLimitForModel = apikit.ModelTokenLimitForModel
