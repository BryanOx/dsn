package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSSubscription represents a WebSocket subscription.
type WSSubscription struct {
	conn      *websocket.Conn
	channel   interface{}
	closeCh   chan struct{}
	mu        sync.Mutex
	isClosed  bool
}

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// SubscribeNewHeads subscribes to new block headers.
func (c *Client) SubscribeNewHeads(ctx context.Context) (<-chan *BlockHeader, error) {
	rawCh, err := c.subscribe(ctx, "newHeads")
	if err != nil {
		return nil, err
	}

	ch := make(chan *BlockHeader, 10)
	go func() {
		defer close(ch)
		for raw := range rawCh {
			var h BlockHeader
			if err := json.Unmarshal(raw, &h); err != nil {
				continue
			}
			select {
			case ch <- &h:
			default:
			}
		}
	}()
	return ch, nil
}

// SubscribeLogs subscribes to logs/events.
func (c *Client) SubscribeLogs(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}

	rawCh, err := c.subscribeWithFilter(ctx, "logs", string(filterJSON))
	if err != nil {
		return nil, err
	}

	ch := make(chan *Event, 10)
	go func() {
		defer close(ch)
		for raw := range rawCh {
			var e Event
			if err := json.Unmarshal(raw, &e); err != nil {
				continue
			}
			select {
			case ch <- &e:
			default:
			}
		}
	}()
	return ch, nil
}

// SubscribePendingTxs subscribes to pending transactions.
func (c *Client) SubscribePendingTxs(ctx context.Context) (<-chan string, error) {
	rawCh, err := c.subscribe(ctx, "pendingTransactions")
	if err != nil {
		return nil, err
	}

	ch := make(chan string, 10)
	go func() {
		defer close(ch)
		for raw := range rawCh {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				select {
				case ch <- string(raw):
				default:
				}
				continue
			}
			select {
			case ch <- s:
			default:
			}
		}
	}()
	return ch, nil
}

// subscribe creates a generic subscription.
func (c *Client) subscribe(ctx context.Context, event string) (<-chan json.RawMessage, error) {
	conn, err := c.dialWS(ctx)
	if err != nil {
		return nil, err
	}

	// Subscribe message
	subMsg := WSMessage{
		Type:  "subscribe",
		Event: event,
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		conn.Close()
		return nil, err
	}

	// Read confirmation
	var resp WSMessage
	if err := conn.ReadJSON(&resp); err != nil {
		conn.Close()
		return nil, err
	}

	ch := make(chan json.RawMessage, 100)

	go c.readWSMessages(conn, ch)

	return ch, nil
}

// subscribeWithFilter creates a subscription with a filter.
func (c *Client) subscribeWithFilter(ctx context.Context, event, filter string) (<-chan json.RawMessage, error) {
	conn, err := c.dialWS(ctx)
	if err != nil {
		return nil, err
	}

	// Subscribe with filter
	subMsg := map[string]interface{}{
		"type":   "subscribe",
		"event":  event,
		"filter": filter,
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		conn.Close()
		return nil, err
	}

	ch := make(chan json.RawMessage, 100)

	go c.readWSMessages(conn, ch)

	return ch, nil
}

// dialWS establishes a WebSocket connection.
func (c *Client) dialWS(ctx context.Context) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial ws: %w", err)
	}

	return conn, nil
}

// readWSMessages reads messages from the WebSocket and sends to channel.
func (c *Client) readWSMessages(conn *websocket.Conn, ch chan<- json.RawMessage) {
	defer close(ch)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		select {
		case ch <- data:
		default:
		}
	}
}

// BlockHeaderFromJSON parses a BlockHeader from JSON data.
func BlockHeaderFromJSON(data json.RawMessage) (*BlockHeader, error) {
	var header BlockHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	return &header, nil
}

// EventFromJSON parses an Event from JSON data.
func EventFromJSON(data json.RawMessage) (*Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// WSClient represents a WebSocket client with auto-reconnect.
type WSClient struct {
	url        string
	conn        *websocket.Conn
	subs       map[string]chan json.RawMessage
	mu         sync.Mutex
	reconnect   bool
	maxRetries int
	backoff    time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWSClient creates a new WebSocket client.
func NewWSClient(url string) *WSClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &WSClient{
		url:        url,
		subs:       make(map[string]chan json.RawMessage),
		reconnect:  true,
		maxRetries: 3,
		backoff:    time.Second,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Connect establishes the WebSocket connection.
func (ws *WSClient) Connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(ws.url, nil)
	if err != nil {
		return err
	}

	ws.conn = conn

	// Start reader
	go ws.readLoop()

	return nil
}

// Subscribe creates a subscription for an event.
func (ws *WSClient) Subscribe(event string) (chan json.RawMessage, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ch := make(chan json.RawMessage, 100)
	ws.subs[event] = ch

	// Send subscription request
	msg := map[string]string{
		"type":  "subscribe",
		"event": event,
	}

	if err := ws.conn.WriteJSON(msg); err != nil {
		delete(ws.subs, event)
		return nil, err
	}

	return ch, nil
}

// Unsubscribe removes a subscription.
func (ws *WSClient) Unsubscribe(event string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ch, ok := ws.subs[event]
	if !ok {
		return nil
	}

	close(ch)
	delete(ws.subs, event)

	msg := map[string]string{
		"type":  "unsubscribe",
		"event": event,
	}

	return ws.conn.WriteJSON(msg)
}

// Close closes the WebSocket connection and all subscriptions.
func (ws *WSClient) Close() error {
	ws.cancel()

	ws.mu.Lock()
	defer ws.mu.Unlock()

	for _, ch := range ws.subs {
		close(ch)
	}
	ws.subs = make(map[string]chan json.RawMessage)

	if ws.conn != nil {
		return ws.conn.Close()
	}

	return nil
}

// readLoop reads messages and dispatches to subscriptions.
func (ws *WSClient) readLoop() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
			ws.mu.Lock()
			conn := ws.conn
			ws.mu.Unlock()

			if conn == nil {
				time.Sleep(time.Second)
				continue
			}

			_, data, err := conn.ReadMessage()
			if err != nil {
				if ws.reconnect {
					ws.reconnectLoop()
				}
				return
			}

			// Parse and dispatch
			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			ws.mu.Lock()
			ch, ok := ws.subs[msg.Event]
			ws.mu.Unlock()

			if ok {
				select {
				case ch <- data:
				default:
				}
			}
		}
	}
}

// reconnectLoop attempts to reconnect.
func (ws *WSClient) reconnectLoop() {
	for i := 0; i < ws.maxRetries; i++ {
		select {
		case <-ws.ctx.Done():
			return
		case <-time.After(ws.backoff * time.Duration(1<<i)):
		}

		if err := ws.Connect(); err == nil {
			// Resubscribe
			ws.mu.Lock()
			events := make([]string, 0, len(ws.subs))
			for event := range ws.subs {
				events = append(events, event)
			}
			ws.mu.Unlock()

			for _, event := range events {
				msg := map[string]string{
					"type":  "subscribe",
					"event": event,
				}
				ws.conn.WriteJSON(msg)
			}

			return
		}
	}
}

// WithReconnect enables auto-reconnect.
func (ws *WSClient) WithReconnect(enabled bool, maxRetries int, backoff time.Duration) *WSClient {
	ws.reconnect = enabled
	ws.maxRetries = maxRetries
	ws.backoff = backoff
	return ws
}