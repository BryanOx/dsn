package network

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
)

// TestChunkScheduler_New

// TestChunkScheduler_New verifies NewChunkScheduler creates scheduler with correct initial state.
func TestChunkScheduler_New(t *testing.T) {
	scheduler := NewChunkScheduler(10)

	if scheduler == nil {
		t.Fatal("NewChunkScheduler returned nil")
	}

	// Verify all chunks start as not received
	if len(scheduler.received) != 10 {
		t.Errorf("received length = %d, want 10", len(scheduler.received))
	}

	// Verify progress is 0.0 initially
	if scheduler.Progress() != 0.0 {
		t.Errorf("Progress() = %v, want 0.0", scheduler.Progress())
	}
}

// TestChunkScheduler_MarkReceived verifies MarkReceived returns true first time, false second time.
func TestChunkScheduler_MarkReceived(t *testing.T) {
	scheduler := NewChunkScheduler(5)

	// First call should return true (first time marking)
	first := scheduler.MarkReceived(2)
	if !first {
		t.Error("MarkReceived(2) first call returned false, want true")
	}

	// Second call should return false (already marked)
	second := scheduler.MarkReceived(2)
	if second {
		t.Error("MarkReceived(2) second call returned true, want false")
	}

	// Mark another chunk
	third := scheduler.MarkReceived(0)
	if !third {
		t.Error("MarkReceived(0) returned false, want true")
	}

	// Verify 2 chunks received
	if scheduler.ReceivedCount() != 2 {
		t.Errorf("ReceivedCount() = %d, want 2", scheduler.ReceivedCount())
	}
}

// TestChunkScheduler_IsComplete verifies IsComplete returns correct state.
func TestChunkScheduler_IsComplete(t *testing.T) {
	scheduler := NewChunkScheduler(5)

	// Should not be complete before any chunks received
	if scheduler.IsComplete() {
		t.Error("IsComplete() returned true before any chunks received")
	}

	// Mark 4 chunks
	for i := uint32(0); i < 4; i++ {
		scheduler.MarkReceived(i)
	}

	// Still not complete
	if scheduler.IsComplete() {
		t.Error("IsComplete() returned true with 4/5 chunks received")
	}

	// Mark the last chunk
	scheduler.MarkReceived(4)

	// Now should be complete
	if !scheduler.IsComplete() {
		t.Error("IsComplete() returned false after all 5 chunks received")
	}
}

// TestChunkScheduler_Progress verifies Progress returns correct values.
func TestChunkScheduler_Progress(t *testing.T) {
	// Test: 0.0 initial with 6 chunks
	scheduler := NewChunkScheduler(6)
	if scheduler.Progress() != 0.0 {
		t.Errorf("Progress() = %v, want 0.0 (initial)", scheduler.Progress())
	}

	// Test: 0.5 at 3/6
	scheduler.MarkReceived(0)
	scheduler.MarkReceived(1)
	scheduler.MarkReceived(2)
	if scheduler.Progress() != 0.5 {
		t.Errorf("Progress() = %v, want 0.5 (3/6)", scheduler.Progress())
	}

	// Test: 1.0 at all received
	scheduler2 := NewChunkScheduler(3)
	scheduler2.MarkReceived(0)
	scheduler2.MarkReceived(1)
	scheduler2.MarkReceived(2)
	if scheduler2.Progress() != 1.0 {
		t.Errorf("Progress() = %v, want 1.0 (all)", scheduler2.Progress())
	}

	// Edge case: zero chunks
	scheduler3 := NewChunkScheduler(0)
	if scheduler3.Progress() != 0.0 {
		t.Errorf("Progress() = %v, want 0.0 (zero chunks)", scheduler3.Progress())
	}
}

