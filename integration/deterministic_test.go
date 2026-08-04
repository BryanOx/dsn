//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// TestDeterministic_TransactionReplay verifies that applying the same transaction
// to two independent nodes produces the same state root.
func TestDeterministic_TransactionReplay(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	hasher := types.SHA256Hasher{}

	// Create the same transaction
	tx := types.NewTransaction(1, 0, kps[0].Address(), 1, types.EncodeTransferPayload(kps[0].Address(), 0), nil, 100, 1000, uint64(time.Now().Unix()))
	err := kps[0].Sign(tx, hasher)
	require.NoError(t, err)

	// Fund sender account on all nodes (each already has validator registration stake)
	for _, n := range nodes {
		FundAccount(t, n, kps[0].Address(), kps[0].PublicKey, 100000)
	}

	// Submit and mine on each node independently
	// Use the same proposer (kps[0]) for all nodes to ensure block validity
	var roots []types.Hash
	for _, n := range nodes {
		blk := SubmitAndMine(t, n, kps[0], kps, tx)
		require.NotNil(t, blk)
		roots = append(roots, n.State().GetStateRoot())
	}

	// All state roots must be identical
	for i := 1; i < len(roots); i++ {
		require.Equal(t, roots[0], roots[i], "node %d has different state root", i)
	}
}

// TestDeterministic_EmptyBlock verifies that mining an empty block on multiple nodes
// produces identical state roots (no state changes = identical root).
func TestDeterministic_EmptyBlock(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	// Mine empty blocks on both nodes using the same proposer (kps[0])
	for _, n := range nodes {
		MineBlock(t, n, kps[0], kps)
	}

	CompareStateRoots(t, nodes)
}

// TestDeterministic_MultipleBlocks mines three blocks on two nodes with the same
// sequence of transactions and verifies state convergence.
func TestDeterministic_MultipleBlocks(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	hasher := types.SHA256Hasher{}

	// Fund sender accounts on all nodes
	for _, n := range nodes {
		for _, kp := range kps {
			FundAccount(t, n, kp.Address(), kp.PublicKey, 100000)
		}
	}

	// Mine 3 blocks with the same sequence of transactions on each node
	// Use MineBlockWithTxs to bypass mempool nonce tracking issues
	for round := 0; round < 3; round++ {
		var roundTxs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000, uint64(time.Now().Unix()))
			err := kp.Sign(tx, hasher)
			require.NoError(t, err)
			roundTxs = append(roundTxs, tx)
		}

		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, roundTxs)
		}
	}

	CompareStateRoots(t, nodes)
}

// TestDeterministic_StatePersistence verifies that state changes are correctly
// persisted to disk and recovered after node restart.
func TestDeterministic_StatePersistence(t *testing.T) {
	n, _ := NewTestNode(t)
	defer n.Close()

	// Fund a test account
	addr := types.Address{0x01, 0x02, 0x03}
	var pubKey [32]byte
	copy(pubKey[:], addr[:])
	FundAccount(t, n, addr, pubKey, 50000)

	// Commit state to persistent storage
	_, err := n.CommitState()
	require.NoError(t, err)

	// Record state root before close
	preCloseRoot := n.State().GetStateRoot()
	require.NotEqual(t, types.Hash{}, preCloseRoot)

	// Record the data directory
	dataDir := n.Config().DataDir
	require.NotEmpty(t, dataDir)

	// Close the node
	err = n.Close()
	require.NoError(t, err)

	// Re-open with same data directory
	n2, err := node.New(node.Config{
		DataDir:        dataDir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()

	// Load state from persistent storage
	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// Verify state root matches
	postOpenRoot := n2.State().GetStateRoot()
	require.Equal(t, preCloseRoot, postOpenRoot,
		"state root should match after restart")

	// Verify account was recovered
	acc, err := n2.State().GetAccount(addr)
	require.NoError(t, err)
	require.Equal(t, 0, types.NewAmount(50000).Cmp(acc.Balance),
		"account balance should be recovered")
}

// TestDeterministic_EmptyStateRecovery verifies that a node with no prior state
// starts correctly (empty state root) — not stale data from a previous instance.
func TestDeterministic_EmptyStateRecovery(t *testing.T) {
	n, _ := NewTestNode(t)
	preRoot := n.State().GetStateRoot()
	dataDir := n.Config().DataDir
	n.Close()

	n2, err := node.New(node.Config{
		DataDir:        dataDir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()

	require.Equal(t, preRoot, n2.State().GetStateRoot(),
		"empty state should recover to the same zero root")
}

// TestDeterministic_PostBlockPersistence verifies that state after block production
// survives a node restart.
func TestDeterministic_PostBlockPersistence(t *testing.T) {
	n, kp := NewTestNode(t)

	// Mine an empty block to produce state changes
	block := MineBlock(t, n, kp, []*wallet.KeyPair{kp})
	require.NotNil(t, block)

	// Commit and record state
	_, err := n.CommitState()
	require.NoError(t, err)
	preRoot := n.State().GetStateRoot()
	dataDir := n.Config().DataDir
	n.Close()

	// Reopen
	n2, err := node.New(node.Config{
		DataDir:        dataDir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()

	// Load state from persistent storage
	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	require.Equal(t, preRoot, n2.State().GetStateRoot(),
		"state root after block production should survive restart")
}

// TestDeterministic_ConvergentPersistence verifies that two independently-run nodes
// converge to the same state and that convergence survives restart.
func TestDeterministic_ConvergentPersistence(t *testing.T) {
	// Create 2 nodes
	nodes, kps := NewMultiNodeNetwork(t, 2)
	hasher := types.SHA256Hasher{}

	// Fund sender accounts on both nodes
	for _, n := range nodes {
		for _, kp := range kps {
			FundAccount(t, n, kp.Address(), kp.PublicKey, 100000)
		}
	}

	// Run 2 rounds of transactions on each node independently
	for round := 0; round < 2; round++ {
		var roundTxs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000, uint64(time.Now().Unix()))
			err := kp.Sign(tx, hasher)
			require.NoError(t, err)
			roundTxs = append(roundTxs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, roundTxs)
		}
	}

	// Verify pre-restart convergence
	CompareStateRoots(t, nodes)

	// Record dirs and roots, close all
	dirs := make([]string, len(nodes))
	roots := make([]types.Hash, len(nodes))
	for i, n := range nodes {
		_, err := n.CommitState()
		require.NoError(t, err)
		roots[i] = n.State().GetStateRoot()
		dirs[i] = n.Config().DataDir
		n.Close()
	}

	// Reopen all nodes
	for i, dir := range dirs {
		n2, err := node.New(node.Config{
			DataDir:        dir,
			P2PPort:        0,
			MempoolMaxSize: 10000,
			MempoolTTL:     300 * time.Second,
		})
		require.NoError(t, err)
		defer n2.Close()

		// Load state from persistent storage
		err = n2.LoadFromPersistent()
		require.NoError(t, err)

		// Verify individual recovery
		require.Equal(t, roots[i], n2.State().GetStateRoot(),
			"node %d: state root should match after restart", i)
	}
}
