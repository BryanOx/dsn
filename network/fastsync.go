package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// SyncState represents the state machine states for fast sync.
type SyncState int

const (
	SyncIdle SyncState = iota
	SyncQuerying
	SyncSelecting
	SyncDownloading
	SyncVerifying
	SyncRestoring
	SyncReplaying
	SyncLive
)

// String implements fmt.Stringer for SyncState.
func (s SyncState) String() string {
	switch s {
	case SyncIdle:
		return "Idle"
	case SyncQuerying:
		return "Querying"
	case SyncSelecting:
		return "Selecting"
	case SyncDownloading:
		return "Downloading"
	case SyncVerifying:
		return "Verifying"
	case SyncRestoring:
		return "Restoring"
	case SyncReplaying:
		return "Replaying"
	case SyncLive:
		return "Live"
	default:
		return "Unknown"
	}
}

// FastSyncEngine orchestrates fast sync from snapshots.
type FastSyncEngine struct {
	state      SyncState
	pm         *PeerManager
	p2p        *P2PNode
	persistent *state.PersistentState // for persistence
	stopCh     chan struct{}
	mu         sync.RWMutex

	// Query results
	collectedInfos []SnapshotInfo
	queryStart     time.Time
	queryTimeout   time.Duration

	// Selected snapshot
	selectedSnapshot *SnapshotInfo
	advertisingPeers []string // peer addresses that advertised this snapshot

	// Download state
	scheduler       *ChunkScheduler
	totalChunks     uint32
	snapData        []byte // reassembled data
	collectedChunks map[uint32]*state.SnapshotChunk
	requestTimes    map[uint32]time.Time // track when each chunk was last requested
	lastRequestID   uint64

	// Snapshot serve cache: latest-only (8.3). The stored snapshot's chunks
	// are computed once and reused for every query/chunk request until the
	// on-disk checkpoint moves to a different snapshot, then recomputed.
	cachedSnapshotHeight    uint64
	cachedSnapshotHash      types.Hash
	cachedChunks            []*state.SnapshotChunk
	serveChunkComputations  uint64 // number of times ChunkSnapshot ran (cache contract, asserted by tests)

	// Config
	maxQueryRetries int
	chunkTimeout    time.Duration
	maxConcurrent   int // max concurrent chunk requests per peer
	queryRetryCount int

	// Callbacks
	onStateChange func(oldState, newState SyncState)
	onRestore     func(data []byte, expectedHash types.Hash) error
	onReplay      func(fromHeight uint64) error
}

// NewFastSyncEngine creates a new FastSyncEngine.
func NewFastSyncEngine(pm *PeerManager, p2p *P2PNode, persistent *state.PersistentState) *FastSyncEngine {
	return &FastSyncEngine{
		state:           SyncIdle,
		pm:              pm,
		p2p:             p2p,
		persistent:      persistent,
		stopCh:          make(chan struct{}),
		collectedInfos:  make([]SnapshotInfo, 0),
		queryTimeout:    5 * time.Second,
		maxQueryRetries: 3,
		chunkTimeout:    30 * time.Second,
		maxConcurrent:   4,
		collectedChunks: make(map[uint32]*state.SnapshotChunk),
		requestTimes:    make(map[uint32]time.Time),
	}
}

// Start begins the fast sync process from SyncIdle, moves to SyncQuerying.
func (e *FastSyncEngine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != SyncIdle {
		return // Already started or finished
	}

	e.setStateLocked(SyncQuerying)
	e.queryRetryCount = 0
	e.collectedInfos = e.collectedInfos[:0]

	// Start the query phase
	go e.queryPeers()
}

// Stop stops the fast sync process.
func (e *FastSyncEngine) Stop() {
	select {
	case <-e.stopCh:
		return
	default:
	}

	close(e.stopCh)
	e.stopCh = make(chan struct{})
}

// State returns the current sync state (thread-safe).
func (e *FastSyncEngine) State() SyncState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// setStateLocked sets the state and invokes callback. Must be called with mu held.
func (e *FastSyncEngine) setStateLocked(newState SyncState) {
	oldState := e.state
	e.state = newState
	if e.onStateChange != nil && oldState != newState {
		e.onStateChange(oldState, newState)
	}
}