// TestChunkScheduler_BitmapRestore verifies Bitmap export/import maintains state.
func TestChunkScheduler_BitmapRestore(t *testing.T) {
	// Create scheduler and mark some chunks
	scheduler := NewChunkScheduler(8)
	scheduler.MarkReceived(1)
	scheduler.MarkReceived(3)
	scheduler.MarkReceived(5)
	scheduler.MarkReceived(7)

	// Export bitmap
	bitmap := scheduler.Bitmap()
	if len(bitmap) != 8 {
		t.Errorf("Bitmap length = %d, want 8", len(bitmap))
	}

	// Create new scheduler and restore bitmap
	scheduler2 := NewChunkScheduler(8)
	scheduler2.RestoreBitmap(bitmap)

	// Verify state matches
	if !reflect.DeepEqual(scheduler.Bitmap(), scheduler2.Bitmap()) {
		t.Error("Restored bitmap does not match original")
	}

	// Verify progress is same
	if scheduler.Progress() != scheduler2.Progress() {
		t.Errorf("Progress after restore = %v, want %v", scheduler2.Progress(), scheduler.Progress())
	}

	// Verify IsComplete returns same value
	if scheduler.IsComplete() != scheduler2.IsComplete() {
		t.Error("IsComplete after restore does not match")
	}

	// Verify ReceivedCount returns same value
	if scheduler.ReceivedCount() != scheduler2.ReceivedCount() {
		t.Errorf("ReceivedCount after restore = %d, want %d", scheduler2.ReceivedCount(), scheduler.ReceivedCount())
	}
}

// TestChunkScheduler_AssignPeer verifies AssignPeer stores peer chunks correctly.
func TestChunkScheduler_AssignPeer(t *testing.T) {
	scheduler := NewChunkScheduler(10)

	peerID := PeerIDFromBytes([]byte("peer1"))

	// Assign chunks to peer
	scheduler.AssignPeer(peerID, []uint32{1, 3, 5, 7})

	// Verify chunks are assigned
	taskPeer, chunkIdx, ok := scheduler.NextTask()
	if !ok {
		t.Error("NextTask() returned false, expected true with assigned peer")
	}
	if taskPeer != peerID {
		t.Errorf("NextTask() returned peer %v, want %v", taskPeer, peerID)
	}
	if chunkIdx != 1 && chunkIdx != 3 && chunkIdx != 5 && chunkIdx != 7 {
		t.Errorf("NextTask() returned chunk %d, want one of {1,3,5,7}", chunkIdx)
	}
}

// TestChunkScheduler_NextTask verifies NextTask returns unassigned chunks from multiple peers.
func TestChunkScheduler_NextTask(t *testing.T) {
	scheduler := NewChunkScheduler(10)

	peer1 := PeerIDFromBytes([]byte("peer1"))
	peer2 := PeerIDFromBytes([]byte("peer2"))

	// Assign different chunks to each peer
	scheduler.AssignPeer(peer1, []uint32{0, 2, 4})
	scheduler.AssignPeer(peer2, []uint32{1, 3, 5})

	// Collect all returned tasks (with safety limit to prevent infinite loop)
	tasks := make(map[uint32]PeerID)
	for i := 0; i < 100; i++ {
		taskPeer, chunkIdx, ok := scheduler.NextTask()
		if !ok {
			break
		}
		tasks[chunkIdx] = taskPeer
	}

	// Verify we got 6 tasks
	if len(tasks) != 6 {
		t.Errorf("NextTask returned %d tasks, want 6", len(tasks))
	}

	// Verify peer1 got 0, 2, 4
	for _, chunk := range []uint32{0, 2, 4} {
		if tasks[chunk] != peer1 {
			t.Errorf("Chunk %d assigned to %v, want peer1", chunk, tasks[chunk])
		}
	}

	// Verify peer2 got 1, 3, 5
	for _, chunk := range []uint32{1, 3, 5} {
		if tasks[chunk] != peer2 {
			t.Errorf("Chunk %d assigned to %v, want peer2", chunk, tasks[chunk])
		}
	}

	// Test: NextTask returns unassigned chunks when no peers assigned
	scheduler2 := NewChunkScheduler(5)
	_, _, ok := scheduler2.NextTask()
	if ok {
		t.Error("NextTask() returned true with no peers assigned, want false")
	}

	// Assign a peer and verify it returns unassigned chunks
	peer3 := PeerIDFromBytes([]byte("peer3"))
	scheduler2.AssignPeer(peer3, []uint32{0}) // only assign chunk 0

	// Mark chunk 0 as received
	scheduler2.MarkReceived(0)

	// NextTask should return the unassigned chunks
	taskPeer, chunkIdx, ok := scheduler2.NextTask()
	if !ok {
		t.Error("NextTask() returned false, expected unassigned chunk")
	}
	if taskPeer != peer3 {
		t.Errorf("NextTask() returned peer %v, want %v", taskPeer, peer3)
	}
	if chunkIdx == 0 {
		t.Error("NextTask() returned chunk 0 which is already received")
	}
}

// TestFastSyncEngine_New verifies engine starts in SyncIdle.
func TestFastSyncEngine_New(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)
	if engine.State() != SyncIdle {
		t.Errorf("expected SyncIdle, got %v", engine.State())
	}
}

