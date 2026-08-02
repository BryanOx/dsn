package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dsn/dsn/types"
	"golang.org/x/time/rate"
)

// connectionLimiter implements connection rate limiting per IP and global peer limits.
type connectionLimiter struct {
	mu        sync.Mutex
	peers     map[string]struct{}      // active connections by peerID
	ipBuckets map[string]*rate.Limiter // per-IP token bucket
	tempBans  map[string]time.Time     // IP → ban expiration
	maxPeers  int
	perIPRate rate.Limit // 5 events/sec
	burst     int
}

// newConnectionLimiter creates a new connection limiter with the specified limits.
func newConnectionLimiter() *connectionLimiter {
	return &connectionLimiter{
		peers:     make(map[string]struct{}),
		ipBuckets: make(map[string]*rate.Limiter),
		tempBans:  make(map[string]time.Time),
		maxPeers:  50,
		perIPRate: rate.Limit(5), // 5 events/sec
		burst:     5,
	}
}

// allow checks if a connection from the given IP should be allowed.
// It checks: temp ban list, max concurrent peers, and per-IP rate limit.
func (cl *connectionLimiter) allow(ip string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Check temp ban list (60s ban)
	if banTime, banned := cl.tempBans[ip]; banned {
		if time.Now().Before(banTime) {
			return false
		}
		// Ban expired, clean up
		delete(cl.tempBans, ip)
	}

	// Check max concurrent peers
	if len(cl.peers) >= cl.maxPeers {
		return false
	}

	// Check per-IP rate limit
	limiter, exists := cl.ipBuckets[ip]
	if !exists {
		limiter = rate.NewLimiter(cl.perIPRate, cl.burst)
		cl.ipBuckets[ip] = limiter
	}

	return limiter.Allow()
}

// addPeer records a new active peer connection.
func (cl *connectionLimiter) addPeer(peerID string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.peers[peerID] = struct{}{}
}

// removePeer removes an active peer connection.
func (cl *connectionLimiter) removePeer(peerID string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	delete(cl.peers, peerID)
}

// blockJob represents a block processing task for the single-consumer channel.
type blockJob struct {
	data []byte // raw block message bytes (includes type byte)
	src  string // peer ID or remote address
}

// P2PNode represents a simple P2P node using TCP connections.
type P2PNode struct {
	addr         string
	listener     net.Listener
	pm           *PeerManager
	connections  map[string]net.Conn // addr -> conn (lightweight TCP tracker)
	connMu       sync.RWMutex
	txHandler    func(tx *types.Transaction)
	blockHandler func(data []byte)
	stopCh       chan struct{}
	// Connection rate limiter
	connLimiter *connectionLimiter
	// Rate limiting (per-message)
	rateLimiters map[string]*TokenBucket // addr -> rate limiter
	rateLimitMu  sync.RWMutex
	// Snapshot handlers
	snapshotQueryHandler   func() *SnapshotInfo
	snapshotInfoHandler    func(info *SnapshotInfo)
	snapshotRequestHandler func(snapshotHash [32]byte, chunkIndex uint32) (*SnapshotChunk, bool)
	snapshotChunkHandler   func(chunk *SnapshotChunk, peer string)
	// Engine references (Task 5.6)
	fastSync  *FastSyncEngine
	blockSync *BlockSyncEngine
	gossip    *GossipEngine
	discovery *PeerDiscovery
	// Block processing: single-consumer channel pattern for goroutine safety
	blockCh   chan blockJob
	blockOnce sync.Once // ensures block processor starts only once
}

// NewP2PNode creates a new P2P node listening on the given port (0 = random).
func NewP2PNode(listenPort int) (*P2PNode, error) {
	addr := fmt.Sprintf(":%d", listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Get actual port if port 0 was specified
	if listenPort == 0 {
		// The listener on :0 already has an assigned port, just use it
		// No need to close and re-create
	}

	n := &P2PNode{
		addr:         listener.Addr().String(),
		listener:     listener,
		pm:           NewPeerManager(nil), // no DB persistence in constructor
		connections:  make(map[string]net.Conn),
		stopCh:       make(chan struct{}),
		connLimiter:  newConnectionLimiter(),
		rateLimiters: make(map[string]*TokenBucket),
		blockCh:      make(chan blockJob, 128), // buffered channel for block processing
	}

	// Start accepting connections in background
	go n.acceptConnections()

	// Lazy-start block processor (single-consumer pattern for goroutine safety)
	n.startBlockProcessor()

	return n, nil
}

// AcceptConnections handles incoming peer connections.
func (n *P2PNode) acceptConnections() {
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		n.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))
		conn, err := n.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if n.listener == nil {
				return // Listener closed
			}
			continue
		}

		// Check connection rate limiter before handling
		remoteIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			remoteIP = conn.RemoteAddr().String() // fallback to full address
		}
		if !n.connLimiter.allow(remoteIP) {
			conn.Close()
			continue
		}

		go n.handleConnection(conn)
	}
}

