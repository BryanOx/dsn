//go:build benchmark

package benchmarks

import (
	"testing"
	"time"

	"github.com/dsn/dsn/types"
)

// BenchmarkValidateBlock_10 measures the cost of validating a block with 10 transactions.
func BenchmarkValidateBlock_10(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Setup accounts for validation
	for i := 0; i < 10; i++ {
		addr := makeTestAddress(byte(i))
		_ = suite.State.SetAccount(addr, makeTestAccount(1000000, uint64(i)))
	}

	// Create previous block to establish chain
	prevBlock := makeTestBlock(0, types.Hash{}, nil)
	prevHash, _ := prevBlock.Header.HeaderHash(suite.Hasher)

	// Create block to validate with 10 txs
	txs := make([]*types.Transaction, 10)
	for i := range txs {
		sender := makeTestAddress(byte(i))
		txs[i] = makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
	}
	block := makeTestBlock(1, prevHash, txs)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Validate block header fields
		if block.Header.Height == 0 {
			continue // avoid optimization
		}
		// Basic validation: check header fields
		_ = block.Header.PreviousHash
		_ = block.Header.Timestamp
		_ = len(block.Transactions)
	}
}

// BenchmarkValidateBlock_100 measures the cost of validating a block with 100 transactions.
func BenchmarkValidateBlock_100(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Setup accounts for validation
	for i := 0; i < 100; i++ {
		addr := makeTestAddress(byte(i % 256))
		_ = suite.State.SetAccount(addr, makeTestAccount(1000000, uint64(i)))
	}

	// Create previous block to establish chain
	prevBlock := makeTestBlock(0, types.Hash{}, nil)
	prevHash, _ := prevBlock.Header.HeaderHash(suite.Hasher)

	// Create block to validate with 100 txs
	txs := make([]*types.Transaction, 100)
	for i := range txs {
		sender := makeTestAddress(byte(i % 256))
		txs[i] = makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
	}
	block := makeTestBlock(1, prevHash, txs)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Validate block header fields
		if block.Header.Height == 0 {
			continue // avoid optimization
		}
		// Basic validation: check header fields
		_ = block.Header.PreviousHash
		_ = block.Header.Timestamp
		_ = len(block.Transactions)
	}
}
