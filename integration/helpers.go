//go:build integration

package integration

import (
	"net"
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// consensusSigner wraps a wallet.KeyPair to implement consensus.Signer interface.
type consensusSigner struct {
	kp *wallet.KeyPair
}

func (s *consensusSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}

// registerValidatorsInState registers validators in the staking system and activates them.
// This is REQUIRED before any block can be built — consensus.BuildBlock calls BeginBlock
// which queries staking.GetActiveValidators.
// Pattern from consensus/block_builder_test.go setupTestValidator.
func registerValidatorsInState(t *testing.T, s *state.InMemoryState, validators []types.Address, keyPairs []*wallet.KeyPair) {
	t.Helper()

	stake := uint64(100_000)
	for i, addr := range validators {
		kp := keyPairs[i]

		// Create account with funds (stake * 2 so there's enough after staking)
		acc := state.NewAccount(addr, kp.PublicKey)
		acc.AddBalance(types.NewAmount(stake * 2))
		s.SetAccount(addr, acc)

		// Register validator (commission=0, currentEpoch=0)
		_, err := staking.RegisterValidator(s, kp.PublicKey, addr, types.NewAmount(stake), 0, 0)
		require.NoError(t, err, "registerValidator: failed for validator %d", i)

		// Deduct stake from account
		acc, err = s.GetAccount(addr)
		require.NoError(t, err)
		acc.SubBalance(types.NewAmount(stake))
		s.SetAccount(addr, acc)
	}

	// Keep economic supply consistent with the balances we just created so
	// fee burns in FinalizeBlock don't fail.
	if err := staking.WriteUint64(s, staking.KeyTotalSupply, uint64(len(validators))*stake*2); err != nil {
		t.Fatalf("registerValidatorsInState: set total supply: %v", err)
	}

	// Process epoch transition (height 100 = epoch boundary with DefaultBlocksPerEpoch=100)
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err, "registerValidatorsInState: ProcessEpochTransition failed")

	// Create snapshot for epoch 1
	_, err = staking.CreateSnapshot(s, 1)
	require.NoError(t, err, "registerValidatorsInState: CreateSnapshot failed")
}

// NewTestNode creates a single ephemeral node with no P2P for testing.
// Returns the node, its keypair, and the hasher.
// Pattern from node_test.go setupPersistentNode:
//   - Config with DataDir=t.TempDir(), P2PPort=0, MaxTxPerBlock=100, ProposerTimeout=100*time.Millisecond
//   - MempoolMaxSize=10000, MempoolTTL=300*time.Second
//   - SnapshotInterval=10
//   - Validators set to the keypair's address
//   - Wallet set via n.SetWallet(kp)
func NewTestNode(t *testing.T) (*node.Node, *wallet.KeyPair) {
	t.Helper()

	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	cfg := node.Config{
		DataDir:          t.TempDir(),
		P2PPort:          0,
		MaxTxPerBlock:    100,
		ProposerTimeout:  100 * time.Millisecond,
		MempoolMaxSize:   10000,
		MempoolTTL:       300 * time.Second,
		SnapshotInterval: 10,
		Validators:       []types.Address{kp.Address()},
	}

	n, err := node.New(cfg)
	require.NoError(t, err)
	n.SetWallet(kp)

	registerValidatorsInState(t, n.State(), []types.Address{kp.Address()}, []*wallet.KeyPair{kp})

	return n, kp
}

// NewMultiNodeNetwork creates N independent nodes, each with its own:
//   - TempDir for persistent storage
//   - Wallet/KeyPair
//   - P2P disabled (P2PPort=0)
//
// Returns nodes and their keypairs.
// Each node must have ALL addresses as Validators.
func NewMultiNodeNetwork(t *testing.T, n int) ([]*node.Node, []*wallet.KeyPair) {
	t.Helper()
	require.Greater(t, n, 0, "NewMultiNodeNetwork: must have at least 1 node")

	// Generate keypairs first to get all validator addresses
	keyPairs := make([]*wallet.KeyPair, n)
	validators := make([]types.Address, n)
	for i := 0; i < n; i++ {
		kp, err := wallet.GenerateKey()
		require.NoError(t, err)
		keyPairs[i] = kp
		validators[i] = kp.Address()
	}

	// Create nodes with all validators
	nodes := make([]*node.Node, n)
	for i := 0; i < n; i++ {
		cfg := node.Config{
			DataDir:          t.TempDir(),
			P2PPort:          0,
			MaxTxPerBlock:    100,
			ProposerTimeout:  100 * time.Millisecond,
			MempoolMaxSize:   10000,
			MempoolTTL:       300 * time.Second,
			SnapshotInterval: 10,
			Validators:       validators, // All nodes have same validator set
		}

		node, err := node.New(cfg)
		require.NoError(t, err)
		node.SetWallet(keyPairs[i])
		nodes[i] = node
	}

	// Register validators in each node's staking state
	for _, n := range nodes {
		registerValidatorsInState(t, n.State(), validators, keyPairs)
	}

	return nodes, keyPairs
}

