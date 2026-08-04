package consensus

import (
	"testing"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestWeightedProposerAtHeight_EqualPower(t *testing.T) {
	// 3 validators, each with 100 power
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 100},
		{ConsensusID: types.Address{2}, VotingPower: 100},
		{ConsensusID: types.Address{3}, VotingPower: 100},
	}

	// Height 0 -> v[0], 100 -> v[1], 200 -> v[2], 300 -> v[0]
	tests := []struct {
		height   uint64
		expected types.Address
	}{
		{0, types.Address{1}},
		{100, types.Address{2}},
		{200, types.Address{3}},
		{300, types.Address{1}}, // wraps around
		{1, types.Address{1}},   // 1 % 300 = 1, first 100 range
		{50, types.Address{1}},  // 50 % 300 = 50, first 100 range
		{150, types.Address{2}}, // 150 % 300 = 150, second 100 range
		{250, types.Address{3}}, // 250 % 300 = 250, third 100 range
	}

	for _, tt := range tests {
		got := WeightedProposerAtHeight(tt.height, validators)
		require.Equal(t, tt.expected, got, "height %d", tt.height)
	}
}

func TestWeightedProposerAtHeight_UnequalPower(t *testing.T) {
	// v1=200, v2=100, v3=100 (total=400)
	// Height 0-199 -> v1, height 200-299 -> v2, height 300-399 -> v3
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 200},
		{ConsensusID: types.Address{2}, VotingPower: 100},
		{ConsensusID: types.Address{3}, VotingPower: 100},
	}

	tests := []struct {
		height   uint64
		expected types.Address
	}{
		{0, types.Address{1}},   // 0-199: v1
		{100, types.Address{1}}, // 0-199: v1
		{199, types.Address{1}}, // 0-199: v1
		{200, types.Address{2}}, // 200-299: v2
		{250, types.Address{2}}, // 200-299: v2
		{299, types.Address{2}}, // 200-299: v2
		{300, types.Address{3}}, // 300-399: v3
		{350, types.Address{3}}, // 300-399: v3
		{399, types.Address{3}}, // 300-399: v3
		{400, types.Address{1}}, // wraps: 400 % 400 = 0 -> v1
		{600, types.Address{2}}, // 600 % 400 = 200 -> v2
		{800, types.Address{1}}, // 800 % 400 = 0 -> v1
	}

	for _, tt := range tests {
		got := WeightedProposerAtHeight(tt.height, validators)
		require.Equal(t, tt.expected, got, "height %d", tt.height)
	}
}

func TestWeightedProposerAtHeight_SingleValidator(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{99}, VotingPower: 500},
	}

	for height := uint64(0); height < 100; height++ {
		got := WeightedProposerAtHeight(height, validators)
		require.Equal(t, validators[0].ConsensusID, got, "height %d: single validator should always be proposer", height)
	}
}

func TestWeightedProposerAtHeight_PanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty validators")
		}
	}()
	WeightedProposerAtHeight(0, []*staking.Validator{})
}

func TestWeightedProposerAtHeight_ZeroTotalPower(t *testing.T) {
	// All validators have 0 voting power - should fall back to round-robin
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 0},
		{ConsensusID: types.Address{2}, VotingPower: 0},
		{ConsensusID: types.Address{3}, VotingPower: 0},
	}

	// Should fall back to simple round-robin: height % len
	tests := []struct {
		height   uint64
		expected types.Address
	}{
		{0, types.Address{1}},
		{1, types.Address{2}},
		{2, types.Address{3}},
		{3, types.Address{1}},
	}

	for _, tt := range tests {
		got := WeightedProposerAtHeight(tt.height, validators)
		require.Equal(t, tt.expected, got, "height %d", tt.height)
	}
}

func TestWeightedProposerAtHeight_Deterministic(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 300},
		{ConsensusID: types.Address{2}, VotingPower: 200},
		{ConsensusID: types.Address{3}, VotingPower: 100},
	}

	// Run same query multiple times - should always get same result
	for i := 0; i < 100; i++ {
		got1 := WeightedProposerAtHeight(150, validators)
		got2 := WeightedProposerAtHeight(150, validators)
		require.Equal(t, got1, got2, "height 150 should be deterministic")
	}
}

