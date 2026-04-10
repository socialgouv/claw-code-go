package mcp

import (
	"testing"
)

func TestNewTransportSSE(t *testing.T) {
	tr, err := NewTransport(TransportConfig{
		Type: TransportSSE,
		URL:  "http://example.com/mcp",
		Auth: "Bearer token123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	// Verify it's an SSETransport.
	sse, ok := tr.(*SSETransport)
	if !ok {
		t.Fatalf("expected *SSETransport, got %T", tr)
	}
	if sse.baseURL != "http://example.com/mcp" {
		t.Errorf("expected baseURL='http://example.com/mcp', got %q", sse.baseURL)
	}
}

func TestNewTransportHTTP(t *testing.T) {
	tr, err := NewTransport(TransportConfig{
		Type: TransportHTTP,
		URL:  "http://example.com/mcp",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := tr.(*SSETransport); !ok {
		t.Fatalf("expected *SSETransport for HTTP type, got %T", tr)
	}
}

func TestNewTransportManagedProxy(t *testing.T) {
	tr, err := NewTransport(TransportConfig{
		Type: TransportManagedProxy,
		URL:  "http://proxy.example.com",
		ID:   "proxy-123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	proxy, ok := tr.(*ManagedProxyTransport)
	if !ok {
		t.Fatalf("expected *ManagedProxyTransport, got %T", tr)
	}
	if proxy.id != "proxy-123" {
		t.Errorf("expected id='proxy-123', got %q", proxy.id)
	}
}

func TestNewTransportUnknown(t *testing.T) {
	_, err := NewTransport(TransportConfig{
		Type: "unknown",
	})
	if err == nil {
		t.Fatal("expected error for unknown transport type")
	}
}

func TestNewTransportWebSocketURLNormalization(t *testing.T) {
	// websocketURL should normalize http:// to ws:// via the factory.
	// We can't actually dial, but we can test the URL conversion function.
	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:8080", "ws://localhost:8080"},
		{"https://example.com", "wss://example.com"},
		{"ws://already.ws", "ws://already.ws"},
		{"wss://already.wss", "wss://already.wss"},
	}
	for _, tt := range tests {
		got := websocketURL(tt.input)
		if got != tt.want {
			t.Errorf("websocketURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTransportTypeConstants(t *testing.T) {
	types := []TransportType{
		TransportStdio,
		TransportSSE,
		TransportHTTP,
		TransportWebSocket,
		TransportManagedProxy,
		TransportSDK,
	}
	for _, tt := range types {
		if string(tt) == "" {
			t.Error("transport type should not be empty")
		}
	}
}
