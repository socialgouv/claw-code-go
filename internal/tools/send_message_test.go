package tools

import (
	"strings"
	"testing"
)

func TestSendUserMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr string
		wantOut string
	}{
		{
			name: "happy path with message and status",
			input: map[string]any{
				"message": "Hello user",
				"status":  "proactive",
			},
			wantOut: "Hello user",
		},
		{
			name:    "missing message",
			input:   map[string]any{"status": "normal"},
			wantErr: "'message' is required",
		},
		{
			name:    "empty message",
			input:   map[string]any{"message": "", "status": "normal"},
			wantErr: "'message' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ExecuteSendUserMessage(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Fatalf("output %q does not contain %q", out, tt.wantOut)
			}
		})
	}
}
