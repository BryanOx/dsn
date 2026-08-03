package network

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
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
