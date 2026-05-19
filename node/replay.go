package node

import (
	"fmt"

	"github.com/dsn/dsn/consensus"
)

// ReplayBlocks re-executes blocks from fromHeight to toHeight (inclusive)
// to bring the node to the latest state after state restoration.
// This is a critical step in fast sync where blocks after the snapshot
// checkpoint are re-executed to verify state and update to the latest tip.
func (n *Node) ReplayBlocks(fromHeight, toHeight uint64) error {
	// Validate parameters
	if fromHeight > toHeight {
		return fmt.Errorf("fromHeight (%d) cannot be greater than toHeight (%d)", fromHeight, toHeight)
	}

	// Need persistent storage to load block headers
	if n.persistent == nil {
		return fmt.Errorf("persistent storage required for block replay")
	}

	// Replay each block in sequence
	for height := fromHeight; height <= toHeight; height++ {
		// Load stored header for this height
		header, err := consensus.LoadBlockHeader(n.persistent, height)
		if err != nil {
			return fmt.Errorf("load header at height %d: %w", height, err)
		}

		// Execute BeginBlock (deterministic system transitions like epoch changes,
		// validator activations, economic issuance)
		_, _, err = consensus.BeginBlock(n.state, height)
		if err != nil {
			return fmt.Errorf("begin block at height %d: %w", height, err)
		}

		// No transactions to replay (not stored in v1)
		// FinalizeBlock is no-op currently

		// Commit state to get the resulting state root
		stateRoot, err := n.state.Commit()
		if err != nil {
			return fmt.Errorf("commit at height %d: %w", height, err)
		}

		// Verify state root matches the stored header
		if stateRoot != header.StateRoot {
			return fmt.Errorf("state root mismatch at height %d: got %x, expected %x",
				height, stateRoot, header.StateRoot)
		}

		// Update node's current height and tip hash
		n.currentHeight = height
		hash, err := header.HeaderHash(n.hasher)
		if err != nil {
			return fmt.Errorf("header hash at height %d: %w", height, err)
		}
		n.currentTipHash = hash

		// Persist the updated tip
		if err := consensus.StoreTip(n.persistent, header); err != nil {
			return fmt.Errorf("store tip at height %d: %w", height, err)
		}
	}

	return nil
}