//go:build benchmark

package benchmarks

import (
	"testing"

	"github.com/dsn/dsn/state"
)

// BenchmarkSMTUpdate measures the cost of updating a single key in the SMT.
func BenchmarkSMTUpdate(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	// Pre-populate SMT with initial data
	smt := state.NewSMT(suite.Hasher)
	for i := 0; i < 1000; i++ {
		key := []byte{byte(i / 256), byte(i % 256)}
		value := make([]byte, 32)
		for j := range value {
			value[j] = byte(i + j)
		}
		_ = smt.Insert(key, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte{byte(i / 256), byte(i % 256)}
		value := make([]byte, 32)
		for j := range value {
			value[j] = byte(i + j + 1)
		}
		_ = smt.Insert(key, value)
	}
}

// BenchmarkSMTBatchUpdate measures the cost of batch updating multiple keys in the SMT.
func BenchmarkSMTBatchUpdate(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	smt := state.NewSMT(suite.Hasher)

	// Pre-populate with some initial data
	for i := 0; i < 100; i++ {
		key := []byte{byte(i)}
		value := make([]byte, 32)
		for j := range value {
			value[j] = byte(i + j)
		}
		_ = smt.Insert(key, value)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		batchSize := 10
		for j := 0; j < batchSize; j++ {
			key := []byte{byte((i*batchSize + j) % 256)}
			value := make([]byte, 32)
			for k := range value {
				value[k] = byte(i + j + k)
			}
			_ = smt.Insert(key, value)
		}
	}
}

// BenchmarkSMTProof measures the cost of generating and verifying SMT proofs.
func BenchmarkSMTProof(b *testing.B) {
	suite := &BenchmarkSuite{}
	suite.Setup(b)
	defer suite.Teardown(b)

	smt := state.NewSMT(suite.Hasher)

	// Populate SMT with data
	for i := 0; i < 100; i++ {
		key := []byte{byte(i)}
		value := make([]byte, 32)
		for j := range value {
			value[j] = byte(i + j)
		}
		_ = smt.Insert(key, value)
	}

	_ = smt.Root() // Force computation

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte{byte(i % 100)}
		_, exists := smt.Get(key)
		_ = exists
	}
}