// Handle an incoming connection from a peer.
func (n *P2PNode) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read the peer's address for identification
	remoteAddr := conn.RemoteAddr().String()

	// Check for duplicate connection - close existing if present
	n.connMu.Lock()
	if existingConn, exists := n.connections[remoteAddr]; exists {
		// Close existing connection, replace with new one
		existingConn.Close()
	}
	n.connections[remoteAddr] = conn
	n.connMu.Unlock()

	defer func() {
		n.connMu.Lock()
		delete(n.connections, remoteAddr)
		n.connMu.Unlock()
		// Clean up rate limiter for this peer
		n.rateLimitMu.Lock()
		delete(n.rateLimiters, remoteAddr)
		n.rateLimitMu.Unlock()
		// Remove from connection limiter
		n.connLimiter.removePeer(remoteAddr)
	}()

	// Register with connection limiter
	n.connLimiter.addPeer(remoteAddr)

	// Register with PeerManager for metadata tracking
	id := PeerIDFromBytes([]byte(remoteAddr))
	n.pm.AddPeer(id, remoteAddr)

	reader := bufio.NewReader(conn)
	failures := 0 // Track consecutive failures for rate limiting
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		// Set deadline for reading
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		// Read message length (4 bytes)
		lenBuf := make([]byte, 4)
		_, err := reader.Read(lenBuf)
		if err != nil {
			return
		}

		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen > MaxPayloadSize {
			return
		}

		// Read message
		msgBuf := make([]byte, msgLen)
		_, err = reader.Read(msgBuf)
		if err != nil {
			return
		}

		// Check message type - transactions don't have type byte, check if it's a known protocol type
		if len(msgBuf) == 0 {
			continue
		}

		// Per-peer rate limiting: check before processing
		n.rateLimitMu.Lock()
		bucket, exists := n.rateLimiters[remoteAddr]
		if !exists {
			bucket = NewTokenBucket(100, 10) // burst 100, refill 10/s
			n.rateLimiters[remoteAddr] = bucket
		}
		n.rateLimitMu.Unlock()

		if !bucket.Allow() {
			// Rate limit exceeded - report failure, increment failures counter
			n.pm.ReportFailure(id, 10) // -10 spam penalty
			failures++
			if failures >= 3 {
				n.pm.Ban(id, 5*time.Minute)
				return // disconnect
			}
			continue
		}
		// Reset failures on successful rate limit check
		failures = 0

		msgType := msgBuf[0]

		// Validate message type using IsKnownType for protocol messages
		// Transactions (no type byte) will fail IsKnownType but should still be processed
		isKnown := IsKnownType(msgType)

		if msgType >= MsgTypeSnapshotQuery && msgType <= MsgTypeSnapshotChunk {
			// Snapshot message
			msgData := msgBuf[1:]
			go n.handleSnapshotMessage(msgType, msgData, remoteAddr)
		} else if msgType == MsgTypeBlock {
			// Block message - push to single-consumer channel for goroutine safety
			if n.blockHandler != nil {
				n.handleBlock(msgBuf, remoteAddr)
			}
		} else if msgType == MsgTypeTransaction {
			// Transaction message (with type byte)
			n.handleTransaction(msgBuf[1:], id)
		} else if isKnown {
			// Other known protocol messages (PEX, block range, ping/pong)
			// Process based on type
			switch msgType {
			case MsgTypePeerExchange, MsgTypeBlockRangeRequest, MsgTypeBlockRangeResponse, MsgTypePing, MsgTypePong:
				// These are valid but we don't have handlers yet
			}
		} else {
			// Transaction message (legacy format - no type byte - raw transaction encoding)
			tx := &types.Transaction{}
			if err := tx.Decode(bytes.NewReader(msgBuf)); err != nil {
				// Penalize via PeerManager for decode failure
				n.pm.ReportFailure(id, 2)
				continue
			}

			if n.txHandler != nil {
				go n.txHandler(tx)
			}
		}
	}
}

