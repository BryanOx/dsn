package network

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
)

// newTestPersistentState creates a temporary persistent state for engine tests
// and returns a cleanup function that closes it.
func newTestPersistentState(t *testing.T) (*state.PersistentState, func()) {
	t.Helper()
	ps, err := state.NewPersistentState(filepath.Join(t.TempDir(), "dsn.db"), types.SHA256Hasher{}, false)
	if err != nil {
		t.Fatal(err)
	}
	return ps, func() { ps.Close() }
}

// TestBlockRangeRequestEncodeDecode tests encoding and decoding of block range requests.
func TestBlockRangeRequestEncodeDecode(t *testing.T) {
	// Encode a request for range [100, 200]
	encoded := encodeBlockRangeRequest(100, 200)

	// Verify encoded length
	if len(encoded) != 16 {
		t.Errorf("expected encoded length 16, got %d", len(encoded))
	}

	// Decode and verify values
	startHeight, endHeight, err := decodeBlockRangeRequest(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if startHeight != 100 {
		t.Errorf("expected startHeight 100, got %d", startHeight)
	}
	if endHeight != 200 {
		t.Errorf("expected endHeight 200, got %d", endHeight)
	}
}

// TestBlockRangeRequestInvalidData tests that truncated data returns an error.
func TestBlockRangeRequestInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short - 1 byte", []byte{0x01}},
		{"too short - 8 bytes", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{"too short - 15 bytes", make([]byte, 15)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeBlockRangeRequest(tt.data)
			if err == nil {
				t.Error("expected error for truncated data, got nil")
			}
		})
	}
}

// TestBlockListEncodeDecode tests encoding and decoding of block lists.
func TestBlockListEncodeDecode(t *testing.T) {
	// Create test blocks of various sizes
	blocks := [][]byte{
		[]byte("block1-small"),
		[]byte("this-is-a-medium-sized-block-data"),
		[]byte("bl"), // smallest possible
		[]byte("a-longer-block-with-more-data-to-test-encoding"),
	}

	// Encode block list
	encoded := encodeBlockList(blocks)

	// Decode and verify
	decoded, err := decodeBlockList(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(blocks) {
		t.Errorf("expected %d blocks, got %d", len(blocks), len(decoded))
	}

	for i, block := range blocks {
		if string(decoded[i]) != string(block) {
			t.Errorf("block %d: expected %q, got %q", i, string(block), string(decoded[i]))
		}
	}
}

// TestBlockListEncodeDecodeEmpty tests that empty block lists round-trip correctly.
func TestBlockListEncodeDecodeEmpty(t *testing.T) {
	// Encode empty list
	encoded := encodeBlockList([][]byte{})

	// Verify it's at least the count bytes
	if len(encoded) < 4 {
		t.Errorf("expected at least 4 bytes for count, got %d", len(encoded))
	}

	// Decode and verify
	decoded, err := decodeBlockList(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(decoded))
	}
}

// TestBlockSyncEngine_New tests that a new engine has correct initial state.
func TestBlockSyncEngine_New(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, nil)

	// Verify initial state
	if engine.IsSyncing() {
		t.Error("expected IsSyncing to be false initially")
	}
	if engine.LastSyncedHeight() != 0 {
		t.Errorf("expected LastSyncedHeight 0, got %d", engine.LastSyncedHeight())
	}
}

// TestBlockSyncEngine_IsSyncing tests that IsSyncing reflects state after Start.
func TestBlockSyncEngine_IsSyncing(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, nil)

	// Before start - not syncing
	if engine.IsSyncing() {
		t.Error("expected IsSyncing to be false before Start")
	}

	// After start - should be false initially (no peers, no catchup needed)
	engine.Start()

	// Give a moment for goroutines to start
	// Note: IsSyncing may be false because there's no target peer
	if engine.IsSyncing() {
		t.Error("expected IsSyncing to be false after Start with no peers")
	}

	engine.Stop()
}

