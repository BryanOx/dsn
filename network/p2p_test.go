package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
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

// TestReadLoop_MalformedSyncHeightPenalizes verifies the wire-path hardening
// (W2): a SyncHeight frame whose payload is not exactly 8 bytes must penalize
// the sender (−2) and leave the recorded height unchanged. The penalty must
// fire on the read-loop path, not only in ParseMessage (which production never
// calls), and must not depend on the block-sync engine being wired.
func TestReadLoop_MalformedSyncHeightPenalizes(t *testing.T) {
	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	// Wire a block-sync engine so the valid path (UpdateSyncHeight +
	// NotifyPeerHeight) is exercised end to end.
	be := NewBlockSyncEngine(n1.PeerManager(), n1, nil)
	n1.SetBlockSyncEngine(be)

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// n1's peer record for n2, pre-seeded with a known height.
	peerID := PeerIDFromBytes([]byte(n2.Addr()))
	peer := n1.pm.GetPeer(peerID)
	if peer == nil {
		t.Fatal("n2 not registered as a peer of n1")
	}
	peer.mu.Lock()
	peer.SyncHeight = 42
	peer.mu.Unlock()

	// Malformed: payload shorter than 8 bytes.
	n2.Broadcast([]byte{byte(MsgTypeSyncHeight), 0x01, 0x02, 0x03, 0x04})
	require.Eventually(t, func() bool {
		peer.mu.RLock()
		defer peer.mu.RUnlock()
		return peer.Score == -2 && peer.SyncHeight == 42
	}, 2*time.Second, 20*time.Millisecond,
		"malformed 0x43 (short payload) must penalize −2 and keep the height")

	// Oversized: payload longer than 8 bytes is also malformed.
	n2.Broadcast(append([]byte{byte(MsgTypeSyncHeight)}, make([]byte, 12)...))
	require.Eventually(t, func() bool {
		peer.mu.RLock()
		defer peer.mu.RUnlock()
		return peer.Score == -4 && peer.SyncHeight == 42
	}, 2*time.Second, 20*time.Millisecond,
		"malformed 0x43 (oversized payload) must penalize −2 and keep the height")

	// Valid 8-byte payload updates the height monotonically.
	var h [8]byte
	binary.BigEndian.PutUint64(h[:], 50)
	n2.Broadcast(append([]byte{byte(MsgTypeSyncHeight)}, h[:]...))
	require.Eventually(t, func() bool {
		peer.mu.RLock()
		defer peer.mu.RUnlock()
		return peer.SyncHeight == 50
	}, 2*time.Second, 20*time.Millisecond, "valid 0x43 must update the height")
}

// TestP2PNode_GossipTransactionTypedFraming verifies the S2 framing fix: a
// transaction whose first encoded byte is a known message type (Version
// 0x0200 encodes 0x02 = MsgTypeTransaction) must arrive intact. The legacy
// [len][raw-tx] framing made the read loop treat that byte as a type prefix,
// strip it, and decode garbage — the tx was never delivered.
func TestP2PNode_GossipTransactionTypedFraming(t *testing.T) {
	hasher := types.SHA256Hasher{}

	n1, _ := NewP2PNode(0)
	defer n1.Close()
	n2, _ := NewP2PNode(0)
	defer n2.Close()

	if err := n1.Connect(n2.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	received := make(chan *types.Transaction, 1)
	n2.SetTxHandler(func(tx *types.Transaction) {
		received <- tx
	})

	tx := &types.Transaction{
		Version:  0x0200, // first encoded byte 0x02 → collides with MsgTypeTransaction
		Nonce:    1,
		MaxFee:   100,
		IntentID: types.Hash{1, 2, 3},
	}
	if err := n1.GossipTransaction(tx, hasher); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		require.Equal(t, tx.IntentID, got.IntentID, "typed-framed gossip must deliver the tx intact")
		require.Equal(t, tx.Version, got.Version, "version must survive the typed framing")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: typed-framed gossiped transaction never arrived")
	}
}

