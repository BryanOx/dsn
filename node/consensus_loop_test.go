package node

import (
	"bytes"
	"sync"
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

// TestConsensus_RebroadcastsPendingProposal is the regression test for the
// connectivity-loss stall: the proposer broadcasts the height-1 proposal once
// at startup, before peers register their inbound connections, and the
// pendingHeight guard made the consensus loop return on every later tick, so
// the proposal was never re-sent and no votes ever arrived — the chain sat at
// height 0 forever. The loop must now re-broadcast the stored proposal on each
// ProposerTimeout while the round is pending, and must stop once the height is
// finalized.
func TestConsensus_RebroadcastsPendingProposal(t *testing.T) {
	// Two validators of equal power, A < B, so A proposes height 1 but cannot
	// reach the 2/3 precommit majority alone and the round stays pending
	// across ticks until B's precommit is delivered.
	var kpA, kpB *wallet.KeyPair
	for {
		a, err := wallet.GenerateKey()
		require.NoError(t, err)
		b, err := wallet.GenerateKey()
		require.NoError(t, err)
		cidA := types.DeriveConsensusID(a.PublicKey)
		cidB := types.DeriveConsensusID(b.PublicKey)
		if bytes.Compare(cidA[:], cidB[:]) < 0 {
			kpA, kpB = a, b
			break
		}
	}

	cfg := DefaultConfig()
	cfg.ProposerTimeout = 50 * time.Millisecond
	cfg.DataDir = t.TempDir() // persistent: finalization stores headers
	cfg.IndexerEnabled = false
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kpA)

	const stake = 100_000
	registerValidator(t, n.State(), kpA, 1, stake)
	registerValidator(t, n.State(), kpB, 2, stake)
	cidA := types.DeriveConsensusID(kpA.PublicKey)
	cidB := types.DeriveConsensusID(kpB.PublicKey)
	require.NoError(t, staking.ActivateValidator(n.State(), cidA, 1))
	require.NoError(t, staking.ActivateValidator(n.State(), cidB, 1))
	require.NoError(t, staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000))

	// With equal power the height-1 proposer is the smaller ConsensusID.
	require.Equal(t, cidA, n.consensusProposerAtHeight(1))

	p2p, err := network.NewP2PNode(0)
	require.NoError(t, err)
	n.p2p = p2p
	n.validators = []types.Address{kpA.Address()}

	// Observer peer records every proposal frame it receives, so we can count
	// how many times the same height-1 proposal is re-broadcast.
	var mu sync.Mutex
	var received []*types.Block
	observer, err := network.NewP2PNode(0)
	require.NoError(t, err)
	defer observer.Close()
	observer.SetBlockHandler(func(data []byte) {
		block, err := consensus.DecodeBlockMessage(data)
		if err != nil {
			return
		}
		mu.Lock()
		received = append(received, block)
		mu.Unlock()
	})
	require.NoError(t, observer.Connect(p2p.Addr()))

	n.StartConsensus()
	defer n.StopConsensus()

	countHeight1 := func() int {
		mu.Lock()
		defer mu.Unlock()
		c := 0
		for _, b := range received {
			if b.Header.Height == 1 {
				c++
			}
		}
		return c
	}

	// The initial broadcast must reach the observer (re-broadcast covers the
	// race where the observer connects after the very first send).
	require.Eventually(t, func() bool { return countHeight1() >= 1 },
		10*time.Second, 20*time.Millisecond)

	// While the round is pending, the proposer re-broadcasts on every timeout.
	first := countHeight1()
	require.Eventually(t, func() bool { return countHeight1() > first },
		10*time.Second, 20*time.Millisecond)
	time.Sleep(8 * cfg.ProposerTimeout)
	second := countHeight1()
	require.Greater(t, second, first+2, "proposal must be re-broadcast on subsequent timeouts")

	// Finalize: deliver B's precommit for the pending block.
	n.voteMu.Lock()
	hash := n.pendingHash
	n.voteMu.Unlock()
	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: hash,
		Validator: cidB,
	}
	require.NoError(t, precommit.Sign(kpB.PrivateKey[:]))
	n.handleVoteMessage(precommit)

	require.Eventually(t, func() bool { return n.CurrentHeight() >= 1 },
		10*time.Second, 20*time.Millisecond)

	// Once the height is finalized the re-broadcast must stop. Allow in-flight
	// proposal frames that raced the finalization to drain, then require that
	// no further height-1 proposals arrive.
	time.Sleep(10 * cfg.ProposerTimeout)
	after := countHeight1()
	time.Sleep(10 * cfg.ProposerTimeout)
	require.Equal(t, after, countHeight1(), "no re-broadcast after finalization")
}

