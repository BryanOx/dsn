package node

import (
	"fmt"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/network"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// SyncMode represents the synchronization mode of the node.
type SyncMode uint8

const (
	SyncModeNormal   SyncMode = 0 // Normal consensus mode
	SyncModeFastSync SyncMode = 1 // Fast sync in progress
	SyncModeReplay   SyncMode = 2 // Block replay in progress
)

// FastSyncState tracks the current fast sync progress for recovery.
type FastSyncState struct {
	Mode         SyncMode
	TargetHeight uint64
	SnapshotHash [32]byte
	// SnapshotChunksReceived int
	// SnapshotChunksTotal    int
	// PendingChunks  map[uint32][]byte
}

// FastSync performs a full fast sync from the network.
// It discovers snapshots from peers, downloads, restores, and replays blocks.
// This is the main entry point for a new node joining the network.
func (n *Node) FastSync() error {
	// Step 1: Validate prerequisites
	if n.p2p == nil {
		return fmt.Errorf("fast sync requires P2P networking (configure P2PPort)")
	}
	if n.persistent == nil {
		return fmt.Errorf("fast sync requires persistent storage (configure DataDir)")
	}

	// Step 2: Check if we already have state (crash recovery path)
	if n.persistent.GetStateRoot() != (types.Hash{}) {
		// We have existing state — verify it's consistent
		latestCP, err := state.LatestCheckpoint(n.persistent)
		if err == nil {
			// Try to replay from last checkpoint to latest known header
			// This is the recovery path
			n.setTip(latestCP.Height, latestCP.BlockHash)

			// Check if we need to replay
			latestHeader, err := consensus.LoadTip(n.persistent)
			if err == nil && latestHeader.Height > latestCP.Height {
				return n.ReplayBlocks(latestCP.Height+1, latestHeader.Height)
			}
			return nil // Already synced
		}
	}

	// Step 3: New node — fetch state from the network via snapshot sync.
	// The FastSyncEngine runs the full pipeline (query → select → download →
	// verify → restore → replay → live); on any failure it falls back to
	// block-range catch-up so the node still reaches the peer tip.
	if err := n.SyncFromNetwork(defaultSnapshotTimeout); err != nil {
		// Fallback: range replay from the on-disk tip (automatic catch-up).
		if n.blockSync != nil {
			n.blockSync.Start()
		}
		return fmt.Errorf("fast sync from network failed, falling back to block-range catch-up: %w", err)
	}
	return nil
}

// defaultSnapshotTimeout bounds the whole snapshot discovery/download/verify
// window before SyncFromNetwork gives up and the caller falls back to range sync.
const defaultSnapshotTimeout = 30 * time.Second

// FastSyncFromCheckpoint performs fast sync from a provided checkpoint.
// The checkpoint can come from a trusted source (config, genesis file, or CLI).
// This is the primary method for v1 fast sync.
//
// Parameters:
//   - cp: checkpoint metadata (height, hashes)
//   - snapData: serialized snapshot bytes
//   - targetHeight: the height to sync up to (0 = no replay, just restore state)
func (n *Node) FastSyncFromCheckpoint(cp *state.Checkpoint, snapData []byte, targetHeight uint64) error {
	// Step 1: Validate prerequisites
	if n.persistent == nil {
		return fmt.Errorf("fast sync requires persistent storage")
	}

	// Step 2: Verify snapshot hash matches checkpoint
	computedHash := state.SnapshotHash(snapData)
	if computedHash != cp.SnapshotHash {
		return fmt.Errorf("snapshot hash mismatch: got %x, expected %x",
			computedHash[:], cp.SnapshotHash[:])
	}

	// Step 3: Restore state from snapshot
	if err := state.RestoreFromSnapshot(n.persistent, snapData, cp.SnapshotHash); err != nil {
		return fmt.Errorf("restore from snapshot: %w", err)
	}

	// Step 4: Rebuild in-memory state from persistent storage
	// Copy all accounts from persistent cache into in-memory state
	if err := n.persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
		return n.state.SetAccount(addr, acc)
	}); err != nil {
		return fmt.Errorf("rebuild in-memory accounts: %w", err)
	}

	// Copy all kvstore entries from persistent cache into in-memory state
	if err := n.persistent.ForEachKV(func(k string, v []byte) error {
		return n.state.SetBytes(k, v)
	}); err != nil {
		return fmt.Errorf("rebuild in-memory kvstore: %w", err)
	}

	// Commit in-memory state to compute the SMT state root
	if _, err := n.state.Commit(); err != nil {
		return fmt.Errorf("commit in-memory state: %w", err)
	}

	// Step 5: Verify state root matches checkpoint
	currentRoot := n.persistent.GetStateRoot()
	if currentRoot != cp.StateRoot {
		return fmt.Errorf("state root mismatch after restore: got %x, expected %x",
			currentRoot[:], cp.StateRoot[:])
	}

	// Step 6: Update node state
	n.setTip(cp.Height, cp.BlockHash)

	// Step 7: Persist tip to ensure recovery works
	if err := consensus.StoreTip(n.persistent, &types.BlockHeader{
		Height:       cp.Height,
		PreviousHash: types.Hash{},
		StateRoot:    cp.StateRoot,
		Timestamp:    cp.Timestamp,
	}); err != nil {
		return fmt.Errorf("store tip: %w", err)
	}

	// Step 8: Replay blocks after checkpoint if targetHeight > checkpoint height
	if targetHeight > cp.Height {
		if err := n.ReplayBlocks(cp.Height+1, targetHeight); err != nil {
			return fmt.Errorf("replay blocks: %w", err)
		}
	}

	return nil
}