// TestP2PNode_CloseJoinsBlockProcessor pins the shutdown-race fix: Close must
// wait for an in-flight block apply (which commits to the persistent DB) to
// finish before returning, and no block apply may run after Close returns.
// Without the join, P2PNode.Close returned immediately while the processor
// goroutine could still select a buffered block job after stopCh fired —
// Node.Close then closed the persistent DB, and the late apply panicked with
// "persistent commit: database not open" (observed intermittently in
// TestConvergence_ConsensusLoop at 2c453ad, pre-existing at PR3).
func TestP2PNode_CloseJoinsBlockProcessor(t *testing.T) {
	n, err := NewP2PNode(0)
	require.NoError(t, err)

	var applied atomic.Int32
	release := make(chan struct{})

	n.SetBlockHandler(func(data []byte) {
		applied.Add(1)
		<-release // hold the apply in flight until the test releases it
	})

	// Enqueue a job the single-consumer processor will pick up.
	n.handleBlock([]byte("job"), "peer")

	// Wait until the apply is in flight.
	require.Eventually(t, func() bool { return applied.Load() == 1 },
		5*time.Second, 10*time.Millisecond, "block handler never invoked")

	// Close while the apply is in flight. Close must NOT return until the
	// in-flight apply completes and the processor goroutine has exited.
	closeDone := make(chan struct{})
	go func() {
		_ = n.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		t.Fatal("Close returned while a block apply was still in flight")
	case <-time.After(200 * time.Millisecond):
		// Expected: Close is joining the processor.
	}

	// Release the in-flight apply; Close must now return.
	close(release)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the in-flight apply finished")
	}

	// The processor goroutine has exited by the time Close returned, so no
	// apply can run afterwards. Give any (buggy) stray dispatch a window to
	// show itself before asserting the final count.
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int32(1), applied.Load(),
		"a block apply ran after Close returned")
}

// TestP2PNode_ConcurrentWritesKeepFraming hammers a single connection with
// concurrent frame writes (4-byte big-endian length prefix + payload) from
// several goroutines and asserts the far side reads exactly N clean frames.
//
// Regression for the snapshot-download flake (W1 in the state-sync verify
// report): SendTo/sendToPeer emitted the length prefix and the payload as two
// separate conn.Write calls while Broadcast serialized only itself. Two
// goroutines sending on the same connection could interleave prefix/payload
// across frames, so the reader parsed a corrupt length — usually exceeding
// MaxPayloadSize — and the read loop tore the connection down, surfacing as
// "write: connection reset by peer" between the snapshot query and the
// download, then "peer not found" on the download manager's re-request.
// Every send path must emit each frame in a single conn.Write call so the
// net.Conn per-call atomicity (never two frames' bytes interleaved) holds.
func TestP2PNode_ConcurrentWritesKeepFraming(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	server := <-accepted
	require.NotNil(t, server)
	defer server.Close()

	// Writer node with a single registered connection — the shape SendTo and
	// sendToPeer operate on.
	n := &P2PNode{
		connections: map[string]net.Conn{"peer": server},
	}

	// Raw reader: parse frames exactly like readLoop does (length, payload).
	reader := bufio.NewReader(client)
	readerDone := make(chan struct{})
	var okFrames, tooBigFrames atomic.Int64
	go func() {
		defer close(readerDone)
		for {
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(reader, lenBuf); err != nil {
				return
			}
			msgLen := binary.BigEndian.Uint32(lenBuf)
			if msgLen > MaxPayloadSize {
				tooBigFrames.Add(1)
				continue
			}
			if _, err := io.ReadFull(reader, make([]byte, msgLen)); err != nil {
				return
			}
			okFrames.Add(1)
		}
	}()

	payload := make([]byte, 150*1024) // like a snapshot chunk
	const (
		goroutines = 4
		perG       = 150
	)
	total := 0

	hammer := func(send func() error) {
		var wg sync.WaitGroup
		writeErrs := make(chan error, goroutines*perG)
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perG; i++ {
					if err := send(); err != nil {
						writeErrs <- err
						return
					}
				}
			}()
		}
		wg.Wait()
		close(writeErrs)
		for err := range writeErrs {
			t.Fatalf("send failed under concurrency: %v", err)
		}
		total += goroutines * perG
	}

	// Phase 1: SendTo (generic frame: type byte + payload as the caller
	// supplies it).
	hammer(func() error { return n.SendTo("peer", payload) })

	// Phase 2: sendToPeer (the snapshot send path; frame = type + payload).
	hammer(func() error {
		return n.sendToPeer("peer", MsgTypeSnapshotChunk, payload)
	})

	// Wait until every frame has arrived at the reader before closing the
	// client: Close discards unread bytes in the receive buffer, which would
	// race the last in-flight frame.
	require.Eventually(t, func() bool { return okFrames.Load() >= int64(total) },
		10*time.Second, 5*time.Millisecond,
		"reader never received all frames (corrupt lengths may have desynced the stream)")
	client.Close()
	<-readerDone

	require.Equal(t, int64(total), okFrames.Load(),
		"every frame must arrive intact; interleaved length/payload pairs corrupt the stream")
	require.Zero(t, tooBigFrames.Load(),
		"no corrupt (oversized) length may be read; > MaxPayloadSize makes the read loop exit")
}