// TestFastSyncEngine_StateTransitions verifies state machine advances correctly.
func TestFastSyncEngine_StateTransitions(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)

	// Start begins at Querying
	engine.Start()
	if engine.State() != SyncQuerying {
		t.Errorf("after Start, expected SyncQuerying, got %v", engine.State())
	}

	// We can't test full state machine without running goroutines
	// Test that Advance transitions work correctly
}

// TestFastSyncEngine_HandleSnapshotInfo verifies snapshot info collection.
func TestFastSyncEngine_HandleSnapshotInfo(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)

	// Manually set state to Querying
	engine.mu.Lock()
	engine.state = SyncQuerying
	engine.mu.Unlock()

	// Inject a snapshot info
	info := &SnapshotInfo{
		Height:       100,
		SnapshotHash: [32]byte{1, 2, 3},
		Epoch:        5,
		Timestamp:    1000,
		ChunkCount:   10,
	}
	engine.HandleSnapshotInfo(info)

	// Verify it was collected
	engine.mu.RLock()
	collected := len(engine.collectedInfos)
	engine.mu.RUnlock()

	if collected != 1 {
		t.Errorf("collected %d infos, want 1", collected)
	}

	// Duplicate hash should be ignored
	info2 := &SnapshotInfo{
		Height:       200,
		SnapshotHash: [32]byte{1, 2, 3}, // same hash
		Epoch:        5,
		Timestamp:    2000,
		ChunkCount:   20,
	}
	engine.HandleSnapshotInfo(info2)

	engine.mu.RLock()
	collected = len(engine.collectedInfos)
	engine.mu.RUnlock()

	if collected != 1 {
		t.Errorf("after duplicate hash, collected %d infos, want 1", collected)
	}
}

// TestFastSyncEngine_HandleSnapshotInfoWrongState verifies info is ignored in wrong state.
func TestFastSyncEngine_HandleSnapshotInfoWrongState(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)
	// Don't change state — stays at SyncIdle

	info := &SnapshotInfo{
		Height:       100,
		SnapshotHash: [32]byte{1, 2, 3},
		Epoch:        5,
		Timestamp:    1000,
		ChunkCount:   10,
	}
	engine.HandleSnapshotInfo(info)

	engine.mu.RLock()
	collected := len(engine.collectedInfos)
	engine.mu.RUnlock()

	if collected != 0 {
		t.Errorf("collected %d infos in wrong state, want 0", collected)
	}
}

// TestFastSyncEngine_SelectBestSnapshot verifies snapshot selection logic.
func TestFastSyncEngine_SelectBestSnapshot(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)

	// Inject multiple snapshots
	engine.collectedInfos = []SnapshotInfo{
		{Height: 100, SnapshotHash: [32]byte{1}, Epoch: 5, Timestamp: 1000, ChunkCount: 10},
		{Height: 200, SnapshotHash: [32]byte{2}, Epoch: 5, Timestamp: 2000, ChunkCount: 20},
		{Height: 150, SnapshotHash: [32]byte{3}, Epoch: 5, Timestamp: 1500, ChunkCount: 15},
	}

	engine.selectBestSnapshot()

	if engine.selectedSnapshot == nil {
		t.Fatal("selectedSnapshot is nil")
	}
	if engine.selectedSnapshot.Height != 200 {
		t.Errorf("selected height = %d, want 200 (highest)", engine.selectedSnapshot.Height)
	}

	// Verify scheduler created with correct chunk count
	if engine.scheduler == nil {
		t.Fatal("scheduler is nil after selection")
	}
	if engine.scheduler.totalChunks != 20 {
		t.Errorf("scheduler totalChunks = %d, want 20", engine.scheduler.totalChunks)
	}
}

