//go:build integration

package soak

import (
	"testing"
	"time"

	"github.com/dsn/dsn/integration"
	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// fundAllAccounts funds all accounts on all nodes with the specified amount.
// This wraps the integration package's FundAccount function.
func fundAllAccounts(t *testing.T, nodes []*node.Node, kps []*wallet.KeyPair, amount uint64) {
	t.Helper()

	for _, n := range nodes {
		for _, kp := range kps {
			integration.FundAccount(t, n, kp.Address(), kp.PublicKey, amount)
		}
	}
}

// generateSoakTransactions creates a batch of transfer transactions for soak testing.
// It reads the current nonce from the node's state for each sender to ensure monotonic nonces.
func generateSoakTransactions(t *testing.T, nodes []*node.Node, kps []*wallet.KeyPair, hasher *types.SHA256Hasher, round int, txCount int) []*types.Transaction {
	t.Helper()

	txs := make([]*types.Transaction, 0, txCount)

	// Read current nonces from the first node's state
	// (all nodes should have same state after convergence)
	currentNonces := make(map[types.Address]uint64)
	for _, kp := range kps {
		acc, err := nodes[0].State().GetAccount(kp.Address())
		if err != nil {
			// Account might not exist yet (first transaction), default to 0
			currentNonces[kp.Address()] = 0
		} else {
			currentNonces[kp.Address()] = acc.Nonce
		}
	}

	for i := 0; i < txCount; i++ {
		senderIdx := i % len(kps)
		kp := kps[senderIdx]
		addr := kp.Address()

		// Use current nonce + 1 (state expects tx.Nonce == account.Nonce + 1)
		// Increment for each tx from this sender in this batch
		txIndex := i / len(kps)
		nonce := currentNonces[addr] + 1 + uint64(txIndex)

		tx := types.NewTransaction(
			1,     // version
			0,     // chainID
			addr,  // sender
			nonce, // nonce
			[]byte("soak-test"),
			nil,  // constraints
			100,  // maxFee
			1000, // gasLimit
			uint64(time.Now().Unix()),
		)
		require.NoError(t, kp.Sign(tx, hasher))
		txs = append(txs, tx)
	}

	return txs
}

// CreateTransferTransaction creates a simple transfer transaction.
func CreateTransferTransaction(t *testing.T, sender *wallet.KeyPair, nonce uint64, hasher *types.SHA256Hasher) *types.Transaction {
	t.Helper()

	tx := types.NewTransaction(
		1,
		0,
		sender.Address(),
		nonce,
		[]byte("soak-transfer"),
		nil,
		100,
		1000,
		uint64(time.Now().Unix()),
	)
	require.NoError(t, sender.Sign(tx, hasher))
	return tx
}

// CreateContractTransaction creates a contract-like transaction (simulated WASM call).
func CreateContractTransaction(t *testing.T, sender *wallet.KeyPair, nonce uint64, hasher *types.SHA256Hasher) *types.Transaction {
	t.Helper()

	tx := types.NewTransaction(
		1,
		0,
		sender.Address(),
		nonce,
		[]byte("soak-contract-call"),
		nil,
		200,
		2000,
		uint64(time.Now().Unix()),
	)
	require.NoError(t, sender.Sign(tx, hasher))
	return tx
}