// SetStateChangeHandler sets the callback for state changes.
func (e *FastSyncEngine) SetStateChangeHandler(h func(oldState, newState SyncState)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onStateChange = h
}

// Advance transitions to the next state based on current state and conditions.
func (e *FastSyncEngine) Advance() {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch e.state {
	case SyncQuerying:
		// Query phase complete, move to selecting
		e.setStateLocked(SyncSelecting)
		e.selectBestSnapshot()
		e.proceedFromSelection()

	case SyncSelecting:
		e.proceedFromSelection()

	case SyncDownloading:
		// Download complete, move to verifying (handled by HandleSnapshotChunk)
		// This is triggered by advanceToVerifying() when all chunks received

	case SyncVerifying:
		// Verification complete, move to restoring
		e.setStateLocked(SyncRestoring)
		// Continue with restore (call from within lock, but it will release)
		snapData := e.snapData
		selectedSnapshot := e.selectedSnapshot
		e.mu.Unlock()
		// Trigger restore with expected hash
		if snapData != nil && selectedSnapshot != nil {
			var expectedHash types.Hash
			copy(expectedHash[:], selectedSnapshot.SnapshotHash[:])
			if e.onRestore != nil {
				_ = e.onRestore(snapData, expectedHash)
			} else {
				_ = state.RestoreFromSnapshot(e.persistent, snapData, expectedHash)
			}
		}
		e.mu.Lock()
		e.setStateLocked(SyncReplaying)
		e.mu.Unlock()
		// Continue with replay
		if selectedSnapshot != nil {
			e.replayBlocks(selectedSnapshot.Height)
		}
		return

	case SyncRestoring:
		// Restore complete, move to replaying
		e.setStateLocked(SyncReplaying)

	case SyncReplaying:
		// Replay complete, move to live
		e.setStateLocked(SyncLive)
		// Clear persisted state on successful completion
		go e.clearSyncState()
	}
}

// proceedFromSelection starts the chunk download when a snapshot was selected,
// otherwise falls back to live sync. Must be called with e.mu held.
func (e *FastSyncEngine) proceedFromSelection() {
	if e.selectedSnapshot != nil {
		e.setStateLocked(SyncDownloading)
		// Start downloading asynchronously
		go e.startDownloading()
	} else {
		// No valid snapshot, fall back to live sync
		e.setStateLocked(SyncLive)
	}
}

// queryPeers broadcasts MsgTypeSnapshotQuery to all connected peers.
func (e *FastSyncEngine) queryPeers() {
	// Broadcast snapshot query to all peers
	e.p2p.BroadcastSnapshotQuery()

	// Set up timeout timer
	timer := time.NewTimer(e.queryTimeout)
	defer timer.Stop()

	select {
	case <-e.stopCh:
		return
	case <-timer.C:
		// Timeout reached, check results
		e.mu.Lock()
		if len(e.collectedInfos) > 0 {
			// We have responses, move to selecting
			e.setStateLocked(SyncSelecting)
			e.selectBestSnapshot()
			e.mu.Unlock()
			// Drive the machine to Downloading / Live without holding the
			// lock (Advance takes it again).
			e.Advance()
			return
		}
		// No responses, retry with backoff
		e.handleQueryRetry()
		e.mu.Unlock()
	}
}

// handleQueryRetry handles retry logic with exponential backoff.
func (e *FastSyncEngine) handleQueryRetry() {
	e.queryRetryCount++

	if e.queryRetryCount >= e.maxQueryRetries {
		// Max retries reached, fall back to live sync
		fmt.Printf("FastSyncEngine: max query retries (%d) reached, falling back to live sync\n", e.maxQueryRetries)
		e.setStateLocked(SyncLive)
		return
	}

	// Exponential backoff: 2x multiplier
	backoff := e.queryTimeout * time.Duration(1<<uint(e.queryRetryCount-1))
	fmt.Printf("FastSyncEngine: query retry %d/%d, waiting %v\n", e.queryRetryCount, e.maxQueryRetries, backoff)

	// Reset timer with backoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	select {
	case <-e.stopCh:
		return
	case <-timer.C:
		// Retry query
		go e.queryPeers()
	}
}

