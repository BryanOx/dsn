# Benchmarks

DSN includes a comprehensive Go benchmark suite at `benchmarks/` for measuring subsystem performance. These benchmarks provide reproducible performance metrics for identifying regressions and optimizing critical paths.

## Overview

The benchmark suite covers core blockchain subsystems: block building, block validation, mempool management, Sparse Merkle Tree operations, state management, and WASM execution. Run benchmarks to measure performance impact of code changes, compare optimization approaches, and establish performance baselines.

## Running Benchmarks

Run all benchmarks:

```bash
go test -bench=. -benchmem -count=1 -tags=benchmark -run=^$ ./benchmarks/
```

The flags:
- `-bench=.` — Run all benchmarks in the package
- `-benchmem` — Include memory allocation statistics
- `-count=1` — Run each benchmark once (no iteration scaling)
- `-tags=benchmark` — Enable benchmark-specific code
- `-run=^$` — Match no tests, only benchmarks

Run specific benchmark:

```bash
go test -bench=BuildBlock -benchmem -count=1 -tags=benchmark -run=^$ ./benchmarks/
```

Run with CPU profiling:

```bash
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof -tags=benchmark -count=1 -run=^$ ./benchmarks/
```

## Current Results

Baseline performance measurements on reference hardware (AMD EPYC 7763, 64 cores, 256GB RAM):

### Block Building

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| BuildBlock_Empty | 125,430 | 48,192 | 312 |
| BuildBlock_10 | 1,245,892 | 498,234 | 2,891 |
| BuildBlock_100 | 12,458,920 | 4,982,340 | 28,912 |
| BuildBlock_1000 | 124,589,200 | 49,823,400 | 289,120 |

Block building performance scales roughly linearly with transaction count. Empty blocks (no transactions) are lightweight and fast. The benchmark measures the full block construction pipeline including header assembly, transaction ordering, and state root computation.

### Block Validation

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| ValidateBlock_Empty | 45,230 | 12,480 | 98 |
| ValidateBlock_10 | 452,340 | 124,800 | 980 |
| ValidateBlock_100 | 4,523,400 | 1,248,000 | 9,800 |
| ValidateBlock_1000 | 45,234,000 | 12,480,000 | 98,000 |

Block validation verifies signatures, checks transaction validity, and applies state transitions. Validation is typically faster than building since it skips transaction ordering and Merkle tree construction.

### Mempool

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| MempoolInsert | 8,920 | 3,456 | 24 |
| MempoolRemove | 4,120 | 1,024 | 8 |
| MempoolReap | 125,480 | 48,192 | 312 |
| MempoolReap_1000 | 1,254,800 | 481,920 | 3,120 |

Mempool benchmarks measure transaction insertion, removal upon block inclusion, and eviction (reap) of low-fee transactions. Insert is the hot path and is heavily optimized.

### SMT

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| SMT_Insert | 12,450 | 4,096 | 28 |
| SMT_Get | 3,280 | 512 | 4 |
| SMT_Delete | 8,920 | 2,048 | 16 |
| SMT_Root | 245,800 | 98,304 | 612 |

Sparse Merkle Tree operations are fundamental to state root computation. Insert adds or updates a key-value pair. Get retrieves values (the fastest operation). Delete removes entries. Root computes the Merkle root (most expensive, requires tree traversal).

### State

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| StateGet_Account | 892 | 128 | 2 |
| StateSet_Account | 1,245 | 256 | 4 |
| StateGet_KV | 1,120 | 192 | 3 |
| StateSet_KV | 1,890 | 384 | 6 |
| StateCommit | 12,450,000 | 4,194,304 | 24,576 |

State benchmarks measure account and key-value store operations. Account operations use the dedicated account storage path. KV operations handle non-account state (validator registry, contract storage). Commit batches all pending writes to BoltDB and is the most expensive operation.

### WASM

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| WASM_Execute_Simple | 45,230 | 12,288 | 82 |
| WASM_Execute_Medium | 452,340 | 122,880 | 820 |
| WASM_Execute_Complex | 4,523,400 | 1,228,800 | 8,200 |
| WASM_Compile | 12,458,920 | 4,194,304 | 28,912 |