// TestBlockSyncEngine_HandleBlockRangeRequest tests that a valid request returns a response.
func TestBlockSyncEngine_HandleBlockRangeRequest(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, nil)

	// Encode a request for blocks 1-5
	payload := encodeBlockRangeRequest(1, 5)

	// Call handler
	var peerID PeerID
	peerID[0] = 9
	response := engine.HandleBlockRangeRequest(payload, peerID)

	// Response should be non-nil (may be empty if no blocks available)
	if response == nil {
		t.Error("response is nil")
	}

	// Response should be valid encoded block list (even if empty)
	decoded, err := decodeBlockList(response)
	if err != nil {
		t.Errorf("response is not valid block list: %v", err)
	}
	// Should be empty since we don't have persistent state with blocks
	if len(decoded) != 0 {
		t.Logf("got %d blocks in response", len(decoded))
	}
}

// TestBlockSyncEngine_HandleBlockRangeResponse tests that callback is invoked for valid blocks.
func TestBlockSyncEngine_HandleBlockRangeResponse(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, nil)

	// Set up a callback that tracks invocations
	callbackInvoked := false
	engine.SetBlockHandler(func(data []byte) (bool, error) {
		callbackInvoked = true
		return true, nil
	})

	// Create a valid block list response with one mock block
	// Format: [count:4][len:4][block_data...]
	mockBlock := []byte{0x01, 0x02, 0x03, 0x04} // minimal mock block data
	responsePayload := encodeBlockList([][]byte{mockBlock})

	// Handle the response - create a valid PeerID
	var peerID PeerID
	peerID[0] = 1 // set first byte to 1 for identification
	engine.HandleBlockRangeResponse(responsePayload, peerID)

	// Callback should have been invoked
	if !callbackInvoked {
		t.Error("callback was not invoked for valid block in response")
	}
}

// TestBlockSyncEngine_RequestServeMax100 verifies the serve path answers ranges
// of at most 100 blocks and rejects larger requests.
func TestBlockSyncEngine_RequestServeMax100(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	// Store two real blocks to exercise the serve path.
	for h := uint64(1); h <= 2; h++ {
		block := &types.Block{Header: types.BlockHeader{Version: 1, Height: h}}
		if err := consensus.StoreBlock(ps, block); err != nil {
			t.Fatalf("StoreBlock(%d): %v", h, err)
		}
	}

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, ps)

	var peerID PeerID
	peerID[0] = 1
	pm.AddPeer(peerID, "peer-addr")

	// A 100-block range is within the limit and served.
	resp := engine.HandleBlockRangeRequest(encodeBlockRangeRequest(1, 100), peerID)
	if resp == nil {
		t.Fatal("expected a response for range 1..100, got nil")
	}
	decoded, err := decodeBlockList(resp)
	if err != nil {
		t.Fatalf("response is not a valid block list: %v", err)
	}
	if len(decoded) > 100 {
		t.Errorf("served %d blocks, want at most 100", len(decoded))
	}
	if len(decoded) != 2 {
		t.Errorf("served %d blocks, want 2 (the stored blocks)", len(decoded))
	}

	// A 101-block range exceeds the limit and must be rejected.
	if resp := engine.HandleBlockRangeRequest(encodeBlockRangeRequest(1, 101), peerID); resp != nil {
		t.Errorf("expected nil response for 101-block range, got %d bytes", len(resp))
	}

	// The oversized range must also penalize the requester (S4), consistent
	// with the malformed-request path (−2).
	p := pm.GetPeer(peerID)
	p.mu.RLock()
	score := p.Score
	p.mu.RUnlock()
	if score != -2 {
		t.Errorf("peer score = %d, want -2 for an oversized range request", score)
	}
}

