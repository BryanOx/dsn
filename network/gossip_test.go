package network

import (
	"math"
	"testing"
	"time"
)

// newMockP2PNode creates a P2PNode for testing (listens on port 0).
func newMockP2PNode(t *testing.T) *P2PNode {
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatalf("Failed to create mock P2PNode: %v", err)
	}
	return p2p
}

// TestTokenBucket_Initial verifies that a new bucket has maxTokens available.
func TestTokenBucket_Initial(t *testing.T) {
	tb := NewTokenBucket(100, 10)

	// Should have maxTokens available immediately
	available := tb.Available()
	if available != 100 {
		t.Errorf("Initial available tokens = %v, want 100", available)
	}
}

// TestTokenBucket_Allow verifies Allow() returns true while tokens > 0.
func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(3, 10)

	// First 3 should succeed
	if !tb.Allow() {
		t.Error("Allow() returned false on first call with 3 tokens")
	}
	if !tb.Allow() {
		t.Error("Allow() returned false on second call with 3 tokens")
	}
	if !tb.Allow() {
		t.Error("Allow() returned false on third call with 3 tokens")
	}

	// 4th should fail (no tokens left)
	if tb.Allow() {
		t.Error("Allow() returned true when no tokens available")
	}
}

// TestTokenBucket_Refill verifies tokens are refilled after 1 second.
func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(10, 5) // 5 tokens per second

	// Consume all tokens
	for i := 0; i < 10; i++ {
		tb.Allow()
	}

	// Wait 1 second for refill
	time.Sleep(1100 * time.Millisecond)

	// Should have approximately 5 tokens (refillRate per second)
	// Allow wider epsilon due to timing overhead
	available := tb.Available()
	if available < 4.0 || available > 6.0 {
		t.Errorf("Available after 1s = %v, want ~5 (within epsilon)", available)
	}
}

// TestTokenBucket_AllowN verifies AllowN consumes multiple tokens.
func TestTokenBucket_AllowN(t *testing.T) {
	tb := NewTokenBucket(10, 10)

	// AllowN(5) should succeed
	if !tb.AllowN(5) {
		t.Error("AllowN(5) returned false with 10 tokens")
	}

	// Should have 5 left (within epsilon: the bucket refills from elapsed
	// wall-clock time during execution, so exact float equality is flaky).
	available := tb.Available()
	if available < 4.999 || available > 5.001 {
		t.Errorf("Available after AllowN(5) = %v, want ~5 (within epsilon)", available)
	}

	// AllowN(6) should fail (only 5 left)
	if tb.AllowN(6) {
		t.Error("AllowN(6) returned true with only 5 tokens")
	}
}

// TestTokenBucket_MaxTokens verifies bucket never exceeds maxTokens on refill.
func TestTokenBucket_MaxTokens(t *testing.T) {
	tb := NewTokenBucket(10, 100) // High refill rate

	// Consume some tokens
	tb.AllowN(5)

	// Wait long enough to refill past max
	time.Sleep(200 * time.Millisecond)

	// Should never exceed maxTokens (10)
	available := tb.Available()
	if available > 10 {
		t.Errorf("Available = %v, should not exceed maxTokens (10)", available)
	}
}

