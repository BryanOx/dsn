package node

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/network"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
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

	// Finalize: deliver B's votes for A's current pending round. Since PR2 the
	// node advances rounds on its deadline even while unvoted, so the votes
	// must target the round A is actually pending on (prevote first, then the
	// gated precommit).
	n.voteMu.Lock()
	hash := n.pendingHash
	round := n.pendingRound
	n.voteMu.Unlock()
	precommit := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     round,
		BlockHash: hash,
		Validator: cidB,
	}
	require.NoError(t, precommit.Sign(kpB.PrivateKey[:]))
	n.handleVoteMessage(precommit)
	precommit = &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     round,
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
	return newTwoValidatorNodeTimeout(t, kp, kpA, kpB, blocksPerEpoch, 50*time.Millisecond)
}

// newTwoValidatorNodeTimeout is newTwoValidatorNode with a configurable
// ProposerTimeout, so round-advance tests can widen the per-round deadline
// window and stay deterministic.
func newTwoValidatorNodeTimeout(t *testing.T, kp, kpA, kpB *wallet.KeyPair, blocksPerEpoch uint64, proposerTimeout time.Duration) *Node {
	t.Helper()
	cfg := DefaultConfig()
	cfg.ProposerTimeout = proposerTimeout
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

// TestConsensus_AdvancesToHigherRoundWhileUnvoted is the S5/S6/S7/S13 RED
// test. A round-0 proposal that receives no votes (peer offline) must not pin
// the node to round 0 forever: on the linear ProposerTimeout×(1+r) deadline
// the node advances its pending round (S5) even across rounds it does not
// propose, and re-proposes when the schedule returns to it (S6). Votes are
// scoped to (height, round): only B's round-2 prevote + precommit close the
// round, and the stored chain holds exactly that single round-2 value (S13).
func TestConsensus_AdvancesToHigherRoundWhileUnvoted(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)

	// 200ms base keeps each round deadline wide enough that B's votes can be
	// delivered deterministically while A is on its round-2 proposal.
	nA := newTwoValidatorNodeTimeout(t, kpA, kpA, kpB, 5, 200*time.Millisecond)
	defer nA.Close()

	require.Equal(t, types.DeriveConsensusID(kpA.PublicKey), nA.consensusProposerAtHeight(1))

	nA.StartConsensus()
	defer nA.StopConsensus()

	// Round 0: A proposes height 1; nobody votes, so the round stalls.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil && nA.pendingRound == 0
	}, 10*time.Second, 20*time.Millisecond)

	// S5: while the round-0 proposal stays unvoted, A must advance past round
	// 1 (B's round, no proposal) and re-propose when round 2 returns to A.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingRound == 2 && nA.pendingBlock != nil && nA.pendingBlock.Header.Round == 2
	}, 15*time.Second, 20*time.Millisecond)

	nA.voteMu.Lock()
	hash := nA.pendingHash
	nA.voteMu.Unlock()

	// S7: votes are scoped to (height, round). B's round-2 prevote lets A gate
	// its precommit on 2/3 prevotes; B's round-2 precommit closes the round.
	cidB := types.DeriveConsensusID(kpB.PublicKey)
	prevote := &types.Vote{VoteType: types.VotePrevote, Height: 1, Round: 2, BlockHash: hash, Validator: cidB}
	require.NoError(t, prevote.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(prevote)
	precommit := &types.Vote{VoteType: types.VotePrecommit, Height: 1, Round: 2, BlockHash: hash, Validator: cidB}
	require.NoError(t, precommit.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(precommit)

	require.Eventually(t, func() bool { return nA.CurrentHeight() >= 1 }, 10*time.Second, 20*time.Millisecond)

	// S13: the chain is single-valued — the stored height-1 block is the
	// round-2 value, never the abandoned round-0/round-1 candidates.
	header, err := consensus.LoadBlockHeader(nA.persistent, 1)
	require.NoError(t, err)
	require.Equal(t, uint32(2), header.Round)
}

// TestConsensus_RoundCappedAtMaxRound is the S8 RED test: the view change
// advances the pending round on the deadline but must never advance past
// MaxRound — at the cap the node waits and re-broadcasts instead of advancing.
func TestConsensus_RoundCappedAtMaxRound(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)

	cfg := DefaultConfig()
	cfg.ProposerTimeout = 50 * time.Millisecond
	cfg.MaxRound = 3
	cfg.DataDir = t.TempDir()
	cfg.IndexerEnabled = false
	nA, err := New(cfg)
	require.NoError(t, err)
	defer nA.Close()
	nA.SetWallet(kpA)
	registerValidator(t, nA.State(), kpA, 1, 100_000)
	registerValidator(t, nA.State(), kpB, 2, 100_000)
	require.NoError(t, staking.ActivateValidator(nA.State(), types.DeriveConsensusID(kpA.PublicKey), 1))
	require.NoError(t, staking.ActivateValidator(nA.State(), types.DeriveConsensusID(kpB.PublicKey), 1))
	require.NoError(t, staking.WriteUint64(nA.State(), staking.KeyTotalSupply, 10_000_000))
	p2p, err := network.NewP2PNode(0)
	require.NoError(t, err)
	nA.p2p = p2p
	nA.validators = []types.Address{kpA.Address()}

	nA.StartConsensus()
	defer nA.StopConsensus()

	// B stays offline. A must advance to the cap (S5) but never beyond it.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingRound == 3
	}, 15*time.Second, 20*time.Millisecond)

	time.Sleep(12 * cfg.ProposerTimeout)

	nA.voteMu.Lock()
	require.Equal(t, uint32(3), nA.pendingRound, "round must not advance past MaxRound")
	nA.voteMu.Unlock()
}

