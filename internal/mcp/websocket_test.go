package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketTransportRoundTrip(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Echo: read request, send response
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req Request
		json.Unmarshal(msg, &req)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"echo": true},
		}
		conn.WriteJSON(resp)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	transport, err := NewWebSocketTransport(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer transport.Close()

	resp, err := transport.Send(t.Context(), Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/echo",
	})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if resp.ID != float64(1) {
		t.Errorf("expected ID=1, got %v", resp.ID)
	}
}

func TestWebSocketTransportConnectionFailure(t *testing.T) {
	_, err := NewWebSocketTransport("ws://127.0.0.1:1", nil)
	if err == nil {
		t.Fatal("expected error for bad address")
	}
}

func TestWebSocketTransportCloseIdempotent(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Keep connection alive until client closes
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	transport, err := NewWebSocketTransport(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second close should be nil, got: %v", err)
	}
}
