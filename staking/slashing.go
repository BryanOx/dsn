package staking

import (
	"encoding/binary"
	"fmt"

	"github.com/dsn/dsn/types"
)

// SlashOffense enumerates slashable offenses.
type SlashOffense uint8

const (
	// SlashDoublePrevote is the slash penalty for double prevoting (prevote two different blocks at same height/round)
	SlashDoublePrevote SlashOffense = 1
	// SlashDoublePrecommit is the slash penalty for double precommitting (precommit two different blocks at same height/round)
	SlashDoublePrecommit SlashOffense = 2
	// SlashInvalidCommit is the slash penalty for invalid commit proof
	SlashInvalidCommit SlashOffense = 3
)

// Slash parameters (basis points: 1% = 100, 5% = 500, 10% = 1000, 20% = 2000)
const (
	// SlashPercentDoublePrevote is the slash penalty for double prevoting (5%)
	SlashPercentDoublePrevote uint64 = 500
	// SlashPercentDoublePrecommit is the slash penalty for double precommitting (10%)
	SlashPercentDoublePrecommit uint64 = 1000
	// SlashPercentInvalidCommit is the slash penalty for invalid commit proof (20%)
	SlashPercentInvalidCommit uint64 = 2000
	// SlashJailEpochs is the default jail duration in epochs
	SlashJailEpochs uint64 = 10
)

// Slash parameter storage keys
const (
	keySlashParams = "staking/slash_params"
)

// SlashParams holds the configurable slash parameters.
type SlashParams struct {
	PercentDoublePrevote   uint64
	PercentDoublePrecommit uint64
	PercentInvalidCommit   uint64
	JailEpochs             uint64
}

// DefaultSlashParams returns the default slash parameters.
func DefaultSlashParams() SlashParams {
	return SlashParams{
		PercentDoublePrevote:   SlashPercentDoublePrevote,
		PercentDoublePrecommit: SlashPercentDoublePrecommit,
		PercentInvalidCommit:   SlashPercentInvalidCommit,
		JailEpochs:             SlashJailEpochs,
	}
}

// SetSlashParams stores the slash parameters in the kvstore.
func SetSlashParams(s KVStore, params SlashParams) error {
	var buf [32]byte
	binary.BigEndian.PutUint64(buf[0:8], params.PercentDoublePrevote)
	binary.BigEndian.PutUint64(buf[8:16], params.PercentDoublePrecommit)
	binary.BigEndian.PutUint64(buf[16:24], params.PercentInvalidCommit)
	binary.BigEndian.PutUint64(buf[24:32], params.JailEpochs)
	return s.SetBytes(keySlashParams, buf[:])
}

// GetSlashParams retrieves the slash parameters from the kvstore.
// Returns defaults if not set.
func GetSlashParams(s KVStore) SlashParams {
	val, ok := s.GetBytes(keySlashParams)
	if !ok {
		return DefaultSlashParams()
	}
	if len(val) < 32 {
		return DefaultSlashParams()
	}
	return SlashParams{
		PercentDoublePrevote:   binary.BigEndian.Uint64(val[0:8]),
		PercentDoublePrecommit: binary.BigEndian.Uint64(val[8:16]),
		PercentInvalidCommit:   binary.BigEndian.Uint64(val[16:24]),
		JailEpochs:             binary.BigEndian.Uint64(val[24:32]),
	}
}

// OffenseFromEvidence maps an evidence type to a slashable offense.
func OffenseFromEvidence(ev types.Evidence) (SlashOffense, error) {
	switch e := ev.(type) {
	case *types.DoubleSignEvidence:
		// Both votes must be the same type per DoubleSignEvidence.Validate()
		if e.VoteA.VoteType == types.VotePrevote {
			return SlashDoublePrevote, nil
		}
		if e.VoteA.VoteType == types.VotePrecommit {
			return SlashDoublePrecommit, nil
		}
		return 0, fmt.Errorf("unsupported vote type in double sign: %v", e.VoteA.VoteType)
	case *types.InvalidCommitEvidence:
		return SlashInvalidCommit, nil
	case *types.MalformedVoteEvidence:
		// MalformedVoteEvidence is not slashable - it's a malformed vote
		// from a non-validator or invalid vote, not a Byzantine fault
		return 0, fmt.Errorf("malformed vote evidence is not slashable")
	default:
		return 0, fmt.Errorf("unknown evidence type: %T", ev)
	}
}

