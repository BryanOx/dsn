package node

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

func TestConsensus_BlockProduction(t *testing.T) {
	// Create wallets for validators
	kp1, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	kp2, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	validators := []types.Address{kp1.Address(), kp2.Address()}

	cfg1 := Config{
		Validators:      validators,
		MaxTxPerBlock:   100,
		ProposerTimeout: 100 * time.Millisecond,
		P2PPort:         0, // P2P disabled for this simple test
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
	}

	n1, err := New(cfg1)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer n1.Close()
	n1.SetWallet(kp1)

	// Set up account with balance for the sender
	var pubKey1 [32]byte
	copy(pubKey1[:], kp1.PublicKey[:])
	acc1 := state.NewAccount(kp1.Address(), pubKey1)
	acc1.AddBalance(types.NewAmount(10000))
	n1.State().SetAccount(kp1.Address(), acc1)

	// Create a transaction
	tx := types.NewTransaction(1, 0, kp1.Address(), 1, types.EncodeTransferPayload(kp1.Address(), 0), nil, 100, 1000, uint64(time.Now().Unix()))
	hasher := types.SHA256Hasher{}
	err = kp1.Sign(tx, hasher)
	if err != nil {
		t.Fatalf("failed to sign transaction: %v", err)
	}

	// Submit transaction to mempool
	err = n1.SubmitTx(tx)
	if err != nil {
		t.Fatalf("failed to submit transaction: %v", err)
	}

	// Verify transaction is in mempool
	if n1.Mempool().Count() != 1 {
		t.Fatalf("expected 1 transaction in mempool, got %d", n1.Mempool().Count())
	}

	// Test that StartConsensus doesn't start without P2P
	// (since P2PPort is 0, P2P is disabled)
	n1.StartConsensus()

	// Without P2P, consensus shouldn't actually run
	// (the method returns early if p2p is nil)
	time.Sleep(200 * time.Millisecond)

	// Transaction should still be in mempool since no consensus loop is running
	if n1.Mempool().Count() != 1 {
		t.Fatalf("expected 1 transaction in mempool (no P2P so no block production), got %d", n1.Mempool().Count())
	}

	// Stop should be safe to call even if not running
	n1.StopConsensus()
}

func TestConsensus_NodeAddress(t *testing.T) {
	kp, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	cfg := DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer n.Close()

	// Without wallet, address should be zero
	addr := n.nodeAddress()
	if addr != (types.Address{}) {
		t.Errorf("expected zero address without wallet, got %v", addr)
	}

	// With wallet, address should match
	n.SetWallet(kp)
	addr = n.nodeAddress()
	if addr != kp.Address() {
		t.Errorf("expected %v, got %v", kp.Address(), addr)
	}
}

func TestConsensus_WalletSigner(t *testing.T) {
	kp, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	signer := &walletSigner{kp: kp}

	// Test signing
	testHash := types.Hash{}
	testHash[0] = 0xAB
	sig, err := signer.Sign(testHash)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}
	if len(sig) == 0 {
		t.Error("expected non-empty signature")
	}

	// Verify signature using ed25519
	if !ed25519.Verify(kp.PublicKey[:], testHash[:], sig) {
		t.Error("signature verification failed")
	}
}

// Helper function to create a node with persistent storage
func setupPersistentNode(t *testing.T) (*Node, string) {
	t.Helper()
	dir := t.TempDir()

	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := Config{
		DataDir:          dir,
		P2PPort:          0,
		Validators:       []types.Address{kp.Address()},
		MaxTxPerBlock:    100,
		ProposerTimeout:  100 * time.Millisecond,
		MempoolMaxSize:   10000,
		MempoolTTL:       300 * time.Second,
		SnapshotInterval: 10,
	}

	n, err := New(cfg)
	require.NoError(t, err)
	n.SetWallet(kp)
	return n, dir
}

