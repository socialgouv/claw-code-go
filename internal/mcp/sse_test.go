package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSSETransport_StaticAuthHeader(t *testing.T) {
	var receivedAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth.Store(r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(Response{ID: 1})
	}))
	defer srv.Close()

	tr := NewSSETransport(srv.URL, "Bearer static-token")
	tr.httpClient = srv.Client()

	if _, err := tr.Send(context.Background(), Request{ID: 1, Method: "ping"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := receivedAuth.Load().(string); got != "Bearer static-token" {
		t.Errorf("expected static auth, got %q", got)
	}
}

func TestSSETransport_DynamicAuthFunc(t *testing.T) {
	var receivedAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth.Store(r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(Response{ID: 1})
	}))
	defer srv.Close()

	var calls atomic.Int32
	tr := NewSSETransport(srv.URL, "Bearer fallback")
	tr.httpClient = srv.Client()
	tr.SetAuthFunc(func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "Bearer dynamic-" + asString(calls.Load()), nil
	})

	if _, err := tr.Send(context.Background(), Request{ID: 1, Method: "ping"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := receivedAuth.Load().(string); got != "Bearer dynamic-1" {
		t.Errorf("expected dynamic auth, got %q", got)
	}

	if _, err := tr.Send(context.Background(), Request{ID: 2, Method: "ping"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := receivedAuth.Load().(string); got != "Bearer dynamic-2" {
		t.Errorf("expected dynamic auth refreshed, got %q", got)
	}
	if calls.Load() != 2 {
		t.Errorf("expected authFunc called 2x, got %d", calls.Load())
	}
}

func TestSSETransport_AuthFuncErrorAbortsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be reached when authFunc errors")
	}))
	defer srv.Close()

	tr := NewSSETransport(srv.URL, "")
	tr.httpClient = srv.Client()
	tr.SetAuthFunc(func(ctx context.Context) (string, error) {
		return "", errBoom
	})

	if _, err := tr.Send(context.Background(), Request{ID: 1, Method: "ping"}); err == nil {
		t.Fatal("expected error when authFunc fails")
	}
}

var errBoom = stringError("boom")

type stringError string

func (e stringError) Error() string { return string(e) }

func asString(n int32) string {
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	}
	return "n"
}
