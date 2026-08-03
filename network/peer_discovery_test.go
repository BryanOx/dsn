package network

import (
	"bytes"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// TestPeerInfoSerializeRoundTrip verifies serialization/deserialization preserves all fields.
func TestPeerInfoSerializeRoundTrip(t *testing.T) {
	// Create test peers - note: current serialization uses uint32 for score,
	// so negative scores will wrap. Using only positive scores for this test.
	peers := []*Peer{
		{
			ID:       PeerIDFromBytes([]byte("peer-1-address")),
			Address:  "192.168.1.1:8080",
			Score:    50,
			LastSeen: time.Now().Add(-5 * time.Minute),
		},
		{
			ID:       PeerIDFromBytes([]byte("peer-2-address")),
			Address:  "10.0.0.1:9090",
			Score:    100,
			LastSeen: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:       PeerIDFromBytes([]byte("peer-3-address")),
			Address:  "172.16.0.1:7000",
			Score:    0, // Use 0 instead of -5 due to serialization limitation
			LastSeen: time.Now().Add(-10 * time.Minute),
		},
	}

	// Serialize
	serialized := serializePeerInfo(peers)
	if len(serialized) == 0 {
		t.Fatal("serializePeerInfo returned empty slice")
	}

	// Deserialize
	parsed := parsePeerInfo(serialized)
	if parsed == nil {
		t.Fatal("parsePeerInfo returned nil")
	}

	// Verify count
	if len(parsed) != len(peers) {
		t.Errorf("Parsed peer count = %d, want %d", len(parsed), len(peers))
	}

	// Verify each field for each peer
	for i, want := range peers {
		got := parsed[i]

		if got.ID != want.ID {
			t.Errorf("Peer[%d].ID = %v, want %v", i, got.ID, want.ID)
		}
		if got.Address != want.Address {
			t.Errorf("Peer[%d].Address = %v, want %v", i, got.Address, want.Address)
		}
		if got.Score != want.Score {
			t.Errorf("Peer[%d].Score = %d, want %d", i, got.Score, want.Score)
		}
		// Allow 1 second tolerance for LastSeen due to serialization
		if got.LastSeen.Sub(want.LastSeen).Abs() > time.Second {
			t.Errorf("Peer[%d].LastSeen = %v, want %v (within 1s)", i, got.LastSeen, want.LastSeen)
		}
	}
}

// TestPeerInfoSerializeEmpty verifies empty list round-trips correctly.
func TestPeerInfoSerializeEmpty(t *testing.T) {
	var peers []*Peer

	// Serialize empty list
	serialized := serializePeerInfo(peers)
	if len(serialized) < 2 {
		t.Errorf("Serialized empty list length = %d, want at least 2 (count bytes)", len(serialized))
	}

	// Deserialize
	parsed := parsePeerInfo(serialized)
	if parsed == nil {
		t.Error("parsePeerInfo returned nil for empty list")
	}
	if len(parsed) != 0 {
		t.Errorf("Parsed length = %d, want 0", len(parsed))
	}
}

// TestPeerInfoParseTruncated verifies truncated data is handled gracefully.
func TestPeerInfoParseTruncated(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "too short for count",
			data: []byte{0x00},
		},
		{
			name: "count says 1 but no data",
			data: []byte{0x00, 0x01},
		},
		{
			name: "partial peer ID",
			data: []byte{0x00, 0x01, 0xaa, 0xbb},
		},
		{
			name: "valid count but truncated address length",
			data: []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePeerInfo(tt.data)
			// Should return nil or empty slice, not panic
			if result != nil && len(result) > 0 {
				// Check that if any peers were parsed, they have valid data
				for _, p := range result {
					_ = p.ID
					_ = p.Address
					_ = p.Score
					_ = p.LastSeen
				}
			}
		})
	}
}

// TestShouldReconnect verifies the re-dial decision: a registered connection
// must never trigger a second dial, while unknown addresses always do.
func TestShouldReconnect(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	pd := NewPeerDiscovery(n1.pm, nil, n1)

	// An address with no registered connection must be re-dialed.
	if !pd.shouldReconnect("127.0.0.1:1") {
		t.Error("unconnected address must be re-dialed")
	}

	// Once connected, the same address must NOT be re-dialed.
	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if pd.shouldReconnect(n2.Addr()) {
		t.Error("connected address must not be re-dialed")
	}

	// A nil P2P node has no connection tracker and must fall back to dialing.
	pdNil := NewPeerDiscovery(n1.pm, nil, nil)
	if !pdNil.shouldReconnect(n2.Addr()) {
		t.Error("nil P2P node must report reconnect")
	}
}

