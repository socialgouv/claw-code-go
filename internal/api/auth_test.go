package api

import (
	"net/http"
	"testing"
)

func TestNoAuth(t *testing.T) {
	a := NoAuth()
	if a.Kind != AuthSourceNone {
		t.Fatalf("expected AuthSourceNone, got %d", a.Kind)
	}
	if a.HasAPIKey() {
		t.Fatal("NoAuth should not have API key")
	}
	if a.HasBearerToken() {
		t.Fatal("NoAuth should not have bearer token")
	}
}

func TestAPIKeyAuth(t *testing.T) {
	a := APIKeyAuth("sk-test-123")
	if a.Kind != AuthSourceAPIKey {
		t.Fatalf("expected AuthSourceAPIKey, got %d", a.Kind)
	}
	if a.APIKey != "sk-test-123" {
		t.Fatalf("expected API key sk-test-123, got %s", a.APIKey)
	}
	if !a.HasAPIKey() {
		t.Fatal("should have API key")
	}
	if a.HasBearerToken() {
		t.Fatal("should not have bearer token")
	}
}

func TestBearerAuth(t *testing.T) {
	a := BearerAuth("tok-abc")
	if a.Kind != AuthSourceBearer {
		t.Fatalf("expected AuthSourceBearer, got %d", a.Kind)
	}
	if a.BearerToken != "tok-abc" {
		t.Fatalf("expected bearer token tok-abc, got %s", a.BearerToken)
	}
	if a.HasAPIKey() {
		t.Fatal("should not have API key")
	}
	if !a.HasBearerToken() {
		t.Fatal("should have bearer token")
	}
}

func TestCombinedAuth(t *testing.T) {
	a := CombinedAuth("sk-key", "tok-bearer")
	if a.Kind != AuthSourceCombined {
		t.Fatalf("expected AuthSourceCombined, got %d", a.Kind)
	}
	if a.APIKey != "sk-key" {
		t.Fatalf("expected API key sk-key, got %s", a.APIKey)
	}
	if a.BearerToken != "tok-bearer" {
		t.Fatalf("expected bearer token tok-bearer, got %s", a.BearerToken)
	}
	if !a.HasAPIKey() {
		t.Fatal("should have API key")
	}
	if !a.HasBearerToken() {
		t.Fatal("should have bearer token")
	}
}

func TestApplyToRequest_None(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	NoAuth().ApplyToRequest(req)
	if req.Header.Get("x-api-key") != "" {
		t.Fatal("None auth should not set x-api-key")
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("None auth should not set Authorization")
	}
}

func TestApplyToRequest_APIKey(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	APIKeyAuth("sk-123").ApplyToRequest(req)
	if got := req.Header.Get("x-api-key"); got != "sk-123" {
		t.Fatalf("expected x-api-key=sk-123, got %s", got)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("APIKey auth should not set Authorization")
	}
}

func TestApplyToRequest_Bearer(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	BearerAuth("tok-abc").ApplyToRequest(req)
	if got := req.Header.Get("Authorization"); got != "Bearer tok-abc" {
		t.Fatalf("expected Authorization=Bearer tok-abc, got %s", got)
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatal("Bearer auth should not set x-api-key")
	}
}

func TestApplyToRequest_Combined(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	CombinedAuth("sk-key", "tok-bearer").ApplyToRequest(req)
	if got := req.Header.Get("x-api-key"); got != "sk-key" {
		t.Fatalf("expected x-api-key=sk-key, got %s", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok-bearer" {
		t.Fatalf("expected Authorization=Bearer tok-bearer, got %s", got)
	}
}

func TestResolveStartupAuth_BothSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-env")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok-env")
	a := ResolveStartupAuth()
	if a.Kind != AuthSourceCombined {
		t.Fatalf("expected Combined, got %d", a.Kind)
	}
	if a.APIKey != "sk-env" {
		t.Fatalf("expected API key sk-env, got %s", a.APIKey)
	}
	if a.BearerToken != "tok-env" {
		t.Fatalf("expected bearer token tok-env, got %s", a.BearerToken)
	}
}

func TestResolveStartupAuth_OnlyAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-only")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	a := ResolveStartupAuth()
	if a.Kind != AuthSourceAPIKey {
		t.Fatalf("expected APIKey, got %d", a.Kind)
	}
	if a.APIKey != "sk-only" {
		t.Fatalf("expected sk-only, got %s", a.APIKey)
	}
}

func TestResolveStartupAuth_OnlyBearer(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok-only")
	a := ResolveStartupAuth()
	if a.Kind != AuthSourceBearer {
		t.Fatalf("expected Bearer, got %d", a.Kind)
	}
	if a.BearerToken != "tok-only" {
		t.Fatalf("expected tok-only, got %s", a.BearerToken)
	}
}

func TestResolveStartupAuth_Neither(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	a := ResolveStartupAuth()
	if a.Kind != AuthSourceNone {
		t.Fatalf("expected None, got %d", a.Kind)
	}
}
