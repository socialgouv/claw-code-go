package apikit

import (
	"errors"
	"testing"
)

func TestModelTokenLimitsKnownModels(t *testing.T) {
	tests := []struct {
		model         string
		maxOutput     uint32
		contextWindow uint32
	}{
		{"claude-opus-4-6", 32_000, 200_000},
		{"claude-sonnet-4-6", 64_000, 200_000},
		{"claude-haiku-4-5-20251213", 64_000, 200_000},
		{"grok-3", 64_000, 131_072},
		{"grok-3-mini", 64_000, 131_072},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			limit := ModelTokenLimitForModel(tt.model)
			if limit == nil {
				t.Fatal("expected non-nil limit")
			}
			if limit.MaxOutputTokens != tt.maxOutput {
				t.Errorf("MaxOutputTokens: got %d, want %d", limit.MaxOutputTokens, tt.maxOutput)
			}
			if limit.ContextWindowTokens != tt.contextWindow {
				t.Errorf("ContextWindowTokens: got %d, want %d", limit.ContextWindowTokens, tt.contextWindow)
			}
		})
	}
}

func TestModelTokenLimitUnknownModelReturnsNil(t *testing.T) {
	limit := ModelTokenLimitForModel("unknown-model-v99")
	if limit != nil {
		t.Error("unknown model should return nil")
	}
}

func TestPreflightCheckPassesForUnknownModel(t *testing.T) {
	err := PreflightCheck("unknown-model", 999_999, 999_999)
	if err != nil {
		t.Errorf("unknown model should pass through, got: %v", err)
	}
}

func TestPreflightCheckPassesWithinLimit(t *testing.T) {
	// claude-opus-4-6: 200k context window
	err := PreflightCheck("claude-opus-4-6", 160_000, 32_000)
	if err != nil {
		t.Errorf("within-limit request should pass, got: %v", err)
	}
}

func TestPreflightCheckFailsExceedingLimit(t *testing.T) {
	// claude-opus-4-6: 200k context window, 190k input + 32k output = 222k > 200k
	err := PreflightCheck("claude-opus-4-6", 190_000, 32_000)
	if err == nil {
		t.Fatal("expected ContextWindowExceeded error")
	}

	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected ApiError")
	}
	if apiErr.Kind != ErrContextWindowExceeded {
		t.Errorf("expected ErrContextWindowExceeded, got %d", apiErr.Kind)
	}
	if apiErr.Model != "claude-opus-4-6" {
		t.Errorf("expected model claude-opus-4-6, got %s", apiErr.Model)
	}
	if apiErr.EstimatedInputTokens != 190_000 {
		t.Errorf("expected 190000 input tokens, got %d", apiErr.EstimatedInputTokens)
	}
	if apiErr.RequestedOutputTokens != 32_000 {
		t.Errorf("expected 32000 output tokens, got %d", apiErr.RequestedOutputTokens)
	}
	if apiErr.ContextWindowTokens != 200_000 {
		t.Errorf("expected 200000 context window, got %d", apiErr.ContextWindowTokens)
	}
}

func TestPreflightCheckExactBoundary(t *testing.T) {
	// Exactly at the limit should pass
	err := PreflightCheck("claude-opus-4-6", 168_000, 32_000) // 200_000 exactly
	if err != nil {
		t.Errorf("exact boundary should pass, got: %v", err)
	}

	// One over should fail
	err = PreflightCheck("claude-opus-4-6", 168_001, 32_000) // 200_001
	if err == nil {
		t.Error("one over boundary should fail")
	}
}

func TestEstimateSerializedTokens(t *testing.T) {
	// Simple string
	tokens := EstimateSerializedTokens("hello world")
	if tokens == 0 {
		t.Error("should estimate non-zero tokens")
	}

	// Larger object should produce more tokens
	small := EstimateSerializedTokens("hi")
	large := EstimateSerializedTokens(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "This is a much longer message that should produce more tokens"},
		},
	})
	if large <= small {
		t.Errorf("larger object should estimate more tokens: small=%d, large=%d", small, large)
	}
}
