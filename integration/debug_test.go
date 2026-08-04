//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

// TestDebug_PersistenceRoot verifies that GetStateRoot() returns the correct
// root after node restart with persistent storage.
// This tests bugfix: Recover() now calls n.state.Commit() to rebuild the SMT.
func TestDebug_PersistenceRoot(t *testing.T) {
	dir := t.TempDir()

	// Create first node and set up state
	n1, err := node.New(node.Config{
		DataDir:        dir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)

	// Set a test account
	addr := types.Address{0: 0xAA, 19: 0xBB}
	acc := state.NewAccount(addr, [32]byte{})
	acc.AddBalance(types.NewAmount(5000))
	n1.State().SetAccount(addr, acc)

	// Commit and persist
	_, err = n1.CommitState()
	require.NoError(t, err)

	preRoot := n1.State().GetStateRoot()
	t.Logf("Pre-close state root: %x", preRoot)
	require.NotEqual(t, types.Hash{}, preRoot, "state root should NOT be zero after commit")

	n1.Close()

	// Create second node with same data dir — Recover() should rebuild SMT
	n2, err := node.New(node.Config{
		DataDir:        dir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()

	postRoot := n2.State().GetStateRoot()
	t.Logf("Post-reopen state root: %x", postRoot)

	// Verify account was recovered (basic sanity)
	recoveredAcc, err := n2.State().GetAccount(addr)
	require.NoError(t, err, "account should be recovered")
	require.Equal(t, 0, types.NewAmount(5000).Cmp(recoveredAcc.Balance))

	// CRITICAL: State root must match after restart
	require.Equal(t, preRoot, postRoot,
		"state root after restart must match pre-close root — SMT must be rebuilt by Recover()")
}