// TestBlockSyncEngine_ResponseProgressEqualsTip verifies that after applying a
// batch, sync progress equals the on-disk tip height (not the number of
// received blocks) and is persisted per batch, so a restart resumes cleanly.
func TestBlockSyncEngine_ResponseProgressEqualsTip(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, ps)

	// The handler decodes the height embedded in each mock block and stores it
	// as the on-disk tip, mirroring the node adapter's apply path.
	engine.SetBlockHandler(func(data []byte) (bool, error) {
		height := binary.BigEndian.Uint64(data)
		if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: height}); err != nil {
			return false, err
		}
		return true, nil
	})

	var peerID PeerID
	peerID[0] = 2

	// One batch of 5 blocks whose tips advance to height 45.
	var blocks [][]byte
	for h := uint64(41); h <= 45; h++ {
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, h)
		blocks = append(blocks, data)
	}
	engine.HandleBlockRangeResponse(encodeBlockList(blocks), peerID)

	// Progress must be the tip height 45, not the 5-block count.
	if got := engine.LastSyncedHeight(); got != 45 {
		t.Errorf("LastSyncedHeight = %d, want 45 (tip height, not block count)", got)
	}

	// Progress must be persisted per batch: a fresh engine on the same DB
	// resumes from max(tip, persisted) + 1 = 46.
	pm2 := NewPeerManager(nil)
	p2p2, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p2.Close()

	engine2 := NewBlockSyncEngine(pm2, p2p2, ps)
	engine2.Start()
	defer engine2.Stop()

	if got := engine2.LastSyncedHeight(); got != 45 {
		t.Errorf("resumed LastSyncedHeight = %d, want 45 (persisted progress)", got)
	}
	if got := engine2.resumeFrom(); got != 46 {
		t.Errorf("resumeFrom = %d, want 46 (max(tip, persisted) + 1)", got)
	}
}

// TestBlockSyncEngine_ResumeTipFloorAfterCrash verifies the crash-after-apply-
// before-persist resume (W5/D4 tip-floor): blocks applied to disk with the tip
// stored but no sync progress persisted must still resume from the on-disk tip
// + 1, never re-requesting an already-applied block.
func TestBlockSyncEngine_ResumeTipFloorAfterCrash(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	// Crash window: tip stored (block 80 applied), progress never persisted.
	if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: 80}); err != nil {
		t.Fatal(err)
	}

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, ps)
	engine.Start()
	defer engine.Stop()

	if got := engine.resumeFrom(); got != 81 {
		t.Errorf("resumeFrom = %d, want 81 (on-disk tip 80 + 1, no progress persisted)", got)
	}
	if got := engine.LastSyncedHeight(); got != 80 {
		t.Errorf("LastSyncedHeight = %d, want 80 (resume floor above the applied tip)", got)
	}
}

// TestBlockSyncEngine_CatchUpRequestStartsAtTipPlusOne proves no double-apply
// at the wire level: after a crash with the tip on disk, the first catch-up
// request starts at tip+1 — never at or below the applied tip.
func TestBlockSyncEngine_CatchUpRequestStartsAtTipPlusOne(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: 80}); err != nil {
		t.Fatal(err)
	}

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	// A pipe connection stands in for the serving peer's socket so the
	// engine's request is observable on the wire.
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const peerAddr = "peer-addr"
	peerID := PeerIDFromBytes([]byte(peerAddr))
	peer := pm.AddPeer(peerID, peerAddr)
	peer.mu.Lock()
	peer.Conn = client
	peer.SyncHeight = 500
	peer.State = PeerConnected
	peer.mu.Unlock()

	// Register the pipe as the P2P node's connection to that peer (SendTo
	// writes framed messages to n.connections[addr]).
	p2p.connMu.Lock()
	p2p.connections[peerAddr] = client
	p2p.connMu.Unlock()

	engine := NewBlockSyncEngine(pm, p2p, ps)
	engine.Start()
	defer engine.Stop()

	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4+1+16)
		if _, err := io.ReadFull(server, buf); err == nil {
			got <- buf
		}
	}()

	engine.startCatchUp()

	select {
	case buf := <-got:
		if buf[4] != MsgTypeBlockRangeRequest {
			t.Fatalf("message type = 0x%02x, want 0x30", buf[4])
		}
		start := binary.BigEndian.Uint64(buf[5:13])
		end := binary.BigEndian.Uint64(buf[13:21])
		if start != 81 {
			t.Errorf("request starts at %d, want 81 (tip 80 + 1) — re-requests would double-apply", start)
		}
		if end != 180 {
			t.Errorf("request ends at %d, want 180 (81 + 99)", end)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no block-range request observed")
	}
}

