package consensus

import (
	"testing"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/types"
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
