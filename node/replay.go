package node

import (
	"fmt"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/types"
)

// ReplayBlocks re-executes blocks from fromHeight to toHeight (inclusive)
// to bring the node to the latest state after state restoration.
// This is a critical step in fast sync where blocks after the snapshot
// checkpoint are re-executed to verify state and update to the latest tip.
//
// Full blocks (header + transactions) are loaded and every transaction is
// re-executed through the shared ApplyTransaction path used by block building
// and validation, so replay produces byte-identical state transitions.
func (n *Node) ReplayBlocks(fromHeight, toHeight uint64) error {
	// Validate parameters
	if fromHeight > toHeight {
		return fmt.Errorf("fromHeight (%d) cannot be greater than toHeight (%d)", fromHeight, toHeight)
	}

	// Need persistent storage to load full blocks
	if n.persistent == nil {
		return fmt.Errorf("persistent storage required for block replay")
	}

	// The VM is required to replay contract transactions
	if n.vm == nil {
		return fmt.Errorf("vm required for block replay")
	}

	// Replay each block in sequence
	for height := fromHeight; height <= toHeight; height++ {
		// Load the stored full block (header + transactions)
		block, err := consensus.LoadBlock(n.persistent, height)
		if err != nil {
			return fmt.Errorf("load block at height %d: %w", height, err)
		}

		// Execute BeginBlock (deterministic system transitions like epoch
		// changes, validator activations, economic issuance)
		_, _, err = consensus.BeginBlock(n.state, height, n.cfg.BlockTimeSec)
		if err != nil {
			return fmt.Errorf("begin block at height %d: %w", height, err)
		}

		// Re-execute every transaction via the shared execution path so the
		// replay matches building and validation exactly. The final fee
		// summary is rebuilt from the charged fees.
		var totalFees uint64
		for i := range block.Transactions {
			exec, err := consensus.ApplyTransaction(n.state, &block.Transactions[i], &block.Header, n.vm, n.hasher, uint32(i))
			if err != nil {
				return fmt.Errorf("apply tx %d at height %d: %w", i, height, err)
			}
			totalFees += exec.Fee
		}

		// FinalizeBlock distributes fees (validator rewards, burn, treasury)
		if err := consensus.FinalizeBlock(n.state, &types.Block{
			FeeSummary: types.NewFeeSummary(totalFees),
		}); err != nil {
			return fmt.Errorf("finalize block at height %d: %w", height, err)
		}

		// Commit state to get the resulting state root
		stateRoot, err := n.state.Commit()
		if err != nil {
			return fmt.Errorf("commit at height %d: %w", height, err)
		}

		// Verify state root matches the stored block header
		if stateRoot != block.Header.StateRoot {
			return fmt.Errorf("state root mismatch at height %d: got %x, expected %x",
				height, stateRoot, block.Header.StateRoot)
		}

		// Update node's current height and tip hash
		hash, err := block.HeaderHash(n.hasher)
		if err != nil {
			return fmt.Errorf("header hash at height %d: %w", height, err)
		}
		n.setTip(height, hash)

		// Persist the updated tip
		if err := consensus.StoreTip(n.persistent, &block.Header); err != nil {
			return fmt.Errorf("store tip at height %d: %w", height, err)
		}
	}

	return nil
}

// RevertBlocks reverts state and removes blocks from `from` down to `to+1`
// (exclusive of `to`). The block at height `to` becomes the new tip.
// Returns an error if `to` is below the finalized height (k=6 finality guard).
func (n *Node) RevertBlocks(from, to uint64) error {
	if to < n.finalizedHeight {
		return fmt.Errorf("cannot revert below finalized height %d", n.finalizedHeight)
	}

	snapID := n.state.Snapshot()

	// Remove blocks from persistence (highest first)
	for h := from; h > to; h-- {
		if n.persistent != nil {
			// Best-effort removal: ignore errors for missing blocks
			_ = consensus.RemoveBlock(n.persistent, h)
		}
	}

	// Revert state to the snapshot taken before the loop.
	// This undoes all state changes from the reverted blocks.
	if err := n.state.RevertToSnapshot(snapID); err != nil {
		return fmt.Errorf("revert state: %w", err)
	}

	n.setTip(to, types.Hash{})
	return nil
}