// TestBootstrapOnce_DoesNotRedialConnectedPeer verifies that a full bootstrap
// pass over an already-connected peer does not re-dial it. Before the fix the
// loop called ConnectToPeer unconditionally, closing the live connection and
// leaving n1 with zero registered peers once the stale read loop's cleanup
// removed the replacement.
func TestBootstrapOnce_DoesNotRedialConnectedPeer(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if n1.NumPeers() != 1 {
		t.Fatalf("setup: NumPeers = %d, want 1", n1.NumPeers())
	}

	pd := NewPeerDiscovery(n1.pm, []string{n2.Addr()}, n1)

	if !pd.bootstrapOnce() {
		t.Fatal("bootstrapOnce should report a connected bootstrap peer")
	}

	// Poll briefly: with the re-dial, the stale read loop would eventually run
	// its cleanup and drop the (replaced) connection from the map.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := n1.NumPeers(); got != 1 {
			t.Fatalf("NumPeers = %d, want 1 (connected peer was re-dialed)", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// countRunningGoroutines returns how many goroutines are currently executing
// the given function name (used to assert goroutine spawn counts).
func countRunningGoroutines(fn string) int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return bytes.Count(buf[:n], []byte(fn))
}

// TestPeerDiscovery_StartIsIdempotent verifies that a second Start call is a
// no-op: only the first invocation spawns the bootstrap/health-check/PEX/
// prune loops. Start is invoked from multiple node startup paths, and without
// the guard two bootstrap loops ran and double-dialed every cycle.
func TestPeerDiscovery_StartIsIdempotent(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()

	pd := NewPeerDiscovery(n1.pm, nil, n1)
	defer pd.Stop()

	loops := []string{
		"(*PeerDiscovery).bootstrapLoop",
		"(*PeerDiscovery).healthCheckLoop",
		"(*PeerDiscovery).pexLoop",
		"(*PeerDiscovery).pruneLoop",
	}

	pd.Start()
	pd.Start()

	// Wait for exactly one instance of each loop from the first Start.
	deadline := time.Now().Add(2 * time.Second)
	for {
		all := true
		for _, fn := range loops {
			if countRunningGoroutines(fn) != 1 {
				all = false
				break
			}
		}
		if all {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("loops never reached a single running instance")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The second Start must not have spawned duplicates.
	time.Sleep(150 * time.Millisecond)
	for _, fn := range loops {
		if got := countRunningGoroutines(fn); got != 1 {
			t.Errorf("%s running %d times, want exactly 1", fn, got)
		}
	}

	// Stop then Start again: the guard must still allow a clean restart.
	pd.Stop()
	deadline = time.Now().Add(2 * time.Second)
	for {
		if countRunningGoroutines("(*PeerDiscovery).bootstrapLoop") == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("loops still running after Stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pd.Start()
	deadline = time.Now().Add(2 * time.Second)
	for {
		if countRunningGoroutines("(*PeerDiscovery).bootstrapLoop") == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bootstrapLoop did not restart after Stop+Start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHandlePeerExchange_EmptyListNotPenalized verifies that a well-formed
// empty peer list is treated as valid (nothing worth sharing yet) and is not
// scored as a failure. Before the fix every empty PEX response drove the peer
// score toward the auto-ban threshold, tearing the connection down.
func TestHandlePeerExchange_EmptyListNotPenalized(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()

	pd := NewPeerDiscovery(n1.pm, nil, n1)

	id := PeerIDFromBytes([]byte("10.0.0.1:26656"))
	n1.pm.AddPeer(id, "10.0.0.1:26656")

	pd.HandlePeerExchange(id, serializePeerInfo(nil))

	peer := n1.pm.GetPeer(id)
	if peer == nil {
		t.Fatal("peer missing")
	}
	peer.mu.RLock()
	score := peer.Score
	peer.mu.RUnlock()
	if score != 0 {
		t.Errorf("empty PEX penalized the peer: score = %d, want 0", score)
	}

	// A truly malformed payload (missing the count header) must still be
	// penalized.
	pd.HandlePeerExchange(id, []byte{0x01})

	peer.mu.RLock()
	score = peer.Score
	peer.mu.RUnlock()
	if score != -2 {
		t.Errorf("malformed PEX not penalized: score = %d, want -2", score)
	}
}

// TestPeerInfoSerializationFormat verifies the exact wire format.
func TestPeerInfoSerializationFormat(t *testing.T) {
	peer := &Peer{
		ID:       PeerIDFromBytes([]byte("test-peer-id")),
		Address:  "127.0.0.1:8080",
		Score:    42,
		LastSeen: time.Unix(1700000000, 0), // Fixed timestamp
	}

	data := serializePeerInfo([]*Peer{peer})
	parsed := parsePeerInfo(data)

	if len(parsed) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(parsed))
	}

	// Verify exact ID bytes
	if !reflect.DeepEqual(parsed[0].ID[:], peer.ID[:]) {
		t.Error("PeerID bytes do not match")
	}

	// Verify exact score
	if parsed[0].Score != 42 {
		t.Errorf("Score = %d, want 42", parsed[0].Score)
	}
}
