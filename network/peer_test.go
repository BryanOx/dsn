package network

import (
	"os"
	"sync"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

// setupTestDB creates a temporary BoltDB for persistence tests.
func setupTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "peers-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := bbolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(f.Name())
	})
	return db
}

// TestPeerIDFromBytes verifies PeerID is a SHA-256 hash, deterministic.
func TestPeerIDFromBytes(t *testing.T) {
	data := []byte("test-peer-address:8080")

	// Compute twice to verify determinism
	id1 := PeerIDFromBytes(data)
	id2 := PeerIDFromBytes(data)

	if id1 != id2 {
		t.Fatal("PeerIDFromBytes is not deterministic")
	}

	// Verify it's 32 bytes (SHA-256 output size)
	if len(id1) != 32 {
		t.Fatalf("PeerID length = %d, want 32", len(id1))
	}

	// Verify different inputs produce different IDs
	id3 := PeerIDFromBytes([]byte("different-address:9090"))
	if id1 == id3 {
		t.Fatal("Different inputs should produce different PeerIDs")
	}
}

// TestPeerAddRemove tests AddPeer, GetPeer, RemovePeer.
func TestPeerAddRemove(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-peer-1"))
	addr := "192.168.1.1:8080"

	// Add peer
	peer := pm.AddPeer(peerID, addr)
	if peer == nil {
		t.Fatal("AddPeer returned nil")
	}
	if peer.ID != peerID {
		t.Errorf("Peer ID = %v, want %v", peer.ID, peerID)
	}
	if peer.Address != addr {
		t.Errorf("Peer address = %v, want %v", peer.Address, addr)
	}

	// Get peer
	p := pm.GetPeer(peerID)
	if p == nil {
		t.Fatal("GetPeer returned nil after AddPeer")
	}
	if p != peer {
		t.Error("GetPeer returned different peer instance")
	}

	// Remove peer
	pm.RemovePeer(peerID)

	// GetPeer returns nil after remove
	if pm.GetPeer(peerID) != nil {
		t.Error("GetPeer should return nil after RemovePeer")
	}
}

// TestPeer_ScoreInitial verifies a new peer starts at score 0.
func TestPeer_ScoreInitial(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-peer-score"))
	pm.AddPeer(peerID, "10.0.0.1:9000")

	peer := pm.GetPeer(peerID)
	if peer == nil {
		t.Fatal("Peer not found")
	}

	if peer.Score != 0 {
		t.Errorf("Initial score = %d, want 0", peer.Score)
	}
	if peer.State != PeerDisconnected {
		t.Errorf("Initial state = %v, want %v", peer.State, PeerDisconnected)
	}
}

// TestPeer_ReportSuccess tests scoring for successful interactions.
func TestPeer_ReportSuccess(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-success"))
	pm.AddPeer(peerID, "10.0.0.2:9001")

	// Set state to Connected directly (avoid actual network connection)
	peer := pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Test block success (+1)
	pm.ReportSuccess(peerID, MsgTypeBlock)
	if pm.GetPeer(peerID).Score != 1 {
		t.Errorf("Score after block success = %d, want 1", pm.GetPeer(peerID).Score)
	}

	// Test snapshot chunk success (+2)
	pm.ReportSuccess(peerID, MsgTypeSnapshotChunk)
	if pm.GetPeer(peerID).Score != 3 {
		t.Errorf("Score after snapshot success = %d, want 3", pm.GetPeer(peerID).Score)
	}

	// Test PEX success (+1)
	pm.ReportSuccess(peerID, MsgTypePeerExchange)
	if pm.GetPeer(peerID).Score != 4 {
		t.Errorf("Score after PEX success = %d, want 4", pm.GetPeer(peerID).Score)
	}

	// Clean up state
	peer = pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerDisconnected
	peer.mu.Unlock()
}

