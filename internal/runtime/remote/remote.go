package remote

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Default constants for remote session configuration.
const (
	DefaultRemoteBaseURL    = "https://api.anthropic.com"
	DefaultSessionTokenPath = "/run/ccr/session_token"
	DefaultSystemCABundle   = "/etc/ssl/certs/ca-certificates.crt"
)

// UpstreamProxyEnvKeys are the environment variable keys propagated to subprocesses.
var UpstreamProxyEnvKeys = [...]string{
	"HTTPS_PROXY",
	"https_proxy",
	"NO_PROXY",
	"no_proxy",
	"SSL_CERT_FILE",
	"NODE_EXTRA_CA_CERTS",
	"REQUESTS_CA_BUNDLE",
	"CURL_CA_BUNDLE",
}

// NoProxyHosts is the hardcoded list of hosts that should bypass the proxy.
var NoProxyHosts = [...]string{
	"localhost",
	"127.0.0.1",
	"::1",
	"169.254.0.0/16",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"anthropic.com",
	".anthropic.com",
	"*.anthropic.com",
	"github.com",
	"api.github.com",
	"*.github.com",
	"*.githubusercontent.com",
	"registry.npmjs.org",
	"index.crates.io",
}

// ---------------------------------------------------------------------------
// RemoteSessionContext
// ---------------------------------------------------------------------------

// RemoteSessionContext holds the remote session configuration.
type RemoteSessionContext struct {
	Enabled   bool
	SessionID *string
	BaseURL   string
}

// FromEnv creates a RemoteSessionContext from the current environment.
func FromEnv() RemoteSessionContext {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return FromEnvMap(env)
}

// FromEnvMap creates a RemoteSessionContext from the given environment map.
func FromEnvMap(env map[string]string) RemoteSessionContext {
	ctx := RemoteSessionContext{
		Enabled: envTruthy(env["CLAUDE_CODE_REMOTE"]),
		BaseURL: DefaultRemoteBaseURL,
	}

	if sid, ok := env["CLAUDE_CODE_REMOTE_SESSION_ID"]; ok && sid != "" {
		ctx.SessionID = &sid
	}
	if base, ok := env["ANTHROPIC_BASE_URL"]; ok && base != "" {
		ctx.BaseURL = base
	}
	return ctx
}

// ---------------------------------------------------------------------------
// UpstreamProxyBootstrap
// ---------------------------------------------------------------------------

// UpstreamProxyBootstrap handles upstream proxy configuration.
type UpstreamProxyBootstrap struct {
	Remote               RemoteSessionContext
	UpstreamProxyEnabled bool
	TokenPath            string
	CABundlePath         string
	SystemCAPath         string
	Token                *string
}

// BootstrapFromEnv creates an UpstreamProxyBootstrap from the current environment.
func BootstrapFromEnv() UpstreamProxyBootstrap {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return BootstrapFromEnvMap(env)
}

// BootstrapFromEnvMap creates an UpstreamProxyBootstrap from the given environment map.
func BootstrapFromEnvMap(env map[string]string) UpstreamProxyBootstrap {
	remote := FromEnvMap(env)

	tokenPath := DefaultSessionTokenPath
	if v, ok := env["CCR_SESSION_TOKEN_PATH"]; ok && v != "" {
		tokenPath = v
	}

	systemCAPath := DefaultSystemCABundle
	if v, ok := env["CCR_SYSTEM_CA_BUNDLE"]; ok && v != "" {
		systemCAPath = v
	}

	caBundlePath := defaultCABundlePath()
	if v, ok := env["CCR_CA_BUNDLE_PATH"]; ok && v != "" {
		caBundlePath = v
	}

	token, _ := ReadToken(tokenPath)

	return UpstreamProxyBootstrap{
		Remote:               remote,
		UpstreamProxyEnabled: envTruthy(env["CCR_UPSTREAM_PROXY_ENABLED"]),
		TokenPath:            tokenPath,
		CABundlePath:         caBundlePath,
		SystemCAPath:         systemCAPath,
		Token:                token,
	}
}

// ShouldEnable returns true if all preconditions for upstream proxy are met.
func (b *UpstreamProxyBootstrap) ShouldEnable() bool {
	return b.Remote.Enabled &&
		b.UpstreamProxyEnabled &&
		b.Remote.SessionID != nil &&
		b.Token != nil
}

