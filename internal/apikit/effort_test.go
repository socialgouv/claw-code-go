package apikit

import (
	"reflect"
	"testing"
)

func TestEffortCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantSupported     []string
		wantDefault       string
		wantSupportedLen  int  // sanity check on length when full slice is too verbose
		wantNotSupported  bool // true when reasoning_effort is not supported by this model
	}{
		{
			name:          "opus 4.7 by canonical",
			input:         "claude-opus-4-7",
			wantSupported: []string{"low", "medium", "high", "xhigh", "max"},
			wantDefault:   "xhigh",
		},
		{
			name:          "opus 4.7 by alias",
			input:         "opus",
			wantSupported: []string{"low", "medium", "high", "xhigh", "max"},
			wantDefault:   "xhigh",
		},
		{
			name:          "opus 4.6 has no xhigh",
			input:         "claude-opus-4-6",
			wantSupported: []string{"low", "medium", "high", "max"},
			wantDefault:   "high",
		},
		{
			name:          "sonnet 4.6 has no xhigh",
			input:         "claude-sonnet-4-6",
			wantSupported: []string{"low", "medium", "high", "max"},
			wantDefault:   "high",
		},
		{
			name:             "haiku does not support effort",
			input:            "claude-haiku-4-5",
			wantNotSupported: true,
		},
		{
			name:             "sonnet 4.7 effort not yet documented",
			input:            "claude-sonnet-4-7",
			wantNotSupported: true,
		},
		{
			name:          "openai gpt-5.5",
			input:         "gpt-5.5",
			wantSupported: []string{"minimal", "low", "medium", "high"},
			wantDefault:   "",
		},
		{
			name:          "openai openrouter alias",
			input:         "openai/gpt-5.4-mini",
			wantSupported: []string{"minimal", "low", "medium", "high"},
			wantDefault:   "",
		},
		{
			name:             "grok no documented matrix",
			input:            "grok-3",
			wantNotSupported: true,
		},
		{
			name:             "unknown model",
			input:            "totally-made-up",
			wantNotSupported: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, def := EffortCapabilities(tt.input)
			if tt.wantNotSupported {
				if got != nil || def != "" {
					t.Errorf("EffortCapabilities(%q) = (%v, %q), want (nil, \"\")", tt.input, got, def)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.wantSupported) {
				t.Errorf("EffortCapabilities(%q) supported = %v, want %v", tt.input, got, tt.wantSupported)
			}
			if def != tt.wantDefault {
				t.Errorf("EffortCapabilities(%q) default = %q, want %q", tt.input, def, tt.wantDefault)
			}
		})
	}
}
