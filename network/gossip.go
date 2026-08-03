package network

import (
	"crypto/rand"
	"math"
	"sync"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/types"
)

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // per second
	lastRefill time.Time
}

// NewTokenBucket creates a new TokenBucket with the specified capacity and refill rate.
func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// refill adds tokens based on elapsed time since last refill.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	tb.tokens = math.Min(tb.maxTokens, tb.tokens+tb.refillRate*elapsed.Seconds())
	tb.lastRefill = now
}

// Allow consumes 1 token and returns true if available.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}

// AllowN consumes n tokens and returns true if all are available.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// Available returns the current number of available tokens without consuming.
func (tb *TokenBucket) Available() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// GossipEngine handles block and transaction gossip with fan-out and rate limiting.
type GossipEngine struct {
	pm      *PeerManager
	seenSet *consensus.SeenSet
	p2p     *P2PNode
	buckets map[PeerID]*TokenBucket
	mu      sync.RWMutex
	stopCh  chan struct{}

	// Rate limiter settings
	burstTokens float64
	refillRate  float64

	// Hasher for computing hashes
	hasher types.Hasher

	// Random selection helper
	randMu sync.Mutex
}

// NewGossipEngine creates a new GossipEngine with the given PeerManager and P2PNode.
func NewGossipEngine(pm *PeerManager, p2p *P2PNode) *GossipEngine {
	// Use combined capacity: 10,000 blocks + 100,000 transactions = 110,000
	// We'll use a single SeenSet with max = 110000
	seenSet := consensus.NewSeenSet(110000)

	ge := &GossipEngine{
		pm:          pm,
		seenSet:     seenSet,
		p2p:         p2p,
		buckets:     make(map[PeerID]*TokenBucket),
		stopCh:      make(chan struct{}),
		burstTokens: 100,
		refillRate:  10, // tokens per second
		hasher:      types.SHA256Hasher{},
	}

	// Start background cleanup goroutine
	go ge.cleanupLoop()

	return ge
}

// cleanupLoop periodically removes stale rate limiter buckets for disconnected peers.
func (ge *GossipEngine) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ge.stopCh:
			return
		case <-ticker.C:
			ge.cleanupStaleBuckets()
		}
	}
}

// cleanupStaleBuckets removes rate limiter buckets for peers that are no longer connected.
func (ge *GossipEngine) cleanupStaleBuckets() {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	connectedPeers := ge.pm.GetPeersByState(PeerConnected)
	connectedIDs := make(map[PeerID]bool)
	for _, p := range connectedPeers {
		connectedIDs[p.ID] = true
	}

	for id := range ge.buckets {
		if !connectedIDs[id] {
			delete(ge.buckets, id)
		}
	}
}

// getBucket returns the rate limiter bucket for a peer, creating one if needed.
func (ge *GossipEngine) getBucket(peerID PeerID) *TokenBucket {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	if bucket, ok := ge.buckets[peerID]; ok {
		return bucket
	}

	bucket := NewTokenBucket(ge.burstTokens, ge.refillRate)
	ge.buckets[peerID] = bucket
	return bucket
}

// computeHash computes the SHA256 hash of the given data.
func (ge *GossipEngine) computeHash(data []byte) types.Hash {
	h, _ := ge.hasher.Hash(data)
	return h
}

// selectPeers selects n random peers from the given list using Fisher-Yates shuffle.
// Uses crypto/rand for secure randomness.
func (ge *GossipEngine) selectPeers(peers []*Peer, n int) []*Peer {
	if len(peers) == 0 {
		return nil
	}

	if n >= len(peers) {
		result := make([]*Peer, len(peers))
		copy(result, peers)
		return result
	}

	// Convert slice to array for Fisher-Yates
	selected := make([]*Peer, n)
	indices := make([]int, len(peers))
	for i := range indices {
		indices[i] = i
	}

	// Fisher-Yates shuffle with crypto/rand
	ge.randMu.Lock()
	defer ge.randMu.Unlock()

	for i := 0; i < n; i++ {
		// Choose random index from i to len-1
		var randIdx int
		rand.Read([]byte{byte(randIdx >> 24), byte(randIdx >> 16), byte(randIdx >> 8), byte(randIdx)})
		// Use bits from rand.Read to pick random index
		r := make([]byte, 4)
		rand.Read(r)
		randIdx = int(r[0])<<24 | int(r[1])<<16 | int(r[2])<<8 | int(r[3])
		randIdx = i + randIdx%(len(indices)-i)

		selected[i] = peers[indices[randIdx]]
		indices[i], indices[randIdx] = indices[randIdx], indices[i]
	}

	return selected
}

