package node

import (
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/network"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// TestConsensus_LoopProducesConsecutiveHeights is the V1 regression test.
// The consensus loop must produce every height exactly once. Before the fix a
// second height++ in the idle-wait branch skipped every other height, so the
// produced heights were 1,3,5,... and even heights (including epoch
// boundaries) were never built. The test asserts that every height up to N is
// produced and stored, and that epochs advance exactly at boundaries.
func TestConsensus_LoopProducesConsecutiveHeights(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.ProposerTimeout = 100 * time.Millisecond
	cfg.DataDir = t.TempDir() // persistent: the loop stores every produced header
	cfg.IndexerEnabled = false
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	// Register and immediately activate a single validator, mirroring how
	// genesis initializes validators (currentEpoch stays 0). A small
	// BlocksPerEpoch lets the test observe several epoch boundaries in a
	// short chain.
	const blocksPerEpoch = 5
	registerValidator(t, n.State(), kp, 1, 100_000)
	require.NoError(t, staking.SetBlocksPerEpoch(n.State(), blocksPerEpoch))
	cid := types.DeriveConsensusID(kp.PublicKey)
	require.NoError(t, staking.ActivateValidator(n.State(), cid, 1))
	require.NoError(t, staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000))

	// The single validator selects itself as proposer for every height.
	require.Equal(t, cid, n.consensusProposerAtHeight(1))

	// Wire P2P + validator set so StartConsensus runs the real loop goroutine.
	p2p, err := network.NewP2PNode(0)
	require.NoError(t, err)
	n.p2p = p2p
	n.validators = []types.Address{kp.Address()}
	n.StartConsensus()
	defer n.StopConsensus()

	const wantHeights = 15
	require.Eventually(t, func() bool {
		return n.CurrentHeight() >= wantHeights
	}, 30*time.Second, 25*time.Millisecond)

	// Every height from 1..wantHeights must have been built and stored. The
	// pre-fix loop skipped even heights, so LoadBlockHeader would fail there.
	for height := uint64(1); height <= wantHeights; height++ {
		header, err := consensus.LoadBlockHeader(n.persistent, height)
		require.NoError(t, err, "height %d must be produced exactly once", height)
		require.Equal(t, height, header.Height)
		require.Equal(t, wantEpochAtHeight(height, blocksPerEpoch), header.Epoch,
			"height %d must carry the epoch expected at that boundary", height)
	}
}

// wantEpochAtHeight returns the header epoch a boundary-aligned chain
// (currentEpoch starts at 0 and advances at each BlocksPerEpoch boundary)
// must carry at the given height.
func wantEpochAtHeight(height, blocksPerEpoch uint64) uint64 {
	if height%blocksPerEpoch == 0 {
		return height / blocksPerEpoch
	}
	return (height - 1) / blocksPerEpoch
}
