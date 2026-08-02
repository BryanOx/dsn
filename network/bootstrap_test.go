package network

import (
	"encoding/json"
	"testing"
	"time"
)

// TestBootstrap_Reconnection tests that the discovery mechanism
// handles bootstrap peer disconnection and reconnection.
func TestBootstrap_Reconnection(t *testing.T) {
	t.Skip("integration: requires network setup")

	// This test would:
	// 1. Create a Discovery with bootstrap peers
	// 2. Start discovery
	// 3. Simulate bootstrap peer disconnect
	// 4. Verify reconnection attempt with backoff
	// 5. "Reconnect" the peer
	// 6. Verify discovery re-establishes connection
	//
	// Implementation would require mock P2P node
}

// TestBootstrap_ExponentialBackoff tests that reconnection uses
// exponential backoff.
func TestBootstrap_ExponentialBackoff(t *testing.T) {
	// Test that the backoff schedule is properly exponential
	d := NewDiscovery(nil, nil, []string{})

	// Backoff schedule should be:
	// [1s, 2s, 4s, 8s, 30s] or similar exponential pattern

	if len(d.backoffSchedule) < 2 {
		t.Fatal("backoff schedule too short")
	}

	// Verify increasing delays
	for i := 1; i < len(d.backoffSchedule); i++ {
		if d.backoffSchedule[i] < d.backoffSchedule[i-1] {
			t.Errorf("backoff should increase: %v vs %v",
				d.backoffSchedule[i], d.backoffSchedule[i-1])
		}
	}

	// Verify first backoff is reasonable (not too long)
	if d.backoffSchedule[0] > 10*time.Second {
		t.Errorf("first backoff too long: %v", d.backoffSchedule[0])
	}
}

// TestBootstrap_MultiplePeers tests handling of multiple bootstrap peers.
func TestBootstrap_MultiplePeers(t *testing.T) {
	d := NewDiscovery(nil, nil, []string{
		"/ip4/127.0.0.1/tcp/1001/p2p/QmPeer1",
		"/ip4/127.0.0.1/tcp/1002/p2p/QmPeer2",
		"/ip4/127.0.0.1/tcp/1003/p2p/QmPeer3",
	})

	if len(d.bootstrapPeers) != 3 {
		t.Errorf("expected 3 bootstrap peers, got %d", len(d.bootstrapPeers))
	}

	// All peers should initially be not connected
	for _, peer := range d.bootstrapPeers {
		if d.IsConnected(peer) {
			t.Errorf("peer %s should not be connected initially", peer)
		}
	}
}

// TestBootstrap_EmptyPeerList tests discovery with no bootstrap peers.
func TestBootstrap_EmptyPeerList(t *testing.T) {
	d := NewDiscovery(nil, nil, []string{})

	if len(d.bootstrapPeers) != 0 {
		t.Errorf("expected 0 bootstrap peers, got %d", len(d.bootstrapPeers))
	}

	// Starting should not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("start panicked with empty peer list: %v", r)
			}
		}()
		d.Start()
		d.Stop()
	}()
}

// TestBootstrap_ConnectedPeersList tests that ConnectedPeers returns
// only actually connected peers.
func TestBootstrap_ConnectedPeersList(t *testing.T) {
	d := NewDiscovery(nil, nil, []string{
		"/ip4/127.0.0.1/tcp/1001/p2p/QmPeer1",
		"/ip4/127.0.0.1/tcp/1002/p2p/QmPeer2",
	})

	connected := d.ConnectedPeers()
	if len(connected) != 0 {
		t.Errorf("expected 0 connected peers initially, got %d", len(connected))
	}
}

// TestBootstrap_ReconnectionOnDisconnect tests that disconnected peers
// are reconnected by the maintain loop.
func TestBootstrap_ReconnectionOnDisconnect(t *testing.T) {
	t.Skip("integration: requires P2P node setup")

	// Test that the maintain loop attempts reconnection for disconnected peers
}

// TestBootstrap_StopClosesChannels tests that Stop properly closes
// the stop channel without panic.
func TestBootstrap_StopClosesChannels(t *testing.T) {
	d := NewDiscovery(nil, nil, []string{})

	// Start and immediately stop
	d.Start()
	d.Stop()

	// Stop again should be safe (idempotent)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("second stop panicked: %v", r)
			}
		}()
		d.Stop()
	}()
}

// TestBootstrap_PeerListExchange tests the peer list exchange protocol.
func TestBootstrap_PeerListExchange(t *testing.T) {
	d := NewDiscovery(nil, nil, []string{})

	// Test that we can create and handle peer list request
	req := PeerListRequest{}
	data, err := encodePeerListRequest(&req)
	if err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	// Handle the request
	err = d.HandlePeerListRequest(data, "testpeer")
	if err != nil {
		t.Errorf("handle peer list request failed: %v", err)
	}
}

// encodePeerListRequest encodes a peer list request for testing
func encodePeerListRequest(req *PeerListRequest) ([]byte, error) {
	return json.Marshal(req)
}
