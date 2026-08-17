package consensus

import (
	"testing"

	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestEvidencePoolAddDedup(t *testing.T) {
	pool := NewEvidencePool(100)

	voteA := types.Vote{
		VoteType:  types.VotePrevote,
		Height:    10,
		Round:     0,
		BlockHash: types.Hash{0xAA},
		Validator: types.Address{0x01},
		Signature: make([]byte, 64),
	}
	voteB := types.Vote{
		VoteType:  types.VotePrevote,
		Height:    10,
		Round:     0,
		BlockHash: types.Hash{0xBB},
		Validator: types.Address{0x01},
		Signature: make([]byte, 64),
	}

	// Two identical DoubleSignEvidence (same votes = same evidence)
	ev1 := &types.DoubleSignEvidence{VoteA: voteA, VoteB: voteB}
	ev2 := &types.DoubleSignEvidence{VoteA: voteA, VoteB: voteB}

	// First add succeeds
	require.NoError(t, pool.Add(ev1))
	require.Equal(t, 1, pool.Len())

	// Second add fails (duplicate)
	require.Error(t, pool.Add(ev2))
	require.Equal(t, 1, pool.Len())

	// Drain returns the single evidence and empties the pool
	drained := pool.Drain(10)
	require.Len(t, drained, 1)
	require.Equal(t, 0, pool.Len())

	// After draining, the pool is empty but ev2 is still "seen" (dedup persists)
	require.Error(t, pool.Add(ev2), "duplicate should be rejected even after drain")
}
