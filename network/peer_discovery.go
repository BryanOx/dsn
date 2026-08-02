package network

import (
	"encoding/binary"
	"log"
	"sync"
	"time"
)

// PeerInfo represents peer information for wire serialization.
type PeerInfo struct {
	ID       PeerID
	Address  string
	Score    int
	LastSeen time.Time
}

// PeerDiscovery handles peer discovery, PEX protocol, and health checks.
type PeerDiscovery struct {
	pm             *PeerManager
	bootstrapAddrs []string
	p2p            *P2PNode
	mu             sync.RWMutex
	stopCh         chan struct{}
	logger         *log.Logger

	// Track failed ping responses per peer
	pingFailures map[PeerID]int

	// Backoff for bootstrap retries
	backoffInterval time.Duration
	maxBackoff      time.Duration
}

// NewPeerDiscovery creates a new PeerDiscovery instance.
func NewPeerDiscovery(pm *PeerManager, bootstrapAddrs []string, p2p *P2PNode) *PeerDiscovery {
	return &PeerDiscovery{
		pm:              pm,
		bootstrapAddrs:  bootstrapAddrs,
		p2p:             p2p,
		stopCh:          make(chan struct{}),
		logger:          log.Default(),
		pingFailures:    make(map[PeerID]int),
		backoffInterval: 5 * time.Second,
		maxBackoff:      60 * time.Second,
	}
}

// Start begins the peer discovery background goroutines.
func (pd *PeerDiscovery) Start() {
	go pd.bootstrapLoop()
	go pd.healthCheckLoop()
	go pd.pexLoop()
	go pd.pruneLoop()
}

// Stop halts all peer discovery background goroutines.
func (pd *PeerDiscovery) Stop() {
	close(pd.stopCh)
	pd.stopCh = make(chan struct{})
}

// bootstrapLoop connects to bootstrap peers with exponential backoff.
func (pd *PeerDiscovery) bootstrapLoop() {
	backoff := pd.backoffInterval

	for {
		select {
		case <-pd.stopCh:
			return
		default:
		}

		connected := false
		for _, addr := range pd.bootstrapAddrs {
			pd.logger.Printf("[PEX] Attempting bootstrap connection to %s", addr)

			// Generate a PeerID from the address
			id := PeerIDFromBytes([]byte(addr))

			err := pd.pm.ConnectToPeer(id, addr)
			if err != nil {
				pd.logger.Printf("[PEX] Failed to connect to bootstrap peer %s: %v", addr, err)
				continue
			}

			// After connecting, send PEX message with our known peers
			pd.logger.Printf("[PEX] Connected to bootstrap peer %s, sending PEX", addr)
			pd.RequestPeerExchange(id)

			connected = true
		}

		if connected {
			backoff = pd.backoffInterval // Reset backoff on success
		} else {
			// Exponential backoff
			if backoff < pd.maxBackoff {
				backoff *= 2
			}
		}

		select {
		case <-pd.stopCh:
			return
		case <-time.After(backoff):
		}
	}
}

// healthCheckLoop pings all connected peers every 30 seconds.
func (pd *PeerDiscovery) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pd.stopCh:
			return
		case <-ticker.C:
			pd.performHealthCheck()
		}
	}
}

// performHealthCheck sends pings to all connected peers.
func (pd *PeerDiscovery) performHealthCheck() {
	connectedPeers := pd.pm.GetPeersByState(PeerConnected)

	for _, peer := range connectedPeers {
		go func(p *Peer) {
			p.mu.Lock()
			conn := p.Conn
			id := p.ID
			p.mu.Unlock()

			if conn == nil {
				return
			}

			// Send ping with timestamp
			err := pd.SendPing(id)
			if err != nil {
				pd.logger.Printf("[PING] Failed to send ping to %s: %v", id.String(), err)
				pd.pm.ReportFailure(id, 1)
				pd.trackPingFailure(id)
			}
		}(peer)
	}
}

// trackPingFailure tracks consecutive ping failures and disconnects after 3.
func (pd *PeerDiscovery) trackPingFailure(id PeerID) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	pd.pingFailures[id]++
	failCount := pd.pingFailures[id]

	if failCount >= 3 {
		pd.logger.Printf("[PING] Peer %s failed %d consecutive pings, disconnecting", id.String(), failCount)
		pd.pm.DisconnectPeer(id)
		delete(pd.pingFailures, id)
	}
}

