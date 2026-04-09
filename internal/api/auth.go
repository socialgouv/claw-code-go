package api

import (
	"net/http"
	"os"
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
