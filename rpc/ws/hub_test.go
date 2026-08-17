package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketRejectsCrossOrigin(t *testing.T) {
	hub := NewHub([]string{"https://app.dsn.io"})
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Attempt WebSocket upgrade from evil.com origin
	header := http.Header{}
	header.Set("Origin", "https://evil.com")

	conn, resp, err := websocket.DefaultDialer.Dial(server.URL, header)
	if err == nil {
		conn.Close()
		t.Fatal("expected WebSocket connection to fail from cross-origin")
	}
	if resp != nil {
		if resp.StatusCode == http.StatusSwitchingProtocols {
			t.Error("expected non-101 status from cross-origin, got 101")
		}
	}
}

func TestWebSocketAllowsConfiguredOrigin(t *testing.T) {
	hub := NewHub([]string{"https://app.dsn.io"})
	go hub.Run()
	defer hub.Stop()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Verify the upgrader CheckOrigin logic directly
	upgrader := newUpgrader([]string{"https://app.dsn.io"})

	// Test allowed origin
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://app.dsn.io")
	if !upgrader.CheckOrigin(req) {
		t.Error("expected CheckOrigin to accept https://app.dsn.io")
	}

	// Test rejected origin
	req2 := httptest.NewRequest("GET", "/ws", nil)
	req2.Header.Set("Origin", "https://evil.com")
	if upgrader.CheckOrigin(req2) {
		t.Error("expected CheckOrigin to reject https://evil.com")
	}
}

func TestWebSocketAllowsAllWhenNoOrigins(t *testing.T) {
	upgrader := newUpgrader(nil)

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://any-origin.com")
	if !upgrader.CheckOrigin(req) {
		t.Error("expected CheckOrigin to accept any origin when allowedOrigins is empty")
	}
}

func TestWebSocketNoOriginHeaderAllowed(t *testing.T) {
	upgrader := newUpgrader([]string{"https://app.dsn.io"})

	req := httptest.NewRequest("GET", "/ws", nil)
	// No Origin header set
	if !upgrader.CheckOrigin(req) {
		t.Error("expected CheckOrigin to allow request with no Origin header")
	}
}

func TestWebSocketCaseInsensitiveOrigin(t *testing.T) {
	upgrader := newUpgrader([]string{"https://App.DSN.io"})

	// Origin matching should be case-insensitive
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://app.dsn.io")
	if !upgrader.CheckOrigin(req) {
		t.Error("expected CheckOrigin to match case-insensitively")
	}

	req2 := httptest.NewRequest("GET", "/ws", nil)
	req2.Header.Set("Origin", "https://APP.DSN.IO")
	if !upgrader.CheckOrigin(req2) {
		t.Error("expected CheckOrigin to match uppercase variant")
	}
}

func TestNewHubWithOrigins(t *testing.T) {
	hub := NewHub([]string{"https://app.dsn.io", "https://admin.dsn.io"})
	if len(hub.allowedOrigins) != 2 {
		t.Errorf("allowedOrigins count = %d, want 2", len(hub.allowedOrigins))
	}
	if hub.allowedOrigins[0] != "https://app.dsn.io" {
		t.Errorf("allowedOrigins[0] = %s, want https://app.dsn.io", hub.allowedOrigins[0])
	}
}

func TestNewHubWithoutOrigins(t *testing.T) {
	hub := NewHub()
	if len(hub.allowedOrigins) != 0 {
		t.Errorf("allowedOrigins = %v, want empty", hub.allowedOrigins)
	}
}

func TestHubStop(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	hub.Stop()
	// Verify no panic on double stop
	hub.Stop()
}

func TestHubConnectionLimit(t *testing.T) {
	hub := NewHub()

	// Acquire max connections
	for i := 0; i < MaxConnections; i++ {
		if !hub.AcquireConnection() {
			t.Fatalf("AcquireConnection failed at %d", i)
		}
	}

	// Next should fail
	if hub.AcquireConnection() {
		t.Error("expected AcquireConnection to fail after limit reached")
	}

	// Release one
	hub.ReleaseConnection()

	// Now should succeed again
	if !hub.AcquireConnection() {
		t.Error("expected AcquireConnection to succeed after release")
	}

	// Clean up
	for i := 0; i < MaxConnections+1; i++ {
		hub.ReleaseConnection()
	}
}

func TestHubSubscribe(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 256),
		subs: make(map[string]bool),
	}

	// Subscribe to an event
	err := hub.Subscribe(client, "newBlock")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Verify the client is registered
	hub.mu.RLock()
	subs := hub.subscribers["newBlock"]
	hub.mu.RUnlock()

	if _, ok := subs[client]; !ok {
		t.Error("expected client to be in subscribers")
	}

	// Verify subscription count
	client.subMu.RLock()
	count := len(client.subs)
	client.subMu.RUnlock()

	if count != 1 {
		t.Errorf("subscription count = %d, want 1", count)
	}
}

func TestHubSubscribeLimit(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 256),
		subs: make(map[string]bool),
	}

	// Subscribe up to the limit
	for i := 0; i < MaxSubscriptions; i++ {
		err := hub.Subscribe(client, strings.Repeat("e", i+1))
		if err != nil {
			t.Fatalf("Subscribe %d: %v", i, err)
		}
	}

	// One more should fail
	err := hub.Subscribe(client, "overflow")
	if err == nil {
		t.Error("expected Subscribe to fail after limit exceeded")
	}
}
