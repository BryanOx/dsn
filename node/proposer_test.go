package node

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// registerValidator replicates the consensus test helper against a KVStore:
// creates the operator account, registers the validator, activates it and
// snapshots the set. Returns the ConsensusID.
func registerValidator(t *testing.T, s staking.StakingState, kp *wallet.KeyPair, operatorID byte, stake uint64) types.Address {
	t.Helper()

	addr := types.Address{}
	addr[0] = operatorID

	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])

	acc := state.NewAccount(addr, pubKey32)
	acc.AddBalance(types.NewAmount(stake * 2))
	s.SetAccount(addr, acc)

	consensusID, err := staking.RegisterValidator(s, pubKey32, addr, types.NewAmount(stake), 0, 0)
	require.NoError(t, err)

	acc, _ = s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(addr, acc)

	return consensusID
}

// TestConsensus_ProposerSelectionUsesConsensusID is the F1 regression test.
// Production proposer selection must use the ACTIVE staking registry with
// weighted voting power over ConsensusIDs — exactly what block validation
// applies — and never the static operator-address set. It fails on the old
// code because consensus.ProposerAtHeight over operator addresses diverges
// from WeightedProposerAtHeight on every block.
func TestConsensus_ProposerSelectionUsesConsensusID(t *testing.T) {
	kpA, err := wallet.GenerateKey()
	require.NoError(t, err)
	kpB, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kpA)

	registerValidator(t, n.State(), kpA, 1, 200_000)
	registerValidator(t, n.State(), kpB, 2, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	activeVals, err := staking.GetActiveValidators(n.State())
	require.NoError(t, err)
	require.Len(t, activeVals, 2)

	nodeConsensusID := types.DeriveConsensusID(kpA.PublicKey)

	// Old production logic: static operator addresses + ProposerAtHeight.
	operatorSet := []types.Address{kpA.Address(), kpB.Address()}
	oldDiverges := false

	for height := uint64(0); height < 30; height++ {
		want := consensus.WeightedProposerAtHeight(height, activeVals)
		got := n.consensusProposerAtHeight(height)
		require.Equal(t, want, got,
			"production proposer selection must match validation's WeightedProposerAtHeight")

		// The proposer is a ConsensusID, never an operator (staking) address.
		require.NotEqual(t, kpA.Address(), got)
		require.NotEqual(t, kpB.Address(), got)

		if got == nodeConsensusID {
			require.Equal(t, nodeConsensusID, n.consensusID())
		}

		// Prove the OLD algorithm diverged from validation.
		if consensus.ProposerAtHeight(height, operatorSet) != want {
			oldDiverges = true
		}
	}
	require.True(t, oldDiverges,
		"old static operator-address selection must diverge from weighted selection")
}

// TestConsensus_BlockProduction_ProposerPassesValidation is the F1 end-to-end
// regression: a block built by the node with its own selected proposer must
// pass full block validation. Before the fix the header carried the node's
// operator address and validation rejected every block with ErrWrongProposer.
func TestConsensus_BlockProduction_ProposerPassesValidation(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	registerValidator(t, n.State(), kp, 1, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	// Fund the validator's operator account and submit a transfer.
	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(kp.Address(), pubKey32)
	acc.AddBalance(types.NewAmount(1_000_000))
	n.State().SetAccount(kp.Address(), acc)

	recipient := types.Address{}
	recipient[0] = 0x42
	tx := types.NewTransaction(1, 0, kp.Address(), 1,
		types.EncodeTransferPayload(recipient, 1000), nil, 100, 1000,
		uint64(time.Now().Unix()))
	require.NoError(t, kp.Sign(tx, n.hasher))
	require.NoError(t, n.Mempool().Submit(tx))

	const height = uint64(1)
	proposer := n.consensusProposerAtHeight(height)
	require.NotEqual(t, types.Address{}, proposer,
		"node must be a validator and select itself as proposer")
	require.Equal(t, types.DeriveConsensusID(kp.PublicKey), proposer)

	block, err := consensus.BuildBlock(n.State(), n.vm, n.Mempool(), height, types.ZeroHash,
		proposer, &walletSigner{kp: n.wallet}, n.hasher, 100, nil, 1)
	require.NoError(t, err)

	// F1: the block must carry the ConsensusID, not the operator address.
	require.Equal(t, proposer, block.Header.Proposer)
	require.NotEqual(t, n.nodeAddress(), block.Header.Proposer)

	// Attach the proposer's commit proof (in production validators attach
	// votes to the block before it is accepted).
	headerHash, err := block.HeaderHash(n.hasher)
	require.NoError(t, err)
	snap, err := staking.GetSnapshot(n.State(), block.Header.Epoch)
	require.NoError(t, err)
	require.NotNil(t, snap)
	vs := consensus.NewVotingState(block.Header.Height, 0, headerHash, snap)
	prevote := &types.Vote{
		VoteType: types.VotePrevote, Height: block.Header.Height,
		Round: 0, BlockHash: headerHash, Validator: proposer,
	}
	require.NoError(t, prevote.Sign(kp.PrivateKey[:]))
	require.NoError(t, vs.AddPrevote(prevote))
	precommit := &types.Vote{
		VoteType: types.VotePrecommit, Height: block.Header.Height,
		Round: 0, BlockHash: headerHash, Validator: proposer,
	}
	require.NoError(t, precommit.Sign(kp.PrivateKey[:]))
	require.NoError(t, vs.AddPrecommit(precommit))
	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	block.CommitProof = proof

	// Full validation path on a fresh, identically-initialized state.
	freshS := state.NewInMemoryState(n.hasher)
	registerValidator(t, freshS, kp, 1, 100_000)
	staking.ProcessEpochTransition(freshS, 100)
	staking.CreateSnapshot(freshS, 1)
	staking.WriteUint64(freshS, staking.KeyTotalSupply, 10_000_000)
	freshAcc := state.NewAccount(kp.Address(), pubKey32)
	freshAcc.AddBalance(types.NewAmount(1_000_000))
	freshS.SetAccount(kp.Address(), freshAcc)

	parent := &types.BlockHeader{Height: 0, PreviousHash: types.ZeroHash}
	err = consensus.ValidateBlock(block, parent, types.ZeroHash, freshS, n.hasher, n.vm, 1)
	require.NoError(t, err, "block built with node-selected proposer must pass validation")
}