// TestPeer_ReportFailure tests scoring for failures.
func TestPeer_ReportFailure(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-failure"))
	pm.AddPeer(peerID, "10.0.0.3:9002")

	// Set state to Connected directly
	peer := pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Test malformed message (-2)
	pm.ReportFailure(peerID, 2)
	if pm.GetPeer(peerID).Score != -2 {
		t.Errorf("Score after malformed = %d, want -2", pm.GetPeer(peerID).Score)
	}

	// Test invalid block/snapshot (-5)
	pm.ReportFailure(peerID, 5)
	if pm.GetPeer(peerID).Score != -7 {
		t.Errorf("Score after invalid = %d, want -7", pm.GetPeer(peerID).Score)
	}

	// Test spam (-10)
	pm.ReportFailure(peerID, 10)
	if pm.GetPeer(peerID).Score != -17 {
		t.Errorf("Score after spam = %d, want -17", pm.GetPeer(peerID).Score)
	}

	// Test timeout (-1)
	pm.ReportFailure(peerID, 1)
	if pm.GetPeer(peerID).Score != -18 {
		t.Errorf("Score after timeout = %d, want -18", pm.GetPeer(peerID).Score)
	}

	// Clean up state
	peer = pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerDisconnected
	peer.mu.Unlock()
}

// TestPeer_ScoreBounds verifies score is clamped to [-100, 100].
func TestPeer_ScoreBounds(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-bounds"))
	pm.AddPeer(peerID, "10.0.0.4:9003")

	// Push score way above 100
	for i := 0; i < 150; i++ {
		pm.ReportSuccess(peerID, MsgTypeBlock)
	}
	if pm.GetPeer(peerID).Score > 100 {
		t.Errorf("Score = %d, should be clamped to 100", pm.GetPeer(peerID).Score)
	}

	// Push score way below -100
	pm.UpdateScore(peerID, -200)
	if pm.GetPeer(peerID).Score < -100 {
		t.Errorf("Score = %d, should be clamped to -100", pm.GetPeer(peerID).Score)
	}
}

// TestPeer_DailyCap verifies block successes are capped at +10 per day.
func TestPeer_DailyCap(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-daily-cap"))
	pm.AddPeer(peerID, "10.0.0.5:9004")

	// Set state to Connected directly
	peer := pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Report 15 block successes
	for i := 0; i < 15; i++ {
		pm.ReportSuccess(peerID, MsgTypeBlock)
	}

	// Score should be capped at +10
	score := pm.GetPeer(peerID).Score
	if score > 10 {
		t.Errorf("Score = %d, should be capped at 10 after 15 block successes", score)
	}

	// Clean up state
	peer = pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerDisconnected
	peer.mu.Unlock()
}

// TestPeer_AutoBan verifies score < -20 triggers auto-ban for 5 minutes.
func TestPeer_AutoBan(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-auto-ban"))
	pm.AddPeer(peerID, "10.0.0.6:9005")

	// Set state to Connected directly
	peer := pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Report failures until score drops below -20
	// -1 per failure, need 21 failures
	for i := 0; i < 21; i++ {
		pm.ReportFailure(peerID, 1)
	}

	peer = pm.GetPeer(peerID)
	if peer.State != PeerBanned {
		t.Errorf("State = %v, want %v", peer.State, PeerBanned)
	}

	// Verify ban duration is approximately 5 minutes
	if peer.BanExpiry.IsZero() {
		t.Error("BanExpiry should not be zero")
	}

	expectedExpiry := time.Now().Add(5 * time.Minute)
	diff := peer.BanExpiry.Sub(expectedExpiry)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("Ban expiry not approximately 5 minutes: got %v", peer.BanExpiry)
	}

	// Verify connection is closed
	if peer.Conn != nil {
		t.Error("Conn should be nil after auto-ban")
	}
}