// FastSyncFromLocalSnapshot loads a snapshot from local storage and syncs.
// This is a convenience method for loading snapshot data from a file or
// pre-stored snapshot in the database.
func (n *Node) FastSyncFromLocalSnapshot(height uint64, targetHeight uint64) error {
	if n.persistent == nil {
		return fmt.Errorf("persistent storage required")
	}

	// Check if snapshot exists at this height
	if !state.SnapshotExists(n.persistent, height) {
		return fmt.Errorf("no snapshot found at height %d", height)
	}

	// Load snapshot data
	snapData, err := state.LoadSnapshot(n.persistent, height)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Load checkpoint at this height
	cp, err := state.LoadCheckpoint(n.persistent, height)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}

	// Perform fast sync from loaded checkpoint
	return n.FastSyncFromCheckpoint(cp, snapData, targetHeight)
}

// StoreStateSnapshot persists a state snapshot of the node's current in-memory
// state at its current tip height, plus a chain checkpoint that references the
// snapshot, so fast-sync peers can discover, verify and restore it. All
// metadata (state root, validator set hash, epoch, timestamp) is taken from the
// block at the snapshot height, guaranteeing a peer that restores this snapshot
// and replays the following blocks converges on the same state root.
//
// This is the operator-facing way to publish a snapshot from live state; the
// consensus layer also stores snapshots automatically at epoch boundaries.
func (n *Node) StoreStateSnapshot() (*state.Checkpoint, error) {
	if n.persistent == nil {
		return nil, fmt.Errorf("persistent storage required")
	}
	height := n.currentHeight.Load()
	if height == 0 {
		return nil, fmt.Errorf("no finalized blocks to snapshot")
	}
	blk, err := n.GetBlock(height)
	if err != nil {
		return nil, fmt.Errorf("load block at snapshot height %d: %w", height, err)
	}

	snap, err := n.state.CreateSnapshot(height, blk.Header.Epoch,
		blk.Header.ValidatorSetHash, blk.Header.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}
	data, err := state.SerializeSnapshot(snap)
	if err != nil {
		return nil, fmt.Errorf("serialize snapshot: %w", err)
	}
	if err := state.StoreSnapshot(n.persistent, height, data); err != nil {
		return nil, fmt.Errorf("store snapshot: %w", err)
	}

	cp := &state.Checkpoint{
		Height:           height,
		BlockHash:        n.GetTipHash(),
		StateRoot:        blk.Header.StateRoot,
		SnapshotHash:     state.SnapshotHash(data),
		ValidatorSetHash: blk.Header.ValidatorSetHash,
		Epoch:            blk.Header.Epoch,
		Timestamp:        blk.Header.Timestamp,
	}
	if err := state.StoreCheckpoint(n.persistent, cp); err != nil {
		return nil, fmt.Errorf("store checkpoint: %w", err)
	}
	return cp, nil
}

// SyncFromNetwork starts the FastSyncEngine against the P2P network and waits
// for it to reach a terminal state. The engine owns the full download path —
// snapshot discovery, chunk download with re-request, reassembly, hash
// verification and restore — via the handlers registered in Node.New.
//
// Returns nil when a verified snapshot was restored (state root non-zero).
// Returns an error when the engine fell back to live sync without restoring
// state (query timeout, hash mismatch, restore error) or when it timed out;
// the caller then falls back to block-range catch-up.
func (n *Node) SyncFromNetwork(snapshotTimeout time.Duration) error {
	if n.p2p == nil {
		return fmt.Errorf("P2P node not available")
	}
	if n.fastSync == nil {
		return fmt.Errorf("fast sync engine not available")
	}
	if n.persistent == nil {
		return fmt.Errorf("persistent storage required for fast sync")
	}

	// Completion signal: the engine reaches SyncLive on success (after
	// restore + replay) and on failure (fallBackToLiveSync).
	done := make(chan struct{}, 1)
	n.fastSync.SetStateChangeHandler(func(oldState, newState network.SyncState) {
		if newState == network.SyncLive {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})

	// Delegated engine start: the info handler wired by SetFastSyncEngine is
	// left untouched (no infoCh clobber) so discovery reaches the engine.
	n.fastSync.Start()

	select {
	case <-done:
		// A verified restore leaves a non-zero state root; a fallback leaves
		// the node empty so range catch-up can take over.
		if n.persistent.GetStateRoot() != (types.Hash{}) {
			return nil
		}
		return fmt.Errorf("snapshot sync failed: no verified snapshot restored")
	case <-time.After(snapshotTimeout):
		return fmt.Errorf("snapshot sync timed out after %v", snapshotTimeout)
	}
}
