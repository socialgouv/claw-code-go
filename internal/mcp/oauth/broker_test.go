package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAuthServer simulates the authorization endpoint. The handler
// records the latest authorization request, lets tests grab the state
// + redirect_uri, and replies with a 302 to the callback URL.
type fakeAuthServer struct {
	*httptest.Server
	lastReq atomic.Value // map[string]string
}

func newFakeAuthServer() *fakeAuthServer {
	f := &fakeAuthServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := map[string]string{}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		f.lastReq.Store(params)
		// We don't actually redirect — tests drive the callback themselves
		// to keep the flow deterministic. Just acknowledge.
		fmt.Fprintln(w, "ok")
	}))
	return f
}

func (f *fakeAuthServer) Params() map[string]string {
	if v := f.lastReq.Load(); v != nil {
		return v.(map[string]string)
	}
	return nil
}

// fakeTokenServer issues tokens. Each test configures the response
// body. exchanges accumulates posted forms for assertion.
type fakeTokenServer struct {
	*httptest.Server
	exchanges []url.Values
	response  func(form url.Values) (int, tokenResponse)
}

func newFakeTokenServer() *fakeTokenServer {
	f := &fakeTokenServer{
		response: func(_ url.Values) (int, tokenResponse) {
			return http.StatusOK, tokenResponse{
				AccessToken:  "fresh-access",
				RefreshToken: "fresh-refresh",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				Scope:        "read",
			}
		},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.exchanges = append(f.exchanges, r.PostForm)
		status, tr := f.response(r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(tr)
	}))
	return f
}

func TestBroker_AuthCodeFlow_HappyPath(t *testing.T) {
	authSrv := newFakeAuthServer()
	defer authSrv.Close()
	tokSrv := newFakeTokenServer()
	defer tokSrv.Close()

	storage := NewStorage(filepath.Join(t.TempDir(), "tokens.json"))

	openerCalled := make(chan struct{}, 1)
	opener := func(authURL string) error {
		// Parse the URL and extract the redirect_uri + state, then
		// drive the callback ourselves. This mimics what a real
		// browser would do once the user authorizes.
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		go func() {
			cb, _ := url.Parse(q.Get("redirect_uri"))
			cbq := url.Values{}
			cbq.Set("code", "the-code")
			cbq.Set("state", q.Get("state"))
			cb.RawQuery = cbq.Encode()
			_, _ = http.Get(cb.String())
		}()
		openerCalled <- struct{}{}
		return nil
	}

	b := NewBroker(
		WithStorage(storage),
		WithHTTPClient(tokSrv.Client()),
		WithAuthOpener(opener),
		WithCallbackTimeout(3*time.Second),
		WithClock(func() time.Time { return time.Unix(1700000000, 0) }),
	)

	cfg := ServerConfig{
		Name:     "github",
		AuthURL:  authSrv.URL + "/authorize",
		TokenURL: tokSrv.URL,
		ClientID: "client-123",
		Scopes:   []string{"read", "write"},
	}

	tok, err := b.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	<-openerCalled
	if tok != "fresh-access" {
		t.Errorf("expected fresh-access token, got %q", tok)
	}

	// Storage must contain the persisted token.
	stored, ok, err := storage.Load("github")
	if err != nil || !ok {
		t.Fatalf("expected stored token, ok=%v err=%v", ok, err)
	}
	if stored.RefreshToken != "fresh-refresh" {
		t.Errorf("refresh token not persisted: %q", stored.RefreshToken)
	}
	if stored.ExpiresAt.IsZero() {
		t.Errorf("expected ExpiresAt set from expires_in")
	}

	// Token endpoint must have received PKCE verifier.
	if len(tokSrv.exchanges) != 1 {
		t.Fatalf("expected 1 token exchange, got %d", len(tokSrv.exchanges))
	}
	form := tokSrv.exchanges[0]
	if form.Get("grant_type") != "authorization_code" {
		t.Errorf("expected grant_type=authorization_code, got %q", form.Get("grant_type"))
	}
	if form.Get("code") != "the-code" {
		t.Errorf("expected code=the-code, got %q", form.Get("code"))
	}
	if form.Get("code_verifier") == "" {
		t.Errorf("missing code_verifier in token request")
	}
}