// TestWeightedProposerAtHeightAndRound_Round0EqualsAtHeight is RED for the
// round-aware schedule (S2): at round 0 the new schedule MUST reproduce the
// height-only schedule exactly.
func TestWeightedProposerAtHeightAndRound_Round0EqualsAtHeight(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 200},
		{ConsensusID: types.Address{2}, VotingPower: 100},
		{ConsensusID: types.Address{3}, VotingPower: 100},
	}

	for height := uint64(0); height < 500; height += 7 {
		require.Equal(t,
			WeightedProposerAtHeight(height, validators),
			WeightedProposerAtHeightAndRound(height, 0, validators),
			"round 0 must equal the height-only schedule at height %d", height)
	}
}

// TestWeightedProposerAtHeightAndRound_RoundRotates is RED for the round-aware
// schedule (S2): offset (height+round) % totalPower, so different rounds select
// different proposers when the offset crosses a power boundary.
func TestWeightedProposerAtHeightAndRound_RoundRotates(t *testing.T) {
	// Equal power 100 each: ranges [0,100) -> v1, [100,200) -> v2, [200,300) -> v3.
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 100},
		{ConsensusID: types.Address{2}, VotingPower: 100},
		{ConsensusID: types.Address{3}, VotingPower: 100},
	}

	tests := []struct {
		height   uint64
		round    uint32
		expected types.Address
	}{
		{0, 0, types.Address{1}},     // (0+0)%300 = 0
		{0, 1, types.Address{1}},     // (0+1)%300 = 1
		{0, 99, types.Address{1}},    // (0+99)%300 = 99
		{0, 100, types.Address{2}},   // (0+100)%300 = 100
		{0, 200, types.Address{3}},   // (0+200)%300 = 200
		{100, 0, types.Address{2}},   // (100+0)%300 = 100
		{100, 100, types.Address{3}}, // (100+100)%300 = 200
		{100, 200, types.Address{1}}, // (100+200)%300 = 0
	}

	for _, tt := range tests {
		got := WeightedProposerAtHeightAndRound(tt.height, tt.round, validators)
		require.Equal(t, tt.expected, got, "height %d round %d", tt.height, tt.round)
	}

	// Explicit S2 lockstep requirement: a round change must be able to change
	// the proposer at the same height.
	require.NotEqual(t,
		WeightedProposerAtHeightAndRound(100, 0, validators),
		WeightedProposerAtHeightAndRound(100, 100, validators),
		"different rounds at the same height must be able to select different proposers")
}

// TestWeightedProposerAtHeightAndRound_ZeroTotalPower triangulates the
// zero-power fallback: (height+round) % len with round included.
func TestWeightedProposerAtHeightAndRound_ZeroTotalPower(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 0},
		{ConsensusID: types.Address{2}, VotingPower: 0},
		{ConsensusID: types.Address{3}, VotingPower: 0},
	}

	tests := []struct {
		height   uint64
		round    uint32
		expected types.Address
	}{
		{0, 0, types.Address{1}}, // (0+0)%3 = 0
		{0, 1, types.Address{2}}, // (0+1)%3 = 1
		{0, 2, types.Address{3}}, // (0+2)%3 = 2
		{1, 0, types.Address{2}}, // (1+0)%3 = 1
		{1, 2, types.Address{1}}, // (1+2)%3 = 0
		{2, 1, types.Address{1}}, // (2+1)%3 = 0
		{2, 0, types.Address{3}}, // (2+0)%3 = 2
	}

	for _, tt := range tests {
		got := WeightedProposerAtHeightAndRound(tt.height, tt.round, validators)
		require.Equal(t, tt.expected, got, "height %d round %d", tt.height, tt.round)
	}
}

// TestWeightedProposerAtHeightAndRound_SingleValidator triangulates the
// single-validator path: the same validator proposes at every height and round.
func TestWeightedProposerAtHeightAndRound_SingleValidator(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{99}, VotingPower: 500},
	}

	for height := uint64(0); height < 50; height++ {
		for round := uint32(0); round < 5; round++ {
			require.Equal(t, validators[0].ConsensusID,
				WeightedProposerAtHeightAndRound(height, round, validators),
				"single validator must propose at every height %d round %d", height, round)
		}
	}
}

// TestWeightedProposerAtHeightAndRound_PanicsOnEmpty triangulates the empty
// validator set guard.
func TestWeightedProposerAtHeightAndRound_PanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty validators")
		}
	}()
	WeightedProposerAtHeightAndRound(0, 0, []*staking.Validator{})
}