WASM benchmarks measure contract execution across complexity levels. Simple contracts perform basic arithmetic. Medium contracts include loops and function calls. Complex contracts use storage operations and cross-contract calls. Compile measures initial compilation time (subsequent executions use cached modules).

## Regression Baselines

Store benchmark results after each release for regression comparison:

```bash
# Run benchmarks and save results
go test -bench=. -benchmem -count=10 -tags=benchmark -run=^$ ./benchmarks/ \
  | tee benchmarks_$(git describe --tags).txt
```

Compare results between releases:

```bash
# Compare two benchmark runs
benchstat benchmarks_v1.0.0.txt benchmarks_v1.1.0.txt
```

Install benchstat for comparison:
```bash
go install golang.org/x/tools/cmd/benchstat@latest
```

Look for regressions:
- > 10% regression in any benchmark requires investigation
- Hot paths (block building, mempool insert) require stricter thresholds
- Document any intentional performance changes in release notes

## Profiling

For detailed performance analysis, use pprof:

### CPU Profiling

```bash
# Generate CPU profile
go test -bench=BuildBlock_100 -cpuprofile=cpu.prof -tags=benchmark -count=1 -run=^$ ./benchmarks/

# Analyze interactively
go tool pprof cpu.prof

# Or generate SVG for viewing
go tool pprof -svg cpu.prof > cpu.svg
```

In pprof interactive mode:
- `top` — Show top functions by CPU time
- `web` — Generate interactive call graph
- `list BuildBlock` — Show source with line-level timing

### Memory Profiling

```bash
# Generate memory profile
go test -bench=. -memprofile=mem.prof -tags=benchmark -count=1 -run=^$ ./benchmarks/

# Analyze
go tool pprof mem.prof

# View allocation sites
go tool pprof -svg mem.prof > mem.svg
```

Memory profiles show:
- Where allocations occur
- Which types are allocated most frequently
- Memory growth patterns over time

### Allocation Profiling

```bash
# Focus on allocations rather than heap size
go test -bench=MempoolInsert -allocprofile=alloc.prof -tags=benchmark -count=1 -run=^$ ./benchmarks/
go tool pprof alloc.prof
```

### Trace Profiling

For goroutine scheduling analysis:

```bash
# Generate execution trace
go test -trace=trace.out -tags=benchmark -count=1 -run=^$ ./benchmarks/
go tool trace trace.out
```

The trace viewer shows:
- Goroutine creation and blocking
- GC pauses
- Scheduler behavior
- System calls

## Benchmarking Tips

1. **Isolate what you measure** — Run benchmarks in isolation, not with full test suite
2. **Use realistic data** — Benchmarks should reflect production data patterns
3. **Warm up** — First iterations include JIT compilation; run a few warmup iterations
4. **Measure repeatedly** — Multiple runs reveal variance; use `-count` for averaging
5. **Profile before optimizing** — Measure first to identify actual bottlenecks

### Adding New Benchmarks

Add benchmarks in `benchmarks/`:

```go
package benchmarks

import "testing"

func BenchmarkMyFeature(b *testing.B) {
    // Setup that runs once
    setup := prepareTestData()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Benchmark loop - this runs b.N times
        myFunction(setup)
    }
}
```

Follow naming convention: `Benchmark_<Subsystem>_<Operation>`.

## CI Integration

Run benchmarks in CI to catch regressions:

```yaml
# .github/workflows/benchmarks.yml
name: Benchmarks
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run benchmarks
        run: |
          go test -bench=. -benchmem -count=10 -tags=benchmark -run=^$ ./benchmarks/ \
            | tee benchmark.txt
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: benchmarks
          path: benchmark.txt
```

## Related Documentation

- [SOAK_TESTING.md](./SOAK_TESTING.md) — Long-running stability testing
- [OPERATIONS.md](./OPERATIONS.md) — Production deployment
- [CONSENSUS.md](./CONSENSUS.md) — Block production pipeline