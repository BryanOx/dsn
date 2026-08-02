package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/dsn/dsn/telemetry"
	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered connections by subscription type
	subscribers map[string]map[*Client]bool

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast messages to subscribers
	broadcast chan *Message

	// Connection count semaphore (max 100)
	connectionLimit chan struct{}

	// Mutex for thread-safe access
	mu sync.RWMutex
}

// Message represents a WebSocket message.
type Message struct {
	Type  string      `json:"type,omitempty"`
	Event string      `json:"event,omitempty"`
	Data  interface{} `json:"data,omitempty"`
	Error *WSError    `json:"error,omitempty"`
}

// WSError represents an error in WebSocket communication.
type WSError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *WSError) Error() string {
	return fmt.Sprintf("WS error %d: %s", e.Code, e.Message)
}

// Client represents a WebSocket client connection.
type Client struct {
	hub   *Hub
	conn  *websocket.Conn
	send  chan []byte
	subs  map[string]bool // Track active subscriptions per client
	subMu sync.RWMutex
}

// Constants for connection limits and timeouts
const (
	MaxConnections    = 100
	MaxSubscriptions  = 10
	HeartbeatInterval = 30 * time.Second
	PongTimeout       = 10 * time.Second
	WriteWait         = 10 * time.Second
	PingPeriod        = (HeartbeatInterval * 9) / 10
	MaxMessageSize    = 512
)

// Upgrader configuration
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins in development
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers:     make(map[string]map[*Client]bool),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		broadcast:       make(chan *Message, 256),
		connectionLimit: make(chan struct{}, MaxConnections),
	}
}

// Run runs the hub's main loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// registerClient adds a new client to the hub.
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize subscription tracking
	client.subMu.Lock()
	client.subs = make(map[string]bool)
	client.subMu.Unlock()

	// Add to subscribers map for each event type
	for event := range client.subs {
		if h.subscribers[event] == nil {
			h.subscribers[event] = make(map[*Client]bool)
		}
		h.subscribers[event][client] = true
	}

	// Update metrics
	h.updateConnectionMetrics()
}

// unregisterClient removes a client from the hub.
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove from all subscription types
	client.subMu.RLock()
	subs := make([]string, 0, len(client.subs))
	for event := range client.subs {
		subs = append(subs, event)
	}
	client.subMu.RUnlock()

	for _, event := range subs {
		if subscribers, ok := h.subscribers[event]; ok {
			delete(subscribers, client)
			if len(subscribers) == 0 {
				delete(h.subscribers, event)
			}
		}
	}

	// Release connection slot
	select {
	case <-h.connectionLimit:
		// Connection slot released
	default:
		// Slot was not acquired (should not happen)
	}

	// Update metrics
	h.updateConnectionMetrics()
}

// updateConnectionMetrics updates the WebSocket connection count metric.
func (h *Hub) updateConnectionMetrics() {
	// Count total active connections
	count := 0
	for _, clients := range h.subscribers {
		count += len(clients)
	}
	telemetry.WSConnections.Set(float64(count))
}

// Subscribe adds a client to an event type.
func (h *Hub) Subscribe(client *Client, event string) error {
	// Check subscription limit per client
	client.subMu.Lock()
	if len(client.subs) >= MaxSubscriptions {
		client.subMu.Unlock()
		return &WSError{Code: -32602, Message: "subscription limit exceeded (max 10)"}
	}
	client.subs[event] = true
	client.subMu.Unlock()

	// Register with hub
	h.mu.Lock()
	if h.subscribers[event] == nil {
		h.subscribers[event] = make(map[*Client]bool)
	}
	h.subscribers[event][client] = true
	h.mu.Unlock()

	return nil
}

// Unsubscribe removes a client from an event type.
func (h *Hub) Unsubscribe(client *Client, event string) {
	client.subMu.Lock()
	delete(client.subs, event)
	client.subMu.Unlock()

	h.mu.Lock()
	if subscribers, ok := h.subscribers[event]; ok {
		delete(subscribers, client)
		if len(subscribers) == 0 {
			delete(h.subscribers, event)
		}
	}
	h.mu.Unlock()
}

// broadcastMessage sends a message to all subscribers of an event.
func (h *Hub) broadcastMessage(msg *Message) {
	event := msg.Event
	if event == "" {
		return
	}

	h.mu.RLock()
	subscribers := h.subscribers[event]
	h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws: failed to marshal message: %v", err)
		return
	}

	for client := range subscribers {
		select {
		case client.send <- data:
		default:
			// Client send buffer full, close connection
			h.unregister <- client
		}
	}
}

// Broadcast sends a message to all subscribers of an event type.
func (h *Hub) Broadcast(event string, data interface{}) {
	msg := &Message{
		Event: event,
		Data:  data,
	}
	h.broadcast <- msg
}

// AcquireConnection tries to acquire a connection slot.
// Returns false if limit exceeded.
func (h *Hub) AcquireConnection() bool {
	select {
	case h.connectionLimit <- struct{}{}:
		return true
	default:
		return false
	}
}

// ReleaseConnection releases a connection slot.
func (h *Hub) ReleaseConnection() {
	select {
	case <-h.connectionLimit:
		// Connection slot released
	default:
		// Slot was not acquired
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	if !h.AcquireConnection() {
		// Reject with CloseCode 1013 (Try Again Later)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.ReleaseConnection()
		log.Printf("ws: failed to upgrade: %v", err)
		return
	}

	// Create client
	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	// Register
	h.register <- client

	// Start pumps
	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(PongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(PongTimeout))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws: read error: %v", err)
			}
			break
		}

		// Handle subscription message
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError(-32700, "parse error")
			continue
		}

		c.handleMessage(&msg)
	}
}

// writePump writes messages to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("ws: write error: %v", err)
				return
			}
			w.Write(message)

			// Add queued messages to the current WebSocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				log.Printf("ws: close writer error: %v", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes subscription/unsubscription messages.
func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case "subscribe":
		err := c.hub.Subscribe(c, msg.Event)
		if err != nil {
			if wsErr, ok := err.(*WSError); ok {
				c.sendError(wsErr.Code, wsErr.Message)
			} else {
				c.sendError(-32000, err.Error())
			}
			return
		}
		// Send confirmation
		c.send <- []byte(`{"type":"subscribed","event":"` + msg.Event + `"}`)

	case "unsubscribe":
		c.hub.Unsubscribe(c, msg.Event)
		// Send confirmation
		c.send <- []byte(`{"type":"unsubscribed","event":"` + msg.Event + `"}`)

	default:
		c.sendError(-32600, "invalid message type")
	}
}

// sendError sends an error message to the client.
func (c *Client) sendError(code int, message string) {
	err := &WSError{Code: code, Message: message}
	resp := &Message{Type: "error", Error: err}
	data, _ := json.Marshal(resp)
	c.send <- data
}
