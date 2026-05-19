package network

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAdversarial_UnreachableBootstrap tests that Discovery handles
// unreachable bootstrap peers gracefully without crashing.
func TestAdversarial_UnreachableBootstrap(t *testing.T) {
	t.Skip("integration: requires network setup")

	// This test would create a Discovery with unreachable bootstrap peers
	// and verify it doesn't crash - continues with backoff and eventually
	// enters "isolated" mode.
	//
	// Implementation:
	// 1. Create a Discovery with obviously unreachable peers (e.g., 10.255.255.1)
	// 2. Start discovery
	// 3. Verify it doesn't crash (no panic)
	// 4. Let it run for a short time
	// 5. Verify it's still running and trying to reconnect (log messages)
	// 6. Stop gracefully

	/*
	unreachablePeers := []string{
		"/ip4/10.255.255.1/tcp/12345/p2p/QmUnreachable1",
		"/ip4/10.255.255.2/tcp/12345/p2p/QmUnreachable2",
	}

	discovery := NewDiscovery(nil, nil, unreachablePeers)

	// Should not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("discovery panicked with unreachable peers: %v", r)
			}
		}()

		discovery.Start()

		// Let it try to connect for a bit
		time.Sleep(2 * time.Second)

		// Check it's still running
		discovery.Stop()
	}()

	// Should exit cleanly without panic
	*/
}

// TestAdversarial_DiscoveryReconnectBackoff tests that the backoff
// schedule is correctly applied.
func TestAdversarial_DiscoveryReconnectBackoff(t *testing.T) {
	// Test that the backoff schedule is properly configured
	discovery := NewDiscovery(nil, nil, []string{})

	// Verify backoff schedule exists
	if len(discovery.backoffSchedule) == 0 {
		t.Error("backoff schedule should not be empty")
	}

	// Verify backoff is exponential (each should be >= previous)
	for i := 1; i < len(discovery.backoffSchedule); i++ {
		if discovery.backoffSchedule[i] < discovery.backoffSchedule[i-1] {
			t.Errorf("backoff schedule should be non-decreasing: %v < %v",
				discovery.backoffSchedule[i], discovery.backoffSchedule[i-1])
		}
	}
}

// TestAdversarial_NilP2PNode tests that Discovery handles nil P2P node.
func TestAdversarial_NilP2PNode(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{"/ip4/127.0.0.1/tcp/1234/p2p/QmTest"})

	// Trying to connect with nil P2P should return an error, not panic
	err := discovery.connectBootstrap("/ip4/127.0.0.1/tcp/1234/p2p/QmTest")
	if err == nil {
		t.Error("expected error when P2P node is nil")
	}
}

// TestAdversarial_DiscoveryStartStop tests that Discovery can be started
// and stopped multiple times without issues.
func TestAdversarial_DiscoveryStartStop(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Start
	discovery.Start()

	// Stop
	discovery.Stop()

	// Try to start again (should be safe)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("re-start panicked: %v", r)
			}
		}()
		discovery.Start()
		discovery.Stop()
	}()
}

// TestAdversarial_ConcurrentStartStop tests that Discovery handles
// concurrent start and stop operations.
func TestAdversarial_ConcurrentStartStop(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	done := make(chan bool)

	// Start in one goroutine
	go func() {
		discovery.Start()
		time.Sleep(100 * time.Millisecond)
		discovery.Stop()
		done <- true
	}()

	// Try to stop from main (may already be stopped, should be safe)
	time.Sleep(50 * time.Millisecond)
	discovery.Stop()

	<-done
}

// TestAdversarial_EmptyPeerList tests handling of empty peer lists.
func TestAdversarial_EmptyPeerList(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Request peer list from empty list
	discovery.requestPeerList()

	// Should not panic - just does nothing
}

// TestAdversarial_PeerListResponseEmpty tests handling of empty peer list
// responses.
func TestAdversarial_PeerListResponseEmpty(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Create empty peer list response
	resp := PeerListResponse{Peers: []string{}}
	data, _ := json.Marshal(resp)

	// Handle empty response - should not panic
	err := discovery.HandlePeerListResponse(data, "testpeer")
	if err != nil {
		t.Errorf("unexpected error handling empty peer list: %v", err)
	}
}

// TestAdversarial_PeerListResponseNil tests handling of nil peers in
// peer list response.
func TestAdversarial_PeerListResponseNil(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Response with nil peers (different from empty)
	resp := PeerListResponse{}
	// Note: JSON unmarshal will give empty slice, not nil, but test anyway

	data, _ := json.Marshal(resp)
	err := discovery.HandlePeerListResponse(data, "testpeer")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestAdversarial_InvalidPeerListResponse tests that invalid peer list
// response JSON is handled gracefully.
func TestAdversarial_InvalidPeerListResponse(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Invalid JSON
	invalidData := []byte("not valid json {{{")

	err := discovery.HandlePeerListResponse(invalidData, "testpeer")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestAdversarial_PeerListRequest tests that peer list requests are handled.
func TestAdversarial_PeerListRequest(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Send peer list request - should not panic
	req := PeerListRequest{}
	data, _ := json.Marshal(req)

	err := discovery.HandlePeerListRequest(data, "testpeer")
	if err != nil {
		t.Errorf("unexpected error handling peer list request: %v", err)
	}
}

// TestAdversarial_DiscoveryMaintainsConnectionState tests that the
// connected state map is properly maintained.
func TestAdversarial_DiscoveryMaintainsConnectionState(t *testing.T) {
	discovery := NewDiscovery(nil, nil, []string{})

	// Initially no peers should be connected
	if discovery.IsConnected("any") {
		t.Error("no peers should be connected initially")
	}

	connected := discovery.ConnectedPeers()
	if len(connected) != 0 {
		t.Errorf("expected 0 connected peers, got %d", len(connected))
	}
}