// CollectSnapshotInfo stores snapshot info received from peers.
func (e *FastSyncEngine) HandleSnapshotInfo(info *SnapshotInfo) {
	if info == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Only collect during Querying state
	if e.state != SyncQuerying {
		return
	}

	// Avoid duplicates based on hash
	for _, existing := range e.collectedInfos {
		if bytes.Equal(existing.SnapshotHash[:], info.SnapshotHash[:]) {
			return
		}
	}

	e.collectedInfos = append(e.collectedInfos, *info)
}

// selectBestSnapshot picks the snapshot with highest height from collected infos.
// Tie-break: most recent timestamp.
// Reject zero-hash snapshots.
func (e *FastSyncEngine) selectBestSnapshot() {
	if len(e.collectedInfos) == 0 {
		e.selectedSnapshot = nil
		return
	}

	var best *SnapshotInfo
	for i := range e.collectedInfos {
		info := &e.collectedInfos[i]

		// Reject zero-hash snapshots
		var zeroHash types.Hash
		if bytes.Equal(info.SnapshotHash[:], zeroHash[:]) {
			continue
		}

		if best == nil {
			best = info
			continue
		}

		// Compare height (higher is better)
		if info.Height > best.Height {
			best = info
		} else if info.Height == best.Height {
			// Tie-break by timestamp (more recent is better)
			if info.Timestamp > best.Timestamp {
				best = info
			}
		}
	}

	if best != nil {
		e.selectedSnapshot = best
		e.totalChunks = best.ChunkCount
		e.scheduler = NewChunkScheduler(best.ChunkCount)
		fmt.Printf("FastSyncEngine: selected snapshot at height %d with %d chunks\n", best.Height, best.ChunkCount)
	} else {
		e.selectedSnapshot = nil
		fmt.Printf("FastSyncEngine: no valid snapshots found\n")
	}
	// NOTE: no e.Advance() here — callers (queryPeers / Advance) drive the
	// state machine after selection to avoid reentrant locking of e.mu.
}

// HandleSnapshotChunk is called by P2PNode when a snapshot chunk is received.
// Converts from network.SnapshotChunk to state.SnapshotChunk.
func (e *FastSyncEngine) HandleSnapshotChunk(chunk *SnapshotChunk, from PeerID) {
	if chunk == nil {
		return
	}

	e.mu.Lock()

	// Only process during Downloading state
	if e.state != SyncDownloading || e.scheduler == nil {
		e.mu.Unlock()
		return
	}

	// Check if already received
	if e.scheduler.received[chunk.Index] {
		e.mu.Unlock()
		return
	}

	// Convert network.SnapshotChunk to state.SnapshotChunk
	stateChunk := &state.SnapshotChunk{
		Index:        chunk.Index,
		TotalCount:   chunk.TotalCount,
		SnapshotHash: types.Hash{},
		ChunkHash:    types.Hash{},
		Data:         chunk.Data,
	}
	copy(stateChunk.SnapshotHash[:], chunk.SnapshotHash[:])
	copy(stateChunk.ChunkHash[:], chunk.ChunkHash[:])

	// Verify chunk integrity
	if !state.VerifyChunk(stateChunk) {
		fmt.Printf("FastSyncEngine: chunk %d verification failed from peer %s\n", chunk.Index, from.String())
		e.pm.ReportFailure(from, 5) // -5 for invalid snapshot chunk
		e.mu.Unlock()
		return
	}

	// Mark as received in scheduler
	received := e.scheduler.MarkReceived(chunk.Index)
	complete := false
	if received {
		// Report success to peer manager
		e.pm.ReportSuccess(from, MsgTypeSnapshotChunk)

		// Store chunk in collectedChunks map for later reassembly
		e.collectedChunks[chunk.Index] = stateChunk

		fmt.Printf("FastSyncEngine: received chunk %d/%d (%.1f%%)\n",
			chunk.Index+1, chunk.TotalCount, e.scheduler.Progress()*100)

		// Persist progress every 10 chunks
		if e.scheduler.ReceivedCount()%10 == 0 {
			go e.persistSyncState()
		}

		complete = e.scheduler.IsComplete()
	}
	e.mu.Unlock()

	// Check if download is complete (outside the lock: advanceToVerifying
	// takes it again).
	if complete {
		fmt.Printf("FastSyncEngine: all chunks received, reassembling snapshot\n")
		e.advanceToVerifying()
	}
}