func TestBroker_StateMismatch_Rejects(t *testing.T) {
	authSrv := newFakeAuthServer()
	defer authSrv.Close()
	tokSrv := newFakeTokenServer()
	defer tokSrv.Close()

	opener := func(authURL string) error {
		u, _ := url.Parse(authURL)
		q := u.Query()
		go func() {
			cb, _ := url.Parse(q.Get("redirect_uri"))
			cbq := url.Values{}
			cbq.Set("code", "the-code")
			cbq.Set("state", "WRONG-STATE")
			cb.RawQuery = cbq.Encode()
			_, _ = http.Get(cb.String())
		}()
		return nil
	}

	b := NewBroker(
		WithHTTPClient(tokSrv.Client()),
		WithAuthOpener(opener),
		WithCallbackTimeout(2*time.Second),
	)
	cfg := ServerConfig{
		Name:     "github",
		AuthURL:  authSrv.URL + "/authorize",
		TokenURL: tokSrv.URL,
		ClientID: "client-123",
	}
	if _, err := b.Acquire(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch error, got %v", err)
	}
}

func TestBroker_RejectsIncompleteConfig(t *testing.T) {
	b := NewBroker()
	if _, err := b.Acquire(context.Background(), ServerConfig{}); err == nil {
		t.Fatal("expected error on empty config")
	}
}

func TestBroker_AcquireUsesCachedTokenWhenFresh(t *testing.T) {
	tokSrv := newFakeTokenServer()
	defer tokSrv.Close()

	storage := NewStorage(filepath.Join(t.TempDir(), "tokens.json"))
	_ = storage.Save("github", Token{
		AccessToken: "cached-access",
		ExpiresAt:   time.Now().Add(time.Hour),
	})

	b := NewBroker(
		WithStorage(storage),
		WithHTTPClient(tokSrv.Client()),
	)
	cfg := ServerConfig{
		Name:     "github",
		AuthURL:  "http://nowhere/authorize",
		TokenURL: tokSrv.URL,
		ClientID: "client-123",
	}
	tok, err := b.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if tok != "cached-access" {
		t.Errorf("expected cached token, got %q", tok)
	}
	if len(tokSrv.exchanges) != 0 {
		t.Errorf("expected no token endpoint hits when cache is fresh, got %d", len(tokSrv.exchanges))
	}
}

func TestBroker_RefreshOnExpiringToken(t *testing.T) {
	tokSrv := newFakeTokenServer()
	defer tokSrv.Close()

	storage := NewStorage(filepath.Join(t.TempDir(), "tokens.json"))
	_ = storage.Save("github", Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(10 * time.Second), // within 30s skew
	})

	b := NewBroker(
		WithStorage(storage),
		WithHTTPClient(tokSrv.Client()),
	)
	cfg := ServerConfig{
		Name:     "github",
		AuthURL:  "http://nowhere/authorize",
		TokenURL: tokSrv.URL,
		ClientID: "client-123",
	}
	tok, err := b.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if tok != "fresh-access" {
		t.Errorf("expected refreshed token, got %q", tok)
	}
	if len(tokSrv.exchanges) != 1 {
		t.Fatalf("expected 1 refresh exchange, got %d", len(tokSrv.exchanges))
	}
	form := tokSrv.exchanges[0]
	if form.Get("grant_type") != "refresh_token" {
		t.Errorf("expected grant_type=refresh_token, got %q", form.Get("grant_type"))
	}
	if form.Get("refresh_token") != "old-refresh" {
		t.Errorf("expected refresh_token=old-refresh, got %q", form.Get("refresh_token"))
	}
}

func TestBroker_RevokeDropsToken(t *testing.T) {
	storage := NewStorage(filepath.Join(t.TempDir(), "tokens.json"))
	_ = storage.Save("github", Token{AccessToken: "x"})

	revoked := atomic.Bool{}
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("token") == "x" {
			revoked.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer revokeSrv.Close()

	b := NewBroker(WithStorage(storage), WithHTTPClient(revokeSrv.Client()))
	cfg := ServerConfig{Name: "github", RevokeURL: revokeSrv.URL, ClientID: "c"}
	if err := b.Revoke(context.Background(), cfg); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if !revoked.Load() {
		t.Errorf("expected revocation endpoint to receive the token")
	}
	if _, ok, _ := storage.Load("github"); ok {
		t.Errorf("expected token cache cleared after revoke")
	}
}
