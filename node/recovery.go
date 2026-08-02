package node

import (
	"fmt"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// Recover attempts to recover node state after a crash.
// BoltDB guarantees atomic transactions, so state on disk is always consistent.
// Recovery handles:
// - State root verification against checkpoints
// - Replay forward if checkpoint predates latest block
// - Snapshot restoration for severe corruption
func (n *Node) Recover() error {
	if n.persistent == nil {
		return nil // in-memory only
	}

	// Get persistent state root
	stateRoot := n.persistent.GetStateRoot()

	// Try to load tip to set current height
	tip, tipErr := consensus.LoadTip(n.persistent)
	if tipErr == nil {
		hash, _ := tip.HeaderHash(n.hasher)
		n.setTip(tip.Height, hash)
	}

	if stateRoot == (types.Hash{}) {
		return nil // fresh database, nothing to recover
	}

	// Load accounts from persistent cache into in-memory state
	if err := n.persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
		return n.state.SetAccount(addr, acc)
	}); err != nil {
		return fmt.Errorf("recovery: load accounts: %w", err)
	}

	if err := n.persistent.ForEachKV(func(k string, v []byte) error {
		return n.state.SetBytes(k, v)
	}); err != nil {
		return fmt.Errorf("recovery: load kvstore: %w", err)
	}

	// Rebuild SMT in memory after loading data
	if _, err := n.state.Commit(); err != nil {
		return fmt.Errorf("recovery: rebuild SMT: %w", err)
	}

	// Verify state against checkpoint if available
	cp, cpErr := state.LatestCheckpoint(n.persistent)
	if cpErr != nil {
		// No checkpoint — state could be at any height
		// It's consistent as long as there's a state root
		return nil
	}

	// Verify state root matches checkpoint
	if stateRoot != cp.StateRoot {
		// State has drifted from checkpoint — try snapshot recovery
		if !state.SnapshotExists(n.persistent, cp.Height) {
			return fmt.Errorf("recovery failed: state root %x doesn't match checkpoint at height %d (%x), and no snapshot available",
				stateRoot, cp.Height, cp.StateRoot)
		}

		snapData, err := state.LoadSnapshot(n.persistent, cp.Height)
		if err != nil {
			return fmt.Errorf("recovery: load snapshot: %w", err)
		}

		if err := state.RestoreFromSnapshot(n.persistent, snapData, cp.SnapshotHash); err != nil {
			return fmt.Errorf("recovery: restore: %w", err)
		}

		// Reload in-memory state from persistent cache (restored from snapshot)
		// Clear first
		n.state = state.NewInMemoryState(n.hasher)
		if err := n.persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
			return n.state.SetAccount(addr, acc)
		}); err != nil {
			return fmt.Errorf("recovery: reload accounts after snapshot: %w", err)
		}
		if err := n.persistent.ForEachKV(func(k string, v []byte) error {
			return n.state.SetBytes(k, v)
		}); err != nil {
			return fmt.Errorf("recovery: reload kvstore after snapshot: %w", err)
		}

		// Rebuild SMT in memory after snapshot restore
		if _, err := n.state.Commit(); err != nil {
			return fmt.Errorf("recovery: rebuild SMT after snapshot: %w", err)
		}

		// Replay forward from checkpoint to tip
		if tipErr == nil && tip.Height > cp.Height {
			if err := n.ReplayBlocks(cp.Height+1, tip.Height); err != nil {
				return fmt.Errorf("recovery: replay: %w", err)
			}
		}
	}

	return nil
}