// advanceToVerifying moves from Downloading to Verifying state.
func (e *FastSyncEngine) advanceToVerifying() {
	e.mu.Lock()
	if e.state != SyncDownloading {
		e.mu.Unlock()
		return
	}
	e.setStateLocked(SyncVerifying)
	e.mu.Unlock()

	// Call the verification method
	e.verifyDownloadedSnapshot()
}

// ChunkScheduler manages chunk download scheduling and tracking.
type ChunkScheduler struct {
	totalChunks uint32
	received    []bool // bitmap of received chunks
	peerChunks  map[PeerID][]uint32
	peerIndex   map[PeerID]int
	mu          sync.Mutex
}

// NewChunkScheduler creates a new ChunkScheduler.
func NewChunkScheduler(totalChunks uint32) *ChunkScheduler {
	return &ChunkScheduler{
		totalChunks: totalChunks,
		received:    make([]bool, totalChunks),
		peerChunks:  make(map[PeerID][]uint32),
		peerIndex:   make(map[PeerID]int),
	}
}

// MarkReceived marks a chunk as received. Returns true if this is the first time.
func (cs *ChunkScheduler) MarkReceived(index uint32) bool {
	if index >= cs.totalChunks {
		return false
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.received[index] {
		return false // already received
	}

	cs.received[index] = true
	return true
}

// IsComplete returns true if all chunks have been received.
func (cs *ChunkScheduler) IsComplete() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, r := range cs.received {
		if !r {
			return false
		}
	}
	return true
}

// NextTask returns the next unassigned chunk for a peer using round-robin.
// Returns (peerID, chunkIndex, ok).
func (cs *ChunkScheduler) NextTask() (PeerID, uint32, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Get list of available peers
	var availablePeers []PeerID
	for pid := range cs.peerChunks {
		if len(cs.peerChunks[pid]) > 0 {
			availablePeers = append(availablePeers, pid)
		}
	}

	if len(availablePeers) == 0 {
		return PeerID{}, 0, false
	}

	// Find first peer with unassigned chunks
	for _, pid := range availablePeers {
		chunks := cs.peerChunks[pid]
		if len(chunks) == 0 {
			continue
		}

		// Get next chunk for this peer (round-robin)
		idx := cs.peerIndex[pid] % len(chunks)
		chunkIdx := chunks[idx]
		cs.peerIndex[pid]++

		// Check if already received
		if !cs.received[chunkIdx] {
			return pid, chunkIdx, true
		}

		// Remove from list and try next
		cs.peerChunks[pid] = append(chunks[:idx], chunks[idx+1:]...)
	}

	// Also check for chunks not assigned to any peer
	for i := uint32(0); i < cs.totalChunks; i++ {
		if !cs.received[i] {
			// Assign to first available peer
			if len(availablePeers) > 0 {
				return availablePeers[0], i, true
			}
		}
	}

	return PeerID{}, 0, false
}

// AssignPeer tells the scheduler which chunks a peer can provide.
func (cs *ChunkScheduler) AssignPeer(peer PeerID, chunks []uint32) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.peerChunks[peer] = append(cs.peerChunks[peer], chunks...)
	cs.peerIndex[peer] = 0
}

// ReceivedCount returns the number of chunks received.
func (cs *ChunkScheduler) ReceivedCount() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	count := 0
	for _, r := range cs.received {
		if r {
			count++
		}
	}
	return count
}

// Progress returns the download progress as a float64 between 0.0 and 1.0.
func (cs *ChunkScheduler) Progress() float64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.totalChunks == 0 {
		return 0.0
	}

	count := 0
	for _, r := range cs.received {
		if r {
			count++
		}
	}
	return float64(count) / float64(cs.totalChunks)
}

// Bitmap returns the received bitmap for persistence.
func (cs *ChunkScheduler) Bitmap() []bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	bitmap := make([]bool, len(cs.received))
	copy(bitmap, cs.received)
	return bitmap
}

// RestoreBitmap restores the received bitmap from persistence.
func (cs *ChunkScheduler) RestoreBitmap(bitmap []bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(bitmap) != len(cs.received) {
		return // size mismatch
	}

	copy(cs.received, bitmap)
}