// TestPeer_BanExpiry verifies that after ban duration, state returns to Disconnected.
func TestPeer_BanExpiry(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-ban-expiry"))
	pm.AddPeer(peerID, "10.0.0.7:9006")

	// Manually set a ban that expired in the past
	peer := pm.GetPeer(peerID)
	peer.mu.Lock()
	peer.State = PeerBanned
	peer.BanExpiry = time.Now().Add(-1 * time.Minute) // Already expired
	peer.mu.Unlock()

	// Trigger cleanup
	pm.cleanupExpiredBans()

	// Verify state returned to Disconnected
	peer = pm.GetPeer(peerID)
	if peer.State != PeerDisconnected {
		t.Errorf("State = %v, want %v after ban expiry", peer.State, PeerDisconnected)
	}

	// Verify BanExpiry is cleared
	if !peer.BanExpiry.IsZero() {
		t.Error("BanExpiry should be zero after expiry cleanup")
	}
}

// TestPeer_GetBestPeers verifies it returns top-n by score, only connected.
func TestPeer_GetBestPeers(t *testing.T) {
	pm := NewPeerManager(nil)

	// Add multiple peers with different scores
	peers := []struct {
		id    PeerID
		addr  string
		score int
		state PeerState
	}{
		{PeerIDFromBytes([]byte("peer-low")), "10.0.1.1:9001", 10, PeerConnected},
		{PeerIDFromBytes([]byte("peer-mid")), "10.0.1.2:9002", 50, PeerConnected},
		{PeerIDFromBytes([]byte("peer-high")), "10.0.1.3:9003", 100, PeerConnected},
		{PeerIDFromBytes([]byte("peer-disconnected")), "10.0.1.4:9004", 100, PeerDisconnected},
		{PeerIDFromBytes([]byte("peer-negative")), "10.0.1.5:9005", -5, PeerConnected},
	}

	for _, p := range peers {
		pm.AddPeer(p.id, p.addr)
		peer := pm.GetPeer(p.id)
		peer.mu.Lock()
		peer.State = p.state
		peer.mu.Unlock()
		pm.UpdateScore(p.id, p.score)
	}

	// Get top 2
	best := pm.GetBestPeers(2)
	if len(best) != 2 {
		t.Fatalf("GetBestPeers(2) returned %d peers, want 2", len(best))
	}

	// Verify they're the highest scores
	if best[0].Score != 100 || best[1].Score != 50 {
		t.Errorf("Best peers scores = [%d, %d], want [100, 50]", best[0].Score, best[1].Score)
	}

	// Disconnected peer should not be included
	for _, p := range best {
		if p.State != PeerConnected {
			t.Errorf("Best peer in state %v, should be Connected", p.State)
		}
	}

	// Negative score peer should not be included
	if len(best) > 0 && best[len(best)-1].Score <= 0 {
		t.Error("GetBestPeers should not include peers with score <= 0")
	}
}

// TestPeer_ConcurrentAccess tests thread safety with concurrent access.
func TestPeer_ConcurrentAccess(t *testing.T) {
	pm := NewPeerManager(nil)

	peerID := PeerIDFromBytes([]byte("test-concurrent"))
	pm.AddPeer(peerID, "10.0.2.1:9001")

	var wg sync.WaitGroup

	// Concurrent successes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pm.ReportSuccess(peerID, MsgTypeBlock)
		}()
	}

	// Concurrent failures
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pm.ReportFailure(peerID, 1)
		}()
	}

	wg.Wait()

	// Verify score is within expected range (some successes, some failures)
	score := pm.GetPeer(peerID).Score
	// 10 successes at +1 each = +10, but capped at 10 due to daily cap
	// 10 failures at -1 each = -10
	// Net should be around 0-10
	if score < -20 || score > 20 {
		t.Errorf("Score = %d, out of reasonable range after concurrent access", score)
	}
}