// TestConsensus_PrecommitGatedOnPrevoteQuorum is the S10/S11 RED test. The
// proposer must not send its precommit before observing a 2/3 prevote
// majority (the optimistic v1 precommit is removed), and must send it the
// moment the threshold is crossed. With a long base timeout the round stays on
// 0 for the whole test, so the gated round-0 finalization needs no view
// change.
func TestConsensus_PrecommitGatedOnPrevoteQuorum(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)
	cidB := types.DeriveConsensusID(kpB.PublicKey)

	// 3s base: A stays on round 0 for the entire gate assertion window.
	nA := newTwoValidatorNodeTimeout(t, kpA, kpA, kpB, 5, 3*time.Second)
	defer nA.Close()

	nA.StartConsensus()
	defer nA.StopConsensus()

	// A proposes round 0 and prevotes (100k of 200k power).
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil && nA.voting != nil
	}, 10*time.Second, 20*time.Millisecond)

	// S10: below 2/3 prevotes the proposer must NOT have precommitted.
	nA.voteMu.Lock()
	precommitPower := nA.voting.PrecommitPower()
	hash := nA.pendingHash
	round := nA.pendingRound
	nA.voteMu.Unlock()
	require.Zero(t, precommitPower, "no precommit below 2/3 prevotes (S10)")

	// B's prevote crosses the 2/3 threshold: A must now send its precommit.
	prevote := &types.Vote{VoteType: types.VotePrevote, Height: 1, Round: round, BlockHash: hash, Validator: cidB}
	require.NoError(t, prevote.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(prevote)

	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.voting != nil && nA.voting.PrecommitPower() >= 100_000
	}, 10*time.Second, 20*time.Millisecond, "A must precommit once 2/3 prevotes are observed (S11)")

	// B's precommit completes the quorum and the round finalizes at round 0.
	precommit := &types.Vote{VoteType: types.VotePrecommit, Height: 1, Round: round, BlockHash: hash, Validator: cidB}
	require.NoError(t, precommit.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(precommit)

	require.Eventually(t, func() bool { return nA.CurrentHeight() >= 1 }, 10*time.Second, 20*time.Millisecond)

	header, err := consensus.LoadBlockHeader(nA.persistent, 1)
	require.NoError(t, err)
	require.Equal(t, uint32(0), header.Round, "round-0 finalization needed no view change")
}

