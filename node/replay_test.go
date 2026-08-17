package node

import (
	"bytes"
	"testing"
	"time"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/vm"
	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// replayEmitEventWasm returns a minimal WASM module that compiles so a
// contract deploy/replay exercise goes through the real VM path.
func replayEmitEventWasm() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
		0x01, 0x0c, 0x02,
		0x60, 0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x00,
		0x60, 0x00, 0x01, 0x7f,
		0x02, 0x12, 0x01,
		0x03, 0x65, 0x6e, 0x76,
		0x0a, 0x65, 0x6d, 0x69, 0x74, 0x5f, 0x65, 0x76, 0x65, 0x6e, 0x74,
		0x00, 0x00,
		0x03, 0x02, 0x01, 0x01,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x08, 0x01, 0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x01,
		0x0b, 0x18, 0x02,
		0x00, 0x41, 0x00, 0x0b, 0x08, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72,
		0x00, 0x41, 0xe4, 0x00, 0x0b, 0x04, 0x01, 0x02, 0x03, 0x04,
		0x0a, 0x11, 0x01,
		0x0f, 0x00,
		0x41, 0x00, 0x41, 0x08, 0x41, 0xe4, 0x00, 0x41, 0x04,
		0x10, 0x00,
		0x41, 0x00,
		0x0b,
	}
}

// buildReplayDeployTx builds a signed contract deploy transaction for the
// given nonce using the minimal test WASM.
func buildReplayDeployTx(t *testing.T, kp *wallet.KeyPair, hasher types.Hasher, nonce uint64) *types.Transaction {
	t.Helper()

	wasmCode := replayEmitEventWasm()
	codeHash := types.Hash(vm.SHA256Sum(wasmCode))
	deployTx := &types.Transaction{
		Version:   1,
		Nonce:     nonce,
		Sender:    kp.Address(),
		TxType:    types.TxTypeDeployContract,
		MaxFee:    1_000_000,
		GasLimit:  1_000_000,
		Timestamp: uint64(time.Now().Unix()),
	}
	inner := &types.DeployContractTx{
		Sender: kp.Address(), Nonce: nonce, WasmCode: wasmCode, CodeHash: codeHash,
		MaxFee: deployTx.MaxFee, GasLimit: deployTx.GasLimit,
	}
	innerID, err := inner.ComputeIntentID(hasher)
	require.NoError(t, err)
	sig, err := kp.SignHash(innerID)
	require.NoError(t, err)
	inner.Signature = sig

	var buf bytes.Buffer
	require.NoError(t, inner.Encode(&buf))
	deployTx.Payload = buf.Bytes()

	intentID, err := deployTx.ComputeIntentID(hasher)
	require.NoError(t, err)
	deployTx.IntentID = intentID
	require.NoError(t, kp.Sign(deployTx, hasher))
	return deployTx
}

