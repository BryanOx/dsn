//go:build benchmark

package benchmarks

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
)

// BenchmarkSuite provides common setup and teardown for all benchmarks.
type BenchmarkSuite struct {
	State  *state.InMemoryState
	Hasher types.Hasher
}

// Setup initializes the benchmark suite with a fresh state and hasher.
func (s *BenchmarkSuite) Setup(b *testing.B) {
	s.Hasher = &types.SHA256Hasher{}
	s.State = state.NewInMemoryState(s.Hasher)
}

// Teardown performs cleanup after benchmarks complete.
func (s *BenchmarkSuite) Teardown(b *testing.B) {
	// InMemoryState doesn't require explicit cleanup
}

// makeTestAccount creates a test account with the given balance and nonce.
func makeTestAccount(balance uint64, nonce uint64) *state.Account {
	return &state.Account{
		Balance: types.NewAmount(balance),
		Nonce:   nonce,
	}
}

// makeTestAddress creates a deterministic address from a seed.
func makeTestAddress(seed byte) types.Address {
	var addr types.Address
	for i := range addr {
		addr[i] = seed
	}
	return addr
}

// makeTestTransaction creates a test transaction with minimal data.
func makeTestTransaction(sender types.Address, nonce uint64, maxFee uint64, timestamp uint64) *types.Transaction {
	return &types.Transaction{
		Version:     1,
		ChainID:     1,
		Sender:      sender,
		Nonce:       nonce,
		Payload:     types.EncodeTransferPayload(sender, 0),
		Constraints: []byte{},
		MaxFee:      maxFee,
		GasLimit:    21000,
		Timestamp:   timestamp,
		Signature:   []byte{1, 2, 3, 4}, // minimal valid signature
		TxType:      types.TxTypeStandard,
	}
}

// makeTestBlock creates a block with the given transactions.
func makeTestBlock(height uint64, prevHash types.Hash, txs []*types.Transaction) *types.Block {
	// Convert []*Transaction to []Transaction
	txSlice := make([]types.Transaction, len(txs))
	for i, tx := range txs {
		txSlice[i] = *tx
	}
	return &types.Block{
		Header: types.BlockHeader{
			Version:      1,
			Height:       height,
			PreviousHash: prevHash,
			StateRoot:    types.Hash{},
			TxRoot:       types.Hash{},
			Timestamp:    uint64(time.Now().Unix()),
		},
		Transactions: txSlice,
		FeeSummary:   types.FeeSummary{},
	}
}