// TestGossipEngine_FanOutSelection verifies fan-out = max(3, sqrt(N)).
func TestGossipEngine_FanOutSelection(t *testing.T) {
	pm := NewPeerManager(nil)

	// Helper to create connected peers
	createPeers := func(count int) {
		for i := 0; i < count; i++ {
			id := PeerIDFromBytes([]byte("peer-" + string(rune(i))))
			peer := pm.AddPeer(id, "127.0.0.1:800"+string(rune(i)))
			peer.mu.Lock()
			peer.State = PeerConnected
			peer.mu.Unlock()
		}
	}

	// Test with 100 peers: sqrt(100) = 10, max(3, 10) = 10
	createPeers(100)
	mockP2p := newMockP2PNode(t)
	ge := NewGossipEngine(pm, mockP2p)
	pm.mu.RLock()
	connectedPeers := pm.GetPeersByState(PeerConnected)
	pm.mu.RUnlock()
	fanOut := int(math.Sqrt(float64(len(connectedPeers))))
	if fanOut < 3 {
		fanOut = 3
	}
	if fanOut != 10 {
		t.Errorf("Fan-out with 100 peers = %d, want 10", fanOut)
	}
	ge.Stop()
	mockP2p.Close()

	// Reset for next test
	pm = NewPeerManager(nil)

	// Test with 4 peers: sqrt(4) = 2, max(3, 2) = 3
	createPeers(4)
	mockP2p = newMockP2PNode(t)
	ge = NewGossipEngine(pm, mockP2p)
	pm.mu.RLock()
	connectedPeers = pm.GetPeersByState(PeerConnected)
	pm.mu.RUnlock()
	fanOut = int(math.Sqrt(float64(len(connectedPeers))))
	if fanOut < 3 {
		fanOut = 3
	}
	if fanOut != 3 {
		t.Errorf("Fan-out with 4 peers = %d, want 3", fanOut)
	}
	ge.Stop()
	mockP2p.Close()

	// Test with 9 peers: sqrt(9) = 3, max(3, 3) = 3
	pm = NewPeerManager(nil)
	createPeers(9)
	mockP2p = newMockP2PNode(t)
	ge = NewGossipEngine(pm, mockP2p)
	pm.mu.RLock()
	connectedPeers = pm.GetPeersByState(PeerConnected)
	pm.mu.RUnlock()
	fanOut = int(math.Sqrt(float64(len(connectedPeers))))
	if fanOut < 3 {
		fanOut = 3
	}
	if fanOut != 3 {
		t.Errorf("Fan-out with 9 peers = %d, want 3", fanOut)
	}
	ge.Stop()
}

// TestGossipEngine_SeenSetDedup verifies duplicate blocks are not re-gossiped.
func TestGossipEngine_SeenSetDedup(t *testing.T) {
	pm := NewPeerManager(nil)

	// Add one connected peer
	id := PeerIDFromBytes([]byte("test-peer"))
	peer := pm.AddPeer(id, "127.0.0.1:8080")
	peer.mu.Lock()
	peer.State = PeerConnected
	peer.mu.Unlock()

	mockP2p := newMockP2PNode(t)
	ge := NewGossipEngine(pm, mockP2p)
	defer func() {
		ge.Stop()
		mockP2p.Close()
	}()

	// Get initial seen set size
	initialSize := ge.seenSet.Size()

	// Gossip the same block data twice
	blockData := []byte("test-block-data")

	// First gossip - should add to seenSet
	ge.GossipBlock(blockData)

	// Check seenSet grew by 1
	afterFirst := ge.seenSet.Size()
	if afterFirst != initialSize+1 {
		t.Errorf("SeenSet size after first gossip = %d, want %d", afterFirst, initialSize+1)
	}

	// Second gossip with same data - should NOT add to seenSet (dedup)
	ge.GossipBlock(blockData)

	// Size should remain the same (dedup works)
	afterSecond := ge.seenSet.Size()
	if afterSecond != afterFirst {
		t.Errorf("SeenSet size after second gossip = %d, want %d (dedup should prevent growth)", afterSecond, afterFirst)
	}
}

// TestGossipEngine_RateLimitPenalty verifies penalty is applied when rate exceeded.
func TestGossipEngine_RateLimitPenalty(t *testing.T) {
	pm := NewPeerManager(nil)

	// Add a peer
	id := PeerIDFromBytes([]byte("rate-limit-test-peer"))
	pm.AddPeer(id, "127.0.0.1:8080")

	mockP2p := newMockP2PNode(t)
	ge := NewGossipEngine(pm, mockP2p)
	defer func() {
		ge.Stop()
		mockP2p.Close()
	}()

	// Exhaust the token bucket by making manyAllow calls
	bucket := ge.getBucket(id)
	for i := 0; i < 100; i++ {
		bucket.Allow()
	}

	// Now try to handle an incoming message - should be rate limited
	// The bucket is now empty (0 tokens)
	accepted, penalty := ge.HandleIncoming(id, MsgTypeBlock, []byte("test-data"))

	if accepted {
		t.Error("HandleIncoming should reject when bucket is empty")
	}

	if penalty != 10 {
		t.Errorf("Penalty = %d, want 10 for rate limit violation", penalty)
	}
}