// freePort returns an available TCP port for a P2P node.
func freePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// NewConsensusLoopNetwork creates N nodes with P2P enabled and fully connected
// in a mesh, each registered as a validator. Unlike NewMultiNodeNetwork (which
// disables P2P so blocks are mined manually), these nodes run the real
// two-phase consensus loop end to end: proposal → prevote/precommit → commit
// proof → final block.
func NewConsensusLoopNetwork(t *testing.T, n int) ([]*node.Node, []*wallet.KeyPair) {
	t.Helper()
	require.Greater(t, n, 0, "NewConsensusLoopNetwork: must have at least 1 node")

	// Generate keypairs first to get all validator addresses
	keyPairs := make([]*wallet.KeyPair, n)
	validators := make([]types.Address, n)
	for i := 0; i < n; i++ {
		kp, err := wallet.GenerateKey()
		require.NoError(t, err)
		keyPairs[i] = kp
		validators[i] = kp.Address()
	}

	// Create nodes with P2P enabled
	nodes := make([]*node.Node, n)
	for i := 0; i < n; i++ {
		cfg := node.Config{
			DataDir:          t.TempDir(),
			P2PPort:          freePort(t),
			MaxTxPerBlock:    100,
			ProposerTimeout:  50 * time.Millisecond,
			MempoolMaxSize:   10000,
			MempoolTTL:       300 * time.Second,
			SnapshotInterval: 10,
			Validators:       validators, // All nodes have same validator set
		}

		node, err := node.New(cfg)
		require.NoError(t, err)
		node.SetWallet(keyPairs[i])
		nodes[i] = node
	}

	// Register validators in each node's staking state
	for _, n := range nodes {
		registerValidatorsInState(t, n.State(), validators, keyPairs)
	}

	// Fully connect the mesh so every node broadcasts (blocks and votes)
	// directly to every other node.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			require.NoError(t, nodes[i].P2P().Connect(nodes[j].P2P().Addr()))
		}
	}

	return nodes, keyPairs
}