// TestConsensus_EquivocationDroppedRoundAdvances is the S12 dedicated test.
// A same-round conflicting proposal for the same height is classic
// equivocation: the node must drop it (no second prevote, no pending-value
// switch), and with no precommit majority the view change must still advance
// past the stalled round — the node re-proposes at its next scheduled round and
// votes exactly once there.
func TestConsensus_EquivocationDroppedRoundAdvances(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)
	cidA := types.DeriveConsensusID(kpA.PublicKey)

	// 500ms base: round 0 lives one tick (wide enough to deliver the conflict
	// deterministically); A re-proposes at round 2 (its round) after ~2s.
	nA := newTwoValidatorNodeTimeout(t, kpA, kpA, kpB, 5, 500*time.Millisecond)
	defer nA.Close()

	nA.StartConsensus()
	defer nA.StopConsensus()

	// Round 0: A proposes height 1 and prevotes; B stays offline so no
	// precommit majority ever forms.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil && nA.voting != nil && nA.pendingRound == 0
	}, 10*time.Second, 20*time.Millisecond)

	nA.voteMu.Lock()
	originalHash := nA.pendingHash
	originalRound := nA.pendingRound
	nA.voteMu.Unlock()

	// A conflicting round-0 proposal for the same height: same proposer, same
	// round, different hash. A shallow copy mutated only in timestamp hashes
	// differently while remaining the same proof-less proposal structure.
	nA.voteMu.Lock()
	conflicting := *nA.pendingBlock
	nA.voteMu.Unlock()
	conflicting.Header.Timestamp -= 1
	conflictHash, err := conflicting.HeaderHash(nA.hasher)
	require.NoError(t, err)
	require.NotEqual(t, originalHash, conflictHash, "conflicting proposal must carry a different hash")

	// S12: the equivocation is dropped at the node's adopt-or-drop decision
	// point — pending value stays, the round counter stays, no second vote.
	nA.handleProposalWithSnap(&conflicting, nil)

	nA.voteMu.Lock()
	require.Equal(t, originalHash, nA.pendingHash, "conflicting round-r proposal must be dropped (S12)")
	require.Equal(t, originalRound, nA.pendingRound, "a dropped equivocation must not advance the round")
	prevotes := nA.voting.Prevotes()
	nA.voteMu.Unlock()
	require.Len(t, prevotes, 1, "the node must have voted exactly once in round r (S12)")
	require.Equal(t, originalHash, prevotes[0].BlockHash, "the single round-r prevote must target the original value")

	// No precommit majority, so after the deadline the view change advances
	// past the equivocated round: A re-proposes at round 2 and votes once.
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingRound == 2 && nA.pendingBlock != nil &&
			nA.pendingBlock.Header.Round == 2 && nA.voting != nil
	}, 15*time.Second, 20*time.Millisecond)

	nA.voteMu.Lock()
	newPrevotes := nA.voting.Prevotes()
	newHash := nA.pendingHash
	nA.voteMu.Unlock()
	require.Len(t, newPrevotes, 1, "the node must vote at most once in round r+1 (S12)")
	require.Equal(t, cidA, newPrevotes[0].Validator, "the new-round prevote must be A's own")
	require.Equal(t, newHash, newPrevotes[0].BlockHash, "the new-round prevote must target the round r+1 value")
}

