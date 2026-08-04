package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
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

// readDeadline is the per-read timeout for P2P connections. It is sized at 2x
// the peer-discovery ping interval (pingInterval in peer_discovery.go): a peer
// that only ever hears pongs — one per interval — must be granted at least one
// interval of slack for the ping round trip, otherwise its read times out
// whenever the pong lands a few milliseconds past the deadline and the
// connection is torn down, forcing a reconnect cycle.
const readDeadline = 2 * pingInterval

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

// blockJobKind discriminates the payloads the single-consumer block processor
// handles: full gossip block frames (which include the message type byte) and
// raw block-range response payloads (type byte already stripped by the read
// loop, per the known-types convention).
type blockJobKind uint8

const (
	blockJobGossip blockJobKind = iota
	blockJobRangeResponse
)

// blockJob represents a block processing task for the single-consumer channel.
type blockJob struct {
	kind blockJobKind
	data []byte // raw message bytes (gossip: full frame; range: payload only)
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
	voteHandler  func(vote *types.Vote)
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
	discovery       *PeerDiscovery // ping/pong + PEX handshake (0x24)
	bootstrapDisc   *Discovery     // bootstrap peer-list exchange (0x21/0x22)
	// syncHeightProvider reports the node's current chain height so a fresh
	// SyncHeight announcement can be sent to each newly connected peer.
	syncHeightProvider func() uint64
	// Block processing: single-consumer channel pattern for goroutine safety
	blockCh   chan blockJob
	blockOnce sync.Once // ensures block processor starts only once
	// blockDone is closed by the block processor goroutine when it has fully
	// exited. Close joins on it so no in-flight block apply (which commits to
	// the persistent DB) can run after Close returns — Node.Close closes the
	// DB after p2p.Close returns, so a late apply would panic.
	blockDone chan struct{}
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
		blockDone:    make(chan struct{}),
	}

	// Outbound connections dialed through the PeerManager (e.g. by peer
	// discovery) must also be registered for broadcast and read.
	n.pm.SetOnConnectedHandler(n.handlePeerConnection)

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

	// Register with PeerManager for metadata tracking
	id := PeerIDFromBytes([]byte(remoteAddr))
	cleanup := n.registerConn(conn, remoteAddr, id)
	defer cleanup()

	n.readLoop(conn, remoteAddr, id)
}

// registerConn tracks a connection for broadcasting, deduplicating against an
// existing connection to the same address, and returns a cleanup function
// that removes the connection from every tracking structure.
func (n *P2PNode) registerConn(conn net.Conn, remoteAddr string, id PeerID) func() {
	n.connMu.Lock()
	if existing, exists := n.connections[remoteAddr]; exists && existing != conn {
		// The connection map only holds connections whose read loop has not
		// exited yet, so a present entry is a live connection. Keep it and
		// discard the duplicate dial instead of tearing the healthy link
		// down; redirect the PeerManager's peer to the survivor so pings and
		// PEX keep flowing over it.
		n.connMu.Unlock()
		if p := n.pm.GetPeerByAddr(remoteAddr); p != nil {
			p.mu.Lock()
			p.Conn = existing
			p.State = PeerConnected
			p.mu.Unlock()
		}
		conn.Close()
		return func() {}
	}
	if existing, exists := n.connections[remoteAddr]; exists && existing != conn {
		existing.Close()
	}
	n.connections[remoteAddr] = conn
	n.connMu.Unlock()

	// Register with connection limiter
	n.connLimiter.addPeer(remoteAddr)

	// Register with PeerManager for metadata tracking and wire the peer's
	// Conn to this connection so the discovery handlers (ping/pong/PEX) can
	// respond over the same socket. This is required for inbound connections,
	// which ConnectToPeer never touches: without it HandlePing finds a nil
	// Conn and no pong is ever sent back, so the remote's read deadline
	// tears the connection down.
	peer := n.pm.AddPeer(id, remoteAddr)
	peer.mu.Lock()
	peer.Conn = conn
	// A registered conn is a live peer: outbound Connect and the accept path
	// both reach here without going through ConnectToPeer, which is the only
	// place State was being set to PeerConnected before. Without this, peers
	// brought up by Connect stay PeerDisconnected and are invisible to
	// consumers that filter on the connected state (e.g. the snapshot
	// engine's query broadcast).
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Announce our height to the fresh peer (late-joiner path): it needs to
	// know we exist and how far ahead we are before it can request ranges.
	if n.syncHeightProvider != nil {
		payload := make([]byte, 8)
		binary.BigEndian.PutUint64(payload, n.syncHeightProvider())
		msg := make([]byte, 1+len(payload))
		msg[0] = MsgTypeSyncHeight
		copy(msg[1:], payload)
		_ = n.SendTo(remoteAddr, msg)
	}

	return func() {
		// Only remove the connection if the entry still points at OUR conn.
		// A stale read loop from an older connection must never delete a newer
		// connection that replaced it for the same address.
		n.connMu.Lock()
		if cur, ok := n.connections[remoteAddr]; ok && cur == conn {
			delete(n.connections, remoteAddr)
		}
		n.connMu.Unlock()
		// Clean up rate limiter for this peer
		n.rateLimitMu.Lock()
		delete(n.rateLimiters, remoteAddr)
		n.rateLimitMu.Unlock()
		// Remove from connection limiter
		n.connLimiter.removePeer(remoteAddr)
	}
}

