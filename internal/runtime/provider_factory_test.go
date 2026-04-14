package runtime

import (
	"testing"
)

func TestSelectProviderXAI(t *testing.T) {
	p := SelectProvider("xai")
	if p == nil {
		t.Fatal("SelectProvider(\"xai\") returned nil")
	}
	if got := p.Name(); got != "openai" {
		t.Errorf("SelectProvider(\"xai\").Name() = %q, want \"openai\"", got)
	}
}

func TestSelectProviderDashScope(t *testing.T) {
	p := SelectProvider("dashscope")
	if p == nil {
		t.Fatal("SelectProvider(\"dashscope\") returned nil")
	}
	if got := p.Name(); got != "openai" {
		t.Errorf("SelectProvider(\"dashscope\").Name() = %q, want \"openai\"", got)
	}
}

func TestSelectProviderOpenAI(t *testing.T) {
	p := SelectProvider("openai")
	if p == nil {
		t.Fatal("SelectProvider(\"openai\") returned nil")
	}
	if got := p.Name(); got != "openai" {
		t.Errorf("SelectProvider(\"openai\").Name() = %q, want \"openai\"", got)
	}
}

func TestSelectProviderAnthropic(t *testing.T) {
	p := SelectProvider("anthropic")
	if p == nil {
		t.Fatal("SelectProvider(\"anthropic\") returned nil")
	}
	if got := p.Name(); got != "anthropic" {
		t.Errorf("SelectProvider(\"anthropic\").Name() = %q, want \"anthropic\"", got)
	}
}

func TestSelectProviderDefaultIsAnthropic(t *testing.T) {
	p := SelectProvider("unknown-provider")
	if p == nil {
		t.Fatal("SelectProvider(\"unknown-provider\") returned nil")
	}
	if got := p.Name(); got != "anthropic" {
		t.Errorf("SelectProvider(\"unknown-provider\").Name() = %q, want \"anthropic\" (default)", got)
	}
}
