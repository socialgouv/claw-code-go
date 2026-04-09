package apikit

import (
	"encoding/json"
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
// For now this is a pass-through; real alias resolution would map
// e.g. "claude-3-7-sonnet-latest" → "claude-sonnet-4-6".
func ResolveModelAlias(model string) string {
	return model
}

// PreflightCheck validates that the estimated token usage fits within the
// model's context window. Returns a ContextWindowExceeded error if it
// doesn't, or nil for unknown models (pass-through).
func PreflightCheck(model string, estimatedInputTokens, requestedOutputTokens uint32) error {
	limit := ModelTokenLimitForModel(model)
	if limit == nil {
		return nil
	}

	total := estimatedInputTokens + requestedOutputTokens
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

// EstimateSerializedTokens estimates token count by serializing to JSON and
// dividing by 4 (rough heuristic matching Rust's implementation).
func EstimateSerializedTokens(value any) uint32 {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return uint32(len(data)/4 + 1)
}
