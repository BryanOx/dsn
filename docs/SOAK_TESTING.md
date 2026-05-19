# Soak Testing

DSN includes a soak testing framework at `tools/soakrunner/` for long-running stability validation. Soak tests exercise the system under sustained load to identify memory leaks, resource exhaustion, consensus degradation, and other issues that only appear over time.

## Overview

The soak runner generates realistic blockchain workload patterns and monitors system health throughout the test. It produces transfers, executes WASM contracts, performs staking operations, and measures key metrics like throughput, memory usage, and state root consistency.

## Quick Start

Run a basic 1-hour soak test with the heavy profile:

```bash
cd tools/soakrunner
go run . --profile heavy --duration 1h
```

This starts a single-node test network, generates load, and monitors for failure conditions. The test completes successfully if no abort conditions are triggered.

## Workload Profiles

The soak runner supports multiple workload profiles targeting different scenarios:

| Profile | TPS | Description |
|---------|-----|-------------|
| `light` | 10 | Basic transfer workload — simple account-to-account transfers. Tests core transaction processing and basic consensus. |
| `moderate` | 100 | Standard load — mixed transfers and simple contract calls. Represents typical production load. |
| `heavy` | 1000 | High throughput — sustained high transaction rate. Tests consensus performance and mempool capacity. |
| `burst` | 0->500 | Traffic spike testing — starts at 0 TPS and ramps to 500 TPS over 5 minutes. Tests system response to sudden load increases. |
| `wasm` | 100 | WASM contract execution — predominantly contract deploys and calls. Tests WASM VM performance and state management. |
| `mixed` | 200 | Transfers + WASM + staking — realistic mix of all transaction types. Closest to production workload. |

Select the profile matching your test objectives. For pre-release validation, use `heavy` or `mixed`. For regression testing, `moderate` provides good coverage without excessive resource consumption.

### Profile Configuration

Each profile can be further customized:

```bash
# Run with custom parameters
go run . \
  --profile custom \
  --tps 500 \
  --wasm-ratio 0.3 \
  --stake-ratio 0.1 \
  --duration 2h
```

## Running Soak Tests

### 24h Soak

Standard pre-production validation:

```bash
cd tools/soakrunner
go run . --profile heavy --duration 24h --log-interval 5m
```

The `--log-interval` flag controls how often progress metrics are logged. Five minutes provides good visibility without overwhelming log volume.

### 72h Soak

Extended validation for production readiness:

```bash
cd tools/soakrunner
go run . --profile mixed --duration 72h --log-interval 10m
```

Extended tests can reveal issues that only appear after 24+ hours of continuous operation, such as memory fragmentation, database compaction issues, or slow resource leaks.

### Multi-Node Soak

Test cluster behavior with multiple nodes:

```bash
# Start first node (seed)
go run . --profile moderate --duration 12h --node-type seed --port 30333

# In another terminal, start validator
go run . --profile moderate --duration 12h --node-type validator --seeds 127.0.0.1:30333 --port 30334
```

Multi-node tests exercise P2P networking, consensus across multiple validators, and state synchronization.

### CI Integration

Run soak tests in CI pipelines:

```yaml
# .github/workflows/soak.yml
name: Soak Test
on: schedule:
  - cron: '0 3 * * 0'  # Weekly Sunday 3AM

jobs:
  soak:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run 24h soak
        run: |
          cd tools/soakrunner
          go run . --profile moderate --duration 24h --log-interval 15m
        timeout: 26h
```

## Monitoring During Soak

During a soak test, continuously monitor key metrics to detect issues early.

### Prometheus Metrics

Watch these metrics via the Prometheus endpoint:

```bash
# Block height should grow steadily
curl -s http://localhost:9090/metrics | grep dsn_block_height

# TPS should be stable (within profile range)
curl -s http://localhost:9090/metrics | grep dsn_mempool_tps

# Memory should plateau, not grow unbounded
curl -s http://localhost:9090/metrics | grep process_resident_memory_bytes

# Goroutine count should stabilize
curl -s http://localhost:9090/metrics | grep go_goroutines
```

Plot these metrics over time in Grafana. Look for:
- Block height linear growth (indicates healthy consensus)
- Flat memory curve (no leaks)
- Stable goroutine count (no blocking or starvation)

### Grafana Dashboard

Import `monitoring/grafana/dsn-dashboard.json` for visual monitoring. The dashboard provides:
- Real-time TPS
- Block production rate
- Memory and CPU trends
- Peer count
- Consensus round progression

### Continuous Verification

The soak runner performs automated checks:

```bash
# Verify state roots match between nodes
curl -s http://localhost:8545/eth/root
curl -s http://localhost:8546/eth/root

# Check finalized heights are advancing
curl -s http://localhost:9090/metrics | grep dsn_consensus_finalized_height
```

## Abort Conditions

The soak runner automatically terminates if these conditions are detected:

| Condition | Threshold | Meaning |
|-----------|-----------|---------|
| State root divergence | Any | Nodes have inconsistent state — indicates consensus failure or data corruption |
| Consensus stall | > 60s at same height | Block production halted — consensus issue or network partition |
| Memory growth | > 2GB from baseline | Memory leak or unbounded cache |
| Goroutine count | > 10000 | Goroutine leak or deadlock in progress |
| DB corruption | Detected | Storage layer failure |
| Replay hash mismatch | Any | State computation divergence |

When an abort condition triggers:

1. The test logs the failure with details
2. Final metrics snapshot is saved
3. Core dumps are captured if applicable
4. Process exits with non-zero status

Example abort output:

```
[ERRO] Soak test aborted: memory growth exceeded threshold
  Baseline: 1.2GB
  Current: 3.8GB
  Threshold: 2GB
  Duration: 4h 32m
[INFO] Final metrics snapshot saved to: soak-results/metrics-20240115-1532.json
```

## Results Interpretation

After a soak test completes, analyze results to assess system stability.

### TPS Trend

Check throughput stability:

```bash
# Extract TPS from logs
grep "TPS:" soak.log | awk '{print $NF}' | sort -n | head -5
grep "TPS:" soak.log | awk '{print $NF}' | sort -n | tail -5
```

Stable performance shows similar min and max values. Large variance indicatesconsensus instability or resource contention.

### Memory Growth Pattern

Memory should plateau within the first few hours:

```bash
# Extract memory readings
grep "memory" soak.log | awk '{print $NF}' | sort -n
```

If memory continuously grows, investigate for:
- Unbounded caches
- Goroutine accumulation
- Memory fragmentation

### Block Time Variance

Block times should be consistent:

```bash
# Calculate block time statistics
grep "block_time" soak.log | awk '{print $NF}' | \
  awk '{sum+=$1; sumsq+=$1*$1; count++} END {
    mean=sum/count; variance=(sumsq/count)-(mean*mean);
    printf "Mean: %.3fs, StdDev: %.3fs\n", mean, sqrt(variance)
  }'
```

High variance indicates consensus instability.

### Event/Receipt Consistency

Verify transaction results match across nodes:

```bash
# Compare event logs between nodes
for node in node1 node2 node3; do
  ssh $node "grep 'tx_hash' /var/log/dsn/soak.log | wc -l"
done
```

Mismatched counts indicate a consensus fork or state divergence.

### Test Summary

The soak runner outputs a summary at completion:

```
=== Soak Test Summary ===
Duration: 24h 0m 0s
Profile: heavy
TPS: 998.4 (target: 1000)
Blocks produced: 172800
Avg block time: 0.500s
Memory start: 1.2GB
Memory end: 1.4GB
Memory growth: 200MB
Goroutines: 234 (stable)
State roots: consistent
Abort conditions: 0
Status: PASSED
```

A `PASSED` status indicates the system operated stably for the test duration without triggering any abort conditions.

## Troubleshooting

### Test Stalls

If the test hangs:
1. Check if nodes are running: `ps aux | grep dsn`
2. Review logs: `journalctl -u dsn -f`
3. Check network connectivity between nodes
4. Verify consensus is advancing

### High Memory During Test

Memory growth beyond 2GB triggers abort. To debug:
1. Capture heap profile: `go tool pprof http://localhost:9090/debug/pprof/heap`
2. Check for growing maps or slices
3. Review recent code changes for resource leaks

### State Root Divergence

State root mismatch indicates:
1. Non-deterministic transaction execution
2. Bug in state management
3. Memory corruption

Capture state for analysis:
```bash
# Save state from both nodes
curl -s http://localhost:8545/eth/getProof > node1-state.json
curl -s http://localhost:8546/eth/getProof > node2-state.json

# Compare state trees
diff node1-state.json node2-state.json
```

## Related Documentation

- [BENCHMARKS.md](./BENCHMARKS.md) — Micro-benchmark reference
- [OPERATIONS.md](./OPERATIONS.md) — Deployment and monitoring
- [CONSENSUS.md](./CONSENSUS.md) — Consensus protocol details
- [PERSISTENCE.md](./PERSISTENCE.md) — State storage