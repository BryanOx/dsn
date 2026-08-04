package consensus

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
)

// VotingThresholdNumerator is the numerator for the 2/3 threshold.
const VotingThresholdNumerator = 2

// VotingThresholdDenominator is the denominator for the 2/3 threshold.
const VotingThresholdDenominator = 3

// VotingState manages vote collection and finality for a single block height/round.
// Validated against a frozen ValidatorSnapshot - NOT the latest validator state.
// This is protocol-critical for deterministic replay.
type VotingState struct {
	height         uint64
	round          uint32
	blockHash      types.Hash
	snapshot       *staking.ValidatorSnapshot
	prevotes       map[types.Address]*types.Vote
	precommits     map[types.Address]*types.Vote
	prevotePower   uint64
	precommitPower uint64
}

// NewVotingState creates a voting state for the given height/round and validator snapshot.
func NewVotingState(height uint64, round uint32, blockHash types.Hash, snapshot *staking.ValidatorSnapshot) *VotingState {
	return &VotingState{
		height:     height,
		round:      round,
		blockHash:  blockHash,
		snapshot:   snapshot,
		prevotes:   make(map[types.Address]*types.Vote),
		precommits: make(map[types.Address]*types.Vote),
	}
}

// AddPrevote adds a prevote for the block hash.
// Returns error if:
// - vote signature is invalid
// - vote height/round/blockHash don't match
// - validator not in snapshot
// - validator already prevoted (no double votes)
func (vs *VotingState) AddPrevote(vote *types.Vote) error {
	return vs.addVote(vote, &vs.prevotePower, vs.prevotes)
}

// AddPrecommit adds a precommit for the block hash.
// Same validation rules as AddPrevote.
func (vs *VotingState) AddPrecommit(vote *types.Vote) error {
	return vs.addVote(vote, &vs.precommitPower, vs.precommits)
}

// addVote adds a vote (prevote or precommit) to the voting state.
func (vs *VotingState) addVote(vote *types.Vote, power *uint64, votes map[types.Address]*types.Vote) error {
	// 1. Check vote height/round/blockHash match
	if vote.Height != vs.height {
		return fmt.Errorf("vote height %d != expected %d", vote.Height, vs.height)
	}
	if vote.Round != vs.round {
		return fmt.Errorf("vote round %d != expected %d", vote.Round, vs.round)
	}
	if vote.BlockHash != vs.blockHash {
		return fmt.Errorf("vote block hash %x != expected %x", vote.BlockHash[:], vs.blockHash[:])
	}

	// 2. Check validator is in snapshot and hasn't already voted
	if _, exists := votes[vote.Validator]; exists {
		return fmt.Errorf("duplicate vote from validator %s", vote.Validator)
	}

	// 3. Look up validator in snapshot
	val, err := vs.snapshot.ValidatorByID(vote.Validator)
	if err != nil {
		return fmt.Errorf("validator %s not in snapshot epoch %d: %v", vote.Validator, vs.snapshot.Epoch, err)
	}
	pubKey := val.PublicKey
	valPower := val.VotingPower

	// 4. Verify signature
	if !vote.Verify(pubKey) {
		return fmt.Errorf("invalid vote signature from validator %s", vote.Validator)
	}

	// 5. Store vote and accumulate power
	votes[vote.Validator] = vote
	*power += valPower
	return nil
}

// PrevotePower returns the total voting power that has prevoted.
func (vs *VotingState) PrevotePower() uint64 {
	return vs.prevotePower
}

// PrecommitPower returns the total voting power that has precommitted.
func (vs *VotingState) PrecommitPower() uint64 {
	return vs.precommitPower
}

// HasPrevoteMajority returns true if prevote power >= 2/3 of total snapshot power.
func (vs *VotingState) HasPrevoteMajority() bool {
	if vs.snapshot.TotalPower == 0 {
		return false
	}
	return vs.prevotePower*VotingThresholdDenominator >= vs.snapshot.TotalPower*VotingThresholdNumerator
}

// HasPrecommitMajority returns true if precommit power >= 2/3 of total snapshot power.
func (vs *VotingState) HasPrecommitMajority() bool {
	if vs.snapshot.TotalPower == 0 {
		return false
	}
	return vs.precommitPower*VotingThresholdDenominator >= vs.snapshot.TotalPower*VotingThresholdNumerator
}

// BuildCommitProof creates a CommitProof from the current precommit state.
// Only precommits for this block's hash are included.
// Returns error if no precommit majority.
func (vs *VotingState) BuildCommitProof() (*types.CommitProof, error) {
	if !vs.HasPrecommitMajority() {
		return nil, fmt.Errorf("no precommit majority: %d/%d", vs.precommitPower, vs.snapshot.TotalPower)
	}

	precommits := make([]types.Vote, 0, len(vs.precommits))
	for _, v := range vs.precommits {
		precommits = append(precommits, *v)
	}

	// Sort precommits by validator address for deterministic order
	sort.Slice(precommits, func(i, j int) bool {
		return bytes.Compare(precommits[i].Validator[:], precommits[j].Validator[:]) < 0
	})

	return &types.CommitProof{
		Height:      vs.height,
		BlockHash:   vs.blockHash,
		Precommits:  precommits,
		TotalPower:  vs.snapshot.TotalPower,
		SignedPower: vs.precommitPower,
		SetHash:     vs.snapshot.SetHash,
	}, nil
}

// Prevotes returns all collected prevotes (copy).
func (vs *VotingState) Prevotes() []*types.Vote {
	result := make([]*types.Vote, 0, len(vs.prevotes))
	for _, v := range vs.prevotes {
		result = append(result, v)
	}
	return result
}

// Precommits returns all collected precommits (copy).
func (vs *VotingState) Precommits() []*types.Vote {
	result := make([]*types.Vote, 0, len(vs.precommits))
	for _, v := range vs.precommits {
		result = append(result, v)
	}
	return result
}

// Snapshot returns the validator snapshot for this voting state.
func (vs *VotingState) Snapshot() *staking.ValidatorSnapshot {
	return vs.snapshot
}
