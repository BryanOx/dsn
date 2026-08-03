package network

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
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

// TestRegisterConn_StaleCleanupKeepsNewerConnection verifies that a stale
// cleanup of a superseded connection does not delete the connection that
// replaced it. Connection A dies and its read loop cleans it up; a fresh
// connection B registers afterwards. A repeated stale cleanup from A must not
// remove B.
func TestRegisterConn_StaleCleanupKeepsNewerConnection(t *testing.T) {
	n, _ := NewP2PNode(0)
	defer n.Close()

	nodeConnA, peerA := net.Pipe()
	nodeConnB, peerB := net.Pipe()
	defer nodeConnA.Close()
	defer nodeConnB.Close()
	defer peerA.Close()
	defer peerB.Close()

	addr := "10.0.0.1:26656"
	id := PeerIDFromBytes([]byte(addr))

	cleanupA := n.registerConn(nodeConnA, addr, id)

	// Connection A dies and its read loop cleans it up.
	cleanupA()

	// A fresh connection registers afterwards and becomes the entry.
	cleanupB := n.registerConn(nodeConnB, addr, id)
	defer cleanupB()

	// Simulate a stale, repeated cleanup of connection A.
	cleanupA()

	n.connMu.RLock()
	cur := n.connections[addr]
	n.connMu.RUnlock()
	if cur != nodeConnB {
		t.Fatal("stale cleanup removed the newer connection from the broadcast set")
	}
}

