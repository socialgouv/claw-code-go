package apikit

import (
	"encoding/json"
	"math"
	"strings"
)

// ModelTokenLimit holds the token limits for a known model.
type ModelTokenLimit struct {
	MaxOutputTokens     uint32
	ContextWindowTokens uint32
}

// ModelTokenLimitForModel returns the token limits for a known model, or nil
// for unknown models. Matches the Rust model_token_limit() lookup table.
func ModelTokenLimitForModel(model string) *ModelTokenLimit {
	canonical := ResolveModelAlias(model)
	switch canonical {
	case "claude-opus-4-6":
		return &ModelTokenLimit{MaxOutputTokens: 32_000, ContextWindowTokens: 200_000}
	case "claude-sonnet-4-6", "claude-haiku-4-5-20251213":
		return &ModelTokenLimit{MaxOutputTokens: 64_000, ContextWindowTokens: 200_000}
	case "grok-3", "grok-3-mini":
		return &ModelTokenLimit{MaxOutputTokens: 64_000, ContextWindowTokens: 131_072}
	default:
		return nil
	}
}

// ResolveModelAlias normalizes model names to their canonical form.
// Matches the Rust resolve_model_alias() in providers/mod.rs:128-155.
func ResolveModelAlias(model string) string {
	trimmed := strings.TrimSpace(model)
	lower := strings.ToLower(trimmed)
	switch lower {
	case "opus":
		return "claude-opus-4-6"
	case "sonnet":
		return "claude-sonnet-4-6"
	case "haiku":
		return "claude-haiku-4-5-20251213"
	case "grok", "grok-3":
		return "grok-3"
	case "grok-mini", "grok-3-mini":
		return "grok-3-mini"
	case "grok-2":
		return "grok-2"
	default:
		return trimmed
	}
}

// PreflightCheck validates that the estimated token usage fits within the
// model's context window. Returns a ContextWindowExceeded error if it
// doesn't, or nil for unknown models (pass-through).
func PreflightCheck(model string, estimatedInputTokens, requestedOutputTokens uint32) error {
	limit := ModelTokenLimitForModel(model)
	if limit == nil {
		return nil
	}

	total := saturatingAddU32(estimatedInputTokens, requestedOutputTokens)
	if total > limit.ContextWindowTokens {
		return &ApiError{
			Kind:                  ErrContextWindowExceeded,
			Model:                 ResolveModelAlias(model),
			EstimatedInputTokens:  estimatedInputTokens,
			RequestedOutputTokens: requestedOutputTokens,
			EstimatedTotalTokens:  total,
			ContextWindowTokens:   limit.ContextWindowTokens,
		}
	}
	return nil
}

// saturatingAddU32 adds two uint32 values, capping at math.MaxUint32 on overflow.
// Matches Rust's u32::saturating_add().
func saturatingAddU32(a, b uint32) uint32 {
	sum := a + b
	if sum < a { // overflow wrapped
		return math.MaxUint32
	}
	return sum
}

// EstimateSerializedTokens estimates token count by serializing to JSON and
// dividing by 4 (rough heuristic matching Rust's implementation).
func EstimateSerializedTokens(value any) uint32 {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return uint32(len(data)/4 + 1)
}