// MineBlock manually builds and returns a block on the given node.
// Uses consensus.BuildBlock with the node's state, VM, and mempool.
// NOTE: this helper only builds — it does NOT store the block on the node
// (no StoreBlockHeader/StoreTip, no applyAcceptedBlock). The chain tip is only
// advanced by the consensus loop (n.StartConsensus()) or by sync from a peer.
// Callers that need the node's height to advance must mine through the
// consensus loop; callers that only need a deterministic state root (e.g.
// CompareStateRoots) can use this builder directly since BuildBlock commits
// state as a side effect.
// The allKeyPairs parameter should contain all validator keypairs for commit proof generation.
// This function automatically determines the correct proposer from active validators.
func MineBlock(t *testing.T, n *node.Node, proposerKP *wallet.KeyPair, allKeyPairs []*wallet.KeyPair) *types.Block {
	t.Helper()

	state := n.State()
	vm := n.VM()
	mempool := n.Mempool()
	hasher := n.Hasher()

	// Get current height and tip hash
	height := n.CurrentHeight() + 1
	prevHash := n.GetTipHash()

	// Get active validators to determine the correct proposer
	activeVals, err := staking.GetActiveValidators(state)
	require.NoError(t, err, "MineBlock: get active validators failed")
	require.Greater(t, len(activeVals), 0, "MineBlock: no active validators")

	// Determine the expected proposer for this height
	expectedProposer := consensus.WeightedProposerAtHeight(height, activeVals)

	// Find the keypair that corresponds to the expected proposer
	var selectedKP *wallet.KeyPair
	for _, kp := range allKeyPairs {
		if types.DeriveConsensusID(kp.PublicKey) == expectedProposer {
			selectedKP = kp
			break
		}
	}
	require.NotNil(t, selectedKP, "MineBlock: could not find keypair for proposer %x", expectedProposer)

	proposer := expectedProposer

	// Build the block
	block, err := consensus.BuildBlock(
		state, vm, mempool,
		height, prevHash,
		proposer,
		&consensusSigner{kp: selectedKP},
		hasher,
		n.Config().MaxTxPerBlock,
		nil, // no evidence
		1,
	)
	require.NoError(t, err, "MineBlock: build block failed")

	// Create commit proof for the block
	// Get snapshot for the block's epoch
	snap, err := staking.GetSnapshot(state, block.Header.Epoch)
	require.NoError(t, err, "MineBlock: get snapshot failed")
	require.NotNil(t, snap, "MineBlock: snapshot is nil")

	// Compute header hash
	headerHash, err := block.HeaderHash(hasher)
	require.NoError(t, err, "MineBlock: header hash failed")

	// Create voting state and add votes from all validators (to achieve 2/3 majority)
	vs := consensus.NewVotingState(block.Header.Height, 0, headerHash, snap)

	// Add prevote and precommit from each validator
	for _, kp := range allKeyPairs {
		validatorID := types.DeriveConsensusID(kp.PublicKey)

		// Add prevote
		prevote := &types.Vote{
			VoteType:  types.VotePrevote,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: validatorID,
		}
		err = prevote.Sign(kp.PrivateKey[:])
		require.NoError(t, err, "MineBlock: prevote sign failed")
		err = vs.AddPrevote(prevote)
		require.NoError(t, err, "MineBlock: add prevote failed")

		// Add precommit
		precommit := &types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: validatorID,
		}
		err = precommit.Sign(kp.PrivateKey[:])
		require.NoError(t, err, "MineBlock: precommit sign failed")
		err = vs.AddPrecommit(precommit)
		require.NoError(t, err, "MineBlock: add precommit failed")
	}

	// Build commit proof
	proof, err := vs.BuildCommitProof()
	require.NoError(t, err, "MineBlock: build commit proof failed")
	block.CommitProof = proof

	return block
}

// SubmitAndMine submits a transaction to the node's mempool, mines a block, and returns it.
// Pattern: submit via n.SubmitTx(tx), then MineBlock.
// The allKeyPairs parameter should contain all validator keypairs for commit proof generation.
// Note: For deterministic replay tests, use MineBlock directly with a transaction that will be
// created during block building (not pre-submitted) to avoid nonce conflicts.
func SubmitAndMine(t *testing.T, n *node.Node, proposerKP *wallet.KeyPair, allKeyPairs []*wallet.KeyPair, tx *types.Transaction) *types.Block {
	t.Helper()

	err := n.SubmitTx(tx)
	require.NoError(t, err, "SubmitAndMine: submit tx failed")

	// After submit, the tx is in mempool with sender nonce = tx.Nonce
	// When MineBlock builds a block, it applies the tx, which increments nonce
	// This is correct behavior - the tx is applied once during block building

	return MineBlock(t, n, proposerKP, allKeyPairs)
}

// CompareStateRoots asserts that all given nodes have identical state roots.
func CompareStateRoots(t *testing.T, nodes []*node.Node) {
	t.Helper()
	require.GreaterOrEqual(t, len(nodes), 2, "CompareStateRoots: need at least 2 nodes")

	refRoot := nodes[0].State().GetStateRoot()
	for i, n := range nodes[1:] {
		nodeRoot := n.State().GetStateRoot()
		require.Equal(t, refRoot, nodeRoot, "node %d state root mismatch: expected %x, got %x", i+1, refRoot, nodeRoot)
	}
}

// CreateTestTransaction creates and signs a standard transfer transaction.
// Pattern from node_test.go TestConsensus_BlockProduction:
//   - types.NewTransaction(1, 0, sender, nonce, types.EncodeTransferPayload(sender, 0), nil, 100, 1000, uint64(time.Now().Unix()))
//   - kp.Sign(tx, hasher)
func CreateTestTransaction(t *testing.T, senderKP *wallet.KeyPair, hasher types.Hasher) *types.Transaction {
	t.Helper()

	tx := types.NewTransaction(
		1,                  // version
		0,                  // chainID
		senderKP.Address(), // sender
		1,                  // nonce
		types.EncodeTransferPayload(senderKP.Address(), 0), // payload
		nil,                       // constraints
		100,                       // maxFee
		1000,                      // gasLimit
		uint64(time.Now().Unix()), // timestamp
	)

	err := senderKP.Sign(tx, hasher)
	require.NoError(t, err)

	return tx
}

