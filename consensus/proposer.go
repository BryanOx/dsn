package consensus

import (
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/types"
)

// WeightedProposerAtHeight selects a proposer using voting-power-weighted round-robin.
// validators must be sorted by VotingPower DESC, ConsensusID ASC (as returned by staking.GetActiveValidators).
// Panics if validators is empty (programming error).
func WeightedProposerAtHeight(height uint64, validators []*staking.Validator) types.Address {
	if len(validators) == 0 {
		panic("consensus: empty validator set")
	}

	// Calculate total voting power
	var totalVotingPower uint64
	for _, v := range validators {
		totalVotingPower += v.VotingPower
	}

	// If total power is 0, fall back to simple round-robin
	if totalVotingPower == 0 {
		return validators[height%uint64(len(validators))].ConsensusID
	}

	// Compute offset into the power range
	offset := height % totalVotingPower

	// Iterate through validators, accumulating cumulative power
	// When offset < cumulative, that validator is selected
	var cumulative uint64
	for i, v := range validators {
		cumulative += v.VotingPower
		if offset < cumulative {
			return v.ConsensusID
		}
		// Handle edge case: if offset exactly equals total (shouldn't happen with modulo)
		// but be safe and return last validator
		if i == len(validators)-1 && offset >= cumulative-totalVotingPower {
			return v.ConsensusID
		}
	}

	// Fallback (should never reach here with proper modulo arithmetic)
	return validators[len(validators)-1].ConsensusID
}