// TestBlockSyncEngine_ResumePrefersPersistedProgress verifies resume uses
// max(tip, progress): persisted progress ahead of the on-disk tip wins, so a
// restart never re-fetches a range the previous run already applied.
func TestBlockSyncEngine_ResumePrefersPersistedProgress(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	// Tip at 100; progress persisted at 120 (progress ahead of tip).
	if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: 100}); err != nil {
		t.Fatal(err)
	}
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, ps)
	engine.mu.Lock()
	engine.lastSyncedHeight = 120
	engine.mu.Unlock()
	engine.persistProgress()

	// A fresh engine on the same DB must resume from the persisted progress.
	pm2 := NewPeerManager(nil)
	p2p2, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p2.Close()

	engine2 := NewBlockSyncEngine(pm2, p2p2, ps)
	if got := engine2.resumeFrom(); got != 121 {
		t.Errorf("resumeFrom = %d, want 121 (progress 120 beats tip 100)", got)
	}
}

// TestBlockSyncEngine_InvalidBlockPenalizesStopsAndResumes verifies the
// invalid-block contract (W4): the serving peer is penalized −5, the remaining
// blocks in the window are rejected, the engine stops, and the last accepted
// height is persisted so the retry resumes narrowed from it.
func TestBlockSyncEngine_InvalidBlockPenalizesStopsAndResumes(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewBlockSyncEngine(pm, p2p, ps)

	var applied []uint64
	engine.SetBlockHandler(func(data []byte) (bool, error) {
		height := binary.BigEndian.Uint64(data)
		if height == 46 {
			return false, errors.New("invalid block")
		}
		if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: height}); err != nil {
			return false, err
		}
		applied = append(applied, height)
		return true, nil
	})

	var peerID PeerID
	peerID[0] = 3
	pm.AddPeer(peerID, "peer-addr")

	// Window [41..46]; block 46 is invalid.
	var blocks [][]byte
	for h := uint64(41); h <= 46; h++ {
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, h)
		blocks = append(blocks, data)
	}
	engine.HandleBlockRangeResponse(encodeBlockList(blocks), peerID)

	// Remaining blocks rejected: only 41..45 reached the handler.
	if len(applied) != 5 {
		t.Errorf("applied %d blocks, want 5 (41..45; 46 must stop the window)", len(applied))
	}

	// Sender penalized −5 for the invalid block.
	p := pm.GetPeer(peerID)
	p.mu.RLock()
	score := p.Score
	p.mu.RUnlock()
	if score != -5 {
		t.Errorf("peer score = %d, want -5", score)
	}

	// Engine stopped.
	if engine.IsSyncing() {
		t.Error("engine must stop after an invalid block")
	}

	// Last accepted height persisted so the retry resumes narrowed from 46.
	if got := engine.LastSyncedHeight(); got != 45 {
		t.Errorf("LastSyncedHeight = %d, want 45 (last accepted)", got)
	}
	if got := engine.loadProgress(); got != 45 {
		t.Errorf("persisted progress = %d, want 45 (resume boundary)", got)
	}
	if got := engine.resumeFrom(); got != 46 {
		t.Errorf("resumeFrom = %d, want 46 (last accepted + 1)", got)
	}
}