// resetPingFailure resets the failure counter for a peer on successful pong.
func (pd *PeerDiscovery) resetPingFailure(id PeerID) {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	delete(pd.pingFailures, id)
}

// pexLoop sends PEX messages to top-scoring peers every 60 seconds.
func (pd *PeerDiscovery) pexLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pd.stopCh:
			return
		case <-ticker.C:
			pd.performPEX()
		}
	}
}

// performPEX sends peer exchange to top 100 peers.
func (pd *PeerDiscovery) performPEX() {
	bestPeers := pd.pm.GetBestPeers(100)
	pd.logger.Printf("[PEX] Sending peer exchange to %d best peers", len(bestPeers))

	for _, peer := range bestPeers {
		pd.mu.Lock()
		conn := peer.Conn
		pd.mu.Unlock()

		if conn != nil {
			pd.RequestPeerExchange(peer.ID)
		}
	}
}

// pruneLoop removes dead peers every 5 minutes.
func (pd *PeerDiscovery) pruneLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-pd.stopCh:
			return
		case <-ticker.C:
			pd.pruneDeadPeers()
		}
	}
}

// pruneDeadPeers removes peers not seen for 24h, or score < -10 and disconnected > 1h.
func (pd *PeerDiscovery) pruneDeadPeers() {
	pd.pm.mu.RLock()
	var toRemove []PeerID
	now := time.Now()

	for id, peer := range pd.pm.peers {
		peer.mu.Lock()
		age := now.Sub(peer.LastSeen)
		disconnectedDuration := now.Sub(peer.ConnectedSince)
		peer.mu.Unlock()

		// Remove if not seen for 24 hours
		if age > 24*time.Hour {
			toRemove = append(toRemove, id)
			continue
		}

		// Remove if score < -10 and disconnected for > 1 hour
		if peer.Score < -10 && peer.State == PeerDisconnected && disconnectedDuration > time.Hour {
			toRemove = append(toRemove, id)
		}
	}
	pd.pm.mu.RUnlock()

	for _, id := range toRemove {
		pd.logger.Printf("[PRUNE] Removing dead peer %s", id.String())
		pd.pm.RemovePeer(id)
	}
}

// HandlePeerExchange processes incoming PEX messages.
func (pd *PeerDiscovery) HandlePeerExchange(peerID PeerID, payload []byte) {
	peers := parsePeerInfo(payload)

	// Penalize for invalid/malformed PEX data
	if peers == nil || len(peers) == 0 {
		pd.logger.Printf("[PEX] Invalid peer info from %s, reporting failure", peerID.String())
		pd.pm.ReportFailure(peerID, 2)
		return
	}

	pd.logger.Printf("[PEX] Received %d peers from %s", len(peers), peerID.String())

	for _, pi := range peers {
		// Add or update peer in our manager
		peer := pd.pm.AddPeer(pi.ID, pi.Address)

		peer.mu.Lock()
		peer.Score = pi.Score
		peer.LastSeen = pi.LastSeen
		peer.mu.Unlock()

		pd.logger.Printf("[PEX] Added/updated peer %s from PEX", pi.ID.String())
	}
}

// RequestPeerExchange sends a PEX message to the specified peer.
func (pd *PeerDiscovery) RequestPeerExchange(peerID PeerID) {
	peer := pd.pm.GetPeer(peerID)
	if peer == nil {
		return
	}

	peer.mu.Lock()
	conn := peer.Conn
	peer.mu.Unlock()

	if conn == nil {
		return
	}

	// Get our best 100 peers
	ourPeers := pd.pm.GetBestPeers(100)

	// Serialize and send
	data := serializePeerInfo(ourPeers)
	msg := FrameMessage(MsgTypePeerExchange, data)

	_, err := conn.Write(msg)
	if err != nil {
		pd.logger.Printf("[PEX] Failed to send PEX to %s: %v", peerID.String(), err)
		pd.pm.ReportFailure(peerID, 1)
	}
}

