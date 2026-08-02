package network

import (
	"testing"
)

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
	response := engine.HandleBlockRangeRequest(payload)

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