// TestBlockSyncEngine_TimeoutReRequestsPending verifies the timeout re-request
// (design: "10s tick re-requests timed-out pending"; S3): a window whose
// response never arrived is re-requested after the timeout instead of being
// abandoned, so catch-up survives a dropped response.
func TestBlockSyncEngine_TimeoutReRequestsPending(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const peerAddr = "peer-addr"
	peerID := PeerIDFromBytes([]byte(peerAddr))
	peer := pm.AddPeer(peerID, peerAddr)
	peer.mu.Lock()
	peer.Conn = client
	peer.SyncHeight = 500
	peer.State = PeerConnected
	peer.mu.Unlock()

	p2p.connMu.Lock()
	p2p.connections[peerAddr] = client
	p2p.connMu.Unlock()

	engine := NewBlockSyncEngine(pm, p2p, nil)
	engine.Start()
	defer engine.Stop()

	// Simulate an in-flight request pending past the timeout.
	engine.mu.Lock()
	engine.isSyncing = true
	engine.targetPeerAddr = peerAddr
	engine.targetHeight = 500
	engine.pendingRequests[81] = time.Now().Add(-blockSyncRequestTimeout - time.Second)
	engine.mu.Unlock()

	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4+1+16)
		if _, err := io.ReadFull(server, buf); err == nil {
			got <- buf
		}
	}()

	engine.recheckPendingRequests()

	select {
	case buf := <-got:
		if buf[4] != MsgTypeBlockRangeRequest {
			t.Fatalf("message type = 0x%02x, want 0x30", buf[4])
		}
		start := binary.BigEndian.Uint64(buf[5:13])
		if start != 81 {
			t.Errorf("re-request starts at %d, want 81 (timed-out window)", start)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no re-request observed for the pending window")
	}

	// The pending entry is refreshed so the next timeout is a full window away.
	engine.mu.RLock()
	_, still := engine.pendingRequests[81]
	engine.mu.RUnlock()
	if !still {
		t.Error("pending entry must be refreshed after the re-request")
	}
}

// TestBlockSyncEngine_PartialResponseNarrowsNextWindow pins the spec's
// "partial responses SHOULD be re-requested narrowed to the missing heights"
// (block-range-sync, partial-and-invalid): when a peer answers with fewer
// blocks than requested, the engine applies what it got and requests the next
// window from last-accepted + 1 — the first missing height — never re-sending
// the already-applied prefix of the window.
func TestBlockSyncEngine_PartialResponseNarrowsNextWindow(t *testing.T) {
	ps, closeDB := newTestPersistentState(t)
	defer closeDB()

	// Tip 80 on disk: the catch-up window starts at 81.
	if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: 80}); err != nil {
		t.Fatal(err)
	}

	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const peerAddr = "peer-addr"
	peerID := PeerIDFromBytes([]byte(peerAddr))
	peer := pm.AddPeer(peerID, peerAddr)
	peer.mu.Lock()
	peer.Conn = client
	peer.SyncHeight = 500
	peer.State = PeerConnected
	peer.mu.Unlock()

	p2p.connMu.Lock()
	p2p.connections[peerAddr] = client
	p2p.connMu.Unlock()

	engine := NewBlockSyncEngine(pm, p2p, ps)
	engine.Start()
	defer engine.Stop()

	engine.SetBlockHandler(func(data []byte) (bool, error) {
		height := binary.BigEndian.Uint64(data)
		if err := consensus.StoreTip(ps, &types.BlockHeader{Version: 1, Height: height}); err != nil {
			return false, err
		}
		return true, nil
	})

	got := make(chan []byte, 2)
	go func() {
		for i := 0; i < 2; i++ {
			buf := make([]byte, 4+1+16)
			if _, err := io.ReadFull(server, buf); err != nil {
				return
			}
			got <- buf
		}
	}()

	engine.startCatchUp()

	// First request covers the full window [81..180].
	select {
	case buf := <-got:
		start := binary.BigEndian.Uint64(buf[5:13])
		if start != 81 {
			t.Fatalf("first request starts at %d, want 81", start)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no initial window request observed")
	}

	// The peer answers only 3 of the 100 requested blocks (a partial window).
	var blocks [][]byte
	for h := uint64(81); h <= 83; h++ {
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, h)
		blocks = append(blocks, data)
	}
	engine.HandleBlockRangeResponse(encodeBlockList(blocks), peerID)

	// The next request must start at 84 — the first missing height — not 81.
	select {
	case buf := <-got:
		if buf[4] != MsgTypeBlockRangeRequest {
			t.Fatalf("message type = 0x%02x, want 0x30", buf[4])
		}
		start := binary.BigEndian.Uint64(buf[5:13])
		if start != 84 {
			t.Errorf("narrowed re-request starts at %d, want 84 (last accepted + 1)", start)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no narrowed re-request for the missing heights")
	}
}