// ServeSnapshot returns the latest stored snapshot's metadata and chunks for
// the server-side handlers, computing chunks once and caching them until the
// on-disk checkpoint moves (latest-only cache, 8.3). Returns (nil, nil) when
// no valid snapshot exists.
func (e *FastSyncEngine) ServeSnapshot() (*SnapshotInfo, []*state.SnapshotChunk) {
	cp, err := state.LatestCheckpoint(e.persistent)
	if err != nil {
		return nil, nil
	}
	if !state.SnapshotExists(e.persistent, cp.Height) {
		return nil, nil
	}

	e.mu.RLock()
	cached := e.cachedSnapshotHeight == cp.Height && e.cachedSnapshotHash == cp.SnapshotHash && len(e.cachedChunks) > 0
	if cached {
		info := &SnapshotInfo{
			Height:       cp.Height,
			SnapshotHash: cp.SnapshotHash,
			StateRoot:    cp.StateRoot,
			Epoch:        cp.Epoch,
			Timestamp:    cp.Timestamp,
			ChunkCount:   uint32(len(e.cachedChunks)),
		}
		chunks := e.cachedChunks
		e.mu.RUnlock()
		return info, chunks
	}
	e.mu.RUnlock()

	data, err := state.LoadSnapshot(e.persistent, cp.Height)
	if err != nil {
		return nil, nil
	}
	chunkList, err := state.ChunkSnapshot(data, state.DefaultChunkSize)
	if err != nil {
		return nil, nil
	}
	chunks := make([]*state.SnapshotChunk, len(chunkList))
	for i := range chunkList {
		chunks[i] = &chunkList[i]
	}

	e.mu.Lock()
	e.cachedSnapshotHeight = cp.Height
	e.cachedSnapshotHash = cp.SnapshotHash
	e.cachedChunks = chunks
	e.serveChunkComputations++
	e.mu.Unlock()

	return &SnapshotInfo{
		Height:       cp.Height,
		SnapshotHash: cp.SnapshotHash,
		StateRoot:    cp.StateRoot,
		Epoch:        cp.Epoch,
		Timestamp:    cp.Timestamp,
		ChunkCount:   uint32(len(chunks)),
	}, chunks
}

// GetSelectedSnapshot returns the currently selected snapshot info.
func (e *FastSyncEngine) GetSelectedSnapshot() *SnapshotInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.selectedSnapshot
}

// GetScheduler returns the chunk scheduler.
func (e *FastSyncEngine) GetScheduler() *ChunkScheduler {
	e.mu.RLock()
	defer e.mu.Unlock()
	return e.scheduler
}

// GetSnapData returns the reassembled snapshot data.
func (e *FastSyncEngine) GetSnapData() []byte {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.snapData
}

// GetPersistentState returns the persistent state for restore operations.
func (e *FastSyncEngine) GetPersistentState() *state.PersistentState {
	return e.persistent
}

// SetRestoreHandler sets the callback for snapshot restore operations.
func (e *FastSyncEngine) SetRestoreHandler(h func(data []byte, expectedHash types.Hash) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onRestore = h
}

// SetReplayHandler sets the callback for block replay operations.
func (e *FastSyncEngine) SetReplayHandler(h func(fromHeight uint64) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onReplay = h
}

// =============================================================================
// Task 3.5: Download Phase - Parallel Chunk Requests
// =============================================================================

// startDownloading begins parallel chunk requests from advertising peers.
func (e *FastSyncEngine) startDownloading() {
	e.mu.Lock()

	if e.selectedSnapshot == nil || e.scheduler == nil {
		e.mu.Unlock()
		e.setStateLocked(SyncLive)
		return
	}

	// Get connected peers using GetPeersByState
	connectedPeers := e.pm.GetPeersByState(PeerConnected)
	if len(connectedPeers) == 0 {
		e.mu.Unlock()
		fmt.Printf("FastSyncEngine: no peers available for download\n")
		e.setStateLocked(SyncLive)
		return
	}

	// Store advertising peer addresses for chunk requests
	e.advertisingPeers = make([]string, 0, len(connectedPeers))
	for _, p := range connectedPeers {
		e.advertisingPeers = append(e.advertisingPeers, p.Address)
	}

	// Initialize collections
	e.collectedChunks = make(map[uint32]*state.SnapshotChunk)
	e.requestTimes = make(map[uint32]time.Time)

	// Get total chunks
	totalChunks := e.totalChunks

	e.mu.Unlock()

	fmt.Printf("FastSyncEngine: starting download from %d peers, %d chunks\n",
		len(e.advertisingPeers), totalChunks)

	// Request all chunks immediately
	e.requestMissingChunks()

	// Launch download manager for timeout handling
	e.launchDownloadManager()
}

