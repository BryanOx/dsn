//go:build integration
// +build integration

package testutil

import (
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

func TestNewTestNode_Basic(t *testing.T) {
	// Generate a validator
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	validators := []types.Address{kp.Address()}

	// Create a test node
	tn := NewTestNode(t, validators)
	require.NotNil(t, tn)
	require.NotNil(t, tn.Node)
	require.NotNil(t, tn.Wallet)

	// Basic checks
	require.Equal(t, uint64(0), tn.Node.CurrentHeight())
	require.Equal(t, types.Hash{}, tn.Node.GetTipHash()) // Genesis block has zero hash

	tn.Node.Close()
}

func TestNewMultiNodeNetwork_Basic(t *testing.T) {
	// Create a 3-node network
	nodes := NewMultiNodeNetwork(t, 3)
	require.Len(t, nodes, 3)

	// Each node should have a wallet
	for _, n := range nodes {
		require.NotNil(t, n.Wallet)
		require.NotNil(t, n.Node)
		n.Node.Close()
	}
}

func TestBuildTransferTx_Basic(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	sender := kp.Address()
	to := types.Address{1, 2, 3}
	amount := uint64(100)
	contractID := types.Hash{9, 9, 9}

	tx := BuildTransferTx(sender, 0, to, amount, kp, contractID)
	require.NotNil(t, tx)
	require.Equal(t, sender, tx.Sender)
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, types.TxTypeCallContract, tx.TxType)
	require.NotEmpty(t, tx.Signature)
}

func TestBuildDeployTokenTx_Basic(t *testing.T) {
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)

	sender := kp.Address()
	wasmCode := []byte{0x00, 0x61, 0x73, 0x6d} // Minimal WASM magic

	tx := BuildDeployTokenTx(sender, 0, 1000000, kp, wasmCode)
	require.NotNil(t, tx)
	require.Equal(t, sender, tx.Sender)
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, types.TxTypeDeployContract, tx.TxType)
	require.NotEmpty(t, tx.Signature)
}

func TestCompareState_Empty(t *testing.T) {
	// Create two nodes
	nodes := NewMultiNodeNetwork(t, 2)

	// Both should have same initial state (empty)
	comp := CompareState(t, nodes)
	require.True(t, comp.StateRootsMatch)
	require.True(t, comp.HeightsMatch)

	for _, n := range nodes {
		n.Node.Close()
	}
}