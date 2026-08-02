package consensus

import (
	"testing"

	"github.com/dsn/dsn/types"
)

func TestProposerAtHeight(t *testing.T) {
	validators := []types.Address{
		{1}, {2}, {3},
	}

	tests := []struct {
		height   uint64
		expected types.Address
	}{
		{0, types.Address{1}},
		{1, types.Address{2}},
		{2, types.Address{3}},
		{3, types.Address{1}}, // wraps around
		{4, types.Address{2}},
		{100, types.Address{2}}, // 100 % 3 = 1
	}

	for _, tt := range tests {
		got := ProposerAtHeight(tt.height, validators)
		if got != tt.expected {
			t.Errorf("height %d: expected %x, got %x", tt.height, tt.expected, got)
		}
	}
}

func TestProposerAtHeight_PanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty validators")
		}
	}()
	ProposerAtHeight(0, []types.Address{})
}

func TestProposerAtHeight_Single(t *testing.T) {
	validators := []types.Address{{99}}
	for height := uint64(0); height < 10; height++ {
		got := ProposerAtHeight(height, validators)
		if got != validators[0] {
			t.Errorf("height %d: single validator should always be proposer", height)
		}
	}
}
