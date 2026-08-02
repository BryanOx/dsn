package staking

import (
	"fmt"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// AccountReaderWriter provides account operations needed for staking.
// Implemented by state.InMemoryState and state.PersistentState.
type AccountReaderWriter interface {
	GetAccount(addr types.Address) (*state.Account, error)
	SetAccount(addr types.Address, account *state.Account) error
}

// StakingState combines KVStore and account access for staking operations.
type StakingState interface {
	KVStore
	AccountReaderWriter
}

// Bond locks tokens from an account into a validator's bonded stake.
// The caller must ensure the validator exists and is in a valid state.
// Returns the validator's updated bonded stake.
func Bond(s StakingState, from types.Address, consensusID types.Address, amount types.Amount) (types.Amount, error) {
	// Get the account and verify balance
	acc, err := s.GetAccount(from)
	if err != nil {
		return types.Amount{}, fmt.Errorf("sender account: %w", err)
	}
	if acc.Balance.Cmp(amount) < 0 {
		return types.Amount{}, fmt.Errorf("insufficient balance: have %s, need %s", acc.Balance, amount)
	}

	// Get the validator
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return types.Amount{}, err
	}
	if v.Status != ValidatorPending && v.Status != ValidatorActive {
		return types.Amount{}, fmt.Errorf("validator %s cannot accept stake in status %s", consensusID, v.Status)
	}

	// Deduct from account
	if err := acc.SubBalance(amount); err != nil {
		return types.Amount{}, err
	}
	if err := s.SetAccount(from, acc); err != nil {
		return types.Amount{}, err
	}

	// Add to validator's bonded stake
	newStake, err := v.BondedStake.Add(amount)
	if err != nil {
		return types.Amount{}, err
	}
	v.BondedStake = newStake

	// Update total bonded
	total, err := TotalBonded(s)
	if err != nil {
		return types.Amount{}, err
	}
	newTotal, err := total.Add(amount)
	if err != nil {
		return types.Amount{}, err
	}
	if err := setTotalBonded(s, newTotal); err != nil {
		return types.Amount{}, err
	}

	// Persist validator changes (recalculates voting power)
	if err := UpdateValidator(s, v); err != nil {
		return types.Amount{}, err
	}

	return v.BondedStake, nil
}

// Slash reduces a validator's bonded stake by the given amount.
// The slashed amount is burned (removed from total bonded supply).
// Returns the actual amount slashed (may be less if stake is insufficient).
func Slash(s StakingState, consensusID types.Address, amount types.Amount) (types.Amount, error) {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return types.Amount{}, err
	}

	// Cannot slash more than bonded stake
	var slashed types.Amount
	if v.BondedStake.Cmp(amount) >= 0 {
		slashed = amount
	} else {
		slashed = v.BondedStake
	}

	newStake, err := v.BondedStake.Sub(slashed)
	if err != nil {
		return types.Amount{}, err
	}
	v.BondedStake = newStake

	// Update total bonded
	total, err := TotalBonded(s)
	if err != nil {
		return types.Amount{}, err
	}
	newTotal, err := total.Sub(slashed)
	if err != nil {
		return types.Amount{}, err
	}
	if err := setTotalBonded(s, newTotal); err != nil {
		return types.Amount{}, err
	}

	// Persist
	if err := UpdateValidator(s, v); err != nil {
		return types.Amount{}, err
	}

	return slashed, nil
}

// ReleaseStake returns a validator's bonded stake to its address and removes it.
// Called after unstaking cooldown completes.
func ReleaseStake(s StakingState, consensusID types.Address) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if v.Status != ValidatorUnstaking {
		return fmt.Errorf("%w: validator %s is %s", ErrValidatorNotUnstaking, consensusID, v.Status)
	}

	// Return bonded stake to validator's operator address
	acc, err := s.GetAccount(v.OperatorAddress)
	if err != nil {
		// If the account doesn't exist, create it (shouldn't happen for validators)
		return fmt.Errorf("validator account not found: %w", err)
	}
	if err := acc.AddBalance(v.BondedStake); err != nil {
		return err
	}
	if err := s.SetAccount(v.OperatorAddress, acc); err != nil {
		return err
	}

	// Remove from registry
	return RemoveValidator(s, consensusID)
}

