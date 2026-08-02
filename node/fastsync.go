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

	// Step 3: New node — need to fetch from network
	// For v1, full P2P snapshot discovery is not yet implemented.
	// Use FastSyncFromCheckpoint with a trusted snapshot file instead.

	// TODO: Implement P2P snapshot discovery:
	// 1. Broadcast snapshot query to peers
	// 2. Wait for snapshot info responses with timeout
	// 3. Select best snapshot (highest height)
	// 4. Download chunks from responding peers
	// 5. Reassemble and verify snapshot
	// 6. Restore state and replay blocks

	return fmt.Errorf("fast sync from network not yet implemented; " +
		"use FastSyncFromCheckpoint with a local snapshot file or implement P2P discovery")
}

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

// SyncFromNetwork attempts to sync from the P2P network (placeholder for future).
// This requires implementing:
// - Peer discovery and selection
// - Snapshot info query/response protocol
// - Chunk download with parallel requests
// - Snapshot verification and restoration
func (n *Node) SyncFromNetwork(snapshotTimeout time.Duration) error {
	if n.p2p == nil {
		return fmt.Errorf("P2P node not available")
	}

	// Step 1: Set up snapshot info handler
	infoCh := make(chan *network.SnapshotInfo, 10)
	n.p2p.SetSnapshotInfoHandler(func(info *network.SnapshotInfo) {
		select {
		case infoCh <- info:
		default:
			// Drop if channel is full
		}
	})

	// Step 2: Broadcast snapshot query
	n.p2p.BroadcastSnapshotQuery()

	// Step 3: Wait for responses and select best snapshot
	var bestInfo *network.SnapshotInfo
	select {
	case info := <-infoCh:
		bestInfo = info
	case <-time.After(snapshotTimeout):
		return fmt.Errorf("no snapshot info received from peers within %v", snapshotTimeout)
	}

	// Log the selected snapshot
	fmt.Printf("Received snapshot info: height=%d, chunks=%d, hash=%x\n",
		bestInfo.Height, bestInfo.ChunkCount, bestInfo.SnapshotHash[:8])

	// TODO: Continue with chunk download, reassembly, and restoration
	// This requires peer tracking to know which peer to request chunks from.
	// The current P2P design doesn't track which peer sent the SnapshotInfo.

	return fmt.Errorf("P2P snapshot download not fully implemented: peer tracking required")
}
