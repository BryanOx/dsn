package consensus

import (
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
)

// WeightedProposerAtHeightAndRound selects a proposer for (height, round) using
// voting-power-weighted round-robin with a round offset: (height+round) %
// totalPower. validators must be sorted by VotingPower DESC, ConsensusID ASC
// (as returned by staking.GetActiveValidators). Round 0 reproduces the
// height-only schedule exactly. Panics if validators is empty (programming error).
func WeightedProposerAtHeightAndRound(height uint64, round uint32, validators []*staking.Validator) types.Address {
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
		return validators[(height+uint64(round))%uint64(len(validators))].ConsensusID
	}

	// Compute offset into the power range
	offset := (height + uint64(round)) % totalVotingPower

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

// WeightedProposerAtHeight selects a proposer using voting-power-weighted round-robin.
// Round-0 alias of WeightedProposerAtHeightAndRound: preserves the historical
// height-only schedule exactly for round 0.
func WeightedProposerAtHeight(height uint64, validators []*staking.Validator) types.Address {
	return WeightedProposerAtHeightAndRound(height, 0, validators)
}