// DistributeRewards distributes fees to validators proportionally to voting power.
// Total fees are split: 70% to validators, 20% burn, 10% treasury.
// Returns (validatorShare, burnShare, treasuryShare).
func DistributeRewards(s StakingState, totalFees types.Amount) (types.Amount, types.Amount, types.Amount, error) {
	validatorShare := amountFraction(totalFees, 70, 100)
	burnShare := amountFraction(totalFees, 20, 100)
	treasuryShare := amountFraction(totalFees, 10, 100)

	if totalFees.IsZero() {
		return types.NewAmount(0), types.NewAmount(0), types.NewAmount(0), nil
	}

	// Distribute validator share proportionally to voting power
	active, err := GetActiveValidators(s)
	if err != nil {
		return types.Amount{}, types.Amount{}, types.Amount{}, err
	}

	if len(active) == 0 || validatorShare.IsZero() {
		return validatorShare, burnShare, treasuryShare, nil
	}

	// Calculate total voting power
	var totalPower uint64
	for _, v := range active {
		totalPower += v.VotingPower
	}

	if totalPower == 0 {
		return validatorShare, burnShare, treasuryShare, nil
	}

	// Distribute proportionally
	// Note: integer division may leave remainder. We use the largest-remainder
	// method common in proportional allocation: distribute floor amounts, then
	// give the remainder to the validator with the most voting power.
	vp := validatorShareToUint64(validatorShare)
	distributed := uint64(0)
	rewards := make([]uint64, len(active))

	for i, v := range active {
		portion := vp * v.VotingPower / totalPower
		rewards[i] = portion
		distributed += portion
	}

	// Give remainder to top validator by voting power
	remainder := vp - distributed
	if remainder > 0 && len(active) > 0 {
		// Top validator is at index 0 (sorted by power DESC)
		rewards[0] += remainder
	}

	// Apply rewards to validator accounts
	for i, v := range active {
		if rewards[i] == 0 {
			continue
		}
		reward := types.NewAmount(rewards[i])

		acc, err := s.GetAccount(v.OperatorAddress)
		if err != nil {
			// Create account if it doesn't exist
			acc = NewValidatorAccount(s, v.OperatorAddress)
		}
		if err := acc.AddBalance(reward); err != nil {
			return types.Amount{}, types.Amount{}, types.Amount{}, err
		}
		if err := s.SetAccount(v.OperatorAddress, acc); err != nil {
			return types.Amount{}, types.Amount{}, types.Amount{}, err
		}
	}

	return validatorShare, burnShare, treasuryShare, nil
}

var BurnAddress = types.Address{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

func BurnTokens(s StakingState, amount types.Amount) error {
	if amount.IsZero() {
		return nil
	}
	acc, err := s.GetAccount(BurnAddress)
	if err != nil {
		acc = state.NewAccount(BurnAddress, [32]byte{})
	}
	if err := acc.AddBalance(amount); err != nil {
		return fmt.Errorf("credit burn address: %w", err)
	}
	if err := s.SetAccount(BurnAddress, acc); err != nil {
		return fmt.Errorf("set burn account: %w", err)
	}
	currentSupply := ReadUint64(s, KeyTotalSupply)
	burnAmount := stakeToUint64(amount)
	if burnAmount > currentSupply {
		return fmt.Errorf("burn amount %d exceeds total supply %d", burnAmount, currentSupply)
	}
	newSupply := currentSupply - burnAmount
	if err := WriteUint64(s, KeyTotalSupply, newSupply); err != nil {
		return fmt.Errorf("update total supply after burn: %w", err)
	}
	return nil
}

// NewValidatorAccount creates an account for a validator with zero balance.
func NewValidatorAccount(s StakingState, addr types.Address) *state.Account {
	return state.NewAccount(addr, [32]byte{})
}

// amountFraction computes (amount * numerator / denominator).
// Uses uint64 arithmetic safe for v1 amounts.
func amountFraction(amount types.Amount, numerator, denominator uint64) types.Amount {
	val := stakeToUint64(amount)
	result := val * numerator / denominator
	return types.NewAmount(result)
}

// validatorShareToUint64 converts an Amount to uint64 for reward computation.
func validatorShareToUint64(amount types.Amount) uint64 {
	return stakeToUint64(amount)
}
