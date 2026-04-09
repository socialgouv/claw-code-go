package apikit

import "testing"

func TestResolveModelAliasRouting(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"opus", "claude-opus-4-6"},
		{"sonnet", "claude-sonnet-4-6"},
		{"haiku", "claude-haiku-4-5-20251213"},
		{"grok", "grok-3"},
		{"grok-3", "grok-3"},
		{"grok-mini", "grok-3-mini"},
		{"grok-3-mini", "grok-3-mini"},
		{"grok-2", "grok-2"},
		// Case insensitivity
		{"Opus", "claude-opus-4-6"},
		{"SONNET", "claude-sonnet-4-6"},
		{"Haiku", "claude-haiku-4-5-20251213"},
		{"GROK", "grok-3"},
		// Unknown model passes through
		{"my-custom-model", "my-custom-model"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveModelAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveModelAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMetadataForModel(t *testing.T) {
	tests := []struct {
		model    string
		wantNil  bool
		provider ProviderKind
		authEnv  string
		baseURL  string
	}{
		{"claude-sonnet-4-6", false, ProviderAnthropic, "ANTHROPIC_API_KEY", "https://api.anthropic.com"},
		{"claude-opus-4-6", false, ProviderAnthropic, "ANTHROPIC_API_KEY", "https://api.anthropic.com"},
		{"grok-3", false, ProviderXai, "XAI_API_KEY", "https://api.x.ai"},
		{"grok-3-mini", false, ProviderXai, "XAI_API_KEY", "https://api.x.ai"},
		{"openai/gpt-4", false, ProviderOpenAI, "OPENAI_API_KEY", "https://api.openai.com"},
		{"gpt-4o", false, ProviderOpenAI, "OPENAI_API_KEY", "https://api.openai.com"},
		{"qwen/qwen-max", false, ProviderOpenAI, "DASHSCOPE_API_KEY", "https://dashscope.aliyuncs.com"},
		{"qwen-turbo", false, ProviderOpenAI, "DASHSCOPE_API_KEY", "https://dashscope.aliyuncs.com"},
		{"unknown-model", true, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			meta := MetadataForModel(tt.model)
			if tt.wantNil {
				if meta != nil {
					t.Errorf("MetadataForModel(%q) = %+v, want nil", tt.model, meta)
				}
				return
			}
			if meta == nil {
				t.Fatalf("MetadataForModel(%q) = nil, want non-nil", tt.model)
			}
			if meta.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", meta.Provider, tt.provider)
			}
			if meta.AuthEnvVar != tt.authEnv {
				t.Errorf("AuthEnvVar = %q, want %q", meta.AuthEnvVar, tt.authEnv)
			}
			if meta.DefaultBaseURL != tt.baseURL {
				t.Errorf("DefaultBaseURL = %q, want %q", meta.DefaultBaseURL, tt.baseURL)
			}
		})
	}
}

func TestDetectProviderKind(t *testing.T) {
	t.Run("explicit prefix wins", func(t *testing.T) {
		got := DetectProviderKind("claude-sonnet-4-6")
		if got != ProviderAnthropic {
			t.Errorf("got %q, want %q", got, ProviderAnthropic)
		}
	})

	t.Run("explicit prefix grok", func(t *testing.T) {
		got := DetectProviderKind("grok-3")
		if got != ProviderXai {
			t.Errorf("got %q, want %q", got, ProviderXai)
		}
	})

	t.Run("fallback to ANTHROPIC_API_KEY", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-test")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("XAI_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		got := DetectProviderKind("unknown-model")
		if got != ProviderAnthropic {
			t.Errorf("got %q, want %q", got, ProviderAnthropic)
		}
	})

	t.Run("fallback to ANTHROPIC_AUTH_TOKEN", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "token-test")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("XAI_API_KEY", "")
		got := DetectProviderKind("unknown-model")
		if got != ProviderAnthropic {
			t.Errorf("got %q, want %q", got, ProviderAnthropic)
		}
	})

	t.Run("fallback to OPENAI_API_KEY", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		t.Setenv("OPENAI_API_KEY", "sk-openai")
		t.Setenv("XAI_API_KEY", "")
		got := DetectProviderKind("unknown-model")
		if got != ProviderOpenAI {
			t.Errorf("got %q, want %q", got, ProviderOpenAI)
		}
	})

	t.Run("fallback to XAI_API_KEY", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("XAI_API_KEY", "xai-test")
		got := DetectProviderKind("unknown-model")
		if got != ProviderXai {
			t.Errorf("got %q, want %q", got, ProviderXai)
		}
	})

	t.Run("default to Anthropic when no env", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("XAI_API_KEY", "")
		got := DetectProviderKind("unknown-model")
		if got != ProviderAnthropic {
			t.Errorf("got %q, want %q", got, ProviderAnthropic)
		}
	})
}

func TestLookupModelTokenLimit(t *testing.T) {
	tests := []struct {
		model       string
		wantNil     bool
		wantOutput  uint32
		wantContext uint32
	}{
		{"claude-opus-4-6", false, 32000, 200000},
		{"claude-sonnet-4-6", false, 64000, 200000},
		{"claude-haiku-4-5-20251213", false, 64000, 200000},
		{"grok-3", false, 64000, 131072},
		{"grok-3-mini", false, 64000, 131072},
		{"unknown-model", true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := LookupModelTokenLimit(tt.model)
			if tt.wantNil {
				if got != nil {
					t.Errorf("LookupModelTokenLimit(%q) = %+v, want nil", tt.model, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("LookupModelTokenLimit(%q) = nil, want non-nil", tt.model)
			}
			if got.MaxOutputTokens != tt.wantOutput {
				t.Errorf("MaxOutputTokens = %d, want %d", got.MaxOutputTokens, tt.wantOutput)
			}
			if got.ContextWindowTokens != tt.wantContext {
				t.Errorf("ContextWindowTokens = %d, want %d", got.ContextWindowTokens, tt.wantContext)
			}
		})
	}
}
