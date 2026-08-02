package node

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
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