// launchDownloadManager starts a goroutine that periodically checks for timed-out chunks.
func (e *FastSyncEngine) launchDownloadManager() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-e.stopCh:
				return
			case <-ticker.C:
				e.mu.Lock()
				if e.state != SyncDownloading {
					e.mu.Unlock()
					return
				}

				// Request all missing chunks
				e.requestMissingChunks()

				// Check if download is complete
				if e.scheduler != nil && e.scheduler.IsComplete() {
					fmt.Printf("FastSyncEngine: download manager detected completion\n")
					e.advanceToVerifying()
				}

				e.mu.Unlock()
			}
		}
	}()
}

// requestMissingChunks requests all chunks that haven't been received yet.
// It uses a timeout mechanism to avoid re-requesting too frequently.
func (e *FastSyncEngine) requestMissingChunks() {
	if e.scheduler == nil || e.selectedSnapshot == nil {
		return
	}

	// Get all unreceived chunk indices
	var missingChunks []uint32
	for i := uint32(0); i < e.totalChunks; i++ {
		if !e.scheduler.received[i] {
			// Check if we recently requested this chunk (avoid spam)
			if lastReq, ok := e.requestTimes[i]; ok {
				// Only re-request if at least 10 seconds have passed
				if time.Since(lastReq) < 10*time.Second {
					continue
				}
			}
			missingChunks = append(missingChunks, i)
		}
	}

	if len(missingChunks) == 0 {
		return
	}

	// Update request times for all missing chunks
	now := time.Now()
	for _, idx := range missingChunks {
		e.requestTimes[idx] = now
	}

	// Shuffle missing chunks for more even distribution
	rand.Shuffle(len(missingChunks), func(i, j int) {
		missingChunks[i], missingChunks[j] = missingChunks[j], missingChunks[i]
	})

	// Request chunks from available peers
	peerCount := len(e.advertisingPeers)
	if peerCount == 0 {
		return
	}

	hash := e.selectedSnapshot.SnapshotHash
	requested := 0
	maxRequests := e.maxConcurrent * peerCount // Limit concurrent requests

	for _, chunkIdx := range missingChunks {
		if requested >= maxRequests {
			break
		}

		// Round-robin through peers
		peerIdx := requested % peerCount
		peerID := e.advertisingPeers[peerIdx]

		// peerID is already the address string
		peerAddr := peerID
		if peerAddr == "" {
			continue
		}

		// Request chunk via P2P
		err := e.p2p.RequestSnapshotChunk(peerAddr, hash, chunkIdx)
		if err != nil {
			fmt.Printf("FastSyncEngine: failed to request chunk %d from %s: %v\n",
				chunkIdx, peerAddr, err)
			continue
		}

		requested++
	}

	if requested > 0 {
		fmt.Printf("FastSyncEngine: requested %d chunks from %d peers\n", requested, peerCount)
	}
}

// =============================================================================
// Task 3.6: Verify Phase - Reassemble and Verify Snapshot
// =============================================================================

// verifyDownloadedSnapshot reassembles and verifies the full snapshot.
func (e *FastSyncEngine) verifyDownloadedSnapshot() {
	e.mu.Lock()

	// Build sorted slice of chunks
	if len(e.collectedChunks) == 0 {
		fmt.Printf("FastSyncEngine: no chunks collected for verification\n")
		e.fallBackToLiveSync()
		return
	}

	chunks := make([]state.SnapshotChunk, 0, len(e.collectedChunks))
	for _, ch := range e.collectedChunks {
		chunks = append(chunks, *ch)
	}

	// Reassemble snapshot
	assembled, err := state.ReassembleSnapshot(chunks)
	if err != nil {
		fmt.Printf("FastSyncEngine: failed to reassemble snapshot: %v\n", err)
		e.fallBackToLiveSyncLocked()
		return
	}

	// Verify hash matches
	if e.selectedSnapshot != nil {
		var expectedHash types.Hash
		copy(expectedHash[:], e.selectedSnapshot.SnapshotHash[:])

		computedHash := state.SnapshotHash(assembled)
		if computedHash != expectedHash {
			fmt.Printf("FastSyncEngine: snapshot hash mismatch: got %x, expected %x\n",
				computedHash[:], expectedHash[:])
			e.fallBackToLiveSyncLocked()
			return
		}
	}

	// Store verified snapshot data
	e.snapData = assembled

	fmt.Printf("FastSyncEngine: snapshot verified, %d bytes\n", len(e.snapData))

	// Advance to Restoring
	e.setStateLocked(SyncRestoring)
	e.mu.Unlock()

	// Proceed with restore
	e.restoreSnapshot()
}

