//go:build benchmark

package benchmarks

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/types"
)

// BenchmarkBuildBlock_Empty measures the cost of building a block with no transactions.
func BenchmarkBuildBlock_Empty(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	var prevHash types.Hash

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		block := makeTestBlock(uint64(i), prevHash, nil)
		hash, _ := block.Header.HeaderHash(suite.Hasher)
		_ = hash
	}
}

// BenchmarkBuildBlock_10 measures the cost of building a block with 10 transactions.
func BenchmarkBuildBlock_10(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	var prevHash types.Hash
	txs := make([]*types.Transaction, 10)
	for i := range txs {
		sender := makeTestAddress(byte(i))
		txs[i] = makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		block := makeTestBlock(uint64(i), prevHash, txs)
		hash, _ := block.Header.HeaderHash(suite.Hasher)
		_ = hash
	}
}

// BenchmarkBuildBlock_100 measures the cost of building a block with 100 transactions.
func BenchmarkBuildBlock_100(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	var prevHash types.Hash
	txs := make([]*types.Transaction, 100)
	for i := range txs {
		sender := makeTestAddress(byte(i % 256))
		txs[i] = makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		block := makeTestBlock(uint64(i), prevHash, txs)
		hash, _ := block.Header.HeaderHash(suite.Hasher)
		_ = hash
	}
}