// TestConsensus_RefreshesStaleProposal covers the connectivity-loss stall from
// the timestamp side: the proposer builds the height-1 proposal once at startup
// and types.ValidateTimestamp rejects blocks older than the skew window, so
// peers that connect late can never validate the SAME proposal bytes and never
// vote. The proposer must instead rebuild the pending proposal with a fresh
// timestamp and hash once it goes stale (and while nobody has voted on it).
// This mirrors the localnet: node0 proposes at boot, node1/node2 connect after
// the skew window and would otherwise stall the chain at height 0 forever.
func TestConsensus_RefreshesStaleProposal(t *testing.T) {
	var kpA, kpB *wallet.KeyPair
	for {
		a, err := wallet.GenerateKey()
		require.NoError(t, err)
		b, err := wallet.GenerateKey()
		require.NoError(t, err)
		cidA := types.DeriveConsensusID(a.PublicKey)
		cidB := types.DeriveConsensusID(b.PublicKey)
		if bytes.Compare(cidA[:], cidB[:]) < 0 {
			kpA, kpB = a, b
			break
		}
	}

	newNode := func(kp *wallet.KeyPair) *Node {
		cfg := DefaultConfig()
		cfg.ProposerTimeout = 50 * time.Millisecond
		cfg.DataDir = t.TempDir()
		cfg.IndexerEnabled = false
		nd, err := New(cfg)
		require.NoError(t, err)
		nd.SetWallet(kp)
		registerValidator(t, nd.State(), kpA, 1, 100_000)
		registerValidator(t, nd.State(), kpB, 2, 100_000)
		require.NoError(t, staking.ActivateValidator(nd.State(), types.DeriveConsensusID(kpA.PublicKey), 1))
		require.NoError(t, staking.ActivateValidator(nd.State(), types.DeriveConsensusID(kpB.PublicKey), 1))
		require.NoError(t, staking.WriteUint64(nd.State(), staking.KeyTotalSupply, 10_000_000))
		p2p, err := network.NewP2PNode(0)
		require.NoError(t, err)
		nd.p2p = p2p
		nd.validators = []types.Address{kp.Address()}
		return nd
	}

	// A proposes height 1 (smaller ConsensusID); B votes. B is kept
	// disconnected until A's proposal has gone stale.
	nA := newNode(kpA)
	defer nA.Close()
	nB := newNode(kpB)
	defer nB.Close()

	require.Equal(t, types.DeriveConsensusID(kpA.PublicKey), nA.consensusProposerAtHeight(1))

	nA.StartConsensus()
	defer nA.StopConsensus()
	nB.StartConsensus()
	defer nB.StopConsensus()

	// Wait for A's pending round, then age its proposal past the skew window.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil
	}, 10*time.Second, 20*time.Millisecond)

	nA.voteMu.Lock()
	oldHash := nA.pendingHash
	nA.pendingBlock.Header.Timestamp -= 60 // now 60s stale
	nA.voteMu.Unlock()

	// A must replace the stale unvoted proposal with a fresh one.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHash != oldHash
	}, 10*time.Second, 20*time.Millisecond)

	// Connect the late peer: it validates the refreshed proposal, votes, and
	// the round finalizes — the chain leaves height 0.
	require.NoError(t, nB.P2P().Connect(nA.P2P().Addr()))

	require.Eventually(t, func() bool {
		return nA.CurrentHeight() >= 1 && nB.CurrentHeight() >= 1
	}, 15*time.Second, 20*time.Millisecond)
}

// generateTwoValidators returns two fresh key pairs ordered so that kpA's
// ConsensusID sorts before kpB's (with equal power, the smaller ConsensusID
// proposes the first height).
func generateTwoValidators(t *testing.T) (kpA, kpB *wallet.KeyPair) {
	t.Helper()
	for {
		a, err := wallet.GenerateKey()
		require.NoError(t, err)
		b, err := wallet.GenerateKey()
		require.NoError(t, err)
		cidA := types.DeriveConsensusID(a.PublicKey)
		cidB := types.DeriveConsensusID(b.PublicKey)
		if bytes.Compare(cidA[:], cidB[:]) < 0 {
			return a, b
		}
	}
}

