package consensus

import (
	"fmt"
	"time"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// BeginBlock executes system transitions for the given block height.
// Called BEFORE user transactions — modifies state in-place.
// Returns the epoch number and validator set hash for this block.
// Must be deterministic and produce identical results on replay.
func BeginBlock(s staking.StakingState, height uint64, blockTimeSec uint64) (epoch uint64, validatorSetHash types.Hash, err error) {
	if height == 0 {
		return 0, types.Hash{}, nil // genesis
	}

	// Check if this is an epoch boundary
	if staking.IsEpochBoundary(s, height) {
		// 0a. Issue epoch inflation tokens for the completing epoch
		// The completed epoch is EpochAtHeight(s, height) at boundary
		completedEpoch := staking.EpochAtHeight(s, height)
		blocksPerEpoch := staking.BlocksPerEpoch(s)

		// Only issue tokens if inflation is enabled
		if staking.InflationEnabled(s) {
			if _, err := staking.IssueEpochTokens(s, completedEpoch, blockTimeSec, blocksPerEpoch); err != nil {
				return 0, types.Hash{}, fmt.Errorf("issue epoch tokens at height %d epoch %d: %w", height, completedEpoch, err)
			}
		}

		// 0b. Distribute validator rewards using snapshot from prior epoch
		if err := staking.DistributeValidatorRewards(s, completedEpoch); err != nil {
			return 0, types.Hash{}, fmt.Errorf("distribute validator rewards at height %d epoch %d: %w", height, completedEpoch, err)
		}

		// Process epoch transition (existing code)
		if err := staking.ProcessEpochTransition(s, height); err != nil {
			return 0, types.Hash{}, fmt.Errorf("epoch transition at height %d: %w", height, err)
		}
	}

	// Get current epoch (updated by transition if boundary)
	epoch, err = staking.CurrentEpoch(s)
	if err != nil {
		return 0, types.Hash{}, fmt.Errorf("current epoch: %w", err)
	}

	// Try to get existing snapshot for this epoch
	snap, err := staking.GetSnapshot(s, epoch)
	if err != nil {
		return 0, types.Hash{}, fmt.Errorf("get snapshot: %w", err)
	}

	if snap == nil {
		// First time seeing this epoch — create snapshot
		snap, err = staking.CreateSnapshot(s, epoch)
		if err != nil {
			return 0, types.Hash{}, fmt.Errorf("create snapshot epoch %d: %w", epoch, err)
		}

		// Note: At epoch boundaries, the caller should call CreateCheckpoint
		// to persist a chain checkpoint for fast sync and crash recovery.
		// This is done in the Node layer (not here) because BeginBlock
		// doesn't have access to persistent storage.
	}

	return epoch, snap.SetHash, nil
}

// FinalizeBlock executes post-transaction system transitions.
// Called AFTER user transactions, BEFORE state commit.
func FinalizeBlock(s staking.StakingState, block *types.Block) error {
	if block.FeeSummary.TotalFees == 0 {
		return nil // no fees to distribute
	}
	totalFees := types.NewAmount(block.FeeSummary.TotalFees)
	_, burnShare, treasuryShare, err := staking.DistributeRewards(s, totalFees)
	if err != nil {
		return fmt.Errorf("distribute rewards: %w", err)
	}
	// Execute the burn (20% of fees)
	if !burnShare.IsZero() {
		if err := staking.BurnTokens(s, burnShare); err != nil {
			return fmt.Errorf("burn tokens: %w", err)
		}
	}
	// Credit the treasury share (10% of fees) to the treasury account
	if !treasuryShare.IsZero() {
		if err := staking.CreditTreasury(s, treasuryShareToUint64(treasuryShare)); err != nil {
			return fmt.Errorf("credit treasury: %w", err)
		}
	}
	return nil
}

// treasuryShareToUint64 converts a types.Amount to uint64 using the same
// big-endian scheme staking uses internally (stakeToUint64). Safe for v1
// amounts which fit in a uint64.
func treasuryShareToUint64(amount types.Amount) uint64 {
	data, err := amount.MarshalBinary()
	if err != nil || len(data) == 0 {
		return 0
	}
	if len(data) > 8 {
		// Truncate to 8 bytes (loses high bits — should not happen in v1)
		data = data[len(data)-8:]
	}
	var val uint64
	for _, b := range data {
		val = (val << 8) | uint64(b)
	}
	return val
}

// CreateCheckpoint persists a chain checkpoint at the given height.
// This is typically called at epoch boundaries after BeginBlock completes.
// Returns the checkpoint, or nil/nil if persistent storage is unavailable.
//
// The checkpoint includes:
//   - Block height and hash
//   - State root (from committed state)
//   - Validator set hash (from BeginBlock)
//   - Epoch number
//
// In future versions, checkpoints will also include full state snapshots.
func CreateCheckpoint(ps *state.PersistentState, height, epoch uint64, stateRoot, blockHash, valSetHash types.Hash) (*state.Checkpoint, error) {
	if ps == nil {
		return nil, nil
	}

	cp := &state.Checkpoint{
		Height:           height,
		BlockHash:        blockHash,
		StateRoot:        stateRoot,
		SnapshotHash:     types.Hash{},
		ValidatorSetHash: valSetHash,
		Epoch:            epoch,
		Timestamp:        uint64(time.Now().Unix()),
	}

	if err := state.StoreCheckpoint(ps, cp); err != nil {
		return nil, fmt.Errorf("store checkpoint: %w", err)
	}

	return cp, nil
}