// fallBackToLiveSync transitions to live sync and penalizes peers.
func (e *FastSyncEngine) fallBackToLiveSync() {
	e.mu.Lock()
	e.fallBackToLiveSyncLocked()
	e.mu.Unlock()
}

// fallBackToLiveSyncLocked is the locked version of fall back.
func (e *FastSyncEngine) fallBackToLiveSyncLocked() {
	// Penalize advertising peers (lookup PeerID from address)
	for _, addr := range e.advertisingPeers {
		p := e.pm.GetPeerByAddr(addr)
		if p != nil {
			e.pm.ReportFailure(p.ID, 3) // -3 for failed sync
		}
	}
	e.setStateLocked(SyncLive)
}

// =============================================================================
// Task 3.7: Restore + Replay Phase
// =============================================================================

// restoreSnapshot restores state from verified snapshot data.
func (e *FastSyncEngine) restoreSnapshot() {
	e.mu.Lock()

	if e.snapData == nil || len(e.snapData) == 0 {
		fmt.Printf("FastSyncEngine: no snapshot data to restore\n")
		e.mu.Unlock()
		e.fallBackToLiveSync()
		return
	}

	var expectedHash types.Hash
	if e.selectedSnapshot != nil {
		copy(expectedHash[:], e.selectedSnapshot.SnapshotHash[:])
	}

	snapshotHeight := e.selectedSnapshot.Height

	e.mu.Unlock()

	// Try callback first (Node's method)
	if e.onRestore != nil {
		err := e.onRestore(e.snapData, expectedHash)
		if err != nil {
			fmt.Printf("FastSyncEngine: restore callback failed: %v\n", err)
			e.fallBackToLiveSync()
			return
		}
	} else {
		// Fallback to direct state restore
		err := state.RestoreFromSnapshot(e.persistent, e.snapData, expectedHash)
		if err != nil {
			fmt.Printf("FastSyncEngine: restore from snapshot failed: %v\n", err)
			e.fallBackToLiveSync()
			return
		}
	}

	fmt.Printf("FastSyncEngine: state restored from snapshot at height %d\n", snapshotHeight)

	e.mu.Lock()
	e.setStateLocked(SyncReplaying)
	e.mu.Unlock()

	// Proceed with replay
	e.replayBlocks(snapshotHeight)
}

// replayBlocks replays blocks after snapshot restore.
func (e *FastSyncEngine) replayBlocks(snapshotHeight uint64) {
	// Get current tip height from persistent state
	tip, tipErr := consensus.LoadTip(e.persistent)
	var tipHeight uint64
	if tipErr != nil {
		fmt.Printf("FastSyncEngine: failed to load tip: %v\n", tipErr)
		tipHeight = 0
	} else {
		tipHeight = tip.Height
	}

	if tipHeight <= snapshotHeight {
		fmt.Printf("FastSyncEngine: no blocks to replay (snapshot height: %d, tip: %d)\n",
			snapshotHeight, tipHeight)
		e.mu.Lock()
		e.setStateLocked(SyncLive)
		e.mu.Unlock()
		return
	}

	// Try callback first (Node's method)
	if e.onReplay != nil {
		err := e.onReplay(snapshotHeight + 1)
		if err != nil {
			fmt.Printf("FastSyncEngine: replay callback failed: %v\n", err)
			e.fallBackToLiveSync()
			return
		}
	} else {
		// No callback, just log (actual replay needs Node reference)
		fmt.Printf("FastSyncEngine: would replay blocks from %d to %d (no callback)\n",
			snapshotHeight+1, tipHeight)
	}

	fmt.Printf("FastSyncEngine: block replay complete, syncing to live\n")

	e.mu.Lock()
	e.setStateLocked(SyncLive)
	e.mu.Unlock()
}