// TestConsensus_OneVotePerHeightAndRound is the S7 node-level test: a
// validator votes at most once per (height, round). The proposer prevotes once
// when the round opens and precommits once when the 2/3 prevote gate is met;
// re-broadcasts of the same round-r vote are dropped by VotingState, so power
// is never double-counted and the round finalizes exactly once.
func TestConsensus_OneVotePerHeightAndRound(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)
	cidA := types.DeriveConsensusID(kpA.PublicKey)
	cidB := types.DeriveConsensusID(kpB.PublicKey)

	// 3s base keeps A on round 0 for the entire assertion window.
	nA := newTwoValidatorNodeTimeout(t, kpA, kpA, kpB, 5, 3*time.Second)
	defer nA.Close()

	nA.StartConsensus()
	defer nA.StopConsensus()

	// A proposes round 0 and prevotes once (100k of 200k power).
	require.Eventually(t, func() bool {
		nA.voteMu.Lock()
		defer nA.voteMu.Unlock()
		return nA.pendingHeight == 1 && nA.pendingBlock != nil && nA.voting != nil
	}, 10*time.Second, 20*time.Millisecond)

	nA.voteMu.Lock()
	hash := nA.pendingHash
	round := nA.pendingRound
	nA.voteMu.Unlock()

	// B's prevote crosses the 2/3 prevote gate; A precommits exactly once.
	prevote := &types.Vote{VoteType: types.VotePrevote, Height: 1, Round: round, BlockHash: hash, Validator: cidB}
	require.NoError(t, prevote.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(prevote)

	nA.voteMu.Lock()
	precommitPower := nA.voting.PrecommitPower()
	selfPrecommits := 0
	for _, pc := range nA.voting.Precommits() {
		if pc.Validator == cidA {
			selfPrecommits++
		}
	}
	nA.voteMu.Unlock()
	require.Equal(t, uint64(100_000), precommitPower, "A must precommit once when the gate is met")
	require.Equal(t, 1, selfPrecommits, "A must not precommit more than once per (height, round)")

	// S7: re-broadcasting the same prevote is dropped — power is not counted
	// twice and the vote set stays single-valued.
	nA.handleVoteMessage(prevote)
	nA.voteMu.Lock()
	prevotePower := nA.voting.PrevotePower()
	prevoteCount := len(nA.voting.Prevotes())
	nA.voteMu.Unlock()
	require.Equal(t, uint64(200_000), prevotePower, "duplicate round-r prevote must not double-count power")
	require.Equal(t, 2, prevoteCount, "exactly one prevote per validator")

	// The proposer's own prevote, re-broadcast alongside the proposal, is
	// dropped the same way and must not double A's precommit.
	nA.voteMu.Lock()
	var ownPrevote *types.Vote
	for _, v := range nA.voting.Prevotes() {
		if v.Validator == cidA {
			ownPrevote = v
		}
	}
	nA.voteMu.Unlock()
	require.NotNil(t, ownPrevote, "A must have prevoted its round-0 proposal")
	nA.handleVoteMessage(ownPrevote)
	nA.voteMu.Lock()
	require.Equal(t, uint64(200_000), nA.voting.PrevotePower(), "re-broadcast own prevote must be dropped")
	require.Equal(t, uint64(100_000), nA.voting.PrecommitPower(), "A must not precommit twice")
	nA.voteMu.Unlock()

	// B's precommit closes the round; the SAME precommit delivered again is
	// dropped (the round is reset after finalization), so there is exactly one
	// round-0 value on the chain.
	precommit := &types.Vote{VoteType: types.VotePrecommit, Height: 1, Round: round, BlockHash: hash, Validator: cidB}
	require.NoError(t, precommit.Sign(kpB.PrivateKey[:]))
	nA.handleVoteMessage(precommit)
	require.Eventually(t, func() bool { return nA.CurrentHeight() >= 1 }, 10*time.Second, 20*time.Millisecond)

	nA.handleVoteMessage(precommit)
	require.Equal(t, uint64(1), nA.CurrentHeight(), "duplicate round-r precommit must not re-finalize or corrupt state")

	header, err := consensus.LoadBlockHeader(nA.persistent, 1)
	require.NoError(t, err)
	require.Equal(t, uint32(0), header.Round, "round-0 finalization needed no view change")
}

// TestConsensus_AcceptsHigherRoundProposalAfterVotingRoundZero is the S6/S13
// RED test for the receiver path: a node that already voted round 0 must
// accept a round-1 proposal (higher round) and switch its pending value, so a
// late higher-round proposal can still finalize instead of being pinned to a
// stale round-0 value.
func TestConsensus_AcceptsHigherRoundProposalAfterVotingRoundZero(t *testing.T) {
	kpA, kpB := generateTwoValidators(t)
	nA := newTwoValidatorNode(t, kpA, kpA, kpB, 5)
	defer nA.Close()
	cidB := types.DeriveConsensusID(kpB.PublicKey)

	// Simulate that A already voted round 0 (the loop is not running, so no
	// goroutine races with the manual round setup).
	nA.voteMu.Lock()
	nA.pendingHeight = 1
	nA.pendingRound = 0
	nA.voteMu.Unlock()

	// B builds a round-1 proposal for height 1 and stamps its header round.
	signer := &walletSigner{kp: kpB}
	block, err := consensus.BuildBlock(nA.State(), nA.vm, nA.Mempool(), 1, nA.GetTipHash(), cidB, signer, nA.hasher, 100, nil, nA.cfg.BlockTimeSec)
	require.NoError(t, err)
	require.NoError(t, consensus.SetProposalRound(block, 1, signer, nA.hasher))
	headerHash, err := block.HeaderHash(nA.hasher)
	require.NoError(t, err)

	// S6: a round-1 proposal replaces the pending round-0 value even though A
	// already voted round 0.
	nA.handleProposal(block)

	nA.voteMu.Lock()
	require.Equal(t, uint32(1), nA.pendingRound, "higher-round proposal must advance the pending round (S6)")
	require.Equal(t, headerHash, nA.pendingHash, "pending value must switch to the round-1 proposal (S6)")
	nA.voteMu.Unlock()
}