// GossipBlock fans out a block to a subset of connected peers.
// Fan-out count = max(3, sqrt(N)) where N is the number of connected peers.
// The data must be a fully-framed block message (consensus.EncodeBlockMessage
// output, i.e. type byte 0x01 + payload). SendTo adds only the transport
// length prefix — framing here again would produce a double length prefix
// that receivers misparse as a legacy transaction.
func (ge *GossipEngine) GossipBlock(data []byte) {
	// Compute block hash for dedup
	blockHash := ge.computeHash(data)

	// Check dedup
	ge.mu.Lock()
	if ge.seenSet.Seen(blockHash) {
		ge.mu.Unlock()
		return
	}
	ge.seenSet.Mark(blockHash)
	ge.mu.Unlock()

	// Get connected peers
	connectedPeers := ge.pm.GetPeersByState(PeerConnected)
	if len(connectedPeers) == 0 {
		return
	}

	// Calculate fan-out count: max(3, sqrt(N))
	fanOut := int(math.Sqrt(float64(len(connectedPeers))))
	if fanOut < 3 {
		fanOut = 3
	}

	// Select random peers for fan-out
	selectedPeers := ge.selectPeers(connectedPeers, fanOut)

	// Send to selected peers, checking rate limit
	for _, peer := range selectedPeers {
		bucket := ge.getBucket(peer.ID)
		if !bucket.Allow() {
			// Rate limited, skip this peer
			continue
		}

		// Send to peer using P2PNode (adds the transport length prefix)
		if err := ge.p2p.SendTo(peer.Address, data); err != nil {
			// Log error or handle peer failure
			_ = err
		}
	}
}

// GossipTransaction broadcasts a transaction to all connected peers.
func (ge *GossipEngine) GossipTransaction(data []byte) {
	// Compute transaction hash for dedup
	txHash := ge.computeHash(data)

	// Check dedup
	ge.mu.Lock()
	if ge.seenSet.Seen(txHash) {
		ge.mu.Unlock()
		return
	}
	ge.seenSet.Mark(txHash)
	ge.mu.Unlock()

	// Get connected peers
	connectedPeers := ge.pm.GetPeersByState(PeerConnected)
	if len(connectedPeers) == 0 {
		return
	}

	// Send to all connected peers, checking rate limit
	for _, peer := range connectedPeers {
		bucket := ge.getBucket(peer.ID)
		if !bucket.Allow() {
			// Rate limited, skip this peer for this round
			continue
		}

		// Single-framed: SendTo adds the transport length prefix, so pass
		// [type][payload]. FrameMessage would add a second length prefix and
		// the read loop would parse the inner one as the type byte (0x00),
		// fall into the legacy transaction branch and drop the message.
		msg := make([]byte, 1+len(data))
		msg[0] = MsgTypeTransaction
		copy(msg[1:], data)

		if err := ge.p2p.SendTo(peer.Address, msg); err != nil {
			_ = err
		}
	}
}

// HandleIncoming processes an incoming gossip message from a peer.
// Returns whether the message was accepted and any penalty to apply.
func (ge *GossipEngine) HandleIncoming(peerID PeerID, msgType byte, data []byte) (accepted bool, penalty int) {
	// Check rate limiter for this peer
	bucket := ge.getBucket(peerID)
	if !bucket.Allow() {
		// Rate limited - reject and apply penalty
		return false, 10
	}

	// For block messages (0x01), check dedup
	if msgType == MsgTypeBlock {
		hash := ge.computeHash(data)
		ge.mu.Lock()
		if ge.seenSet.Seen(hash) {
			ge.mu.Unlock()
			return false, 0 // Already seen, but not a penalty
		}
		ge.seenSet.Mark(hash)
		ge.mu.Unlock()
	}

	return true, 0
}

// Stop shuts down the gossip engine.
func (ge *GossipEngine) Stop() {
	close(ge.stopCh)
}

// Shutdown is an alias for Stop for clarity.
func (ge *GossipEngine) Shutdown() {
	ge.Stop()
}
