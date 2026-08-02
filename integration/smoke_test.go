//go:build integration

package integration

import (
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// TestFramework_SingleNode creates one node, mines a block, and verifies state root is non-zero.
func TestFramework_SingleNode(t *testing.T) {
	n, kp := NewTestNode(t)
	defer n.Close()

	block := MineBlock(t, n, kp, []*wallet.KeyPair{kp})
	require.NotNil(t, block)
	require.Greater(t, block.Header.Height, uint64(0))
	root := n.State().GetStateRoot()
	require.NotEqual(t, types.Hash{}, root)
}

// TestFramework_MultiNodeConvergence creates 2 nodes, submits the same tx to both,
// mines blocks on both, and verifies they produce the same state root.
func TestFramework_MultiNodeConvergence(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	hasher := types.SHA256Hasher{}
	tx := CreateTestTransaction(t, kps[0], hasher)

	// Fund sender account on all nodes
	for _, n := range nodes {
		FundAccount(t, n, kps[0].Address(), kps[0].PublicKey, 100000)
	}

	// Submit and mine on both nodes using the same proposer (kps[0])
	for _, n := range nodes {
		SubmitAndMine(t, n, kps[0], kps, tx)
	}

	CompareStateRoots(t, nodes)
}

// TestFramework_DeterministicReplay creates a node, mines blocks with state changes,
// restarts from persistent storage, and verifies state root matches.
func TestFramework_DeterministicReplay(t *testing.T) {
	t.Skip("T10-2: Will be fully implemented in next phase")
}