// handleTransaction processes an incoming transaction message and forwards it to the txHandler.
func (n *P2PNode) handleTransaction(data []byte, peerID PeerID) {
	tx := &types.Transaction{}
	if err := tx.Decode(bytes.NewReader(data)); err != nil {
		// Penalize via PeerManager for decode failure
		n.pm.ReportFailure(peerID, 2)
		return
	}

	if n.txHandler != nil {
		go n.txHandler(tx)
	}
}

// ID returns a simple node identifier (based on listen address).
func (n *P2PNode) ID() string {
	return hex.EncodeToString([]byte(n.addr))[:16]
}

// Addr returns the listen address of this node.
func (n *P2PNode) Addr() string {
	return n.addr
}

// Connect connects to a peer at the given address.
func (n *P2PNode) Connect(addr string) error {
	// Check if already connected
	n.connMu.RLock()
	if _, ok := n.connections[addr]; ok {
		n.connMu.RUnlock()
		return fmt.Errorf("already connected to %s", addr)
	}
	n.connMu.RUnlock()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	// Store in connections map
	n.connMu.Lock()
	n.connections[addr] = conn
	n.connMu.Unlock()

	// Register with PeerManager for metadata tracking
	id := PeerIDFromBytes([]byte(addr))
	n.pm.AddPeer(id, addr)

	// Note: The server side (peer) will handle reading via acceptConnections -> handleConnection
	// We don't need to start a reader here because we only write (Broadcast/GossipTransaction)

	return nil
}

// NumPeers returns the number of connected peers.
func (n *P2PNode) NumPeers() int {
	n.connMu.RLock()
	defer n.connMu.RUnlock()
	return len(n.connections)
}

// isConnected checks if already connected to an address.
func (n *P2PNode) isConnected(addr string) bool {
	n.connMu.RLock()
	defer n.connMu.RUnlock()
	_, exists := n.connections[addr]
	return exists
}

// GossipTransaction broadcasts a transaction to all connected peers.
func (n *P2PNode) GossipTransaction(tx *types.Transaction, hasher types.Hasher) error {
	var buf bytes.Buffer
	if err := tx.Encode(&buf); err != nil {
		return err
	}

	msgLen := uint32(buf.Len())
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, msgLen)

	n.connMu.Lock()
	defer n.connMu.Unlock()

	var failedPeers []string
	for addr, conn := range n.connections {
		_, err := conn.Write(lenBuf)
		if err != nil {
			failedPeers = append(failedPeers, addr)
			continue
		}
		_, err = conn.Write(buf.Bytes())
		if err != nil {
			failedPeers = append(failedPeers, addr)
			continue
		}
	}

	// Remove stale peers that failed to write
	for _, addr := range failedPeers {
		if conn, ok := n.connections[addr]; ok {
			conn.Close()
			delete(n.connections, addr)
		}
	}

	return nil
}

// SetTxHandler registers a callback for incoming transactions.
func (n *P2PNode) SetTxHandler(handler func(tx *types.Transaction)) {
	n.txHandler = handler
}

// SetBlockHandler registers a callback for incoming blocks.
func (n *P2PNode) SetBlockHandler(handler func(data []byte)) {
	n.blockHandler = handler
}

// Broadcast sends a message to all connected peers.
func (n *P2PNode) Broadcast(data []byte) {
	msgLen := uint32(len(data))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, msgLen)

	n.connMu.Lock()
	defer n.connMu.Unlock()

	var failedPeers []string
	for addr, conn := range n.connections {
		_, err := conn.Write(lenBuf)
		if err != nil {
			failedPeers = append(failedPeers, addr)
			continue
		}
		_, err = conn.Write(data)
		if err != nil {
			failedPeers = append(failedPeers, addr)
			continue
		}
	}

	// Remove stale peers that failed to write
	for _, addr := range failedPeers {
		if conn, ok := n.connections[addr]; ok {
			conn.Close()
			delete(n.connections, addr)
		}
	}
}