// serializePeerInfo serializes a list of peers into wire format.
func serializePeerInfo(peers []*Peer) []byte {
	// Format: [count(2)][PeerInfo_1][PeerInfo_2]...
	// PeerInfo: [PeerID(32)][AddrLen(2)][Address(variable)][Score(4)][LastSeen(8)]

	var result []byte

	// Count (2 bytes)
	count := uint16(len(peers))
	result = append(result, 0, 0)
	binary.BigEndian.PutUint16(result[0:2], count)

	for _, peer := range peers {
		peer.mu.Lock()
		addr := peer.Address
		score := peer.Score
		lastSeen := peer.LastSeen
		id := peer.ID
		peer.mu.Unlock()

		// PeerID (32 bytes)
		result = append(result, id[:]...)

		// Address length (2 bytes)
		addrLen := uint16(len(addr))
		result = append(result, 0, 0)
		binary.BigEndian.PutUint16(result[len(result)-2:], addrLen)

		// Address (variable)
		result = append(result, []byte(addr)...)

		// Score (4 bytes)
		scoreBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(scoreBuf, uint32(score))
		result = append(result, scoreBuf...)

		// LastSeen (8 bytes - Unix timestamp)
		tsBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(tsBuf, uint64(lastSeen.Unix()))
		result = append(result, tsBuf...)
	}

	return result
}

// parsePeerInfo deserializes peer info from wire format.
func parsePeerInfo(data []byte) []PeerInfo {
	if len(data) < 2 {
		return nil
	}

	count := binary.BigEndian.Uint16(data[0:2])
	offset := 2

	result := make([]PeerInfo, 0, count)

	for i := 0; i < int(count) && offset < len(data); i++ {
		// Need at least 32 bytes for PeerID
		if offset+32 > len(data) {
			break
		}

		var id PeerID
		copy(id[:], data[offset:offset+32])
		offset += 32

		// Need 2 bytes for address length
		if offset+2 > len(data) {
			break
		}

		addrLen := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		// Need addrLen bytes for address
		if offset+int(addrLen) > len(data) {
			break
		}

		addr := string(data[offset : offset+int(addrLen)])
		offset += int(addrLen)

		// Need 4 bytes for score
		if offset+4 > len(data) {
			break
		}

		score := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4

		// Need 8 bytes for LastSeen
		if offset+8 > len(data) {
			break
		}

		ts := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
		offset += 8

		result = append(result, PeerInfo{
			ID:       id,
			Address:  addr,
			Score:    score,
			LastSeen: time.Unix(ts, 0),
		})
	}

	return result
}

// HandlePing processes incoming ping messages.
func (pd *PeerDiscovery) HandlePing(peerID PeerID, payload []byte) {
	peer := pd.pm.GetPeer(peerID)
	if peer == nil {
		return
	}

	peer.mu.Lock()
	conn := peer.Conn
	peer.mu.Unlock()

	if conn == nil {
		return
	}

	// Respond with pong, echoing the timestamp
	msg := FrameMessage(MsgTypePong, payload)
	_, err := conn.Write(msg)
	if err != nil {
		pd.logger.Printf("[PONG] Failed to send pong to %s: %v", peerID.String(), err)
	}
}

// HandlePong processes incoming pong messages.
func (pd *PeerDiscovery) HandlePong(peerID PeerID, payload []byte) {
	peer := pd.pm.GetPeer(peerID)
	if peer == nil {
		return
	}

	peer.mu.Lock()
	now := time.Now()

	// If payload has timestamp, calculate latency
	if len(payload) >= 8 {
		timestamp := int64(binary.BigEndian.Uint64(payload[:8]))
		sentTime := time.Unix(timestamp, 0)
		latency := now.Sub(sentTime)

		// Update rolling average latency
		if peer.Latency == 0 {
			peer.Latency = latency
		} else {
			peer.Latency = (peer.Latency*4 + latency) / 5
		}
	}

	peer.LastSeen = now
	peer.mu.Unlock()

	// Reset failure counter on successful pong
	pd.resetPingFailure(peerID)

	pd.logger.Printf("[PONG] Received from %s, latency: %v", peerID.String(), peer.Latency)
}

// SendPing sends a ping message to the specified peer.
func (pd *PeerDiscovery) SendPing(peerID PeerID) error {
	peer := pd.pm.GetPeer(peerID)
	if peer == nil {
		return nil
	}

	peer.mu.Lock()
	conn := peer.Conn
	peer.mu.Unlock()

	if conn == nil {
		return nil
	}

	// Create payload with timestamp (8 bytes)
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, uint64(time.Now().Unix()))

	msg := FrameMessage(MsgTypePing, payload)
	_, err := conn.Write(msg)

	return err
}