// FundAccount creates an account with the given balance on a node's state.
// This is needed because mempool validation requires sender accounts to exist with sufficient balance.
func FundAccount(t *testing.T, n *node.Node, addr types.Address, pubKey [32]byte, amount uint64) {
	t.Helper()
	acc := state.NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(amount))
	n.State().SetAccount(addr, acc)

	cur := staking.ReadUint64(n.State(), staking.KeyTotalSupply)
	if err := staking.WriteUint64(n.State(), staking.KeyTotalSupply, cur+amount); err != nil {
		t.Fatalf("FundAccount: update total supply: %v", err)
	}
}

// testMempool implements consensus.MempoolI for building blocks with explicit transactions.
type testMempool struct {
	txs []*types.Transaction
}

func (m *testMempool) PendingTxs() []*types.Transaction { return m.txs }
func (m *testMempool) Remove(_ types.Hash)              {}

// MineBlockWithTxs builds a block with the given transactions on the specified node.
// Unlike MineBlock/SubmitAndMine, this bypasses the node's mempool entirely,
// avoiding mempool nextNonce tracking issues when building multiple blocks.
// The node's internal height/tip are NOT updated (they are private fields),
// so this is best used for state convergence tests rather than chain tests.
func MineBlockWithTxs(t *testing.T, n *node.Node, allKeyPairs []*wallet.KeyPair, txs []*types.Transaction) *types.Block {
	t.Helper()

	state := n.State()
	vm := n.VM()
	hasher := n.Hasher()

	// Get current height and tip hash
	height := n.CurrentHeight() + 1
	prevHash := n.GetTipHash()

	// Get active validators to determine the correct proposer
	activeVals, err := staking.GetActiveValidators(state)
	require.NoError(t, err, "MineBlockWithTxs: get active validators failed")
	require.Greater(t, len(activeVals), 0, "MineBlockWithTxs: no active validators")

	// Determine the expected proposer for this height
	expectedProposer := consensus.WeightedProposerAtHeight(height, activeVals)

	// Find the keypair that corresponds to the expected proposer
	var selectedKP *wallet.KeyPair
	for _, kp := range allKeyPairs {
		if types.DeriveConsensusID(kp.PublicKey) == expectedProposer {
			selectedKP = kp
			break
		}
	}
	require.NotNil(t, selectedKP, "MineBlockWithTxs: could not find keypair for proposer %x", expectedProposer)

	// Build block directly with explicit tx list (bypasses mempool)
	block, err := consensus.BuildBlock(
		state, vm,
		&testMempool{txs: txs},
		height, prevHash,
		expectedProposer,
		&consensusSigner{kp: selectedKP},
		hasher,
		n.Config().MaxTxPerBlock,
		nil, // no evidence
		1,
	)
	require.NoError(t, err, "MineBlockWithTxs: build block failed")

	// Create commit proof for the block
	// Get snapshot for the block's epoch
	snap, err := staking.GetSnapshot(state, block.Header.Epoch)
	require.NoError(t, err, "MineBlockWithTxs: get snapshot failed")
	require.NotNil(t, snap, "MineBlockWithTxs: snapshot is nil")

	// Compute header hash
	headerHash, err := block.HeaderHash(hasher)
	require.NoError(t, err, "MineBlockWithTxs: header hash failed")

	// Create voting state and add votes from all validators (to achieve 2/3 majority)
	vs := consensus.NewVotingState(block.Header.Height, 0, headerHash, snap)

	// Add prevote and precommit from each validator
	for _, kp := range allKeyPairs {
		validatorID := types.DeriveConsensusID(kp.PublicKey)

		// Add prevote
		prevote := &types.Vote{
			VoteType:  types.VotePrevote,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: validatorID,
		}
		err = prevote.Sign(kp.PrivateKey[:])
		require.NoError(t, err, "MineBlockWithTxs: prevote sign failed")
		err = vs.AddPrevote(prevote)
		require.NoError(t, err, "MineBlockWithTxs: add prevote failed")

		// Add precommit
		precommit := &types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: validatorID,
		}
		err = precommit.Sign(kp.PrivateKey[:])
		require.NoError(t, err, "MineBlockWithTxs: precommit sign failed")
		err = vs.AddPrecommit(precommit)
		require.NoError(t, err, "MineBlockWithTxs: add precommit failed")
	}

	// Build commit proof
	proof, err := vs.BuildCommitProof()
	require.NoError(t, err, "MineBlockWithTxs: build commit proof failed")
	block.CommitProof = proof

	return block
}