// TestFastSyncEngine_SelectBestSnapshotRejectsZeroHash verifies zero-hash snapshots are ignored.
func TestFastSyncEngine_SelectBestSnapshotRejectsZeroHash(t *testing.T) {
	pm := NewPeerManager(nil)
	p2p, err := NewP2PNode(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p2p.Close()

	engine := NewFastSyncEngine(pm, p2p, nil)

	// All snapshots have zero hash
	engine.collectedInfos = []SnapshotInfo{
		{Height: 100, SnapshotHash: [32]byte{}, Epoch: 5, Timestamp: 1000, ChunkCount: 10},
		{Height: 200, SnapshotHash: [32]byte{}, Epoch: 5, Timestamp: 2000, ChunkCount: 20},
	}

	engine.selectBestSnapshot()

	if engine.selectedSnapshot != nil {
		t.Error("selectedSnapshot should be nil when all have zero hash")
	}
}

// storeChunkedSnapshot persists a snapshot at the given height and returns its
// serialized data and checkpoint. The payload is big enough to split into 2
// chunks at the default chunk size.
func storeChunkedSnapshot(t *testing.T, ps *state.PersistentState, height uint64) ([]byte, *state.Checkpoint) {
	t.Helper()

	src := state.NewInMemoryState(types.SHA256Hasher{})
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
		t.Fatalf("storeChunkedSnapshot: commit source state: %v", err)
	}

	snap, err := src.CreateSnapshot(height, 2, types.Hash{0xAA}, 1000000)
	if err != nil {
		t.Fatalf("storeChunkedSnapshot: create snapshot: %v", err)
	}
	snapData, err := state.SerializeSnapshot(snap)
	if err != nil {
		t.Fatalf("storeChunkedSnapshot: serialize snapshot: %v", err)
	}
	if err := state.StoreSnapshot(ps, height, snapData); err != nil {
		t.Fatalf("storeChunkedSnapshot: store snapshot: %v", err)
	}

	cp := &state.Checkpoint{
		Height:           height,
		BlockHash:        types.Hash{0xBB},
		StateRoot:        root,
		SnapshotHash:     state.SnapshotHash(snapData),
		ValidatorSetHash: types.Hash{0xAA},
		Epoch:            2,
		Timestamp:        1000000,
	}
	if err := state.StoreCheckpoint(ps, cp); err != nil {
		t.Fatalf("storeChunkedSnapshot: store checkpoint: %v", err)
	}
	return snapData, cp
}

// TestFastSyncEngine_ServeSnapshotCachesLatest verifies the latest-only
// snapshot serve cache (8.3): the first ServeSnapshot call computes chunks
// from the stored snapshot, subsequent calls reuse the cached result, and the
// cache is recomputed once the on-disk checkpoint moves to a newer snapshot.
func TestFastSyncEngine_ServeSnapshotCachesLatest(t *testing.T) {
	ps, err := state.NewPersistentState(filepath.Join(t.TempDir(), "node.db"), types.SHA256Hasher{}, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ps.Close() })

	engine := NewFastSyncEngine(nil, nil, ps)

	snapData, cp := storeChunkedSnapshot(t, ps, 100)

	info, chunks := engine.ServeSnapshot()
	if info == nil {
		t.Fatal("ServeSnapshot returned nil for a stored snapshot")
	}
	if info.Height != 100 || info.SnapshotHash != cp.SnapshotHash || info.ChunkCount != 2 {
		t.Fatalf("ServeSnapshot info mismatch: got height %d chunks %d hash %x", info.Height, info.ChunkCount, info.SnapshotHash)
	}
	if len(chunks) != 2 {
		t.Fatalf("ServeSnapshot returned %d chunks, want 2", len(chunks))
	}
	want, err := state.ChunkSnapshot(snapData, state.DefaultChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	for i := range chunks {
		if chunks[i].Data == nil || !bytesEqual(chunks[i].Data, want[i].Data) || chunks[i].SnapshotHash != cp.SnapshotHash {
			t.Fatalf("chunk %d data/hash mismatch", i)
		}
	}

	// Cache hit: a second call must not recompute chunks from disk.
	info2, chunks2 := engine.ServeSnapshot()
	if info2.Height != 100 || len(chunks2) != 2 {
		t.Fatalf("second ServeSnapshot did not return the cached snapshot: %+v", info2)
	}
	if got := engine.serveChunkComputations; got != 1 {
		t.Fatalf("expected 1 chunk computation after two calls, got %d (cache not honored)", got)
	}

	// Invalidation: storing a newer snapshot must recompute and serve it.
	storeChunkedSnapshot(t, ps, 101)
	info3, chunks3 := engine.ServeSnapshot()
	if info3.Height != 101 || len(chunks3) != 2 {
		t.Fatalf("ServeSnapshot did not switch to the newer snapshot: %+v", info3)
	}
	if got := engine.serveChunkComputations; got != 2 {
		t.Fatalf("expected recomputation on checkpoint change, got %d computations", got)
	}
}

// bytesEqual compares byte slices without relying on reflect.DeepEqual.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