// newTwoValidatorNode builds a node whose wallet is kp and whose state is
// registered with two equal-power validators (kpA and kpB) activated at epoch
// 1. blocksPerEpoch configures the epoch-boundary cadence.
func newTwoValidatorNode(t *testing.T, kp, kpA, kpB *wallet.KeyPair, blocksPerEpoch uint64) *Node {
	t.Helper()
	cfg := DefaultConfig()
	cfg.ProposerTimeout = 50 * time.Millisecond
	cfg.DataDir = t.TempDir()
	cfg.IndexerEnabled = false
	nd, err := New(cfg)
	require.NoError(t, err)
	nd.SetWallet(kp)
	registerValidator(t, nd.State(), kpA, 1, 100_000)
	registerValidator(t, nd.State(), kpB, 2, 100_000)
	require.NoError(t, staking.ActivateValidator(nd.State(), types.DeriveConsensusID(kpA.PublicKey), 1))
	require.NoError(t, staking.ActivateValidator(nd.State(), types.DeriveConsensusID(kpB.PublicKey), 1))
	require.NoError(t, staking.WriteUint64(nd.State(), staking.KeyTotalSupply, 10_000_000))
	require.NoError(t, staking.SetBlocksPerEpoch(nd.State(), blocksPerEpoch))
	p2p, err := network.NewP2PNode(0)
	require.NoError(t, err)
	nd.p2p = p2p
	nd.validators = []types.Address{kp.Address()}
	return nd
}

// TestConsensus_RefreshesStaleBoundaryProposal is the regression test for the
// epoch-transition double-validation bug. proposeForHeight used to leave the
// proposer's state one block ahead of currentHeight after its self-validation
// replay, so at an epoch boundary the transition ran twice: re-validating the
// echoed proposal failed with "epoch mismatch", and a needed refresh could not
// rebuild the block at all (BuildBlock's BeginBlock hit the same mismatch),
// stalling the chain. The fix reverts the proposal replay and applies the block
// in finalizeLocalBlock — mirroring the receiver path — so the transition runs
// exactly once, when the boundary block finalizes. blocksPerEpoch=1 makes
// height 1 the first boundary.
func TestConsensus_RefreshesStaleBoundaryProposal(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)

	const blocksPerEpoch = 1 // every height is an epoch boundary; height 1 is the first
	nA := newTwoValidatorNode(t, kpA, kpA, kpB, blocksPerEpoch)
	defer nA.Close()
	nB := newTwoValidatorNode(t, kpB, kpA, kpB, blocksPerEpoch)
	defer nB.Close()

	require.Equal(t, types.DeriveConsensusID(kpA.PublicKey), nA.consensusProposerAtHeight(1))

	nA.StartConsensus()
	defer nA.StopConsensus()
	nB.StartConsensus()
	defer nB.StopConsensus()

	// A proposes the boundary block at height 1; B is disconnected, so no
	// votes arrive and the round stays pending across ticks.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil
	}, 10*time.Second, 20*time.Millisecond)

	// Proposing the boundary block must NOT advance the epoch: the proposal
	// replay is reverted, so the transition belongs to finalization. Before
	// the fix the state was left at epoch 1 here.
	nA.voteMu.Lock()
	epoch, err := staking.CurrentEpoch(nA.State())
	nA.voteMu.Unlock()
	require.NoError(t, err)
	require.Equal(t, uint64(0), epoch, "proposing a boundary block must not advance the epoch")

	// A stale, unvoted boundary proposal must be refreshable: rebuilding it
	// runs BuildBlock's BeginBlock on a pre-transition state, so no "epoch
	// mismatch" aborts the refresh.
	nA.voteMu.Lock()
	oldHash := nA.pendingHash
	nA.pendingBlock.Header.Timestamp -= 60 // now 60s stale
	nA.voteMu.Unlock()

	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHash != oldHash
	}, 10*time.Second, 20*time.Millisecond)

	// The late peer validates the refreshed boundary proposal and votes; the
	// round finalizes across the boundary with exactly one epoch transition on
	// both nodes.
	require.NoError(t, nB.P2P().Connect(nA.P2P().Addr()))

	require.Eventually(t, func() bool {
		return nA.CurrentHeight() >= 1 && nB.CurrentHeight() >= 1
	}, 15*time.Second, 20*time.Millisecond)

	for _, tc := range []struct {
		name string
		node *Node
	}{{"A", nA}, {"B", nB}} {
		epoch, err := staking.CurrentEpoch(tc.node.State())
		require.NoError(t, err)
		require.Equal(t, uint64(1), epoch, "%s must transition the epoch exactly once", tc.name)
	}

	// The finalized boundary block must carry the new epoch.
	header, err := consensus.LoadBlockHeader(nA.persistent, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), header.Epoch)
}