func TestNewNode_PersistentRecovery(t *testing.T) {
	// Create first node with persistent storage
	n1, dir := setupPersistentNode(t)
	defer n1.Close()

	// Use a hardcoded address (not dependent on wallet)
	addr := types.Address{0: 0xBB, 19: 0x01}
	var pubKey [32]byte
	copy(pubKey[:], addr[:])
	acc := state.NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(5000))
	n1.State().SetAccount(addr, acc)

	// Commit state to persist
	_, err := n1.CommitState()
	require.NoError(t, err)

	// Close first node
	n1.Close()

	// Create second node with the same DataDir (recovery)
	cfg2 := Config{
		DataDir:        dir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n2, err := New(cfg2)
	require.NoError(t, err)
	defer n2.Close()

	// Verify account was recovered with same balance (using same hardcoded addr)
	acc2, err := n2.State().GetAccount(addr)
	require.NoError(t, err)
	require.Equal(t, 0, types.NewAmount(5000).Cmp(acc2.Balance))
}

func TestNewNode_InMemoryPersistence(t *testing.T) {
	// Create node WITHOUT persistent storage (empty DataDir)
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := Config{
		DataDir:        "", // empty = in-memory only
		P2PPort:        0,
		Validators:     []types.Address{kp.Address()},
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n, err := New(cfg)
	require.NoError(t, err, "should not crash with empty DataDir")
	defer n.Close()
}

func TestRecover_FreshDB(t *testing.T) {
	// Create node with persistent storage but no state (fresh DB)
	dir := t.TempDir()

	cfg := Config{
		DataDir:        dir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n, err := New(cfg)
	require.NoError(t, err)
	// Recover should not error on fresh DB
	defer n.Close()
}

func TestFastSyncFromCheckpoint_Local(t *testing.T) {
	// Create source state
	hasher := types.SHA256Hasher{}
	sourceState := state.NewInMemoryState(hasher)

	addr1 := types.Address{0: 0x01}
	pubKey1 := [32]byte{0: 0x01}
	acc1 := state.NewAccount(addr1, pubKey1)
	acc1.AddBalance(types.NewAmount(5000))
	sourceState.SetAccount(addr1, acc1)

	sourceState.SetBytes("test_key", []byte("test_value"))

	// Commit to get state root
	stateRoot, err := sourceState.Commit()
	require.NoError(t, err)

	// Create snapshot
	snap, err := sourceState.CreateSnapshot(100, 2, types.Hash{0xAA}, 1000000)
	require.NoError(t, err)

	// Serialize
	snapData, err := state.SerializeSnapshot(snap)
	require.NoError(t, err)

	// Build checkpoint
	cp := &state.Checkpoint{
		Height:           100,
		BlockHash:        types.Hash{0xBB},
		StateRoot:        stateRoot,
		SnapshotHash:     state.SnapshotHash(snapData),
		ValidatorSetHash: types.Hash{0xAA},
		Epoch:            2,
		Timestamp:        1000000,
	}

	// Create target node with fresh persistent storage
	dir := t.TempDir()
	cfg := Config{
		DataDir:        dir,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}
	targetNode, err := New(cfg)
	require.NoError(t, err)
	defer targetNode.Close()

	// Fast sync from checkpoint
	err = targetNode.FastSyncFromCheckpoint(cp, snapData, 0)
	require.NoError(t, err)

	// Verify state restored correctly
	restoredAcc, err := targetNode.State().GetAccount(addr1)
	require.NoError(t, err)
	require.Equal(t, 0, types.NewAmount(5000).Cmp(restoredAcc.Balance))

	restoredVal, ok := targetNode.State().GetBytes("test_key")
	require.True(t, ok)
	require.Equal(t, []byte("test_value"), restoredVal)

	// Verify state root matches
	require.Equal(t, stateRoot, targetNode.State().GetStateRoot())

	// Verify state root in persistent matches checkpoint
	require.Equal(t, stateRoot, targetNode.persistent.GetStateRoot())
}

func TestNewNode_DefaultConfig(t *testing.T) {
	// Create node with DefaultConfig()
	cfg := DefaultConfig()

	n, err := New(cfg)
	require.NoError(t, err, "should not crash with DefaultConfig")
	defer n.Close()
}

func TestReplayBlocks_NoHeaders(t *testing.T) {
	// Create a persistent node
	dir := t.TempDir()

	cfg := Config{
		DataDir:        dir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()

	// Call ReplayBlocks without any headers stored
	// Should return error because there are no headers
	err = n.ReplayBlocks(1, 10)
	require.Error(t, err, "should return error when no headers stored")
}

func TestNodeNew_RecoveryPersistent(t *testing.T) {
	// Create initial node with persistent state
	dir := t.TempDir()

	cfg := Config{
		DataDir:        dir,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n1, err := New(cfg)
	require.NoError(t, err)

	// Add state and commit
	addr1 := types.Address{0: 0xAA}
	pub1 := [32]byte{0xAA, 0xBB, 0xCC, 0xDD}
	acc1 := state.NewAccount(addr1, pub1)
	acc1.AddBalance(types.NewAmount(5000))
	n1.State().SetAccount(addr1, acc1)

	root1, err := n1.CommitState()
	require.NoError(t, err)
	require.NotEqual(t, types.Hash{}, root1)

	// Store a tip via n1's own persistent storage
	header := &types.BlockHeader{
		Height:    42,
		StateRoot: root1,
		Timestamp: 1000,
	}
	err = consensus.StoreTip(n1.persistent, header)
	require.NoError(t, err)
	err = consensus.StoreBlockHeader(n1.persistent, header)
	require.NoError(t, err)

	// CLOSE n1 before reopening
	n1.Close()

	// Create a NEW node with the same DataDir — should recover state
	n2, err := New(cfg)
	require.NoError(t, err)
	defer n2.Close()

	// Height should be recovered from tip
	require.Equal(t, uint64(42), n2.CurrentHeight())
}

func TestNodeNew_FreshPersistent(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		DataDir:        dir,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}

	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()

	// Fresh DB should have zero state root
	require.Equal(t, types.Hash{}, n.state.GetStateRoot())
	require.Equal(t, uint64(0), n.CurrentHeight())
	require.Equal(t, types.Hash{}, n.GetTipHash())
}

func TestFastSync_HashMismatch(t *testing.T) {
	hasher := types.SHA256Hasher{}
	sourceState := state.NewInMemoryState(hasher)

	addr1 := types.Address{0: 0x01}
	sourceState.SetAccount(addr1, state.NewAccount(addr1, [32]byte{}))
	sourceState.Commit()

	snap, err := sourceState.CreateSnapshot(50, 1, types.Hash{}, 2000)
	require.NoError(t, err)

	snapData, err := state.SerializeSnapshot(snap)
	require.NoError(t, err)

	// Wrong checkpoint hash
	cp := &state.Checkpoint{
		Height:       50,
		StateRoot:    sourceState.GetStateRoot(),
		SnapshotHash: types.Hash{0xFF, 0xFF, 0xFF}, // wrong hash
	}

	dir := t.TempDir()
	cfg := Config{DataDir: dir}
	targetNode, err := New(cfg)
	require.NoError(t, err)
	defer targetNode.Close()

	err = targetNode.FastSyncFromCheckpoint(cp, snapData, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "snapshot hash mismatch")
}

func TestReplayBlocks_InvalidRange(t *testing.T) {
	dir := t.TempDir()
	n, err := New(Config{DataDir: dir})
	require.NoError(t, err)
	defer n.Close()

	err = n.ReplayBlocks(10, 5)
	require.Error(t, err)
}

func TestNodeNew_NoP2P_NoValidators(t *testing.T) {
	// When P2P is disabled and no validators, StartConsensus should be a no-op
	cfg := DefaultConfig()
	n, err := New(cfg)
	require.NoError(t, err)
	defer n.Close()

	// Should not panic
	n.StartConsensus()
	n.StopConsensus()
}

// ---------------------------------------------------------------------------
// Block-sync adapter tests (tasks 1.4)
// ---------------------------------------------------------------------------

// registerStakingValidator registers a single validator in the node's staking
// state so blocks can be built (BuildBlock → BeginBlock queries the active
// validator set). Mirror of the integration package's registerValidatorsInState.
func registerStakingValidator(t *testing.T, n *Node, kp *wallet.KeyPair) {
	t.Helper()
	stake := uint64(100_000)

	acc := state.NewAccount(kp.Address(), kp.PublicKey)
	acc.AddBalance(types.NewAmount(stake * 2))
	n.State().SetAccount(kp.Address(), acc)

	_, err := staking.RegisterValidator(n.State(), kp.PublicKey, kp.Address(), types.NewAmount(stake), 0, 0)
	require.NoError(t, err)

	acc, err = n.State().GetAccount(kp.Address())
	require.NoError(t, err)
	acc.SubBalance(types.NewAmount(stake))
	n.State().SetAccount(kp.Address(), acc)

	require.NoError(t, staking.WriteUint64(n.State(), staking.KeyTotalSupply, stake*2))
	require.NoError(t, staking.ProcessEpochTransition(n.State(), 100))
	_, err = staking.CreateSnapshot(n.State(), 1)
	require.NoError(t, err)
}

// newSyncedNode creates a persistent node with P2P disabled and one registered
// validator — the minimal setup for exercising the block-sync adapter.
func newSyncedNode(t *testing.T) (*Node, *wallet.KeyPair) {
	t.Helper()
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	return newSyncedNodeWith(t, kp), kp
}

// newSyncedNodeWith creates a persistent node registered with the GIVEN keypair
// as its single validator. Tests that build a block on a scratch node and apply
// it to a second node must use the same keypair on both, because proposer
// selection and the commit proof bind the block to one validator identity.
func newSyncedNodeWith(t *testing.T, kp *wallet.KeyPair) *Node {
	t.Helper()
	cfg := Config{
		DataDir:          t.TempDir(),
		P2PPort:          0,
		Validators:       []types.Address{kp.Address()},
		MaxTxPerBlock:    100,
		ProposerTimeout:  100 * time.Millisecond,
		MempoolMaxSize:   10000,
		MempoolTTL:       300 * time.Second,
		SnapshotInterval: 10,
		BlockTimeSec:     1,
	}
	n, err := New(cfg)
	require.NoError(t, err)
	n.SetWallet(kp)
	registerStakingValidator(t, n, kp)
	return n
}

// buildSignedBlock builds a signed final block (with commit proof) for the
// next height on the node's state, without applying it to the chain.
func buildSignedBlock(t *testing.T, n *Node, kp *wallet.KeyPair) *types.Block {
	t.Helper()

	active, err := staking.GetActiveValidators(n.State())
	require.NoError(t, err)
	require.Greater(t, len(active), 0, "no active validators")
	height := n.CurrentHeight() + 1
	proposer := consensus.WeightedProposerAtHeight(height, active)

	block, err := consensus.BuildBlock(
		n.State(), n.vm, n.mempool,
		height, n.GetTipHash(),
		proposer,
		&walletSigner{kp: kp},
		n.hasher, n.cfg.MaxTxPerBlock, nil, n.cfg.BlockTimeSec,
	)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(n.State(), block.Header.Epoch)
	require.NoError(t, err)
	require.NotNil(t, snap, "epoch snapshot missing")

	headerHash, err := block.HeaderHash(n.hasher)
	require.NoError(t, err)
	vs := consensus.NewVotingState(block.Header.Height, 0, headerHash, snap)

	validatorID := types.DeriveConsensusID(kp.PublicKey)
	prevote := &types.Vote{VoteType: types.VotePrevote, Height: block.Header.Height, Round: 0, BlockHash: headerHash, Validator: validatorID}
	require.NoError(t, prevote.Sign(kp.PrivateKey[:]))
	require.NoError(t, vs.AddPrevote(prevote))
	precommit := &types.Vote{VoteType: types.VotePrecommit, Height: block.Header.Height, Round: 0, BlockHash: headerHash, Validator: validatorID}
	require.NoError(t, precommit.Sign(kp.PrivateKey[:]))
	require.NoError(t, vs.AddPrecommit(precommit))

	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	block.CommitProof = proof
	return block
}

// TestDecodeSyncedBlock_ReaddsTypeByte verifies the block-sync decode choke
// point: range responses carry EncodeBlockMessage frames with the leading
// BlockMessageType byte stripped (network.encodeBlockAsWireFromBlock), so
// decodeSyncedBlock must re-add it before DecodeBlockMessage can parse the
// frame. The round trip must yield an identical block, and a frame that
// already carries the type byte must fail to decode.
func TestDecodeSyncedBlock_ReaddsTypeByte(t *testing.T) {
	n, kp := newSyncedNode(t)
	defer n.Close()

	block := buildSignedBlock(t, n, kp)

	wire, err := consensus.EncodeBlockMessage(block)
	require.NoError(t, err)
	served := wire[1:] // served block list strips the leading type byte

	decoded, err := decodeSyncedBlock(served)
	require.NoError(t, err)

	// Round-trip identity: header (height, hashes, state root), commit proof
	// and transactions must survive decode exactly. Comparing the re-encoded
	// bytes to the original wire also sidesteps nil-vs-empty slice
	// representation differences in the tx list.
	require.Equal(t, block.Header, decoded.Header)
	require.Equal(t, block.CommitProof, decoded.CommitProof)
	require.Equal(t, len(block.Transactions), len(decoded.Transactions))
	reencoded, err := consensus.EncodeBlockMessage(decoded)
	require.NoError(t, err)
	require.Equal(t, wire, reencoded)

	// A frame that already carries the type byte must not decode: the extra
	// prepend corrupts the length prefix.
	_, err = decodeSyncedBlock(wire)
	require.Error(t, err)
}

// TestApplySyncedBlock_RootMismatchAborts verifies that a block whose state
// root fails re-execution is rejected without touching the node's chain state.
// The root check runs before the commit-proof check in ValidateBlock, so the
// proof attached by the builder is irrelevant to the outcome.
func TestApplySyncedBlock_RootMismatchAborts(t *testing.T) {
	n, kp := newSyncedNode(t)
	defer n.Close()

	// Build the block on a scratch node with identical genesis AND the same
	// validator keypair so the block passes every check except the tampered
	// state root: proposer selection and the commit proof bind the block to
	// one validator identity (n and scratch must share it).
	scratch := newSyncedNodeWith(t, kp)
	defer scratch.Close()
	block := buildSignedBlock(t, scratch, kp)

	// Tamper the state root after building.
	block.Header.StateRoot[0] ^= 0xFF

	wire, err := consensus.EncodeBlockMessage(block)
	require.NoError(t, err)
	served := wire[1:]

	accepted, err := n.applySyncedBlock(served)
	require.False(t, accepted)
	require.ErrorIs(t, err, consensus.ErrStateRootMismatch)

	// Chain state untouched: tip still genesis, height unchanged, and the
	// reverted validator account still holds its post-registration balance.
	// Note: InMemoryState snapshots cover accounts+kvstore only, so the SMT
	// root memo is stale after the revert and is recomputed on the next
	// successful apply — assert the reverted KV evidence instead.
	require.Equal(t, uint64(0), n.CurrentHeight())
	require.Equal(t, types.Hash{}, n.GetTipHash())
	acc, err := n.State().GetAccount(kp.Address())
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(100_000), acc.Balance)
}

// TestApplySyncedBlock_AlreadyFinalizedSkipped verifies the already-applied
// guard (D4/S1): a block at or below the finalized height must be skipped
// silently — reported as accepted with no error so the serving peer is never
// penalized for replaying an already-finalized range.
func TestApplySyncedBlock_AlreadyFinalizedSkipped(t *testing.T) {
	n, kp := newSyncedNode(t)
	defer n.Close()

	block := buildSignedBlock(t, n, kp)
	n.finalizedHeight = 10 // the built block (height 1) is already finalized

	wire, err := consensus.EncodeBlockMessage(block)
	require.NoError(t, err)
	served := wire[1:]

	accepted, err := n.applySyncedBlock(served)
	require.NoError(t, err, "already-finalized block must not error")
	require.True(t, accepted, "already-finalized block must be reported accepted (skip, not penalize)")

	// Chain untouched: nothing was applied.
	require.Equal(t, uint64(0), n.CurrentHeight())
	require.Equal(t, types.Hash{}, n.GetTipHash())
}