// TestReplayBlocks_FaithfulReplay is the F3 regression test. It builds a
// chain of full blocks (transactions + finalization), stores them
// persistently, rewinds in-memory state to an earlier height, and verifies
// ReplayBlocks re-executes the full blocks and reproduces the exact state
// roots that the original execution produced.
func TestReplayBlocks_FaithfulReplay(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	registerValidator(t, n.State(), kp, 1, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(kp.Address(), pubKey32)
	acc.AddBalance(types.NewAmount(10_000_000))
	require.NoError(t, n.State().SetAccount(kp.Address(), acc))

	recipient := types.Address{0x42}

	// A dedicated deployer account: the contract path never advances the
	// account nonce, so a shared sender's nonce sequence would collide.
	// Created before mining so the snapshot below captures it.
	deployerKP, err := wallet.GenerateKey()
	require.NoError(t, err)
	var deployerPub [32]byte
	copy(deployerPub[:], deployerKP.PublicKey[:])
	depAcc := state.NewAccount(deployerKP.Address(), deployerPub)
	depAcc.AddBalance(types.NewAmount(10_000_000))
	require.NoError(t, n.State().SetAccount(deployerKP.Address(), depAcc))

	nextNonce := uint64(1)
	mkTransfer := func() *types.Transaction {
		tx := types.NewTransaction(1, 0, kp.Address(), nextNonce,
			types.EncodeTransferPayload(recipient, 1000), nil, 100, 1000,
			uint64(time.Now().Unix()))
		require.NoError(t, kp.Sign(tx, n.hasher))
		nextNonce++
		return tx
	}

	buildBlock := func(height uint64, prevHash types.Hash) *types.Block {
		t.Helper()
		proposer := n.consensusProposerAtHeight(height)
		require.NotEqual(t, types.Address{}, proposer)
		block, err := consensus.BuildBlock(n.State(), n.vm, n.Mempool(), height, prevHash,
			proposer, &walletSigner{kp: n.wallet}, n.hasher, 100, nil, 1)
		require.NoError(t, err)
		return block
	}

	// Mine blocks 1..2 fully: state applied, full blocks persisted.
	roots := make([]types.Hash, 5)
	for height := uint64(1); height <= 2; height++ {
		require.NoError(t, n.Mempool().Submit(mkTransfer()))
		block := buildBlock(height, n.GetTipHash())
		roots[height] = n.State().GetStateRoot()
		require.NotEqual(t, types.Hash{}, roots[height])
		n.applyAcceptedBlock(block)
	}
	require.Equal(t, uint64(2), n.CurrentHeight())

	// Build blocks 3..4 without advancing the committed state: snapshot,
	// apply, record, store the full blocks, then rewind.
	snapID := n.State().Snapshot()
	prevHash := n.GetTipHash()
	for height := uint64(3); height <= 4; height++ {
		var tx *types.Transaction
		if height == 3 {
			// Include a contract deploy to exercise the VM path in replay.
			tx = buildReplayDeployTx(t, deployerKP, n.hasher, 1)
		} else {
			tx = mkTransfer()
		}
		require.NoError(t, n.Mempool().Submit(tx))

		block := buildBlock(height, prevHash)
		roots[height] = n.State().GetStateRoot()
		require.NotEqual(t, roots[height-1], roots[height],
			"block %d must change the state root", height)
		require.NoError(t, consensus.StoreBlock(n.persistent, block))

		// BuildBlock reads but does not remove mempool txs; drop them so the
		// next block only sees its own transactions.
		for i := range block.Transactions {
			n.Mempool().Remove(block.Transactions[i].IntentID)
		}

		headerHash, err := block.HeaderHash(n.hasher)
		require.NoError(t, err)
		prevHash = headerHash
	}

	// Rewind in-memory state to height 2 and persist it, simulating a node
	// that has stored full blocks 3..4 but whose state is only at height 2.
	require.NoError(t, n.State().RevertToSnapshot(snapID))
	_, err = n.state.Commit()
	require.NoError(t, err)
	require.Equal(t, roots[2], n.State().GetStateRoot(),
		"reverted state must match height 2")
	_, err = n.CommitState()
	require.NoError(t, err)

	// Replay blocks 3..4 and verify the final state root matches the
	// originally built state.
	require.NoError(t, n.ReplayBlocks(3, 4))
	require.Equal(t, uint64(4), n.CurrentHeight())
	require.Equal(t, roots[4], n.State().GetStateRoot(),
		"replayed state must match the originally built state")

	// The persisted tip must reflect the replayed height.
	tip, err := consensus.LoadTip(n.persistent)
	require.NoError(t, err)
	require.Equal(t, uint64(4), tip.Height)
}

// TestReplayBlocks_StateRootMismatch verifies replay fails fast when a stored
// block cannot be re-executed faithfully (here: a corrupted state root).
func TestReplayBlocks_StateRootMismatch(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	registerValidator(t, n.State(), kp, 1, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	// Fund the sender's account (registerValidator uses a distinct operator
	// address, so kp.Address() needs its own balance).
	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(kp.Address(), pubKey32)
	acc.AddBalance(types.NewAmount(10_000_000))
	require.NoError(t, n.State().SetAccount(kp.Address(), acc))

	// Build and store a single block with a transfer.
	recipient := types.Address{0x42}
	tx := types.NewTransaction(1, 0, kp.Address(), 1,
		types.EncodeTransferPayload(recipient, 1000), nil, 100, 1000,
		uint64(time.Now().Unix()))
	require.NoError(t, kp.Sign(tx, n.hasher))
	require.NoError(t, n.Mempool().Submit(tx))

	proposer := n.consensusProposerAtHeight(1)
	snapID := n.State().Snapshot()
	block, err := consensus.BuildBlock(n.State(), n.vm, n.Mempool(), 1, types.ZeroHash,
		proposer, &walletSigner{kp: n.wallet}, n.hasher, 100, nil, 1)
	require.NoError(t, err)

	// Rewind state to before the block, then store a corrupted version of it.
	require.NoError(t, n.State().RevertToSnapshot(snapID))
	_, err = n.state.Commit()
	require.NoError(t, err)
	block.Header.StateRoot = types.Hash{}
	require.NoError(t, consensus.StoreBlock(n.persistent, block))

	// Replay from a state that does not include the transfer.
	err = n.ReplayBlocks(1, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "state root mismatch")
}

// TestReorgWithinFinality verifies that RevertBlocks works within the finality
// window. It builds a chain, then reverts the most recent block.
func TestReorgWithinFinality(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	registerValidator(t, n.State(), kp, 1, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(kp.Address(), pubKey32)
	acc.AddBalance(types.NewAmount(10_000_000))
	require.NoError(t, n.State().SetAccount(kp.Address(), acc))

	// Build and apply 3 blocks
	prevHash := n.GetTipHash()
	for height := uint64(1); height <= 3; height++ {
		block, err := consensus.BuildBlock(n.State(), n.vm, n.Mempool(), height, prevHash,
			n.consensusProposerAtHeight(height), &walletSigner{kp: n.wallet}, n.hasher, 100, nil, 1)
		require.NoError(t, err)
		n.applyAcceptedBlock(block)
		headerHash, _ := block.HeaderHash(n.hasher)
		prevHash = headerHash
	}
	require.Equal(t, uint64(3), n.CurrentHeight())

	// Set finalizedHeight so that reverting from 3 to 2 is within the window
	n.finalizedHeight = 0 // allow all reverts

	// Revert block 3 → tip becomes height 2
	require.NoError(t, n.RevertBlocks(3, 2))
	require.Equal(t, uint64(2), n.CurrentHeight())

	// Block at height 2 should no longer be accessible from persistence
	// (block 3 was removed)
	if n.persistent != nil {
		_, err := consensus.LoadBlock(n.persistent, 3)
		require.Error(t, err, "block at height 3 should be removed")
	}
}

// TestReorgBeyondFinalityRejected verifies that RevertBlocks returns an error
// when trying to revert below the finalized height.
func TestReorgBeyondFinalityRejected(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()
	n.SetWallet(kp)

	registerValidator(t, n.State(), kp, 1, 100_000)
	staking.ProcessEpochTransition(n.State(), 100)
	staking.CreateSnapshot(n.State(), 1)
	staking.WriteUint64(n.State(), staking.KeyTotalSupply, 10_000_000)

	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(kp.Address(), pubKey32)
	acc.AddBalance(types.NewAmount(10_000_000))
	require.NoError(t, n.State().SetAccount(kp.Address(), acc))

	// Build and apply 10 blocks so finalized height advances
	prevHash := n.GetTipHash()
	for height := uint64(1); height <= 10; height++ {
		block, err := consensus.BuildBlock(n.State(), n.vm, n.Mempool(), height, prevHash,
			n.consensusProposerAtHeight(height), &walletSigner{kp: n.wallet}, n.hasher, 100, nil, 1)
		require.NoError(t, err)
		n.applyAcceptedBlock(block)
		headerHash, _ := block.HeaderHash(n.hasher)
		prevHash = headerHash
	}
	require.Equal(t, uint64(10), n.CurrentHeight())

	// Set finalizedHeight to k=6: block at height 10 means finalized = 4
	n.finalizedHeight = 10 - finalityK // = 4

	// Trying to revert from height 5 to height 3 should fail (to=3 < finalizedHeight=4)
	err = n.RevertBlocks(5, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "finalized height")
}
