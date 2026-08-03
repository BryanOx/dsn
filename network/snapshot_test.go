package network

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// setupSnapshotServer builds a P2PNode whose FastSyncEngine serves a stored
// snapshot at height 100 (300 KiB of data => 2 chunks at DefaultChunkSize).
// The serve closures must answer queries and chunk requests from the stored
// checkpoint + snapshot (task 5.2).
func setupSnapshotServer(t *testing.T) (*P2PNode, *state.Checkpoint, []state.SnapshotChunk) {
	t.Helper()

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(filepath.Join(t.TempDir(), "node.db"), hasher, false)
	if err != nil {
		t.Fatalf("setupSnapshotServer: persistent state: %v", err)
	}
	t.Cleanup(func() { _ = ps.Close() })

	// Source state: one account plus a >DefaultChunkSize KV entry so the
	// snapshot splits into 2 chunks.
	src := state.NewInMemoryState(hasher)
	addr := types.Address{0: 0x01}
	acc := state.NewAccount(addr, [32]byte{0: 0x01})
	acc.AddBalance(types.NewAmount(5000))
	src.SetAccount(addr, acc)
	big := make([]byte, 300*1024)
	for i := range big {
		big[i] = byte(i % 251)
	}
	src.SetBytes("big", big)
	root, err := src.Commit()
	if err != nil {
		t.Fatalf("setupSnapshotServer: commit source state: %v", err)
	}

	snap, err := src.CreateSnapshot(100, 2, types.Hash{0xAA}, 1000000)
	if err != nil {
		t.Fatalf("setupSnapshotServer: create snapshot: %v", err)
	}
	snapData, err := state.SerializeSnapshot(snap)
	if err != nil {
		t.Fatalf("setupSnapshotServer: serialize snapshot: %v", err)
	}

	cp := &state.Checkpoint{
		Height:           100,
		BlockHash:        types.Hash{0xBB},
		StateRoot:        root,
		SnapshotHash:     state.SnapshotHash(snapData),
		ValidatorSetHash: types.Hash{0xAA},
		Epoch:            2,
		Timestamp:        1000000,
	}
	if err := state.StoreSnapshot(ps, 100, snapData); err != nil {
		t.Fatalf("setupSnapshotServer: store snapshot: %v", err)
	}
	if err := state.StoreCheckpoint(ps, cp); err != nil {
		t.Fatalf("setupSnapshotServer: store checkpoint: %v", err)
	}

	chunks, err := state.ChunkSnapshot(snapData, state.DefaultChunkSize)
	if err != nil {
		t.Fatalf("setupSnapshotServer: chunk snapshot: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("setupSnapshotServer: expected 2 chunks, got %d", len(chunks))
	}

	server, err := NewP2PNode(0)
	if err != nil {
		t.Fatalf("setupSnapshotServer: new p2p: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	// Wire the serve closures (SetFastSyncEngine registers query + chunk
	// request handlers against this engine's persistent state).
	engine := NewFastSyncEngine(server.PeerManager(), server, ps)
	server.SetFastSyncEngine(engine)

	return server, cp, chunks
}

// TestSnapshotServe_QueryAnswered verifies the query path: a peer's broadcast
// snapshot query is answered with the latest stored snapshot's metadata,
// including the state root (task 5.1 wire change) and chunk count.
func TestSnapshotServe_QueryAnswered(t *testing.T) {
	server, cp, chunks := setupSnapshotServer(t)

	client, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got := make(chan *SnapshotInfo, 1)
	client.SetSnapshotInfoHandler(func(info *SnapshotInfo) {
		got <- info
	})

	if err := client.Connect(server.Addr()); err != nil {
		t.Fatal(err)
	}
	// Let both read loops reach steady state.
	time.Sleep(100 * time.Millisecond)

	client.BroadcastSnapshotQuery()

	select {
	case info := <-got:
		if info.Height != 100 {
			t.Errorf("Height = %d, want 100", info.Height)
		}
		if info.SnapshotHash != cp.SnapshotHash {
			t.Errorf("SnapshotHash = %x, want %x", info.SnapshotHash, cp.SnapshotHash)
		}
		if info.StateRoot != cp.StateRoot {
			t.Errorf("StateRoot = %x, want %x (wire round-trip lost the root)", info.StateRoot, cp.StateRoot)
		}
		if info.Epoch != cp.Epoch {
			t.Errorf("Epoch = %d, want %d", info.Epoch, cp.Epoch)
		}
		if info.ChunkCount != uint32(len(chunks)) {
			t.Errorf("ChunkCount = %d, want %d", info.ChunkCount, len(chunks))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: snapshot query was never answered")
	}
}

// TestSnapshotServe_ChunkServed verifies the chunk path: every valid chunk of
// the advertised snapshot is served with its snapshot hash and chunk hash.
func TestSnapshotServe_ChunkServed(t *testing.T) {
	server, cp, _ := setupSnapshotServer(t)

	client, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got := make(chan *SnapshotChunk, 4)
	client.SetSnapshotChunkHandler(func(chunk *SnapshotChunk, peer string) {
		got <- chunk
	})

	if err := client.Connect(server.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Request both valid chunks.
	if err := client.RequestSnapshotChunk(server.Addr(), cp.SnapshotHash, 0); err != nil {
		t.Fatalf("request chunk 0: %v", err)
	}
	if err := client.RequestSnapshotChunk(server.Addr(), cp.SnapshotHash, 1); err != nil {
		t.Fatalf("request chunk 1: %v", err)
	}

	received := make(map[uint32]bool)
	deadline := time.After(2 * time.Second)
	for len(received) < 2 {
		select {
		case chunk := <-got:
			if chunk.SnapshotHash != cp.SnapshotHash {
				t.Errorf("chunk %d: SnapshotHash = %x, want %x", chunk.Index, chunk.SnapshotHash, cp.SnapshotHash)
			}
			if !state.VerifyChunk(&state.SnapshotChunk{
				Index:        chunk.Index,
				TotalCount:   chunk.TotalCount,
				SnapshotHash: chunk.SnapshotHash,
				ChunkHash:    chunk.ChunkHash,
				Data:         chunk.Data,
			}) {
				t.Errorf("chunk %d: chunk hash does not match data", chunk.Index)
			}
			received[chunk.Index] = true
		case <-deadline:
			t.Fatalf("timeout: received %d/2 chunks", len(received))
		}
	}

	if received[0] != true || received[1] != true {
		t.Fatalf("expected chunks 0 and 1, got %v", received)
	}
}

// TestSnapshotServe_UnknownChunkNoResponse verifies that a chunk request beyond
// the snapshot's chunk count is not answered and no state is corrupted.
func TestSnapshotServe_UnknownChunkNoResponse(t *testing.T) {
	server, cp, chunks := setupSnapshotServer(t)

	client, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got := make(chan *SnapshotChunk, 1)
	client.SetSnapshotChunkHandler(func(chunk *SnapshotChunk, peer string) {
		got <- chunk
	})

	if err := client.Connect(server.Addr()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Index beyond ChunkCount must not be served.
	outOfRange := uint32(len(chunks))
	if err := client.RequestSnapshotChunk(server.Addr(), cp.SnapshotHash, outOfRange); err != nil {
		t.Fatalf("request out-of-range chunk: %v", err)
	}

	select {
	case chunk := <-got:
		t.Fatalf("unexpected chunk %d served for out-of-range request", chunk.Index)
	case <-time.After(1 * time.Second):
		// Expected: no response for an unknown chunk.
	}
}