// TestBroadcast_AfterStaleCleanupUsesNewerConnection verifies that a broadcast
// still reaches the newer connection after a stale cleanup of a superseded one.
func TestBroadcast_AfterStaleCleanupUsesNewerConnection(t *testing.T) {
	n, _ := NewP2PNode(0)
	defer n.Close()

	// Connection A dies and is cleaned up before connection B registers.
	nodeConnA, peerA := net.Pipe()
	nodeConnB, peerB := net.Pipe()
	defer nodeConnA.Close()
	defer nodeConnB.Close()
	defer peerA.Close()
	defer peerB.Close()

	addr := "10.0.0.2:26656"
	id := PeerIDFromBytes([]byte(addr))

	cleanupA := n.registerConn(nodeConnA, addr, id)
	cleanupA()

	cleanupB := n.registerConn(nodeConnB, addr, id)
	defer cleanupB()

	// A stale, repeated cleanup of the dead connection must not remove B.
	cleanupA()

	received := make(chan byte, 1)
	go func() {
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(peerB, lenBuf); err != nil {
			received <- 0
			return
		}
		payload := make([]byte, 1)
		if _, err := io.ReadFull(peerB, payload); err != nil {
			received <- 0
			return
		}
		received <- payload[0]
	}()

	n.Broadcast([]byte{0x42})

	select {
	case got := <-received:
		if got != 0x42 {
			t.Errorf("payload = %#x, want 0x42", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: broadcast never reached the newer connection")
	}
}

// TestRegisterConn_KeepsLiveExistingConnection verifies that registering a
// duplicate connection while a live connection to the same address exists
// keeps the existing connection and discards the duplicate. Before the fix
// every re-dial closed the live connection, amplifying reconnect churn.
func TestRegisterConn_KeepsLiveExistingConnection(t *testing.T) {
	n, _ := NewP2PNode(0)
	defer n.Close()

	nodeConnA, peerA := net.Pipe()
	nodeConnB, peerB := net.Pipe()
	defer nodeConnA.Close()
	defer nodeConnB.Close()
	defer peerA.Close()
	defer peerB.Close()

	addr := "10.0.0.3:26656"
	id := PeerIDFromBytes([]byte(addr))

	cleanupA := n.registerConn(nodeConnA, addr, id)
	defer cleanupA()

	// Registering B (a duplicate dial) must keep A and discard B.
	cleanupB := n.registerConn(nodeConnB, addr, id)

	n.connMu.RLock()
	cur := n.connections[addr]
	n.connMu.RUnlock()
	if cur != nodeConnA {
		t.Fatal("registerConn replaced the live existing connection with the duplicate")
	}

	// The surviving connection must still carry broadcasts.
	received := make(chan byte, 1)
	go func() {
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(peerA, lenBuf); err != nil {
			received <- 0
			return
		}
		payload := make([]byte, 1)
		if _, err := io.ReadFull(peerA, payload); err != nil {
			received <- 0
			return
		}
		received <- payload[0]
	}()
	n.Broadcast([]byte{0x42})
	select {
	case got := <-received:
		if got != 0x42 {
			t.Errorf("payload = %#x, want 0x42", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: broadcast never reached the surviving connection")
	}

	// The duplicate connection was closed and the peer redirects to the
	// surviving connection so pings/PEX keep flowing over a live socket.
	if p := n.pm.GetPeerByAddr(addr); p == nil {
		t.Fatal("peer not registered")
	} else if p.Conn != nodeConnA {
		t.Fatal("peer connection was not redirected to the surviving connection")
	}

	cleanupB()
	if _, err := peerB.Read(make([]byte, 1)); err == nil {
		t.Fatal("duplicate connection was not closed")
	}
}

// TestReadLoop_PingGetsPong verifies the read loop dispatches an incoming
// ping to HandlePing and that the pong reaches the sender over the same
// socket. Before the fix the ping was dropped in the read loop switch and no
// pong ever flowed, so the sender's 30s read deadline tore the connection
// down.
func TestReadLoop_PingGetsPong(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// Wire discovery on both ends so the read loops can dispatch ping/pong.
	pd1 := NewPeerDiscovery(n1.pm, nil, n1)
	n1.SetDiscovery(pd1)
	pd2 := NewPeerDiscovery(n2.pm, nil, n2)
	n2.SetDiscovery(pd2)

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Send a ping with a timestamp 3s in the past so the latency measured by
	// HandlePong is clearly nonzero. The frame is written directly to the
	// outbound connection (as discovery's SendPing does); SendTo would add a
	// second transport length prefix.
	pingPayload := make([]byte, 8)
	binary.BigEndian.PutUint64(pingPayload, uint64(time.Now().Add(-3*time.Second).Unix()))
	n1.connMu.RLock()
	outConn := n1.connections[n2.Addr()]
	n1.connMu.RUnlock()
	if outConn == nil {
		t.Fatal("outbound connection not registered")
	}
	if _, err := outConn.Write(FrameMessage(MsgTypePing, pingPayload)); err != nil {
		t.Fatal(err)
	}

	// n1 must receive the pong on its outbound connection; HandlePong turns
	// it into a latency update on the peer.
	id := PeerIDFromBytes([]byte(n2.Addr()))
	deadline := time.Now().Add(3 * time.Second)
	for {
		p := n1.pm.GetPeer(id)
		if p != nil {
			p.mu.RLock()
			latency := p.Latency
			p.mu.RUnlock()
			if latency >= 3*time.Second {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout: pong never arrived on the outbound connection")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestP2PNode_OutboundReceivesGossip verifies that a transaction gossiped by
// the remote node is received on the OUTBOUND connection. Before the fix,
// Connect only registered the connection for writing and never started a
// reader, so a tx pushed over an outbound socket was dropped.
func TestP2PNode_OutboundReceivesGossip(t *testing.T) {
	hasher := types.SHA256Hasher{}

	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// n1 dials n2; n1's side of the socket is the outbound connection.
	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}

	// n1 must read over its outbound connection.
	received := make(chan *types.Transaction, 1)
	n1.SetTxHandler(func(tx *types.Transaction) {
		received <- tx
	})

	time.Sleep(100 * time.Millisecond)

	tx := &types.Transaction{
		Version:  1,
		Nonce:    1,
		MaxFee:   100,
		IntentID: types.Hash{1, 2, 3},
	}
	if err := n2.GossipTransaction(tx, hasher); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.IntentID != tx.IntentID {
			t.Fatalf("intent ID mismatch after outbound gossip: got %x want %x", got.IntentID, tx.IntentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: outbound connection never read the gossiped transaction")
	}
}

// TestP2PNode_OutboundReceivesBlock verifies that a block broadcast by the
// remote node is received on the OUTBOUND connection. Before the fix the
// outbound reader was missing entirely.
func TestP2PNode_OutboundReceivesBlock(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}

	received := make(chan []byte, 1)
	n1.SetBlockHandler(func(data []byte) {
		received <- data
	})

	time.Sleep(100 * time.Millisecond)

	// Framed block message: type byte 0x01 (MsgTypeBlock) + payload.
	blockMsg := []byte{byte(MsgTypeBlock), 0xAA, 0xBB}
	n2.Broadcast(blockMsg)

	select {
	case got := <-received:
		if len(got) != len(blockMsg) || got[1] != blockMsg[1] {
			t.Fatalf("block message mismatch: got %x want %x", got, blockMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: outbound connection never read the broadcast block")
	}
}

// TestP2PNode_OutboundReceivesVote verifies that a consensus vote broadcast
// by the remote node is received on the OUTBOUND connection and forwarded to
// the vote handler, decoded from the raw Vote encoding.
func TestP2PNode_OutboundReceivesVote(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}

	received := make(chan *types.Vote, 1)
	n1.SetVoteHandler(func(vote *types.Vote) {
		received <- vote
	})

	time.Sleep(100 * time.Millisecond)

	vote := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    42,
		Round:     0,
		BlockHash: types.ZeroHash,
		Validator: types.Address{0x01, 0x02, 0x03},
	}
	var buf bytes.Buffer
	if err := vote.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	voteMsg := append([]byte{byte(MsgTypeVote)}, buf.Bytes()...)
	n2.Broadcast(voteMsg)

	select {
	case got := <-received:
		if got.VoteType != types.VotePrecommit || got.Height != 42 || got.Round != 0 {
			t.Fatalf("vote mismatch: got %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: outbound connection never delivered the vote")
	}
}

// TestPeerDiscovery_DialRegistersConnection verifies that a connection
// dialed through the PeerManager (as peer discovery does) is registered in
// the P2P node's broadcast set AND read. Before the fix ConnectToPeer stored
// the connection only on the Peer and neither registered nor read it.
func TestPeerDiscovery_DialRegistersConnection(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	id := PeerIDFromBytes([]byte(n2.Addr()))
	if err := n1.pm.ConnectToPeer(id, n2.Addr()); err != nil {
		t.Fatal(err)
	}

	if n1.NumPeers() == 0 {
		t.Fatal("connection dialed via PeerManager must be registered for broadcast")
	}

	received := make(chan *types.Transaction, 1)
	n1.SetTxHandler(func(tx *types.Transaction) {
		received <- tx
	})

	time.Sleep(100 * time.Millisecond)

	tx := &types.Transaction{
		Version:  1,
		Nonce:    1,
		MaxFee:   100,
		IntentID: types.Hash{9, 9, 9},
	}
	if err := n2.GossipTransaction(tx, types.SHA256Hasher{}); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.IntentID != tx.IntentID {
			t.Fatalf("intent ID mismatch after pm dial: got %x want %x", got.IntentID, tx.IntentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: connection dialed via PeerManager was never read")
	}
}