// SendTo sends a framed message to a specific peer by address.
// Returns error if peer not found or write fails.
func (n *P2PNode) SendTo(addr string, data []byte) error {
	msgLen := uint32(len(data))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, msgLen)

	n.connMu.RLock()
	conn, ok := n.connections[addr]
	n.connMu.RUnlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", addr)
	}

	_, err := conn.Write(lenBuf)
	if err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// Close shuts down the node and all connections.
func (n *P2PNode) Close() error {
	close(n.stopCh)

	n.connMu.Lock()
	for _, conn := range n.connections {
		conn.Close()
	}
	n.connections = make(map[string]net.Conn)
	n.connMu.Unlock()

	// Clean up rate limiters
	n.rateLimitMu.Lock()
	n.rateLimiters = make(map[string]*TokenBucket)
	n.rateLimitMu.Unlock()

	if n.listener != nil {
		return n.listener.Close()
	}
	return nil
}

// SetFastSyncEngine registers a FastSyncEngine with the node and wires up
// all snapshot message handlers for both client and server functionality.
func (n *P2PNode) SetFastSyncEngine(e *FastSyncEngine) {
	n.fastSync = e

	// Client-side: handle incoming snapshot info messages
	n.snapshotInfoHandler = func(info *SnapshotInfo) {
		e.HandleSnapshotInfo(info)
	}

	// Client-side: handle incoming snapshot chunks with peer for failure reporting
	n.snapshotChunkHandler = func(chunk *SnapshotChunk, peer string) {
		// Convert peer string to PeerID for failure reporting
		var peerID PeerID
		if p := n.pm.GetPeerByAddr(peer); p != nil {
			peerID = p.ID
		}
		e.HandleSnapshotChunk(chunk, peerID)
	}

	// Server-side: respond to snapshot queries - returns latest snapshot info
	// Note: state.LoadLatestSnapshotInfo doesn't exist, so return nil to skip serving
	n.snapshotQueryHandler = func() *SnapshotInfo {
		// TODO: Implement when state.LoadLatestSnapshotInfo is added
		// This would load the latest checkpoint and return snapshot metadata
		return nil
	}

	// Server-side: respond to chunk requests - returns requested chunk data
	// Note: state.LoadSnapshotChunk doesn't exist, so return nil to skip serving
	n.snapshotRequestHandler = func(snapshotHash [32]byte, chunkIndex uint32) (*SnapshotChunk, bool) {
		// TODO: Implement when state.LoadSnapshotChunk is added
		// This would load chunk data from persistent storage and convert to network.SnapshotChunk
		return nil, false
	}
}

// SetBlockSyncEngine registers a BlockSyncEngine with the node.
func (n *P2PNode) SetBlockSyncEngine(e *BlockSyncEngine) {
	n.blockSync = e
}

// SetGossipEngine registers a GossipEngine with the node.
func (n *P2PNode) SetGossipEngine(e *GossipEngine) {
	n.gossip = e
}

// SetDiscovery registers a PeerDiscovery with the node.
func (n *P2PNode) SetDiscovery(e *PeerDiscovery) {
	n.discovery = e
}

// PeerManager returns the node's PeerManager for engine wiring.
func (n *P2PNode) PeerManager() *PeerManager {
	return n.pm
}

// startBlockProcessor launches a single background goroutine that consumes
// block jobs sequentially from the channel. Uses sync.Once to ensure only
// one processor ever starts (even if called multiple times).
func (n *P2PNode) startBlockProcessor() {
	n.blockOnce.Do(func() {
		go func() {
			for {
				select {
				case <-n.stopCh:
					close(n.blockCh)
					return
				case job, ok := <-n.blockCh:
					if !ok {
						return // channel closed
					}
					// Process block in the single consumer goroutine
					// Pass raw bytes to handler (same as original implementation)
					if n.blockHandler != nil {
						n.blockHandler(job.data)
					}
				}
			}
		}()
	})
}

// handleBlock enqueues a block job to the single-consumer channel.
// The actual processing happens in the single-consumer block processor goroutine.
func (n *P2PNode) handleBlock(data []byte, src string) {
	select {
	case n.blockCh <- blockJob{data: data, src: src}:
		// Job enqueued successfully
	default:
		// Channel full - drop block (backpressure)
	}
}
