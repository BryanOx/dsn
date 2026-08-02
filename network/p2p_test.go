package network

import (
	"testing"
	"time"

	"github.com/dsn/dsn/types"
)

func TestNewP2PNode(t *testing.T) {
	// Test basic node creation with a random port
	n, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	if n == nil {
		t.Fatal("node is nil")
	}
	if n.Addr() == "" {
		t.Error("address is empty")
	}
}

func TestP2PNode_Connect(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// Connect n1 to n2
	err := n1.Connect(n2.Addr())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if n1.NumPeers() == 0 {
		t.Error("n1 should have at least 1 peer")
	}
}

func TestP2PNode_GossipTransaction(t *testing.T) {
	hasher := types.SHA256Hasher{}

	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// n1 connects to n2
	n1.Connect(n2.Addr())
	time.Sleep(200 * time.Millisecond)

	// Create a channel to receive gossiped txs on n2
	received := make(chan *types.Transaction, 1)
	n2.SetTxHandler(func(tx *types.Transaction) {
		received <- tx
	})

	// Create a tx on n1 and gossip it
	tx := &types.Transaction{
		Version:  1,
		Nonce:    1,
		MaxFee:   100,
		IntentID: types.Hash{1, 2, 3},
	}
	n1.GossipTransaction(tx, hasher)

	// Wait for it to arrive on n2
	select {
	case got := <-received:
		if got.IntentID != tx.IntentID {
			t.Error("intent ID mismatch after gossip")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for gossiped transaction")
	}
}

// TestP2PNode_PeerManagerIntegration verifies connection registers peer in PeerManager.
func TestP2PNode_PeerManagerIntegration(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// Connect n1 to n2
	err := n1.Connect(n2.Addr())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify n1 has 1 peer via PeerManager
	if n1.NumPeers() == 0 {
		t.Error("n1 should have at least 1 peer via PeerManager")
	}
}

// TestP2PNode_EngineSetters verifies engine references can be set.
func TestP2PNode_EngineSetters(t *testing.T) {
	n, _ := NewP2PNode(0)
	defer n.Close()

	pm := NewPeerManager(nil)

	// Create engines (nil persistent state is OK for this test)
	p2p2, _ := NewP2PNode(0)
	defer p2p2.Close()

	fe := NewFastSyncEngine(pm, p2p2, nil)
	be := NewBlockSyncEngine(pm, p2p2, nil)
	ge := NewGossipEngine(pm, p2p2)

	n.SetFastSyncEngine(fe)
	n.SetBlockSyncEngine(be)
	n.SetGossipEngine(ge)

	// No panic means setters work
}

// TestP2PNode_ConnectDuplicate verifies reconnecting to same peer is handled.
func TestP2PNode_ConnectDuplicate(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// Connect n1 to n2 twice
	err := n1.Connect(n2.Addr())
	if err != nil {
		t.Fatal(err)
	}

	err = n1.Connect(n2.Addr())
	// Second connection should not panic — may return nil or error depending on implementation
	// Just verify it doesn't crash
	_ = err

	time.Sleep(200 * time.Millisecond)

	// NumPeers should still work
	peers := n1.NumPeers()
	if peers == 0 {
		t.Error("n1 should have peers after duplicate connect attempt")
	}
}
