package staking

import (
	"fmt"
	"strconv"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// DistributeValidatorRewards distributes the validator reward pool for a completed epoch
// proportionally to validator voting power from the epoch's snapshot.
//
// The snapshot for epoch N is created at the beginning of epoch N (at the boundary),
// so it represents validators active during epoch N. Therefore, rewards for epoch N
// use snapshot (epoch - 1).
func DistributeValidatorRewards(s StakingState, epoch uint64) error {
	// 1. Read validator pool amount for the epoch
	poolKey := KeyEpochValidatorPool + strconv.FormatUint(epoch, 10)
	validatorPoolAmount := readUint64(s, poolKey)

	// 2. If no pool for this epoch, nothing to distribute
	if validatorPoolAmount == 0 {
		return nil
	}

	// 3. Get the snapshot for the epoch (epoch - 1 because snapshot N covers epoch N+1)
	snapshotEpoch := epoch - 1
	if snapshotEpoch == 0 {
		// Epoch 1 has no prior snapshot, nothing to distribute
		return nil
	}

	snap, err := GetSnapshot(s, snapshotEpoch)
	if err != nil {
		return fmt.Errorf("get snapshot for epoch %d: %w", snapshotEpoch, err)
	}

	// 4. If no snapshot or no validators, nothing to distribute
	if snap == nil || len(snap.Validators) == 0 || snap.TotalPower == 0 {
		return nil
	}

	// 5. Calculate total voting power from snapshot
	totalPower := snap.TotalPower
	if totalPower == 0 {
		return nil
	}

	// 6. Distribute proportionally using largest-remainder method
	// (same pattern as DistributeRewards in staking.go)
	amount := validatorPoolAmount
	distributed := uint64(0)
	rewards := make([]uint64, len(snap.Validators))

	for i, v := range snap.Validators {
		portion := amount * v.VotingPower / totalPower
		rewards[i] = portion
		distributed += portion
	}

	// Give remainder to top validator (largest-voting-power validator at index 0)
	remainder := amount - distributed
	if remainder > 0 && len(snap.Validators) > 0 {
		rewards[0] += remainder
	}

	// 7. Apply rewards to validator accounts
	for i, v := range snap.Validators {
		if rewards[i] == 0 {
			continue
		}

		reward := types.NewAmount(rewards[i])

		// Use RewardAddress if set, otherwise fall back to OperatorAddress
		rewardAddr := v.RewardAddress
		if rewardAddr == (types.Address{}) {
			rewardAddr = v.OperatorAddress
		}

		acc, err := s.GetAccount(rewardAddr)
		if err != nil {
			// Account doesn't exist - create a new one
			acc = state.NewAccount(rewardAddr, [32]byte{})
		}
		if err := acc.AddBalance(reward); err != nil {
			return fmt.Errorf("reward to %s: %w", rewardAddr, err)
		}
		if err := s.SetAccount(rewardAddr, acc); err != nil {
			return fmt.Errorf("set account %s: %w", rewardAddr, err)
		}
	}

	return nil
}