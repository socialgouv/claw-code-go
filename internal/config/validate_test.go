package config

import (
	"strings"
	"testing"
)

func TestValidateSettingsJSONValid(t *testing.T) {
	data := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"maxTokens": 200000,
		"theme": "dark"
	}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if !result.IsClean() {
		t.Errorf("expected clean result, got errors=%d warnings=%d", len(result.Errors), len(result.Warnings))
		t.Log(FormatDiagnostics(&result))
	}
}

func TestValidateSettingsJSONEmpty(t *testing.T) {
	data := []byte(`{}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if !result.IsClean() {
		t.Errorf("expected clean result for empty config, got errors=%d warnings=%d", len(result.Errors), len(result.Warnings))
	}
}

func TestValidateSettingsJSONUnknownKeyWithSuggestion(t *testing.T) {
	data := []byte(`{"modle": "claude-sonnet-4-20250514"}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning for unknown key 'modle'")
	}
	found := false
	for _, w := range result.Warnings {
		ukd, ok := w.Kind.(UnknownKeyDiag)
		if ok && ukd.Suggestion == "model" {
			found = true
		}
	}
	if !found {
		t.Error("expected suggestion 'model' for typo 'modle'")
	}
}

func TestValidateSettingsJSONUnknownKeyShortNoSuggestion(t *testing.T) {
	// Short keys (< 4 chars) should NOT get suggestions to avoid false positives.
	data := []byte(`{"foo": "bar"}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning for unknown key 'foo'")
	}
	for _, w := range result.Warnings {
		ukd, ok := w.Kind.(UnknownKeyDiag)
		if ok && ukd.Suggestion != "" {
			t.Errorf("short key should not get suggestion, got %q", ukd.Suggestion)
		}
	}
}

func TestValidateSettingsJSONWrongType(t *testing.T) {
	data := []byte(`{"maxTokens": "abc"}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if len(result.Errors) == 0 {
		t.Fatal("expected error for wrong type maxTokens")
	}
	found := false
	for _, e := range result.Errors {
		if _, ok := e.Kind.(WrongTypeDiag); ok && e.Field == "maxTokens" {
			found = true
		}
	}
	if !found {
		t.Error("expected WrongTypeDiag for maxTokens")
	}
}

func TestValidateSettingsJSONDeprecated(t *testing.T) {
	data := []byte(`{"permissionMode": "default"}`)
	result := ValidateSettingsJSON(data, "settings.json")
	found := false
	for _, w := range result.Warnings {
		if dd, ok := w.Kind.(DeprecatedDiag); ok && dd.Replacement == "permissions.defaultMode" {
			found = true
		}
	}
	if !found {
		t.Error("expected deprecated warning for permissionMode")
	}
}

func TestValidateSettingsJSONInvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	result := ValidateSettingsJSON(data, "settings.json")
	if len(result.Errors) == 0 {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateSettingsJSONNestedHooksUnknownKey(t *testing.T) {
	data := []byte(`{"hooks": {"PreToolUse": ["cmd1"], "BadHook": ["cmd2"]}}`)
	result := ValidateSettingsJSON(data, "settings.json")
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "BadHook") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for unknown hook key 'BadHook'")
	}
}

func TestValidateSettingsJSONHooksWrongType(t *testing.T) {
	data := []byte(`{"hooks": "not an object"}`)
	result := ValidateSettingsJSON(data, "settings.json")
	if len(result.Errors) == 0 {
		t.Fatal("expected error for hooks wrong type")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"model", "modle", 2},
		{"theme", "theem", 2},
		{"cat", "car", 1},
	}
	for _, tt := range tests {
		got := levenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFormatDiagnostics(t *testing.T) {
	result := &ValidationResult{
		Errors: []ConfigDiagnostic{
			{Path: "s.json", Field: "x", Kind: WrongTypeDiag{Expected: "string", Got: "number"}},
		},
		Warnings: []ConfigDiagnostic{
			{Path: "s.json", Field: "y", Kind: UnknownKeyDiag{Suggestion: "z"}},
		},
	}
	out := FormatDiagnostics(result)
	if !strings.Contains(out, "error:") || !strings.Contains(out, "warning:") {
		t.Errorf("unexpected format: %s", out)
	}
}
