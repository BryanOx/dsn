//go:build benchmark

package benchmarks

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/mempool"
	"github.com/BryanOx/dsn/types"
)

// BenchmarkMempoolInsert measures the cost of inserting transactions into the mempool.
func BenchmarkMempoolInsert(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Pre-populate state with accounts
	for i := 0; i < 1000; i++ {
		addr := makeTestAddress(byte(i % 256))
		_ = suite.State.SetAccount(addr, makeTestAccount(10000000, uint64(i)))
	}

	// Create mempool
	mp := mempool.New(10000, 5*time.Minute, suite.State)

	// Pre-generate transactions
	seed := time.Now().UnixNano()
	txs := make([]*types.Transaction, b.N)
	for i := range txs {
		sender := makeTestAddress(byte((int(seed) + i) % 256))
		txs[i] = makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = mp.Submit(txs[i])
	}
}

// BenchmarkMempoolEvict measures the cost of evicting transactions from the mempool.
func BenchmarkMempoolEvict(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Pre-populate state with accounts
	for i := 0; i < 1000; i++ {
		addr := makeTestAddress(byte(i % 256))
		_ = suite.State.SetAccount(addr, makeTestAccount(10000000, uint64(i)))
	}

	// Create mempool with limited size
	mp := mempool.New(100, 5*time.Minute, suite.State)

	// Pre-populate mempool with transactions
	seed := int64(12345)
	for i := 0; i < 100; i++ {
		sender := makeTestAddress(byte((seed + int64(i)) % 256))
		tx := makeTestTransaction(sender, uint64(i+1), 1000, uint64(time.Now().UnixNano()))
		_ = mp.Submit(tx)
	}

	// Generate new transactions to trigger eviction
	newTxs := make([]*types.Transaction, b.N)
	for i := range newTxs {
		sender := makeTestAddress(byte((seed + int64(i+100)) % 256))
		newTxs[i] = makeTestTransaction(sender, uint64(i+101), 2000, uint64(time.Now().UnixNano()))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = mp.Submit(newTxs[i])
	}
}