// SlashAmount calculates the slash amount from a stake and percentage (in basis points).
// Example: 500 = 5%, 1000 = 10%, 2000 = 20%
func SlashAmount(slashPercent uint64, stake types.Amount) (types.Amount, error) {
	if slashPercent > 10000 {
		return types.Amount{}, fmt.Errorf("slash percent %d exceeds max 10000", slashPercent)
	}
	// Amount doesn't support Mul/Div, so convert to uint64
	stakeUint64 := amountToUint64(stake)
	slashUint64 := stakeUint64 * slashPercent / 10000
	return types.NewAmount(slashUint64), nil
}

// amountToUint64 converts an Amount to uint64.
// This is the same logic as stakeToUint64 in registry.go.
func amountToUint64(amt types.Amount) uint64 {
	data, _ := amt.MarshalBinary()
	if len(data) == 0 {
		return 0
	}
	// Pad to 8 bytes max (safe for v1 since stake fits in uint64)
	if len(data) > 8 {
		data = data[len(data)-8:]
	}
	var val uint64
	for _, b := range data {
		val = (val << 8) | uint64(b)
	}
	return val
}

// ApplySlash slashes a validator by a given percentage and jails them for the specified epochs.
// This is the core slashing function that:
// 1. Reduces the validator's bonded stake
// 2. Updates total bonded
// 3. Jails active validators
func ApplySlash(s KVStore, consensusID types.Address, slashPercent uint64, jailEpochs uint64) error {
	// Get validator
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}

	// Only validators that haven't been removed can be slashed
	// (pending, active, jailed, unstaking all can be slashed for historical offenses)
	if v.Status == ValidatorRemoved {
		return fmt.Errorf("cannot slash removed validator %s", consensusID)
	}

	// Calculate slash amount
	slashAmount, err := SlashAmount(slashPercent, v.BondedStake)
	if err != nil {
		return err
	}

	// If slash amount is zero, nothing to do
	if slashAmount.IsZero() {
		return nil
	}

	// Subtract slash amount from bonded stake
	newStake, err := v.BondedStake.Sub(slashAmount)
	if err != nil {
		return fmt.Errorf("slash underflow: %w", err)
	}
	v.BondedStake = newStake

	// Update total bonded
	total, err := TotalBonded(s)
	if err != nil {
		return err
	}
	newTotal, err := total.Sub(slashAmount)
	if err != nil {
		return fmt.Errorf("update total bonded: %w", err)
	}
	if err := setTotalBonded(s, newTotal); err != nil {
		return err
	}

	// If validator is Active, jail them
	if v.Status == ValidatorActive {
		currentEpoch, err := CurrentEpoch(s)
		if err != nil {
			return err
		}
		if err := v.transitionTo(ValidatorJailed); err != nil {
			return fmt.Errorf("jail validator: %w", err)
		}
		v.JailedUntil = currentEpoch + jailEpochs
		v.VotingPower = 0
	}

	// For Jailed/Unstaking validators: don't change status, just reduce stake
	// The reduced stake will be reflected when they unjail or unstake

	return UpdateValidator(s, v)
}

// SlashValidator is a convenience wrapper that resolves the offense from evidence
// and applies the appropriate slash using default parameters.
func SlashValidator(s KVStore, consensusID types.Address, ev types.Evidence) error {
	// Resolve offense from evidence
	offense, err := OffenseFromEvidence(ev)
	if err != nil {
		return err
	}

	// Get slash params
	params := GetSlashParams(s)

	// Determine slash percentage based on offense
	var slashPercent uint64
	switch offense {
	case SlashDoublePrevote:
		slashPercent = params.PercentDoublePrevote
	case SlashDoublePrecommit:
		slashPercent = params.PercentDoublePrecommit
	case SlashInvalidCommit:
		slashPercent = params.PercentInvalidCommit
	default:
		return fmt.Errorf("unknown slash offense: %d", offense)
	}

	// Apply slash with jail duration
	return ApplySlash(s, consensusID, slashPercent, params.JailEpochs)
}