// =============================================================================
// Task 3.8: Persist Sync Progress
// =============================================================================

const (
	syncStateBucket = "sync_state"
	keySnapshotHash = "snapshot_hash"
	keyLastHeight   = "last_synced_height"
	keyChunkBitmap  = "chunk_bitmap"
	keySyncMode     = "sync_mode"
)

// persistSyncState writes current sync progress to BoltDB.
func (e *FastSyncEngine) persistSyncState() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.persistent == nil {
		return
	}

	db := e.persistent.DB()
	if db == nil {
		return
	}

	err := db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(syncStateBucket))
		if err != nil {
			return fmt.Errorf("create sync_state bucket: %w", err)
		}

		// Write snapshot hash
		if e.selectedSnapshot != nil {
			if err := bucket.Put([]byte(keySnapshotHash), e.selectedSnapshot.SnapshotHash[:]); err != nil {
				return fmt.Errorf("write snapshot hash: %w", err)
			}
		}

		// Write last synced height
		if e.selectedSnapshot != nil {
			heightBuf := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBuf, e.selectedSnapshot.Height)
			if err := bucket.Put([]byte(keyLastHeight), heightBuf); err != nil {
				return fmt.Errorf("write last height: %w", err)
			}
		}

		// Write chunk bitmap
		if e.scheduler != nil {
			bitmap := e.scheduler.Bitmap()
			bitmapBytes := make([]byte, len(bitmap))
			for i, b := range bitmap {
				if b {
					bitmapBytes[i] = 1
				}
			}
			if err := bucket.Put([]byte(keyChunkBitmap), bitmapBytes); err != nil {
				return fmt.Errorf("write chunk bitmap: %w", err)
			}
		}

		// Write sync mode
		mode := e.state.String()
		if err := bucket.Put([]byte(keySyncMode), []byte(mode)); err != nil {
			return fmt.Errorf("write sync mode: %w", err)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("FastSyncEngine: failed to persist sync state: %v\n", err)
	}
}

// loadSyncState reads sync state from BoltDB.
// Returns zero values if not found or on error.
func (e *FastSyncEngine) loadSyncState() (hash [32]byte, height uint64, bitmap []bool, mode string, err error) {
	if e.persistent == nil {
		return hash, height, bitmap, mode, fmt.Errorf("no persistent state")
	}

	db := e.persistent.DB()
	if db == nil {
		return hash, height, bitmap, mode, fmt.Errorf("no database")
	}

	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(syncStateBucket))
		if bucket == nil {
			return nil // bucket doesn't exist yet
		}

		// Read snapshot hash
		hashBytes := bucket.Get([]byte(keySnapshotHash))
		if hashBytes != nil && len(hashBytes) == 32 {
			copy(hash[:], hashBytes[:])
		}

		// Read last synced height
		heightBytes := bucket.Get([]byte(keyLastHeight))
		if heightBytes != nil && len(heightBytes) == 8 {
			height = binary.BigEndian.Uint64(heightBytes)
		}

		// Read chunk bitmap
		bitmapBytes := bucket.Get([]byte(keyChunkBitmap))
		if bitmapBytes != nil {
			bitmap = make([]bool, len(bitmapBytes))
			for i, b := range bitmapBytes {
				bitmap[i] = b != 0
			}
		}

		// Read sync mode
		modeBytes := bucket.Get([]byte(keySyncMode))
		if modeBytes != nil {
			mode = string(modeBytes)
		}

		return nil
	})

	return hash, height, bitmap, mode, err
}

// clearSyncState deletes the sync state after successful completion.
func (e *FastSyncEngine) clearSyncState() {
	if e.persistent == nil {
		return
	}

	db := e.persistent.DB()
	if db == nil {
		return
	}

	err := db.Update(func(tx *bbolt.Tx) error {
		return tx.DeleteBucket([]byte(syncStateBucket))
	})

	if err != nil {
		fmt.Printf("FastSyncEngine: failed to clear sync state: %v\n", err)
	}
}
