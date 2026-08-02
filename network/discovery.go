package network

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Discovery maintains connections to bootstrap peers and discovers new peers
// through peer list exchange protocol.
type Discovery struct {
	pm             *PeerManager
	p2p            *P2PNode
	bootstrapPeers []string
	connected      map[string]bool
	mu             sync.RWMutex
	stopCh         chan struct{}

	// Reconnection backoff configuration
	backoffSchedule []time.Duration

	// Message handlers
	peerListRequestHandler  func(addr string) error
	peerListResponseHandler func(addr string, peers []string) error
}

// NewDiscovery creates a new Discovery instance.
func NewDiscovery(pm *PeerManager, p2p *P2PNode, bootstrapPeers []string) *Discovery {
	return &Discovery{
		pm:             pm,
		p2p:            p2p,
		bootstrapPeers: bootstrapPeers,
		connected:      make(map[string]bool),
		stopCh:         make(chan struct{}),
		backoffSchedule: []time.Duration{
			time.Second,
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			30 * time.Second,
		},
	}
}

// Start begins the discovery process - connects to bootstrap peers
// and maintains connections over time.
func (d *Discovery) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Recreate stopCh so Discovery can be stopped and restarted
	d.stopCh = make(chan struct{})

	// Initial connection to all bootstrap peers
	for _, peer := range d.bootstrapPeers {
		go d.connectWithBackoff(peer)
	}

	// Start the maintenance goroutine
	go d.maintain()
}

// Stop halts the discovery process.
func (d *Discovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	select {
	case <-d.stopCh:
		// already closed
	default:
		close(d.stopCh)
	}
}

// maintain runs periodic tasks to maintain connections and discover peers.
func (d *Discovery) maintain() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.maintainConnections()
			d.requestPeerList()
		}
	}
}

// maintainConnections ensures all bootstrap peers remain connected.
func (d *Discovery) maintainConnections() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, peer := range d.bootstrapPeers {
		if !d.connected[peer] {
			// Attempt reconnection in background with backoff
			go d.connectWithBackoff(peer)
		}
	}
}

// connectBootstrap establishes a connection to a bootstrap peer.
func (d *Discovery) connectBootstrap(addr string) error {
	if d.p2p == nil {
		return fmt.Errorf("P2P node not configured")
	}

	// Check if already connected
	if d.p2p.isConnected(addr) {
		d.mu.Lock()
		d.connected[addr] = true
		d.mu.Unlock()
		return nil
	}

	// Attempt to connect
	if err := d.p2p.Connect(addr); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	d.mu.Lock()
	d.connected[addr] = true
	d.mu.Unlock()

	fmt.Printf("[Discovery] Connected to bootstrap peer: %s\n", addr)
	return nil
}

// connectWithBackoff attempts to connect with exponential backoff.
func (d *Discovery) connectWithBackoff(addr string) {
	attempt := 0

	for {
		select {
		case <-d.stopCh:
			return
		default:
		}

		if err := d.connectBootstrap(addr); err == nil {
			return
		}

		// Determine delay from backoff schedule
		delay := d.backoffSchedule[attempt]
		if attempt < len(d.backoffSchedule)-1 {
			attempt++
		}

		fmt.Printf("[Discovery] Failed to connect to %s, retrying in %v (attempt %d)\n", addr, delay, attempt+1)

		select {
		case <-d.stopCh:
			return
		case <-time.After(delay):
		}
	}
}

// requestPeerList sends peer list requests to all connected bootstrap peers.
func (d *Discovery) requestPeerList() {
	d.mu.RLock()
	connectedPeers := make([]string, 0, len(d.connected))
	for peer, isConnected := range d.connected {
		if isConnected {
			connectedPeers = append(connectedPeers, peer)
		}
	}
	d.mu.RUnlock()

	for _, peer := range connectedPeers {
		d.sendPeerListRequest(peer)
	}
}

// sendPeerListRequest sends a peer list request to a specific peer.
func (d *Discovery) sendPeerListRequest(addr string) {
	req := PeerListRequest{}
	data, err := json.Marshal(req)
	if err != nil {
		return
	}

	msg := FrameMessage(MsgTypePeerListRequest, data)
	d.p2p.SendTo(addr, msg)
}

// sendPeerListResponse sends a peer list response to a specific peer.
func (d *Discovery) sendPeerListResponse(addr string, peers []string) {
	resp := PeerListResponse{Peers: peers}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	msg := FrameMessage(MsgTypePeerListResponse, data)
	d.p2p.SendTo(addr, msg)
}

// HandlePeerListRequest processes an incoming peer list request.
func (d *Discovery) HandlePeerListRequest(data []byte, src string) error {
	if d.peerListRequestHandler != nil {
		return d.peerListRequestHandler(src)
	}

	// Default: respond with known peers from peer manager
	// Get all peers and extract their addresses
	var peerAddrs []string
	if d.pm == nil {
		return nil // no peer manager available
	}
	// Get all connected peers as known peers
	connectedPeers := d.pm.GetPeersByState(PeerConnected)
	for _, peer := range connectedPeers {
		peerAddrs = append(peerAddrs, peer.Address)
	}
	d.sendPeerListResponse(src, peerAddrs)
	return nil
}

// HandlePeerListResponse processes an incoming peer list response.
func (d *Discovery) HandlePeerListResponse(data []byte, src string) error {
	var resp PeerListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to parse peer list response: %w", err)
	}

	// Add discovered peers to peer manager
	if d.pm != nil {
		for _, peerAddr := range resp.Peers {
			d.pm.AddPeer(PeerIDFromBytes([]byte(peerAddr)), peerAddr)
		}
	}

	fmt.Printf("[Discovery] Received %d peers from %s\n", len(resp.Peers), src)

	if d.peerListResponseHandler != nil {
		return d.peerListResponseHandler(src, resp.Peers)
	}

	return nil
}

// SetPeerListRequestHandler sets a custom handler for peer list requests.
func (d *Discovery) SetPeerListRequestHandler(handler func(addr string) error) {
	d.peerListRequestHandler = handler
}

// SetPeerListResponseHandler sets a custom handler for peer list responses.
func (d *Discovery) SetPeerListResponseHandler(handler func(addr string, peers []string) error) {
	d.peerListResponseHandler = handler
}

// IsConnected checks if a bootstrap peer is currently connected.
func (d *Discovery) IsConnected(addr string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected[addr]
}

// ConnectedPeers returns a list of currently connected bootstrap peers.
func (d *Discovery) ConnectedPeers() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]string, 0, len(d.connected))
	for peer, connected := range d.connected {
		if connected {
			result = append(result, peer)
		}
	}
	return result
}

// PeerListRequest is a message requesting a peer's known peers.
type PeerListRequest struct{}

// PeerListResponse is a message containing a list of known peers.
type PeerListResponse struct {
	Peers []string `json:"peers"`
}

// Message types for peer list exchange protocol.
const (
	MsgTypePeerListRequest  byte = 0x21 // Request peer list from connected node
	MsgTypePeerListResponse byte = 0x22 // Response with known peers
)

// Init registers the peer list message types with the message handling system.
func init() {
	// Register message types
	knownMessageTypes[MsgTypePeerListRequest] = true
	knownMessageTypes[MsgTypePeerListResponse] = true

	maxPayloadByType[MsgTypePeerListRequest] = MaxPayloadSize
	maxPayloadByType[MsgTypePeerListResponse] = MaxPayloadSize
}
