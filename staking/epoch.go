package staking

import (
	"encoding/binary"
	"fmt"

	"github.com/BryanOx/dsn/types"
)

// BlocksPerEpoch returns the number of blocks per epoch.
func BlocksPerEpoch(s KVStore) uint64 {
	val, ok := s.GetBytes(keyBlocksPerEpoch)
	if !ok {
		return DefaultBlocksPerEpoch
	}
	return binary.BigEndian.Uint64(val)
}

// SetBlocksPerEpoch sets the number of blocks per epoch.
func SetBlocksPerEpoch(s KVStore, blocks uint64) error {
	if blocks == 0 {
		return fmt.Errorf("blocks per epoch must be > 0")
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blocks)
	return s.SetBytes(keyBlocksPerEpoch, buf[:])
}

// UnstakeCooldown returns the number of epochs a validator must wait before unstaking completes.
func UnstakeCooldown(s KVStore) uint64 {
	val, ok := s.GetBytes(keyUnstakeCooldown)
	if !ok {
		return DefaultUnstakeCooldown
	}
	return binary.BigEndian.Uint64(val)
}

// SetUnstakeCooldown sets the unstaking cooldown in epochs.
func SetUnstakeCooldown(s KVStore, cooldown uint64) error {
	if cooldown == 0 {
		return fmt.Errorf("unstake cooldown must be > 0")
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], cooldown)
	return s.SetBytes(keyUnstakeCooldown, buf[:])
}

// CurrentEpoch returns the current epoch number.
func CurrentEpoch(s KVStore) (uint64, error) {
	val, ok := s.GetBytes(keyCurrentEpoch)
	if !ok {
		return 0, nil // epoch 0: genesis
	}
	return binary.BigEndian.Uint64(val), nil
}

// setCurrentEpoch stores the current epoch number.
func setCurrentEpoch(s KVStore, epoch uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], epoch)
	return s.SetBytes(keyCurrentEpoch, buf[:])
}

// EpochAtHeight returns the epoch number for a given block height.
// Genesis (height 0) is epoch 0.
// Blocks 1-99 are epoch 1, 100-199 are epoch 2, etc.
func EpochAtHeight(s KVStore, height uint64) uint64 {
	if height == 0 {
		return 0
	}
	bpe := BlocksPerEpoch(s)
	return ((height - 1) / bpe) + 1
}

// IsEpochBoundary returns true if block `height` is the last block of an epoch.
// That means the NEXT block will trigger an epoch transition.
func IsEpochBoundary(s KVStore, height uint64) bool {
	if height == 0 {
		return false
	}
	bpe := BlocksPerEpoch(s)
	return height%bpe == 0
}

// ProcessEpochTransition processes the transition from the current epoch to the next.
// This is called after a block is committed at an epoch boundary.
//
// Transition steps:
// 1. Activate pending validators whose ActivationEpoch == nextEpoch
// 2. Remove unstaking validators whose UnstakeEpoch <= nextEpoch
// 3. Auto-unjail validators whose JailedUntil <= nextEpoch
// 4. Increment the current epoch counter
func ProcessEpochTransition(s StakingState, height uint64) error {
	currentEpoch, err := CurrentEpoch(s)
	if err != nil {
		return err
	}
	nextEpoch := currentEpoch + 1

	// Verify we're at a valid boundary
	expectedEpoch := EpochAtHeight(s, height)
	if expectedEpoch != nextEpoch {
		return fmt.Errorf("epoch mismatch: height %d is at epoch %d, expected next epoch %d",
			height, expectedEpoch, nextEpoch)
	}

	// 1. Activate pending validators whose ActivationEpoch == nextEpoch
	count, err := ValidatorCount(s)
	if err != nil {
		return err
	}
	for seq := uint64(1); seq <= count; seq++ {
		consensusID, err := getValidatorIDBySeq(s, seq)
		if err != nil {
			continue // skip missing
		}
		v, err := GetValidator(s, consensusID)
		if err != nil {
			continue
		}
		if v.Status == ValidatorPending && v.ActivationEpoch == nextEpoch {
			if err := ActivateValidator(s, consensusID, nextEpoch); err != nil {
				return fmt.Errorf("activate validator %s: %w", consensusID, err)
			}
		}
	}

	// 2. Remove unstaking validators whose UnstakeEpoch <= nextEpoch
	for seq := uint64(1); seq <= count; seq++ {
		consensusID, err := getValidatorIDBySeq(s, seq)
		if err != nil {
			continue
		}
		v, err := GetValidator(s, consensusID)
		if err != nil {
			continue
		}
		if v.Status == ValidatorUnstaking && v.UnstakeEpoch <= nextEpoch {
			if err := ReleaseStake(s, consensusID); err != nil {
				return fmt.Errorf("release validator %s: %w", consensusID, err)
			}
		}
	}

	// 3. Auto-unjail validators whose jail term has expired
	for seq := uint64(1); seq <= count; seq++ {
		consensusID, err := getValidatorIDBySeq(s, seq)
		if err != nil {
			continue
		}
		v, err := GetValidator(s, consensusID)
		if err != nil {
			continue
		}
		if v.Status == ValidatorJailed && v.JailedUntil <= nextEpoch {
			if err := UnjailValidator(s, consensusID); err != nil {
				return fmt.Errorf("auto-unjail validator %s: %w", consensusID, err)
			}
		}
	}

	// 4. Advance the epoch counter
	if err := setCurrentEpoch(s, nextEpoch); err != nil {
		return err
	}

	return nil
}

// EpochInfo returns summary information about the current epoch state.
type EpochInfo struct {
	CurrentEpoch     uint64
	BlocksPerEpoch   uint64
	TotalValidators  uint64
	ActiveValidators int
	TotalBonded      types.Amount
}

// GetEpochInfo returns current epoch information.
func GetEpochInfo(s StakingState) (*EpochInfo, error) {
	currentEpoch, err := CurrentEpoch(s)
	if err != nil {
		return nil, err
	}

	totalValidators, err := ValidatorCount(s)
	if err != nil {
		return nil, err
	}

	active, err := GetActiveValidators(s)
	if err != nil {
		return nil, err
	}

	totalBonded, err := TotalBonded(s)
	if err != nil {
		return nil, err
	}

	return &EpochInfo{
		CurrentEpoch:     currentEpoch,
		BlocksPerEpoch:   BlocksPerEpoch(s),
		TotalValidators:  totalValidators,
		ActiveValidators: len(active),
		TotalBonded:      totalBonded,
	}, nil
}
