package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func TestRemoteContextReadsEnvState(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"CLAUDE_CODE_REMOTE":            "true",
		"CLAUDE_CODE_REMOTE_SESSION_ID": "session-123",
		"ANTHROPIC_BASE_URL":            "https://remote.test",
	}
	ctx := FromEnvMap(env)

	if !ctx.Enabled {
		t.Error("Enabled should be true")
	}
	if ctx.SessionID == nil || *ctx.SessionID != "session-123" {
		t.Errorf("SessionID = %v, want session-123", ctx.SessionID)
	}
	if ctx.BaseURL != "https://remote.test" {
		t.Errorf("BaseURL = %q, want https://remote.test", ctx.BaseURL)
	}
}

func TestRemoteContextDefaultBaseURL(t *testing.T) {
	t.Parallel()
	ctx := FromEnvMap(map[string]string{})

	if ctx.Enabled {
		t.Error("Enabled should be false")
	}
	if ctx.SessionID != nil {
		t.Errorf("SessionID = %v, want nil", ctx.SessionID)
	}
	if ctx.BaseURL != DefaultRemoteBaseURL {
		t.Errorf("BaseURL = %q, want %q", ctx.BaseURL, DefaultRemoteBaseURL)
	}
}

func TestBootstrapFailsOpenWhenTokenOrSessionIsMissing(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"CLAUDE_CODE_REMOTE":         "1",
		"CCR_UPSTREAM_PROXY_ENABLED": "true",
	}
	bootstrap := BootstrapFromEnvMap(env)

	if bootstrap.ShouldEnable() {
		t.Error("ShouldEnable should be false")
	}
	state := bootstrap.StateForPort(8080)
	if state.Enabled {
		t.Error("state.Enabled should be false")
	}
}

func TestBootstrapDerivesProxyStateAndEnv(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	tokenPath := filepath.Join(root, "session_token")
	os.WriteFile(tokenPath, []byte("secret-token\n"), 0644)

	env := map[string]string{
		"CLAUDE_CODE_REMOTE":            "1",
		"CCR_UPSTREAM_PROXY_ENABLED":    "true",
		"CLAUDE_CODE_REMOTE_SESSION_ID": "session-123",
		"ANTHROPIC_BASE_URL":            "https://remote.test",
		"CCR_SESSION_TOKEN_PATH":        tokenPath,
		"CCR_CA_BUNDLE_PATH":            filepath.Join(root, "ca-bundle.crt"),
	}

	bootstrap := BootstrapFromEnvMap(env)

	if !bootstrap.ShouldEnable() {
		t.Error("ShouldEnable should be true")
	}
	if bootstrap.Token == nil || *bootstrap.Token != "secret-token" {
		t.Errorf("Token = %v, want secret-token", bootstrap.Token)
	}
	if bootstrap.WsURL() != "wss://remote.test/v1/code/upstreamproxy/ws" {
		t.Errorf("WsURL = %q, want wss://remote.test/v1/code/upstreamproxy/ws", bootstrap.WsURL())
	}

	state := bootstrap.StateForPort(9443)
	if !state.Enabled {
		t.Error("state.Enabled should be true")
	}

	subEnv := state.SubprocessEnv()
	if subEnv["HTTPS_PROXY"] != "http://127.0.0.1:9443" {
		t.Errorf("HTTPS_PROXY = %q, want http://127.0.0.1:9443", subEnv["HTTPS_PROXY"])
	}
	expectedCA := filepath.Join(root, "ca-bundle.crt")
	if subEnv["SSL_CERT_FILE"] != expectedCA {
		t.Errorf("SSL_CERT_FILE = %q, want %q", subEnv["SSL_CERT_FILE"], expectedCA)
	}
}

