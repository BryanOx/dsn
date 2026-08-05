package consensus

import (
	"testing"

	"github.com/BryanOx/dsn/staking"
	"github.com/stretchr/testify/require"
)

// TestHarness_RunRoundAt_RoundGreaterThanZero is the 2.8 RED test: the harness
// must be able to simulate a full round at a non-zero round — proposer
// selected via the round-aware schedule, a round-r proposal, round-r prevotes
// and precommits, and a committed round-r block whose commit proof carries
// round-r precommits (matching what block validation expects for a final
// block). RunRoundAt(0) must remain byte-compatible with the old RunRound.
func TestHarness_RunRoundAt_RoundGreaterThanZero(t *testing.T) {
	harness := NewBFTTestHarness(t, BFTConfig{NumValidators: 3, ByzantineIdx: -1})

	harness.RunRoundAt(1)

	committed := harness.GetCommittedBlock(1)
	require.NotNil(t, committed, "round-1 round must commit a block")
	require.Equal(t, uint32(1), committed.Header.Round, "committed block must carry the round it was proposed in")
	require.NotNil(t, committed.CommitProof)
	require.GreaterOrEqual(t, len(committed.CommitProof.Precommits), 1)
	for _, pc := range committed.CommitProof.Precommits {
		require.Equal(t, uint32(1), pc.Round, "commit proof must carry round-1 precommits")
	}

	// The proposer for (1,1) follows the round-aware schedule.
	activeVals, err := staking.GetActiveValidators(harness.State)
	require.NoError(t, err)
	require.Equal(t, WeightedProposerAtHeightAndRound(1, 1, activeVals), committed.Header.Proposer)

	// The old entry point must keep producing round-0 commits.
	harness.RunRoundAt(0)
	require.NotNil(t, harness.GetCommittedBlock(2), "round-0 commit at height 2")
	require.Equal(t, uint32(0), harness.GetCommittedBlock(2).Header.Round)
}