// TestPeer_PersistLoad tests persistence round-trip.
func TestPeer_PersistLoad(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create manager and add peers with various states
	pm1 := NewPeerManager(db)

	peer1ID := PeerIDFromBytes([]byte("persist-peer-1"))
	peer2ID := PeerIDFromBytes([]byte("persist-peer-2"))
	peer3ID := PeerIDFromBytes([]byte("persist-peer-3"))

	pm1.AddPeer(peer1ID, "192.168.1.1:8000")
	pm1.AddPeer(peer2ID, "192.168.1.2:8001")
	pm1.AddPeer(peer3ID, "192.168.1.3:8002")

	// Set various scores and states directly
	pm1.UpdateScore(peer1ID, 50)
	pm1.UpdateScore(peer2ID, -30)
	pm1.UpdateScore(peer3ID, 75)

	p1 := pm1.GetPeer(peer1ID)
	p1.mu.Lock()
	p1.State = PeerConnected
	p1.mu.Unlock()

	p2 := pm1.GetPeer(peer2ID)
	p2.mu.Lock()
	p2.State = PeerConnected
	p2.mu.Unlock()

	// Persist
	if err := pm1.Persist(); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Create new manager and load
	pm2 := NewPeerManager(db)
	if err := pm2.LoadPeers(); err != nil {
		t.Fatalf("LoadPeers failed: %v", err)
	}

	// Verify all peers loaded
	if pm2.NumPeers() != 3 {
		t.Errorf("NumPeers = %d, want 3", pm2.NumPeers())
	}

	// Verify peer 1
	p1 = pm2.GetPeer(peer1ID)
	if p1 == nil {
		t.Fatal("Peer 1 not found after load")
	}
	if p1.Address != "192.168.1.1:8000" {
		t.Errorf("Peer 1 address = %v, want 192.168.1.1:8000", p1.Address)
	}
	if p1.Score != 50 {
		t.Errorf("Peer 1 score = %d, want 50", p1.Score)
	}
	if p1.State != PeerConnected {
		t.Errorf("Peer 1 state = %v, want %v", p1.State, PeerConnected)
	}

	// Verify peer 2 (was auto-banned due to score < -20)
	p2 = pm2.GetPeer(peer2ID)
	if p2 == nil {
		t.Fatal("Peer 2 not found after load")
	}
	if p2.Score != -30 {
		t.Errorf("Peer 2 score = %d, want -30", p2.Score)
	}
	// Note: State might be PeerBanned because UpdateScore auto-bans at < -20

	// Verify peer 3
	p3 := pm2.GetPeer(peer3ID)
	if p3 == nil {
		t.Fatal("Peer 3 not found after load")
	}
	if p3.Address != "192.168.1.3:8002" {
		t.Errorf("Peer 3 address = %v, want 192.168.1.3:8002", p3.Address)
	}
	if p3.Score != 75 {
		t.Errorf("Peer 3 score = %d, want 75", p3.Score)
	}
}

// TestPeer_PersistEmpty tests persistence with no peers.
func TestPeer_PersistEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create manager with no peers
	pm1 := NewPeerManager(db)

	// Persist empty
	if err := pm1.Persist(); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Create new manager and load
	pm2 := NewPeerManager(db)
	if err := pm2.LoadPeers(); err != nil {
		t.Fatalf("LoadPeers failed: %v", err)
	}

	// Verify no peers
	if pm2.NumPeers() != 0 {
		t.Errorf("NumPeers = %d, want 0", pm2.NumPeers())
	}
}

// TestPeer_PersistenceOverwrite tests persistence with updated scores.
func TestPeer_PersistenceOverwrite(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	pm := NewPeerManager(db)

	peerID := PeerIDFromBytes([]byte("persist-overwrite"))
	pm.AddPeer(peerID, "10.5.5.5:5000")

	// First persist with score 10
	pm.UpdateScore(peerID, 10)
	if err := pm.Persist(); err != nil {
		t.Fatalf("First Persist failed: %v", err)
	}

	// Update score to 30
	pm.UpdateScore(peerID, 20)

	// Second persist
	if err := pm.Persist(); err != nil {
		t.Fatalf("Second Persist failed: %v", err)
	}

	// Create new manager and load
	pm2 := NewPeerManager(db)
	if err := pm2.LoadPeers(); err != nil {
		t.Fatalf("LoadPeers failed: %v", err)
	}

	// Verify latest score is loaded
	p := pm2.GetPeer(peerID)
	if p == nil {
		t.Fatal("Peer not found after load")
	}
	if p.Score != 30 {
		t.Errorf("Loaded score = %d, want 30 (latest value)", p.Score)
	}
}