// WsURL returns the WebSocket URL derived from the base URL.
func (b *UpstreamProxyBootstrap) WsURL() string {
	return UpstreamProxyWsURL(b.Remote.BaseURL)
}

// StateForPort returns the proxy state for a given port.
func (b *UpstreamProxyBootstrap) StateForPort(port uint16) UpstreamProxyState {
	if !b.ShouldEnable() {
		return DisabledState()
	}
	return UpstreamProxyState{
		Enabled:      true,
		ProxyURL:     strPtr(fmt.Sprintf("http://127.0.0.1:%d", port)),
		CABundlePath: &b.CABundlePath,
		NoProxy:      NoProxyList(),
	}
}

// ---------------------------------------------------------------------------
// UpstreamProxyState
// ---------------------------------------------------------------------------

// UpstreamProxyState represents the active proxy state.
type UpstreamProxyState struct {
	Enabled      bool
	ProxyURL     *string
	CABundlePath *string
	NoProxy      string
}

// DisabledState returns a disabled proxy state.
func DisabledState() UpstreamProxyState {
	return UpstreamProxyState{
		Enabled: false,
		NoProxy: NoProxyList(),
	}
}

// SubprocessEnv returns environment variables for child processes.
func (s *UpstreamProxyState) SubprocessEnv() map[string]string {
	if !s.Enabled || s.ProxyURL == nil || s.CABundlePath == nil {
		return map[string]string{}
	}

	return map[string]string{
		"HTTPS_PROXY":         *s.ProxyURL,
		"https_proxy":         *s.ProxyURL,
		"NO_PROXY":            s.NoProxy,
		"no_proxy":            s.NoProxy,
		"SSL_CERT_FILE":       *s.CABundlePath,
		"NODE_EXTRA_CA_CERTS": *s.CABundlePath,
		"REQUESTS_CA_BUNDLE":  *s.CABundlePath,
		"CURL_CA_BUNDLE":      *s.CABundlePath,
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ReadToken reads a session token from the given path.
// Returns (nil, nil) if the file doesn't exist or is empty.
func ReadToken(path string) (*string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return nil, nil
	}
	return &token, nil
}

// UpstreamProxyWsURL converts a base URL to a WebSocket URL.
func UpstreamProxyWsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")

	var wsBase string
	if strings.HasPrefix(base, "https://") {
		wsBase = "wss://" + strings.TrimPrefix(base, "https://")
	} else if strings.HasPrefix(base, "http://") {
		wsBase = "ws://" + strings.TrimPrefix(base, "http://")
	} else {
		wsBase = "wss://" + base
	}
	return wsBase + "/v1/code/upstreamproxy/ws"
}

// NoProxyList returns the comma-separated no-proxy host list.
func NoProxyList() string {
	hosts := make([]string, 0, len(NoProxyHosts)+3)
	hosts = append(hosts, NoProxyHosts[:]...)
	hosts = append(hosts, "pypi.org", "files.pythonhosted.org", "proxy.golang.org")
	return strings.Join(hosts, ",")
}

// InheritedUpstreamProxyEnv returns proxy env vars from the given map,
// only if both HTTPS_PROXY and SSL_CERT_FILE are set.
func InheritedUpstreamProxyEnv(env map[string]string) map[string]string {
	if _, hasProxy := env["HTTPS_PROXY"]; !hasProxy {
		return map[string]string{}
	}
	if _, hasCert := env["SSL_CERT_FILE"]; !hasCert {
		return map[string]string{}
	}

	result := make(map[string]string)
	for _, key := range UpstreamProxyEnvKeys {
		if v, ok := env[key]; ok {
			result[key] = v
		}
	}
	return result
}

// ReadTokenFromReader reads a token from an io.Reader (for testing).
func ReadTokenFromReader(r io.Reader) (*string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return nil, nil
	}
	return &token, nil
}

func defaultCABundlePath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".ccr", "ca-bundle.crt")
}

func envTruthy(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func strPtr(s string) *string {
	return &s
}

// EnvMapSorted returns sorted keys for deterministic output (for testing).
func EnvMapSorted(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