func TestTokenReaderTrimsAndHandlesMissingFiles(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	tokenPath := filepath.Join(root, "session_token")
	os.WriteFile(tokenPath, []byte(" abc123 \n"), 0644)

	token, err := ReadToken(tokenPath)
	if err != nil {
		t.Fatalf("ReadToken() error: %v", err)
	}
	if token == nil || *token != "abc123" {
		t.Errorf("token = %v, want abc123", token)
	}

	missing, err := ReadToken(filepath.Join(root, "missing"))
	if err != nil {
		t.Fatalf("ReadToken(missing) error: %v", err)
	}
	if missing != nil {
		t.Errorf("missing token = %v, want nil", missing)
	}
}

func TestTokenReaderEmptyFile(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	tokenPath := filepath.Join(root, "empty_token")
	os.WriteFile(tokenPath, []byte("  \n"), 0644)

	token, err := ReadToken(tokenPath)
	if err != nil {
		t.Fatalf("ReadToken() error: %v", err)
	}
	if token != nil {
		t.Errorf("token = %v, want nil for empty file", token)
	}
}

func TestInheritedProxyEnvRequiresProxyAndCA(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"HTTPS_PROXY":   "http://127.0.0.1:8888",
		"SSL_CERT_FILE": "/tmp/ca-bundle.crt",
		"NO_PROXY":      "localhost",
	}
	inherited := InheritedUpstreamProxyEnv(env)
	if len(inherited) != 3 {
		t.Errorf("len(inherited) = %d, want 3", len(inherited))
	}
	if inherited["NO_PROXY"] != "localhost" {
		t.Errorf("NO_PROXY = %q, want localhost", inherited["NO_PROXY"])
	}

	empty := InheritedUpstreamProxyEnv(map[string]string{})
	if len(empty) != 0 {
		t.Errorf("len(empty) = %d, want 0", len(empty))
	}
}

func TestHelperOutputsMatchExpectedShapes(t *testing.T) {
	t.Parallel()

	wsURL := UpstreamProxyWsURL("http://localhost:3000/")
	if wsURL != "ws://localhost:3000/v1/code/upstreamproxy/ws" {
		t.Errorf("WsURL = %q, want ws://localhost:3000/v1/code/upstreamproxy/ws", wsURL)
	}

	noProxy := NoProxyList()
	if !containsSubstring(noProxy, "anthropic.com") {
		t.Error("NoProxyList should contain anthropic.com")
	}
	if !containsSubstring(noProxy, "github.com") {
		t.Error("NoProxyList should contain github.com")
	}
}

func TestWsURLConversions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"https://api.anthropic.com", "wss://api.anthropic.com/v1/code/upstreamproxy/ws"},
		{"http://localhost:3000/", "ws://localhost:3000/v1/code/upstreamproxy/ws"},
		{"api.anthropic.com", "wss://api.anthropic.com/v1/code/upstreamproxy/ws"},
	}
	for _, tc := range cases {
		got := UpstreamProxyWsURL(tc.input)
		if got != tc.want {
			t.Errorf("UpstreamProxyWsURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDisabledStateSubprocessEnv(t *testing.T) {
	t.Parallel()
	state := DisabledState()
	env := state.SubprocessEnv()
	if len(env) != 0 {
		t.Errorf("len(env) = %d, want 0 for disabled state", len(env))
	}
}

func TestEnvTruthyValues(t *testing.T) {
	t.Parallel()
	truthy := []string{"1", "true", "yes", "on", " TRUE ", " Yes "}
	for _, v := range truthy {
		ctx := FromEnvMap(map[string]string{"CLAUDE_CODE_REMOTE": v})
		if !ctx.Enabled {
			t.Errorf("envTruthy(%q) = false, want true", v)
		}
	}

	falsy := []string{"0", "false", "no", "off", ""}
	for _, v := range falsy {
		ctx := FromEnvMap(map[string]string{"CLAUDE_CODE_REMOTE": v})
		if ctx.Enabled {
			t.Errorf("envTruthy(%q) = true, want false", v)
		}
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
