package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// AuthSourceKind discriminates AuthSource variants.
type AuthSourceKind int

const (
	AuthSourceNone     AuthSourceKind = iota
	AuthSourceAPIKey                  // API key only
	AuthSourceBearer                  // Bearer token only
	AuthSourceCombined                // both API key and bearer token
)

// AuthSource holds authentication credentials for API requests.
type AuthSource struct {
	Kind        AuthSourceKind
	APIKey      string
	BearerToken string
}

// NoAuth returns an AuthSource with no credentials.
func NoAuth() AuthSource {
	return AuthSource{Kind: AuthSourceNone}
}

// APIKeyAuth returns an AuthSource using an API key.
func APIKeyAuth(key string) AuthSource {
	return AuthSource{Kind: AuthSourceAPIKey, APIKey: key}
}

// BearerAuth returns an AuthSource using a bearer token.
func BearerAuth(token string) AuthSource {
	return AuthSource{Kind: AuthSourceBearer, BearerToken: token}
}

// CombinedAuth returns an AuthSource using both API key and bearer token.
func CombinedAuth(apiKey, bearerToken string) AuthSource {
	return AuthSource{Kind: AuthSourceCombined, APIKey: apiKey, BearerToken: bearerToken}
}

// ResolveStartupAuth resolves auth from environment variables.
// Priority: ANTHROPIC_API_KEY + ANTHROPIC_AUTH_TOKEN (combined if both present),
// then ANTHROPIC_API_KEY alone, then ANTHROPIC_AUTH_TOKEN alone.
func ResolveStartupAuth() AuthSource {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	bearerToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")

	switch {
	case apiKey != "" && bearerToken != "":
		return CombinedAuth(apiKey, bearerToken)
	case apiKey != "":
		return APIKeyAuth(apiKey)
	case bearerToken != "":
		return BearerAuth(bearerToken)
	default:
		return NoAuth()
	}
}

// ApplyToRequest sets appropriate headers on an HTTP request.
func (a AuthSource) ApplyToRequest(req *http.Request) {
	switch a.Kind {
	case AuthSourceAPIKey:
		req.Header.Set("x-api-key", a.APIKey)
	case AuthSourceBearer:
		req.Header.Set("Authorization", "Bearer "+a.BearerToken)
	case AuthSourceCombined:
		req.Header.Set("x-api-key", a.APIKey)
		req.Header.Set("Authorization", "Bearer "+a.BearerToken)
	case AuthSourceNone:
		// no-op
	}
}

// HasAPIKey returns true if an API key is present.
func (a AuthSource) HasAPIKey() bool {
	return a.APIKey != ""
}

// HasBearerToken returns true if a bearer token is present.
func (a AuthSource) HasBearerToken() bool {
	return a.BearerToken != ""
}

// MaskedAuthorizationHeader returns a redacted representation of the auth header.
// Matches Rust's masked_authorization_header() for safe logging.
func (a AuthSource) MaskedAuthorizationHeader() string {
	if a.HasBearerToken() {
		return "Bearer [REDACTED]"
	}
	return "<absent>"
}

// ResolveStartupAuthWithOAuth resolves auth from environment variables, falling
// back to a saved OAuth token if available. The loadOAuth callback loads and
// optionally refreshes a persisted token set. Matches Rust's
// resolve_startup_auth_source() precedence chain.
func ResolveStartupAuthWithOAuth(loadOAuth func() (*OAuthTokenSet, error)) (AuthSource, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	bearerToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")

	// Env vars take highest priority.
	if apiKey != "" && bearerToken != "" {
		return CombinedAuth(apiKey, bearerToken), nil
	}
	if apiKey != "" {
		return APIKeyAuth(apiKey), nil
	}
	if bearerToken != "" {
		return BearerAuth(bearerToken), nil
	}

	// Fallback to saved OAuth token.
	if loadOAuth != nil {
		token, err := loadOAuth()
		if err != nil {
			return NoAuth(), err
		}
		if token != nil && token.AccessToken != "" {
			return BearerAuth(token.AccessToken), nil
		}
	}

	return NoAuth(), nil
}

// OAuthTokenSet represents a saved OAuth token set for auth resolution.
type OAuthTokenSet struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	ExpiresAt    *uint64  `json:"expires_at,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

// IsExpired returns true if the token has an expiration time that has passed.
func (t *OAuthTokenSet) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return *t.ExpiresAt <= uint64(time.Now().Unix())
}

// EnrichBearerAuthError adds a helpful hint when an API key (sk-ant-*) is
// mistakenly used as a Bearer token. Matches Rust's enrich_bearer_auth_error().
func EnrichBearerAuthError(errMsg string, auth AuthSource) string {
	if auth.Kind != AuthSourceBearer {
		return errMsg
	}
	if strings.HasPrefix(auth.BearerToken, "sk-ant-") {
		return fmt.Sprintf("%s\n\nHint: It looks like you're using an API key (sk-ant-*) as a Bearer token. "+
			"API keys should be passed via the x-api-key header instead. "+
			"Set the ANTHROPIC_API_KEY environment variable, or use APIKeyAuth() instead of BearerAuth().", errMsg)
	}
	return errMsg
}