// readLoop reads framed messages from a connection and dispatches them until
// the peer disconnects, the read fails, or the node stops. It runs for both
// incoming and outgoing connections so every connection is drained.
func (n *P2PNode) readLoop(conn net.Conn, remoteAddr string, id PeerID) {
	reader := bufio.NewReader(conn)
	failures := 0 // Track consecutive failures for rate limiting
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		// Set deadline for reading. Must comfortably exceed the peer-discovery
		// ping interval (readDeadline = 2x pingInterval): a peer that only
		// hears pongs once per interval would otherwise time out whenever the
		// pong lands just past the deadline, tearing the connection down.
		conn.SetReadDeadline(time.Now().Add(readDeadline))

		// Read message length (4 bytes)
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return
		}

		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen > MaxPayloadSize {
			return
		}

		// Read message (ReadFull: a single Read may return a partial payload
		// for large messages such as snapshot chunks).
		msgBuf := make([]byte, msgLen)
		if _, err := io.ReadFull(reader, msgBuf); err != nil {
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
		} else if msgType == MsgTypeVote {
			// Vote message
			n.handleVote(msgBuf, id)
		} else if isKnown {
			// Other known protocol messages (PEX, block range, ping/pong)
			// Process based on type. Handlers expect the payload WITHOUT the
			// leading type byte (the type is passed separately).
			switch msgType {
			case MsgTypePeerExchange:
				if n.discovery != nil {
					n.discovery.HandlePeerExchange(id, msgBuf[1:])
				}
			case MsgTypePeerListRequest:
				if n.bootstrapDisc != nil {
					n.bootstrapDisc.HandlePeerListRequest(msgBuf[1:], remoteAddr)
				}
			case MsgTypePeerListResponse:
				if n.bootstrapDisc != nil {
					n.bootstrapDisc.HandlePeerListResponse(msgBuf[1:], remoteAddr)
				}
			case MsgTypePing:
				if n.discovery != nil {
					n.discovery.HandlePing(id, msgBuf[1:])
				}
			case MsgTypePong:
				if n.discovery != nil {
					n.discovery.HandlePong(id, msgBuf[1:])
				}
			case MsgTypeBlockRangeRequest:
				// Serve block ranges without blocking the read loop. The
				// engine returns nil for malformed or out-of-bounds requests
				// (reporting the failure itself); the response goes back over
				// the same connection.
				if n.blockSync != nil {
					req := msgBuf[1:]
					peer := id
					src := remoteAddr
					go func() {
						resp := n.blockSync.HandleBlockRangeRequest(req, peer)
						if resp != nil {
							out := make([]byte, 1+len(resp))
							out[0] = MsgTypeBlockRangeResponse
							copy(out[1:], resp)
							_ = n.SendTo(src, out)
						}
					}()
				}
			case MsgTypeBlockRangeResponse:
				// Route through the single-consumer block channel so range
				// responses apply serially with gossip blocks (D5).
				if n.blockSync != nil {
					n.enqueueBlockRangeResponse(msgBuf[1:], remoteAddr)
				}
			case MsgTypeSyncHeight:
				// Height announcement: the payload must be exactly 8 bytes
				// (W2). Any other length is malformed: penalize the sender
				// and ignore the message so the recorded height stays
				// unchanged. The penalty fires regardless of whether the
				// block-sync engine is wired.
				if len(msgBuf) != 1+8 {
					n.pm.ReportFailure(id, 2)
					continue
				}
				// Record the height monotonically and wake the block-sync
				// engine so catch-up starts without waiting for its
				// periodic tick.
				if n.blockSync != nil {
					h := binary.BigEndian.Uint64(msgBuf[1:9])
					n.pm.UpdateSyncHeight(id, h)
					n.blockSync.NotifyPeerHeight(h)
				}
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

// handleVote processes an incoming vote message and forwards it to the voteHandler.
// The payload starts with the message type byte; handleVote strips it before decoding the Vote.
func (n *P2PNode) handleVote(data []byte, peerID PeerID) {
	vote := &types.Vote{}
	if err := vote.Decode(bytes.NewReader(data[1:])); err != nil {
		// Penalize via PeerManager for decode failure
		n.pm.ReportFailure(peerID, 2)
		return
	}

	if n.voteHandler != nil {
		n.voteHandler(vote)
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

// Connect connects to a peer at the given address and starts reading from the
// outbound connection. Before the fix the outbound connection was only used
// for writing, so messages a peer sent back over that socket (or that arrived
// after both sides dialed each other) were never processed.
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

	id := PeerIDFromBytes([]byte(addr))
	cleanup := n.registerConn(conn, addr, id)

	go func() {
		defer cleanup()
		defer conn.Close()
		n.readLoop(conn, addr, id)
	}()

	return nil
}

// handlePeerConnection is installed as the PeerManager's on-connected hook so
// connections established through pm.ConnectToPeer (for example by peer
// discovery) are registered for broadcast and read just like connections
// established by Connect or accepted by the listener. It runs after the peer
// lock has been released.
func (n *P2PNode) handlePeerConnection(conn net.Conn, addr string) {
	id := PeerIDFromBytes([]byte(addr))
	cleanup := n.registerConn(conn, addr, id)
	go func() {
		defer cleanup()
		defer conn.Close()
		n.readLoop(conn, addr, id)
	}()
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

// GossipTransaction broadcasts a transaction to all connected peers as a
// single-framed typed message ([0x02][tx]). The legacy [len][raw-tx] framing
// misrouted any tx whose first encoded byte is a known message type — a
// Version-2 (0x0200) tx encodes 0x02 (MsgTypeTransaction) and the read loop
// stripped that byte as a type prefix, shifting every field. Aligned with
// GossipEngine.GossipTransaction (PR1c); the type byte makes the framing work
// for every tx version (S2).
func (n *P2PNode) GossipTransaction(tx *types.Transaction, hasher types.Hasher) error {
	var buf bytes.Buffer
	if err := tx.Encode(&buf); err != nil {
		return err
	}

	msg := make([]byte, 1+buf.Len())
	msg[0] = MsgTypeTransaction
	copy(msg[1:], buf.Bytes())
	n.Broadcast(msg)
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

// SetVoteHandler registers a callback for incoming consensus votes.
func (n *P2PNode) SetVoteHandler(handler func(vote *types.Vote)) {
	n.voteHandler = handler
}

// Broadcast sends a message to all connected peers.
func (n *P2PNode) Broadcast(data []byte) {
	// Frame the message (4-byte big-endian length + payload) in one buffer
	// and write it with a single conn.Write. net.Conn serializes individual
	// Write calls, so one call per frame guarantees no other concurrent
	// sender (SendTo/sendToPeer) can interleave its length prefix between
	// our prefix and payload, which would corrupt the frame stream and tear
	// the connection down (read loop exits on a corrupt > MaxPayloadSize
	// length).
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)

	n.connMu.Lock()
	defer n.connMu.Unlock()

	var failedPeers []string
	for addr, conn := range n.connections {
		if _, err := conn.Write(frame); err != nil {
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
	// One conn.Write per frame (see Broadcast): two separate Write calls
	// from concurrent senders on the same connection could interleave a
	// length prefix with another frame's payload and corrupt the stream.
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)

	n.connMu.RLock()
	conn, ok := n.connections[addr]
	n.connMu.RUnlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", addr)
	}

	if _, err := conn.Write(frame); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// blockProcessorJoinTimeout bounds how long Close waits for the block
// processor goroutine to finish. The processor applies blocks that commit to
// the persistent DB, so Close must join it before the caller (Node.Close)
// closes the DB — but a wedged handler must not hang Close forever.
const blockProcessorJoinTimeout = 2 * time.Second

// Close shuts down the node and all connections.
func (n *P2PNode) Close() error {
	close(n.stopCh)

	// Join the block processor before returning: the select in the processor
	// may pick a buffered block job after stopCh fired, and processing it
	// calls n.blockHandler (block apply → DB commit). Node.Close closes the
	// persistent DB after this returns, so an in-flight apply would panic on
	// the closed DB. Waiting for blockDone guarantees no apply can run after
	// Close returns.
	select {
	case <-n.blockDone:
	case <-time.After(blockProcessorJoinTimeout):
	}

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

	// Server-side: respond to snapshot queries with the latest stored
	// snapshot's metadata (task 5.2, cached per 8.3). Serve handlers must not
	// return nil when snapshots exist — the stored checkpoint + snapshot data
	// is the source.
	n.snapshotQueryHandler = func() *SnapshotInfo {
		info, _ := e.ServeSnapshot()
		return info
	}

	// Server-side: respond to chunk requests from the stored snapshot data.
	// A chunk index beyond the snapshot's chunk count is not served. Chunks
	// come from the latest-only cache (8.3).
	n.snapshotRequestHandler = func(snapshotHash [32]byte, chunkIndex uint32) (*SnapshotChunk, bool) {
		info, chunks := e.ServeSnapshot()
		if info == nil || info.SnapshotHash != snapshotHash || chunkIndex >= uint32(len(chunks)) {
			return nil, false
		}
		ch := chunks[chunkIndex]
		return &SnapshotChunk{
			Index:        ch.Index,
			TotalCount:   ch.TotalCount,
			SnapshotHash: ch.SnapshotHash,
			ChunkHash:    ch.ChunkHash,
			Data:         ch.Data,
		}, true
	}
}

// SetBlockSyncEngine registers a BlockSyncEngine with the node.
func (n *P2PNode) SetBlockSyncEngine(e *BlockSyncEngine) {
	n.blockSync = e
}

// SetSyncHeightProvider registers a callback reporting the node's current chain
// height. It is used to announce the height to each newly connected peer so
// late joiners and restarted nodes are immediately visible.
func (n *P2PNode) SetSyncHeightProvider(fn func() uint64) {
	n.syncHeightProvider = fn
}

// SetGossipEngine registers a GossipEngine with the node.
func (n *P2PNode) SetGossipEngine(e *GossipEngine) {
	n.gossip = e
}

// SetDiscovery registers a PeerDiscovery with the node.
func (n *P2PNode) SetDiscovery(e *PeerDiscovery) {
	n.discovery = e
}

// SetBootstrapDiscovery registers the bootstrap peer-list exchange (0x21/0x22)
// with the node so the read loop can dispatch requests and responses to it.
func (n *P2PNode) SetBootstrapDiscovery(e *Discovery) {
	n.bootstrapDisc = e
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
			defer close(n.blockDone) // signal full exit so Close can join us
			for {
				select {
				case <-n.stopCh:
					close(n.blockCh)
					return
				case job, ok := <-n.blockCh:
					if !ok {
						return // channel closed
					}
					// Dispatch by job kind: gossip frames go to the gossip
					// handler, range responses to the block-sync engine. Both
					// are processed in this single consumer goroutine so no
					// two block applies ever run concurrently.
					switch job.kind {
					case blockJobRangeResponse:
						if n.blockSync != nil {
							var peerID PeerID
							if p := n.pm.GetPeerByAddr(job.src); p != nil {
								peerID = p.ID
							}
							n.blockSync.HandleBlockRangeResponse(job.data, peerID)
						}
					default: // blockJobGossip
						if n.blockHandler != nil {
							n.blockHandler(job.data)
						}
					}
				}
			}
		}()
	})
}

// handleBlock enqueues a gossip block job to the single-consumer channel.
// The actual processing happens in the single-consumer block processor goroutine.
func (n *P2PNode) handleBlock(data []byte, src string) {
	select {
	case <-n.stopCh:
		// Shutting down — drop the block. Without this guard a read loop that
		// passed its stop check before the processor exited could send on the
		// processor-closed blockCh and panic.
	case n.blockCh <- blockJob{kind: blockJobGossip, data: data, src: src}:
		// Job enqueued successfully
	default:
		// Channel full - drop block (backpressure)
	}
}

// enqueueBlockRangeResponse routes a block-range response payload through the
// single-consumer block channel so it is applied serially with gossip blocks.
// Concurrent applies from multiple read loops would break determinism (D5).
func (n *P2PNode) enqueueBlockRangeResponse(data []byte, src string) {
	select {
	case <-n.stopCh:
		// Shutting down — drop the response (see handleBlock).
	case n.blockCh <- blockJob{kind: blockJobRangeResponse, data: data, src: src}:
		// Job enqueued successfully
	default:
		// Channel full - drop response (backpressure)
	}
}
