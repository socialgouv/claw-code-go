package tools

import (
	"strings"
	"testing"
)

func TestPlanMode(t *testing.T) {
	t.Run("enter plan mode sets active true", func(t *testing.T) {
		active := false
		out, err := ExecuteEnterPlanMode(&active)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !active {
			t.Fatal("expected active to be true")
		}
		if !strings.Contains(out, `"changed": true`) {
			t.Fatalf("expected changed=true in output, got %s", out)
		}
	})

	t.Run("enter plan mode when already active", func(t *testing.T) {
		active := true
		out, err := ExecuteEnterPlanMode(&active)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"changed": false`) {
			t.Fatalf("expected changed=false in output, got %s", out)
		}
	})

	t.Run("exit plan mode sets active false", func(t *testing.T) {
		active := true
		out, err := ExecuteExitPlanMode(&active)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if active {
			t.Fatal("expected active to be false")
		}
		if !strings.Contains(out, `"changed": true`) {
			t.Fatalf("expected changed=true in output, got %s", out)
		}
	})

	t.Run("exit plan mode when not active", func(t *testing.T) {
		active := false
		out, err := ExecuteExitPlanMode(&active)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"changed": false`) {
			t.Fatalf("expected changed=false in output, got %s", out)
		}
	})

	t.Run("enter nil pointer returns error", func(t *testing.T) {
		_, err := ExecuteEnterPlanMode(nil)
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("expected nil pointer error, got %v", err)
		}
	})

	t.Run("exit nil pointer returns error", func(t *testing.T) {
		_, err := ExecuteExitPlanMode(nil)
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("expected nil pointer error, got %v", err)
		}
	})
}
