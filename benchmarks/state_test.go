//go:build benchmark

package benchmarks

import (
	"testing"

	"github.com/dsn/dsn/types"
)

// BenchmarkStateLoad measures the cost of loading accounts from state.
func BenchmarkStateLoad(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Populate state with accounts
	for i := 0; i < 1000; i++ {
		addr := makeTestAddress(byte(i % 256))
		_ = suite.State.SetAccount(addr, makeTestAccount(1000000, uint64(i)))
	}

	// Commit to build the SMT
	_, _ = suite.State.Commit()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			addr := makeTestAddress(byte(j))
			_, _ = suite.State.GetAccount(addr)
		}
	}
}

// BenchmarkStateStore measures the cost of storing accounts in state.
func BenchmarkStateStore(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Pre-create accounts
	accounts := make([]types.Address, 100)
	for i := range accounts {
		accounts[i] = makeTestAddress(byte(i))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := range accounts {
			_ = suite.State.SetAccount(accounts[j], makeTestAccount(1000000, uint64(i+j)))
		}
	}
}

// BenchmarkStateCommit measures the cost of committing state changes.
func BenchmarkStateCommit(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Populate state with accounts
	for i := 0; i < 100; i++ {
		addr := makeTestAddress(byte(i))
		_ = suite.State.SetAccount(addr, makeTestAccount(1000000, uint64(i)))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = suite.State.Commit()

		// Re-populate for next iteration
		for j := 0; j < 100; j++ {
			addr := makeTestAddress(byte(j))
			_ = suite.State.SetAccount(addr, makeTestAccount(1000000, uint64(i+j)))
		}
	}
}
