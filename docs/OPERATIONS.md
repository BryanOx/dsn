# Operations Guide

## Overview

DSN is a Byzantine Fault Tolerant blockchain with a production-ready deployment architecture supporting multiple validator configurations. This guide covers deployment, configuration, monitoring, backup, upgrade, and troubleshooting procedures for production environments.

- Production deployment architecture
- Network topology (3+ validators)

## Deployment Options

DSN supports two primary deployment methods: Helm/Kubernetes for orchestrated environments and systemd for standalone servers.

### Helm/Kubernetes

The Helm chart in `charts/dsn/` provides configurable deployment with staging and production overlays. The chart supports both Deployment and StatefulSet modes depending on persistence requirements.

```bash
# Install staging
helm install dsn-staging charts/dsn/ -f charts/dsn/values-staging.yaml -n dsn-staging

# Install production
helm install dsn-prod charts/dsn/ -f charts/dsn/values-production.yaml -n dsn-prod

# Upgrade
helm upgrade dsn-prod charts/dsn/ -f charts/dsn/values-production.yaml -n dsn-prod
```

The Helm chart includes built-in support for:
- Persistent volume claims for stateful storage
- Configurable resource limits and requests
- Liveness and readiness probes
- Service monitoring via Prometheus annotations

### Systemd

For single-node or bare-metal deployments, use the systemd service unit at `deploy/systemd/dsn.service`.

```bash
# Install service
sudo cp deploy/systemd/dsn.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dsn
sudo systemctl start dsn

# Check status
sudo systemctl status dsn

# View logs
journalctl -u dsn -f
```

## Configuration

DSN can be configured via environment variables, CLI flags, or TOML configuration files. The configuration precedence is: CLI flags > environment variables > config file > defaults.

### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DSN_CHAIN_ID` | Chain identifier | `dsn-1` |
| `DSN_P2P_PORT` | P2P networking port | `30333` |
| `DSN_RPC_PORT` | RPC API port | `8545` |
| `DSN_METRICS_PORT` | Prometheus metrics port | `9090` |
| `DSN_DATA_DIR` | Data directory path | `/var/lib/dsn` |
| `DSN_P2P_SEEDS` | Comma-separated seed nodes | `node1@ip:port,node2@ip:port` |
| `DSN_LOG_LEVEL` | Log verbosity (debug, info, warn, error) | `info` |

### CLI Flags

```bash
dsn node start \
  --chain-id dsn-1 \
  --p2p.listen-address /ip4/0.0.0.0/tcp/30333 \
  --rpc.listen-address 0.0.0.0:8545 \
  --metrics.enabled \
  --metrics.addr 0.0.0.0:9090 \
  --data-dir /var/lib/dsn \
  --log.level info
```

| Flag | Description | Default |
|------|-------------|---------|
| `--chain-id` | Chain identifier | `dsn-1` |
| `--p2p.listen-address` | P2P listen address | `/ip4/0.0.0.0/tcp/30333` |
| `--rpc.listen-address` | RPC server address | `0.0.0.0:8545` |
| `--metrics.enabled` | Enable Prometheus metrics | `false` |
| `--metrics.addr` | Metrics server address | `0.0.0.0:9090` |
| `--data-dir` | Data directory | `./data` |
| `--log.level` | Log level | `info` |

### Config File (TOML)

Create a `config.toml` file for complex configurations:

```toml
[chain]
id = "dsn-1"
genesis = ""

[p2p]
port = 30333
seeds = "seed1@10.0.0.1:30333,seed2@10.0.0.2:30333"
persistent_peers = "peer1@10.0.0.3:30333"
max_peers = 50

[rpc]
port = 8545
cors_allowed_origins = ["*"]
api = ["eth", "net", "web3"]

[metrics]
enabled = true
port = 9090

[consensus]
timeout_propose = "3s"
timeout_precommit = "1s"
skip_timeout_commit = false

[store]
database = "boltdb"
sync = true
```

### Data Directory Layout

The data directory contains:

```
data/
├── chaindata/          # Blockchain data
│   ├── block.db        # Block storage
│   └── state.db        # State database (BoltDB)
├── snapshots/          # State snapshots
│   └── latest.json     # Latest snapshot metadata
├── keystore/           # Validator keys
└── config.toml         # Local configuration
```

### Port Reference

| Port | Protocol | Purpose |
|------|----------|---------|
| 30333 | TCP | P2P networking (block sync, consensus) |
| 8545 | HTTP | RPC API (JSON-RPC) |
| 8546 | WebSocket | RPC WebSocket API |
| 9090 | HTTP | Prometheus metrics endpoint |

## Monitoring

DSN exposes Prometheus metrics at `http://localhost:9090/metrics`. The monitoring stack includes Grafana dashboards and Prometheus alerting rules.

### Prometheus Metrics

Key metrics to monitor:

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `dsn_block_height` | Current block height | Not increasing for 5m |
| `dsn_consensus_round` | Current consensus round | > 100 |
| `dsn_mempool_size` | Transactions in mempool | > 10000 |
| `dsn_p2p_peer_count` | Connected peers | < 3 |
| `dsn_consensus_block_time_seconds` | Block production time | > 5s |
| `process_cpu_seconds_total` | CPU usage | > 80% |
| `process_resident_memory_bytes` | Memory usage | > 90% of limit |

### Grafana Dashboard

Import the dashboard from `monitoring/grafana/dsn-dashboard.json` for visual monitoring. The dashboard includes:

- Block production rate
- Consensus round progression
- Mempool transaction count
- Peer connectivity status
- CPU and memory usage
- Network traffic metrics

### Alerting Rules

Prometheus alerting rules are defined in `monitoring/prometheus/rules.yml`. Key alerts:

- `DSNBlockHeightNotIncreasing` — Block production stalled
- `DSNSyncLagHigh` — Node behind peers by >10 blocks
- `DSNPeerCountLow` — Fewer than 3 peers
- `DSNValidatorDown` — Node unreachable
- `DSNCPUUsageHigh` — CPU above 80%
- `DSNMemoryUsageHigh` — Memory above 90%

## Backup and Restore

### Data Directory Backup

```bash
# Create backup
DATA_DIR="/var/lib/dsn"
BACKUP_DIR="/backup/dsn-$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"
cp -r "$DATA_DIR" "$BACKUP_DIR"

# Compress for long-term storage
tar -czf "dsn-backup-$(date +%Y%m%d).tar.gz" -C /var/lib dsn

# Automated backup script
#!/bin/bash
set -e
BACKUP_PATH="/backup/dsn"
DATA_PATH="/var/lib/dsn"
mkdir -p "$BACKUP_PATH"
tar -czf "$BACKUP_PATH/dsn-$(date +%Y%m%d-%H%M%S).tar.gz" -C "$(dirname "$DATA_PATH")" "$(basename "$DATA_PATH")"
find "$BACKUP_PATH" -name "dsn-*.tar.gz" -mtime +7 -delete
```

### Snapshot-Based Restore

```bash
# Stop the node
systemctl stop dsn

# Restore from snapshot
SNAPSHOT_DIR="/backup/snapshots/latest"
rm -rf /var/lib/dsn/chaindata/*
tar -xzf "$SNAPSHOT_DIR/state.tar.gz" -C /var/lib/dsn/chaindata/

# Start the node
systemctl start dsn

# Verify state root matches network
curl -s http://localhost:8545/eth/root | jq '.state_root'
```

### WAL-Based Recovery

BoltDB provides automatic WAL recovery. For manual recovery:

```bash
# Check database integrity
bbolt verify /var/lib/dsn/chaindata/state.db

# Rebuild from raw data
dsn node recover --data-dir /var/lib/dsn --from-snapshot /backup/snapshots/latest
```

## Upgrades

### Rolling Upgrade Procedure

For production environments with multiple validators, perform rolling updates to maintain consensus:

```bash
# 1. Upgrade first validator
kubectl rollout restart statefulset/dsn-0 -n dsn-prod

# 2. Wait for sync and block production
sleep 30
curl -s http://dsn-0.dsn-prod:9090/metrics | grep dsn_block_height

# 3. Verify the upgraded node is producing blocks
kubectl logs -n dsn-prod dsn-0 -f | grep "proposing block"

# 4. Proceed with next validator
kubectl rollout restart statefulset/dsn-1 -n dsn-prod
```

### StatefulSet Update Strategy

The Helm chart uses `RollingUpdate` strategy by default. Configure in values:

```yaml
statefulset:
  enabled: true
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      partition: 0
```

### Helm Chart Upgrades

```bash
# Check current version
helm list -n dsn-prod

# Upgrade to new version
helm upgrade dsn-prod charts/dsn/ -f values-production.yaml -n dsn-prod

# Rollback if needed
helm rollback dsn-prod 1 -n dsn-prod

# View history
helm history dsn-prod -n dsn-prod
```

## Troubleshooting

### Node Won't Start

1. Check configuration syntax:
   ```bash
   dsn validate-config --config /etc/dsn/config.toml
   ```

2. Verify data directory permissions:
   ```bash
   ls -la /var/lib/dsn
   chown -R dsn:dsn /var/lib/dsn
   ```

3. Check for port conflicts:
   ```bash
   ss -tlnp | grep -E '(30333|8545|9090)'
   ```

4. Review logs:
   ```bash
   journalctl -u dsn -n 100 --no-pager
   ```

### Validator Not Producing Blocks

1. Verify sync status:
   ```bash
   curl -s http://localhost:8545/eth/syncing | jq '.'
   ```

2. Check validator is in active set:
   ```bash
   curl -s http://localhost:8545/dsn/validators | jq '.validators[] | select(.status == "active")'
   ```

3. Verify consensus participation:
   ```bash
   curl -s http://localhost:9090/metrics | grep dsn_consensus_votes
   ```

4. Check for slashing:
   ```bash
   curl -s http://localhost:8545/dsn/slashing | jq '.'
   ```

### Peers Not Connecting

1. Check P2P configuration:
   ```bash
   # Verify seeds and persistent peers
   grep -E '(seeds|persistent_peers)' /etc/dsn/config.toml

   # Test network connectivity
   nc -zv seed.example.com 30333
   ```

2. Check firewall rules:
   ```bash
   iptables -L -n | grep 30333
   # Or for UFW:
   ufw status | grep 30333
   ```

3. View peer connections:
   ```bash
   curl -s http://localhost:8545/net/peers | jq '.peers | length'
   ```

4. Check for duplicate peer IDs:
   ```bash
   dsn node info 2>&1 | grep -i peer
   ```

### State Root Mismatch

1. Verify state root:
   ```bash
   curl -s http://localhost:8545/eth/root

   # Compare with peers
   for peer in "${PEERS[@]}"; do
     curl -s "http://$peer/eth/root"
   done
   ```

2. Rebuild state from snapshot:
   ```bash
   systemctl stop dsn
   dsn node rebuild-state --snapshot /backup/snapshots/latest
   systemctl start dsn
   ```

3. Replay from genesis if needed:
   ```bash
   rm -rf /var/lib/dsn/chaindata/*
   dsn node init --genesis genesis.json
   dsn node start --replay
   ```

## Related Documentation

- [CONSENSUS.md](./CONSENSUS.md) — BFT consensus protocol
- [PERSISTENCE.md](./PERSISTENCE.md) — State persistence and snapshots
- [VALIDATOR_GUIDE.md](./VALIDATOR_GUIDE.md) — Validator setup and operations
- [RECOVERY_RUNBOOK.md](./RECOVERY_RUNBOOK.md) — Incident response procedures
- [SOAK_TESTING.md](./SOAK_TESTING.md) — Long-running stability testing
- [BENCHMARKS.md](./BENCHMARKS.md) — Performance benchmarks

## Node Operation

# Node Operations Manual

**DSN v0.1.0-sandbox**

> This document covers everything an operator needs to run, configure, monitor, and recover a DSN node. Only features implemented in v0.1.0-sandbox are documented. See [OPERATIONS.md](./OPERATIONS.md) for deployment (Helm/systemd) and production procedures.

---

## 1. Overview

A DSN node is the core process that participates in the DSN network. It handles block production, transaction validation, peer-to-peer networking, state persistence, and metrics export. Every validator, seed node, and observer runs the same binary with different configurations.

---

## 2. Architecture

The node (`node/` package) is a central orchestrator that coordinates several subsystems:

| Component | Package | Role |
|-----------|---------|------|
| **State** | `state.InMemoryState` | In-memory SMT-based state, accounts, KV store |
| **Persistent State** | `state.PersistentState` | BoltDB-backed durable storage for state.db |
| **Mempool** | `mempool.Mempool` | Pending transaction pool (max 10,000, TTL 300s) |
| **P2P** | `network.P2PNode` | libp2p networking, peer discovery, gossip |
| **VM** | `vm.VM` | WASM execution engine (30s timeout) |
| **Indexer** | `indexer.Indexer` | Optional event/log indexing (requires DataDir) |
| **Consensus** | Engine (fastSync, blockSync, gossip, discovery) | BFT consensus, sync engines |
| **Metrics** | HTTP server | `/metrics` (Prometheus) and `/health` endpoints |

**Node struct core fields:**

```
cfg (node.Config)
state (*state.InMemoryState)
persistent (*state.PersistentState)
mempool (*mempool.Mempool)
p2p (*network.P2PNode)
vm (*vm.VM)
indexer (*indexer.Indexer)
syncMode (SyncMode)
fastSync, blockSync, gossip, discovery (engine refs)
finalizedHeight, currentHeight, currentTipHash, currentEpoch
genesisDoc, genesisHash, validators, consensusParams, epochParams
metricsServer
```

---

## 3. Installation

### Build from source

```bash
git clone <repo-url> tdb
cd tdb
go build -o bin/dsn ./cmd/dsn
```

The binary is placed at `bin/dsn`. Verify with:

```bash
bin/dsn version
bin/dsn node start --help
```

### Prerequisites

- Go 1.21+
- 4+ cores, 8GB+ RAM (see [Hardware Requirements](#13-hardware-requirements))

---

## 4. Configuration

### 4.1 Configuration Hierarchy

```
CLI flags  >  Environment Variables (DSN_*)  >  TOML config file  >  Hardcoded defaults
```

### 4.2 Node Config Fields

The node uses a flat `node.Config` struct (NOT the same as `config.Config`). Created by the CLI before passing to `node.New()`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ChainID` | uint32 | `0` | 0 = devnet |
| `DataDir` | string | `""` | Empty = in-memory only (data lost on restart) |
| `MempoolMaxSize` | int | `10000` | Max pending transactions in mempool |
| `MempoolTTL` | duration | `300s` | Max time a tx stays in mempool |
| `RPCPort` | int | `8545` | 0 = disabled |
| `P2PPort` | int | `0` | 0 = disabled |
| `Validators` | []Address | `[]` | Pre-configured validator addresses |
| `MaxTxPerBlock` | int | `100` | Max transactions per block |
| `ProposerTimeout` | duration | `5s` | Block proposal timeout |
| `GenesisFile` | string | `""` | Path to genesis JSON file |
| `ValidatorKeyFile` | string | `""` | Empty = read-only mode (no consensus participation) |
| `MaxPeers` | int | `50` | Max P2P connections |
| `MetricsPort` | int | `9464` | 0 = disabled |
| `IndexerEnabled` | bool | `false` | Requires DataDir to be set |
| `SnapshotInterval` | uint64 | `10` | Epochs between state snapshots |
| `FastSyncEnabled` | bool | `false` | Enable fast sync from checkpoint |
| `TrustedCheckpointHeight` | uint64 | `0` | Fast sync checkpoint height |
| `TrustedCheckpointHash` | string | `""` | Fast sync checkpoint block hash |
| `BootstrapPeers` | []string | `nil` | Initial peer multiaddrs |
| `VMTimeoutSeconds` | uint64 | `30` | WASM execution timeout |
| `FSync` | bool | `false` | BoltDB synchronous writes (enable for production durability) |

### 4.3 CLI Flags

```bash
dsn node start \
  --p2p-port 26656 \
  --rpc-port 8545 \
  --metrics-port 9464 \
  --data-dir /var/lib/dsn \
  --max-peers 50 \
  --genesis-file ./genesis.json \
  --validator-key ./validator.key \
  --fast-sync \
  --trusted-height 1000 \
  --trusted-hash "0x..."
```

### 4.4 Environment Variables

| Variable | Maps To |
|----------|---------|
| `DSN_P2P_LISTEN_ADDR` | P2P listen multiaddr |
| `DSN_P2P_PORT` | P2PPort |
| `DSN_P2P_MAX_PEERS` | MaxPeers |
| `DSN_P2P_BOOTSTRAP_PEERS` | BootstrapPeers |
| `DSN_P2P_PING_INTERVAL` | P2P ping interval |
| `DSN_RPC_ENABLED` | Enable RPC |
| `DSN_RPC_LISTEN_ADDR` | RPC listen address |
| `DSN_RPC_PORT` | RPCPort |
| `DSN_RPC_CORS_ORIGINS` | CORS origins |
| `DSN_METRICS_ENABLED` | Enable metrics |
| `DSN_METRICS_LISTEN_ADDR` | Metrics listen address |
| `DSN_METRICS_PORT` | MetricsPort |
| `DSN_DATA_DIR` | DataDir |
| `DSN_MAX_DB_SIZE` | Max database size |
| `DSN_SNAPSHOT_ENABLED` | Enable snapshots |
| `DSN_SNAPSHOT_INTERVAL` | SnapshotInterval |
| `DSN_SNAPSHOT_MAX` | Max snapshots to keep |
| `DSN_SNAPSHOT_OUTPUT_DIR` | Snapshot output directory |
| `DSN_VALIDATOR_KEY_FILE` | ValidatorKeyFile |
| `DSN_VALIDATOR_STAKE` | Validator stake |
| `DSN_VALIDATOR_COMMISSION_RATE` | Commission rate |
| `DSN_LOG_LEVEL` | Log level (debug, info, warn, error) |
| `DSN_LOG_FORMAT` | Log format (text, json) |
| `DSN_LOG_OUTPUT` | Log output (stdout, stderr, file path) |
| `DSN_LOG_FILE_ENABLED` | Enable file logging |
| `DSN_CHAIN_ID` | ChainID |
| `DSN_MEMPOOL_SIZE` | MempoolMaxSize |
| `DSN_MEMPOOL_TTL` | MempoolTTL |
| `DSN_MAX_TX_PER_BLOCK` | MaxTxPerBlock |
| `DSN_PROPOSER_TIMEOUT` | ProposerTimeout |
| `DSN_INDEXER` | IndexerEnabled |
| `DSN_FAST_SYNC` | FastSyncEnabled |
| `DSN_TRUSTED_HEIGHT` | TrustedCheckpointHeight |
| `DSN_TRUSTED_HASH` | TrustedCheckpointHash |

### 4.5 TOML Config File

Config fields match struct field names exactly (no TOML tags). Place in `<DataDir>/config.toml` or point via `--config`.

```toml
P2PPort = 26656
P2PMaxPeers = 50
BootstrapPeers = ["/ip4/1.2.3.4/tcp/26656/p2p/PeerID"]
RPCPort = 8545
MetricsPort = 9464
DataDir = "/var/lib/dsn"
IndexerEnabled = true
FastSyncEnabled = false
SnapshotInterval = 10
MempoolMaxSize = 10000
MaxTxPerBlock = 100
ProposerTimeout = "5s"
VMTimeoutSeconds = 30
FSync = true
```

### 4.6 Config Validation Rules

| Field | Rule |
|-------|------|
| P2PPort | 0 (disabled) or 1024–65535 |
| RPCPort | 1024–65535 (cannot be 0) |
| MetricsPort | 0 (disabled) or 1024–65535 |
| MaxPeers | Must be > 0 |
| IndexerEnabled | Requires DataDir to be set |
| MempoolMaxSize | Must be > 0 |
| MaxTxPerBlock | Must be > 0 |
| CommissionRate | ≤ 10000 (100%) |

---

## 5. Startup Sequence

The node executes 14 phases during startup:

### Phase 1–2: Load and Validate Config

- Create DataDir if it doesn't exist
- Validate port ranges (see validation table above)
- Validate max peers
- Apply defaults for unset fields

### Phase 3–4: Load and Validate Genesis

- Load genesis JSON from `GenesisFile` path
- Validate genesis structure

### Phase 5: Hash Genesis

- Compute SHA-256 of genesis document
- Verify against stored hash in DB (detects data corruption on restart)

> **WARNING**: If the genesis hash doesn't match the stored hash, the node will refuse to start. This protects against data corruption or accidental genesis replacement. See [Recovery Procedures](#9-recovery-procedures).

### Phase 6: Init BoltDB

- Open `state.db` in DataDir
- Create genesis bucket

### Phase 7: Init Genesis State

**Fresh DB (first run):**
- Initialize treasury
- Create genesis accounts and balances
- Register genesis validators
- Initialize SMT with empty state root

**Existing DB (restart):**
- Load all accounts from persistent store into InMemoryState
- Load all KV entries from persistent store into InMemoryState

### Phase 8: Init Validator Registry

- Load validators from DB
- If `ValidatorKeyFile` is set, verify the key corresponds to a registered validator
- If no key provided, node runs in read-only mode (no consensus participation)

### Phase 9: Init Consensus Engine

- Copy consensus parameters from genesis (block size, voting period, etc.)
- Copy epoch parameters from genesis
- Initialize engine components (fastSync, blockSync, gossip, discovery)

### Phase 10: Init Networking

- Start libp2p host if P2PPort > 0
- Start peer discovery (if bootstrap peers configured)
- Listen for incoming connections

### Phase 11: Init RPC

- Announce RPC port (server process is managed by CLI, not node)
- Register JSON-RPC handlers

### Phase 12: Init Metrics

- Start HTTP server at configured `MetricsPort`
- Register `/metrics` endpoint (Prometheus text format)
- Register `/health` endpoint (returns 200 OK)

### Phase 13: Enter Sync Mode

Determines synchronization strategy based on state and config:

| Mode | Value | Condition |
|------|-------|-----------|
| `SyncModeNormal` | 0 | Normal consensus — node is up to date |
| `SyncModeFastSync` | 1 | `FastSyncEnabled=true` and checkpoint configured |
| `SyncModeReplay` | 2 | Declared in code, NEVER actively set in v0.1.0-sandbox |

### Phase 14: Transition to Live

- If validator key is present and node is in Normal sync mode → start consensus loop
- If no validator key → wait (read-only observer)
- If in FastSync mode → `FastSyncFromCheckpoint()` is called (see [Fast Sync](#52-fast-sync))

---

## 6. Running a Node

### 6.1 dsn node start

```bash
# Read-only observer (no validator key)
dsn node start \
  --data-dir /var/lib/dsn \
  --p2p-port 26656 \
  --rpc-port 8545 \
  --genesis-file ./genesis.json

# Validator node
dsn node start \
  --data-dir /var/lib/dsn \
  --validator-key ./validator.key \
  --p2p-port 26656 \
  --rpc-port 8545 \
  --genesis-file ./genesis.json

# Fast sync from local snapshot
dsn node start \
  --data-dir /var/lib/dsn \
  --fast-sync \
  --trusted-height 1000 \
  --trusted-hash "0xabc123..." \
  --genesis-file ./genesis.json

# Enable metrics and indexing
dsn node start \
  --data-dir /var/lib/dsn \
  --metrics-port 9464 \
  --indexer \
  --genesis-file ./genesis.json
```

### 6.2 dsn devnet

For local development with a single-node devnet:

```bash
dsn devnet --validators 1 --data-dir ./devnet-data
```

This starts a pre-configured devnet with the specified number of validators. Internally it sets `ChainID = 0` and generates a development genesis.

> **Note**: Devnet mode is designed for local testing. Do NOT use in production.

---

## 7. Shutdown

### Graceful shutdown sequence

When a node receives SIGINT/SIGTERM (Ctrl+C or `kill`):

1. Close shutdown channel (signals all goroutines to stop)
2. Stop consensus engine (cease block proposal/voting)
3. Stop indexer (flush pending writes)
4. Stop fast sync, block sync, and discovery engines
5. Persist peer database, stop PeerManager, close P2P host
6. Stop metrics HTTP server
7. Close VM (halt WASM execution)
8. Store final tip (height, state root, timestamp)
9. Close persistent state (BoltDB `state.db`)

```bash
# Graceful stop
kill -TERM <pid>

# Or via systemd
systemctl stop dsn
```

> **WARNING**: Force kill (`kill -9`) may corrupt BoltDB. Always use SIGTERM first and wait for clean shutdown.

---

## 8. Restart

### With existing data

On restart with a populated DataDir:

1. Genesis hash is verified against stored hash (Phase 5)
2. Tip is loaded from consensus store
3. All accounts and KV entries are loaded into memory
4. SMT is rebuilt in memory
5. State root is verified against checkpoint

> **If state root mismatch**: see [Recovery — State Root Mismatch](#92-state-root-mismatch)

### Write flags

For production durability on restart, set `FSync = true`. This forces BoltDB to sync writes to disk on every transaction, slowing writes but preventing data loss on crash.

---

## 9. Recovery Procedures

### 9.1 Genesis Hash Mismatch

The computed SHA-256 of the genesis file does not match the stored hash in BoltDB.

```
Possible causes:
- Genesis file was replaced or modified
- BoltDB corruption (state.db)
- Data directory copied from another node with different genesis
```

**Resolution:**
1. Restore original genesis file from backup
2. If genesis is correct but DB is corrupted, restore from snapshot (see [OPERATIONS.md](./OPERATIONS.md#snapshot-based-restore))
3. As last resort: wipe DataDir and re-sync from genesis (fast sync)

### 9.2 State Root Mismatch

After loading state from DB on restart, the computed SMT root doesn't match the stored checkpoint.

```
Possible causes:
- BoltDB corruption
- Incomplete WAL replay
- Manual data directory tampering
```

**Resolution:**
- If snapshots are enabled, the node restores from the latest snapshot automatically and replays blocks
- If snapshots are NOT available, the node attempts to replay blocks from the last checkpoint
- If both fail: restore from external backup or re-sync

### 9.3 Fast Sync Recovery

Fast sync uses `FastSyncFromCheckpoint()` which:
1. Restores state from a local snapshot file
2. Verifies state root against `TrustedCheckpointHash`
3. Replays blocks from checkpoint height to tip

> **Note**: Network fast sync (downloading snapshots from peers) is NOT implemented in v0.1.0-sandbox. Only local snapshot restore works.

```bash
# Prepare snapshot at checkpoint
# Start node with fast sync enabled
dsn node start \
  --data-dir /var/lib/dsn \
  --fast-sync \
  --trusted-height <CHECKPOINT_HEIGHT> \
  --trusted-hash "<CHECKPOINT_HASH>" \
  --genesis-file ./genesis.json
```

If `SyncFromNetwork()` is called, it returns an error: "fast sync from network not yet implemented; use FastSyncFromCheckpoint with a local snapshot file".

### 9.4 Corrupted BoltDB

```bash
# Verify bolt DB integrity (requires bbolt tool)
bbolt check /var/lib/dsn/state.db

# If corrupted, restore from backup
# or re-sync from genesis
```

---

## 10. Monitoring

### 10.1 Metrics Server

When `MetricsPort > 0`, the node starts an HTTP server with two endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /metrics` | Prometheus text format metrics |
| `GET /health` | Returns 200 OK (health check) |

**Available metric:**

```
# HELP dsn_block_height Current block height
# TYPE dsn_block_height gauge
dsn_block_height <value>
```

```bash
# Check metrics
curl http://localhost:9464/metrics

# Health check
curl http://localhost:9464/health
```

### 10.2 Logging

Configurable via environment variables:

| Variable | Values | Default |
|----------|--------|---------|
| `DSN_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `DSN_LOG_FORMAT` | `text`, `json` | `text` |
| `DSN_LOG_OUTPUT` | `stdout`, `stderr`, file path | `stdout` |
| `DSN_LOG_FILE_ENABLED` | `true`, `false` | `false` |

### 10.3 Important Log Messages

| Log Pattern | Meaning |
|-------------|---------|
| `"genesis hash mismatch"` | Data corruption or genesis replaced |
| `"fast sync from network not yet implemented"` | Network sync attempted |
| `"entering sync mode: normal"` | Node is up to date |
| `"entering sync mode: fast sync"` | Node syncing from checkpoint |
| `"transitioning to live"` | Consensus engine starting |
| `"storing final tip: height=%d"` | Graceful shutdown |

---

## 11. Data Management

### 11.1 Directory Layout

When `DataDir` is set, the directory structure is:

```
<DataDir>/
├── state.db          # BoltDB database (all persistent state)
├── snapshots/        # State snapshots (if SnapshotInterval > 0)
│   ├── snapshot_<epoch>.snap
│   └── ...
└── config.toml       # Optional config file
```

### 11.2 Storage

| Store | Backend | Content |
|-------|---------|---------|
| Persistent state | BoltDB (`state.db`) | Genesis, validators, consensus params, tip, accounts, KV store, peer DB |
| In-memory state | RAM (SMT) | Full account trie, KV store (loaded from BoltDB on restart) |

### 11.3 Snapshots

- **Interval**: Every `SnapshotInterval` epochs (default: 10)
- **Format**: Binary snapshot files in `<DataDir>/snapshots/`
- **Purpose**: Fast recovery and fast sync checkpoint restoration
- **Manual trigger**: Not supported via CLI in v0.1.0-sandbox

### 11.4 Estimating Storage

BoltDB grows with chain data. Rough estimation factors:
- Each account: ~200 bytes + key/value storage
- Each KV entry: key size + value size + metadata
- Blocks: not stored in BoltDB (only tip and state)

---

## 12. Security Recommendations

- **Data directory**: Set permissions to `0700`
  ```bash
  chmod 0700 /var/lib/dsn
  ```

- **Validator key file**: Set permissions to `0600`
  ```bash
  chmod 0600 /var/lib/dsn/validator.key
  ```

- **RPC port**: Restrict to trusted IPs via firewall. Do NOT expose publicly on sandbox.

- **Secrets**: Use environment variables (not config files) for sensitive values.

- **FSync**: Enable `FSync = true` in production for crash durability (performance tradeoff).

- **Regular backups**: Backup the entire DataDir periodically.

- **Disk space**: Monitor BoltDB growth. The database grows with chain data and does not auto-shrink.

- **Run as non-root**: Create a dedicated user:
  ```bash
  useradd --system --no-create-home dsn
  chown -R dsn:dsn /var/lib/dsn
  ```

---

## 13. Hardware Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 4 cores | 8+ cores |
| RAM | 8 GB | 16+ GB |
| Storage | 100 GB SSD | 500 GB SSD |
| Network | 100 Mbps | 1 Gbps |

The SMT state is held entirely in RAM. Larger state = more RAM required.

---

## 14. Troubleshooting

> For detailed troubleshooting, see [TROUBLESHOOTING.md](./TROUBLESHOOTING.md).

### Node fails to start

1. Verify genesis file exists and is valid JSON
2. Check port availability (no conflicts)
3. Verify DataDir permissions
4. Check logs for "genesis hash mismatch"

### Node not producing blocks

1. Verify `ValidatorKeyFile` is set and points to a valid key
2. Check sync mode: must be `SyncModeNormal`
3. Verify node is not stuck in FastSync

### Validator key not accepted

- Ensure the key corresponds to a validator registered in genesis
- Check `ValidatorKeyFile` path is readable

### Node running in read-only mode

- If no `ValidatorKeyFile` is provided, the node operates as an observer only

### Cannot fast sync

- Network fast sync is NOT implemented. Only local snapshot restore is supported.
- Ensure `TrustedCheckpointHeight` and `TrustedCheckpointHash` are set correctly
- Ensure a valid snapshot file exists at the checkpoint

### Metrics not available

- Verify `MetricsPort` is set to a valid port (not 0)
- Check firewall allows access to the metrics port

### State corruption

- See [Recovery — State Root Mismatch](#92-state-root-mismatch)
- Or [OPERATIONS.md](./OPERATIONS.md#wal-based-recovery)

---

## Related Documentation

- [QUICKSTART.md](./QUICKSTART.md) — First steps
- [OPERATIONS.md](./OPERATIONS.md) — Deployment (Helm/systemd), production procedures
- [CLI_REFERENCE.md](./CLI_REFERENCE.md) — All CLI commands and flags
- [CONSENSUS.md](./CONSENSUS.md) — BFT consensus protocol
- [PERSISTENCE.md](./PERSISTENCE.md) — State persistence and snapshots
- [VALIDATOR_GUIDE.md](./VALIDATOR_GUIDE.md) — Validator setup
- [RECOVERY_RUNBOOK.md](./RECOVERY_RUNBOOK.md) — Incident response
- [NETWORKING.md](./NETWORKING.md) — P2P networking detail
- [SANDBOX_LIMITATIONS.md](./SANDBOX_LIMITATIONS.md) — v0.1.0-sandbox known limitations

## Deployment

### Sandbox Deployment
# DSN v0.1.0-sandbox — Sandbox Deployment Guide

> Public deployment guide for the DSN sandbox. Covers all implemented deployment options. See [NODE_OPERATION.md](./NODE_OPERATION.md) for node configuration details and [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) for issue resolution.

---

## 1. Overview

DSN offers multiple deployment modes, each with different tradeoffs:

| Mode | Use Case | State | Persistence | Network |
|------|----------|-------|-------------|---------|
| Devnet (single node) | Development, testing | In-memory | Temp dir, lost on reboot | Standalone |
| Docker multi-node | Local multi-validator testing | Ephemeral | Docker volumes | 3 validators |
| Production single-node | Public-facing sandbox | Persistent | BoltDB on disk | P2P network |
| Kubernetes (Helm) | Container orchestration | Persistent PVC | StatefulSet | Cluster-internal |

---

## 2. Devnet (Single Node)

Fastest way to get a DSN node running for development and testing.

```bash
./dsn devnet --rpc-port 8545 --explorer-port 8080
```

### What runs automatically

| Component | Endpoint | Notes |
|-----------|----------|-------|
| RPC Server | `:8545` | JSON-RPC API |
| Explorer (REST) | `:8080/api/v1` | Block/transaction explorer |
| Faucet | `POST /api/v1/faucet` | 100 DSN per request, 60s rate limit per IP |
| Metrics | `:9464/metrics` | Prometheus-format metrics |
| Health | `:9464/health` | Health check endpoint |
| Block producer | Built-in | 3-second ticker |

### Optional flags

| Flag | Effect |
|------|--------|
| `--indexer` | Enables block/transaction event indexing for explorer queries |
| `--reset` | Clears state and starts fresh (deletes temp data dir) |
| `--p2p-port` | P2P port (default: 0 = disabled for devnet) |

### Data directory

- **Unix**: `$TMPDIR/dsn-devnet`
- **Windows**: `$TEMP/dsn-devnet`
- **Linux**: `/tmp/dsn-devnet`

### Limitations

- ⚠ State is LOST on reboot (temp directory)
- ⚠ No WASM contract execution (VM is nil on devnet)
- ⚠ No P2P networking (single node only)
- ⚠ Explorer frontend HTML may call incorrect RPC methods (broken UX)

---

## 3. Docker Multi-Node Devnet

Runs 3 validator nodes locally using Docker Compose. Files in `dev/localnet/`:

| File | Purpose |
|------|---------|
| `docker-compose.yml` | 3-validator service definition |
| `Dockerfile` | Container image build |
| `init.sh` | Initialization script (creates configs + keys) |
| `templates/config.toml.tpl` | Node configuration template |
| `genesis.go` | Genesis generation for localnet |

### Setup

```bash
# 1. Generate devnet structure (node dirs, genesis, keys)
./dsn genesis devnet --validators 3

# 2. Start the network
cd dev/localnet
docker-compose up -d
```

### Node ports

| Node | P2P | RPC | Metrics |
|------|-----|-----|---------|
| node0 | 26656 | 8545 | 9464 |
| node1 | 26657 | 8546 | 9465 |
| node2 | 26658 | 8547 | 9466 |

### Network topology

- All nodes on `dsn-localnet` bridge network (`10.0.1.0/24`)
- Discovery enabled between nodes
- Shared genesis file mounted read-only

### Caveats

- ⚠ `genesis devnet` generates configs with PLACEHOLDER peer IDs
- ⚠ The network may NOT form full consensus without manual peer ID replacement
- ⚠ Need to replace `persistent_peers` with actual node IDs after first boot: inspect each node's `node_key.json` to get the peer ID, update configs, and restart

---

## 4. Production Single-Node

For a persistent, public-facing sandbox node.

```bash
./dsn node start \
  --genesis /path/to/genesis.json \
  --validator-key ~/.dsn/validator.key \
  --data-dir /var/lib/dsn
```

### Required files

| File | Description | Default |
|------|-------------|---------|
| `genesis.json` | Chain genesis state | Required, no default |
| `validator.key` | Ed25519 private key | `~/.dsn/validator.key` |
| Data directory | BoltDB state storage | `./data` |

### Optional flags

| Flag | Description |
|------|-------------|
| `--rpc-port` | JSON-RPC port (default: 8545) |
| `--p2p-port` | P2P listen port (default: 26656, 0 = disabled) |
| `--metrics-port` | Prometheus metrics port (default: 9464) |
| `--indexer` | Enable block/transaction indexer (requires data dir) |
| `--reset` | Clear existing state and start fresh |

### Data directory contents

```
/var/lib/dsn/
├── state.db          # BoltDB state
├── validator.key     # Validator private key
├── node_key.json     # P2P node identity
├── config.toml       # Node configuration
├── genesis.json      # Genesis file (first boot)
└── data/
    ├── blocks.db     # Block storage (if indexer enabled)
    └── index/        # Event/log index (if indexer enabled)
```

### TLS

DSN does NOT include built-in TLS. For HTTPS access, use a reverse proxy (see [Section 10 — Recommended Topology](#10-recommended-topology)).

---

## 5. Systemd Service

For Linux deployments with systemd. Files in `deploy/systemd/`.

### Files

| File | Purpose |
|------|---------|
| `dsn.service` | Systemd unit file |
| `install.sh` | Automated installation script |

### Automated install

```bash
# Run as root from deploy/systemd/
sudo ./install.sh
```

This creates:
- `dsn` system user
- `/var/lib/dsn` data directory
- `/etc/dsn` config directory
- Enables the `dsn` service

### Manual install

```bash
sudo cp deploy/systemd/dsn.service /etc/systemd/system/
sudo mkdir -p /var/lib/dsn /etc/dsn
sudo systemctl daemon-reload
sudo systemctl enable dsn
```

### Service config

DSN reads environment from `/etc/dsn/config.env`. Example:

```env
DSN_GENESIS=/etc/dsn/genesis.json
DSN_VALIDATOR_KEY=/var/lib/dsn/validator.key
DSN_DATA_DIR=/var/lib/dsn
```

### Security hardening (from unit file)

| Setting | Value |
|---------|-------|
| User/Group | `dsn:dsn` |
| PrivateTmp | true |
| ProtectSystem | full |
| ProtectHome | true |
| NoNewPrivileges | true |
| LimitNOFILE | 65535 |

---

## 6. Monitoring Stack

### Prometheus

Config in `monitoring/prometheus/`:

| File | Description |
|------|-------------|
| `prometheus.yml` | Scrape configuration targeting `:9464/metrics` |
| `rules.yml` | Alerting rules |

**Alerting rules** (8 total):

| Severity | Rule | Description |
|----------|------|-------------|
| CRITICAL | `DSNNodeDown` | Node unreachable for >60s |
| CRITICAL | `DSNHighMemoryUsage` | Memory >90% of 4GB limit |
| CRITICAL | `DSNHighCPUUsage` | CPU >90% for >5min |
| CRITICAL | `DSNRPCLatencyHigh` | RPC latency >5s |
| CRITICAL | `DSNBlockProductionStopped` | No new blocks for >30s |
| WARNING | `DSNMempoolBacklog` | Mempool >5000 pending txs |
| WARNING | `DSNPeerCountLow` | Fewer than 3 connected peers |
| WARNING | `DSNDiskUsageHigh` | Disk usage >80% |

### Grafana

Dashboard JSON in `monitoring/grafana/dsn-dashboard.json`.

**Dashboard panels**:
- Block height (current vs finalised)
- Transaction throughput (TPS)
- Mempool size
- Connected peers
- CPU / memory usage
- RPC request latency
- Validator set size

### Starting the stack

```bash
# Example prometheus.yml expects targets on localhost:9464
prometheus --config.file=monitoring/prometheus/prometheus.yml

# Import dsn-dashboard.json into Grafana
# Data source: Prometheus (http://localhost:9090)
```

### Caveats

- ⚠ Alert rules reference metrics that may not exist yet (e.g., `dsn_rpc_request_duration_seconds` was partially fixed, but some metrics classes may still be missing)
- ⚠ Some Grafana dashboard panel queries may reference metrics that don't map to actual telemetry emitted by the node

---

## 7. Kubernetes / Helm

Helm chart at `charts/dsn/`.

### Chart structure

```
charts/dsn/
├── Chart.yaml          # Chart metadata (v0.1.0)
├── values.yaml         # Default configuration
└── templates/
    ├── configmap.yaml  # Node configuration as ConfigMap
    ├── deployment.yaml # Deployment (for non-persistent mode)
    ├── service.yaml    # Service with p2p + metrics ports
    ├── statefulset.yaml# StatefulSet (for persistent storage)
    └── _helpers.tpl    # Template helpers
```

### Default values

| Parameter | Default |
|-----------|---------|
| `replicaCount` | 1 |
| `image.repository` | `dsn/node` |
| `service.type` | ClusterIP |
| `persistence.size` | 10Gi |
| `resources.limits.cpu` | 2000m |
| `resources.limits.memory` | 4Gi |

### Deploy

```bash
helm install dsn ./charts/dsn --values ./charts/dsn/values.yaml
```

### Staging overlay

For staging deployments, `deploy/staging/` provides a Kustomize overlay:

```
deploy/staging/
├── kustomization.yaml
├── namespace.yaml
├── network-policy.yaml
├── pvc-storage-class.yaml
├── bootstrap-configmap.yaml
└── monitoring.yaml
```

### Caveats

- ⚠ `values.yaml` uses placeholder peer IDs and genesis hashes — replace before deploying
- ⚠ NOT validated for production use — this is a sandbox chart
- ⚠ `statefulset.enabled` must be `true` for persistent storage
- ⚠ No `HorizontalPodAutoscaler` or `PodDisruptionBudget` configured

---

## 8. CI/CD

### GitHub Actions

**`ci.yml`** — On push/PR to main:
- Go build (cross-platform)
- Run unit tests
- Lint

**`release.yml`** — On git tag:
- Cross-compile binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Create GitHub release with artifacts

---

## 9. Current Limitations for Public Sandbox

| Area | Limitation |
|------|------------|
| Devnet state | Temp directory, lost on reboot |
| Contract execution | NOT available on devnet (nil VM) |
| Multi-node Docker | May not form consensus (placeholder peer IDs) |
| Explorer frontend | Frontend HTML calls wrong RPC methods (broken UX) |
| Faucet | 60s rate limit per IP, 100 DSN per request |
| RPC | Events/validators/state root/pending txs are stubs |
| Fast sync | Network sync not implemented; local snapshot only |
| TLS/HTTPS | No built-in TLS. Use reverse proxy (nginx/caddy) |
| Authentication | No RPC authentication. Use firewall/network isolation |
| Backup | No built-in backup tooling. Manual data dir copy |
| WASM VM | Devnet VM is nil; production node has a 30s execution timeout |
| Explorer | Only REST API exists; no frontend SPA in this release |

See [SANDBOX_LIMITATIONS.md](./SANDBOX_LIMITATIONS.md) for the full limitations document including cryptographic, consensus, and state machine constraints.

---

## 10. Recommended Topology

### Architecture

```
Internet
    │
    ▼
[Reverse Proxy (Caddy) :443]
    │
    ├── /api/* ───────────► DSN Node :8545 (RPC)
    ├── /explorer/* ──────► DSN Node :8080 (Explorer)
    └── /metrics/* ───────► [Authenticated Proxy] ──► Node :9464
                                                          │
                                                     ┌────┴────┐
                                                     ▼         ▼
                                                 Prometheus  Grafana
```

### Recommendations

1. **Reverse proxy**: Caddy recommended for auto-TLS via Let's Encrypt
2. **RPC exposure**: Use proxy rules to restrict to read-only methods in public sandbox
3. **Explorer**: Expose REST API publicly (it's read-only)
4. **Metrics**: Keep private or require authentication (basic auth or IP whitelist)
5. **Indexer**: Run with `--indexer` for block/transaction lookup via explorer
6. **Data backups**: Regularly copy data directory (see [Section 11 — Management](#11-management))

### Example Caddyfile

```caddy
sandbox.dsn.example {
    reverse_proxy /api/* localhost:8545
    reverse_proxy /explorer/* localhost:8080
    reverse_proxy /health localhost:9464

    @metrics {
        path /metrics*
        remote_ip 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16
    }
    handle @metrics {
        reverse_proxy localhost:9464
    }
    handle /metrics* {
        respond 403
    }
}
```

### Firewall rules

| Source | Destination | Port | Protocol | Purpose |
|--------|-------------|------|----------|---------|
| Internet | Reverse proxy | 443 | TCP | HTTPS |
| Internet | Node | 26656 | TCP | P2P (if joining public network) |
| Reverse proxy | Node | 8545 | TCP | RPC (internal only) |
| Reverse proxy | Node | 8080 | TCP | Explorer (internal only) |
| Prometheus | Node | 9464 | TCP | Metrics (internal only) |
| Grafana | Prometheus | 9090 | TCP | Dashboard queries |

---

## 11. Management Operations

### Service lifecycle (systemd)

```bash
# Start
sudo systemctl start dsn

# Stop
sudo systemctl stop dsn

# Restart
sudo systemctl restart dsn

# Enable on boot
sudo systemctl enable dsn

# Status
sudo systemctl status dsn

# Logs
sudo journalctl -u dsn -f

# Logs (last 100 lines)
sudo journalctl -u dsn -n 100
```

### Process management (non-systemd)

```bash
# Start in background
./dsn node start --genesis genesis.json --validator-key ~/.dsn/validator.key --data-dir /var/lib/dsn &

# Graceful stop (SIGINT)
kill -INT <PID>

# Force stop
kill -9 <PID>
```

### Backup

```bash
# Stop the node first (BoltDB must not be open)
sudo systemctl stop dsn

# Backup data directory
sudo cp -r /var/lib/dsn /backup/dsn-$(date +%Y%m%d-%H%M%S)

# Restart the node
sudo systemctl start dsn
```

### Restore

```bash
sudo systemctl stop dsn

# Replace data directory
sudo mv /var/lib/dsn /var/lib/dsn.corrupted
sudo cp -r /backup/dsn-20260101-120000 /var/lib/dsn
sudo chown -R dsn:dsn /var/lib/dsn

sudo systemctl start dsn
```

### Upgrade

```bash
# 1. Download new binary
sudo cp dsn /usr/local/bin/dsn

# 2. Restart service
sudo systemctl restart dsn

# 3. Verify
sudo journalctl -u dsn -n 20
# Look for: "node started" with new version
```

### Monitoring checks

```bash
# Health endpoint
curl -s http://localhost:9464/health

# Metrics
curl -s http://localhost:9464/metrics | grep dsn_

# Block height
curl -s http://localhost:8545 -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# Connected peers (if P2P enabled)
curl -s http://localhost:8545 -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'
```

---

## 12. Cross-References

| Document | Content |
|----------|---------|
| [NODE_OPERATION.md](./NODE_OPERATION.md) | Node architecture, config, RPC methods, validator management |
| [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) | Genesis mismatch, validator key errors, port conflicts, sync issues |
| [SANDBOX_LIMITATIONS.md](./SANDBOX_LIMITATIONS.md) | Full cryptographic, consensus, state machine, and networking limitations |
| [CLI_REFERENCE.md](./CLI_REFERENCE.md) | Complete command reference for `dsn` binary |
| [VALIDATOR_GUIDE.md](./VALIDATOR_GUIDE.md) | Validator key management, staking, rewards |
| [OPERATIONS.md](./OPERATIONS.md) | Production procedures, DR, upgrade sequences |
| [NETWORKING.md](./NETWORKING.md) | P2P networking, peer discovery, NAT traversal

### Staging Validation
# DSN v0.1.0-sandbox — Staging Validation

This document provides comprehensive validation procedures for deploying a 3-validator DSN cluster in a Kubernetes staging environment.

## Prerequisites

- Kubernetes cluster (minikube, kind, or cloud)
- Helm v3
- kubectl
- Go 1.21+

### Required Tools Check

```bash
# Verify Kubernetes access
kubectl cluster-info

# Verify Helm installation
helm version

# Verify Go version
go version
```

## Deployment Steps

### 1. Deploy Helm Chart

Option A: Direct Helm install with staging values:

```bash
# Create staging values file
cat > values-staging.yaml << 'EOF'
replicaCount: 3
statefulset:
  enabled: true
persistence:
  enabled: true
  storageClass: "standard"
  size: 50Gi
config:
  chain:
    id: "dsn-staging"
  p2p:
    seeds: "seed1@dsn-seed-1:26656,seed2@dsn-seed-2:26656"
  metrics:
    enabled: true
resources:
  limits:
    cpu: 4000m
    memory: 8Gi
  requests:
    cpu: 1000m
    memory: 2Gi
EOF

# Install the chart
helm install dsn-sandbox charts/dsn/ \
  --namespace dsn-sandbox \
  --create-namespace \
  -f values-staging.yaml
```

Option B: Kustomize overlay (recommended):

```bash
# Generate Helm template output as base
helm template charts/dsn/ > deploy/staging/base.yaml

# Build and apply the staging overlay
kustomize build deploy/staging/ | kubectl apply -f -
```

### 2. Verify Deployment

```bash
# Check pods
kubectl -n dsn-sandbox get pods -l app.kubernetes.io/component=validator

# Check PVCs
kubectl -n dsn-sandbox get pvc

# Check services
kubectl -n dsn-sandbox get svc

# View logs
kubectl -n dsn-sandbox logs -l app.kubernetes.io/component=validator

# Verify pod events if issues arise
kubectl -n dsn-sandbox describe pod dsn-validator-0
```

### 3. Verify Monitoring

Deploy the monitoring stack:

```bash
# Apply monitoring configuration
kubectl apply -f deploy/staging/monitoring.yaml

# Check monitoring pods
kubectl -n dsn-monitoring get pods

# Port-forward Grafana
kubectl -n dsn-monitoring port-forward svc/grafana 3000:3000
```

Access Grafana at `http://localhost:3000` (default credentials: admin/admin)

### 4. Validate Helm Chart

Run the validation script:

```bash
# Make script executable
chmod +x deploy/scripts/validate-helm.sh

# Run validation
./deploy/scripts/validate-helm.sh
```

This validates:
- Helm lint
- Template rendering (default, staging, production)
- StatefulSet and PVC templates
- Deployment templates

## Validation Tests

### Test 1: Rolling Restart

**Purpose**: Validate that individual validator restarts maintain cluster consensus and quorum.

**Procedure**:
1. Record baseline state root before any restarts
2. Restart validator-0, verify it rejoins consensus
3. Produce 2+ blocks while validator-0 is down
4. Restart validator-1, verify catch-up
5. Restart validator-2, verify state convergence

**Expected**:
- Cluster maintains quorum (2/3 validators active)
- No rollback of confirmed blocks
- State roots converge across all validators

**Validation Commands**:
```bash
# Restart individual pods
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Check pod recreation
kubectl -n dsn-sandbox get pods -w

# Verify block production continues
kubectl -n dsn-sandbox logs dsn-validator-1 | grep -i "block"
```

**Checklist**:
- [ ] validator-0 restarted → still producing blocks
- [ ] validator-1 restarted → still producing blocks
- [ ] validator-2 restarted → all nodes synced

### Test 2: PVC Persistence

**Purpose**: Validate that state persists across pod deletions and recreations (StatefulSet behavior).

**Procedure**:
1. Record pre-deletion state root
2. Delete validator pod (PVC should persist)
3. Wait for StatefulSet to recreate pod
4. Verify state root matches pre-deletion

**Expected**:
- PVC binds to new pod automatically
- State root matches before/after deletion
- All WAL entries replayed correctly
- Snapshot at same height available

**Validation Commands**:
```bash
# Get PVC before deletion
kubectl -n dsn-sandbox get pvc

# Delete pod (not PVC)
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Verify PVC still bound
kubectl -n dsn-sandbox get pvc -l app.kubernetes.io/name=dsn

# Check state in persistent volume
kubectl -n dsn-sandbox exec dsn-validator-0 -- ls -la /data
```

**Checklist**:
- [ ] Pod deleted, recreated with same PVC
- [ ] State root matches before/after deletion
- [ ] All blocks processed
- [ ] WAL replayed from persisted files

### Test 3: Network Partition Recovery

**Purpose**: Validate validator can re-sync after being isolated from the network.

**Procedure**:
1. Record pre-partition state root
2. Induce partition (block P2P port 30333 on one validator)
3. Let other validators produce 10+ blocks
4. Heal partition (unblock port)
5. Verify partitioned validator catches up

**Expected**:
- Partitioned validator has pre-partition state
- After healing, validator catches up to chain tip
- State roots converge

**Validation Commands**:
```bash
# Simulate partition - block P2P port (use NetworkPolicy or firewall)
# In Kubernetes, apply network policy:

apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: partition-validator-2
  namespace: dsn-sandbox
spec:
  podSelector:
    matchLabels:
      statefulset.kubernetes.io/pod-name: dsn-validator-2
  policyTypes:
    - Ingress
    - Egress

# Wait and let other validators produce blocks
sleep 60

# Check block height on other validators
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn status

# Remove NetworkPolicy to heal partition
kubectl delete -f partition-validator-2.yaml

# Verify catch-up
kubectl -n dsn-sandbox exec dsn-validator-2 -- dsn status
```

**Checklist**:
- [ ] Partition induced (block port 30333)
- [ ] 10+ blocks produced by other validators
- [ ] Partition healed (unblock port)
- [ ] Partitioned validator catches up

### Test 4: Snapshot Recovery

**Purpose**: Validate node can recover from snapshot after state deletion.

**Procedure**:
1. Take snapshot backup of current state
2. Delete state data on one validator
3. Restart node and trigger snapshot restore
4. Verify correct state recovery and chain sync

**Expected**:
- Snapshot file exists and is valid
- Node recovers to snapshot height
- Continues block production from snapshot point

**Validation Commands**:
```bash
# Create snapshot backup
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn snapshot backup

# Verify snapshot created
kubectl -n dsn-sandbox exec dsn-validator-0 -- ls /data/snapshots/

# Delete state (simulate corruption)
kubectl -n dsn-sandbox exec dsn-validator-0 -- rm -rf /data/state/*
kubectl -n dsn-sandbox exec dsn-validator-0 -- rm -rf /data/wal/*

# Restart validator to trigger recovery
kubectl -n dsn-sandbox delete pod dsn-validator-0

# Check recovery
kubectl -n dsn-sandbox logs dsn-validator-0 | grep -i snapshot
```

**Checklist**:
- [ ] Snapshot backup taken
- [ ] State deleted
- [ ] Snapshot restored
- [ ] Node syncs correctly

### Test 5: Crash Recovery

**Purpose**: Validate node can recover from abrupt process termination (SIGKILL).

**Procedure**:
1. Record current state root and height
2. Send SIGKILL to validator process
3. Restart validator process
4. Verify WAL replay and state recovery

**Expected**:
- Uncommitted WAL entries are replayed
- State root matches pre-crash after recovery
- Node catches up to current chain

**Validation Commands**:
```bash
# Find validator process
PID=$(kubectl -n dsn-sandbox exec dsn-validator-0 -- pgrep dsn)

# Kill the process (simulate crash)
kubectl -n dsn-sandbox exec dsn-validator-0 -- kill -9 $PID

# Wait for restart
sleep 10

# Check recovery in logs
kubectl -n dsn-sandbox logs dsn-validator-0 | grep -i "wal\|replay\|recovery"

# Verify state root
kubectl -n dsn-sandbox exec dsn-validator-0 -- dsn status
```

**Checklist**:
- [ ] SIGKILL validator process
- [ ] Process restarted
- [ ] WAL replayed
- [ ] State root matches

## Validation Results

| Test | Status | Duration | Notes |
|------|--------|----------|-------|
| Rolling Restart | ⬜ | — | — |
| PVC Persistence | ⬜ | — | — |
| Network Partition | ⬜ | — | — |
| Snapshot Recovery | ⬜ | — | — |
| Crash Recovery | ⬜ | — | — |

## Metrics Validation

### Prometheus Targets

```bash
# Port-forward Prometheus
kubectl -n dsn-monitoring port-forward svc/prometheus 9090:9090

# Check targets
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job == "dsn-validators")'
```

**Checklist**:
- [ ] Prometheus targets are UP (check /targets)
- [ ] Grafana dashboard shows block height increasing
- [ ] Alerting rules evaluate correctly
- [ ] Node-exporter metrics available
- [ ] Logs are being collected

### Alert Rules Verification

Verify the following alerts are configured in `monitoring/prometheus/rules.yml`:

| Alert | Expression | Severity |
|-------|------------|----------|
| DSNBlockHeightNotIncreasing | `rate(dsn_block_height[5m]) == 0` | critical |
| DSNSyncLagHigh | `dsn_node_sync_lag_blocks > 10` | warning |
| DSNPeerCountLow | `dsn_p2p_peer_count < 3` | warning |
| DSNValidatorDown | `up{job="dsn"} == 0` | critical |
| DSNCPUUsageHigh | `rate(process_cpu_seconds_total[5m]) > 0.8` | warning |
| DSNMemoryUsageHigh | `process_resident_memory_bytes / 4294967296 > 0.9` | warning |
| PanicRecoveryRate | `rate(dsn_panic_recovery_total[5m]) > 0` | critical |
| GoroutineSpike | `go_goroutines > 10000` | warning |
| MempoolPressure | `dsn_mempool_current_size / dsn_mempool_max_size > 0.8` | warning |

### Grafana Dashboard

Access `monitoring/grafana/dsn-dashboard.json` for panels:

- Block Height (stat panel)
- Peer Count (stat panel)
- Active Validators (stat panel)
- Block Time (timeseries)
- CPU Usage (timeseries)
- Memory Usage (timeseries)
- Consensus Round (timeseries)
- Sync Lag (timeseries)

## Infrastructure Validation

### Core Components

```bash
# Check all validator pods are running
kubectl -n dsn-sandbox get pods -l app.kubernetes.io/component=validator

# Verify persistent volumes are bound
kubectl -n dsn-sandbox get pvc

# Check services are exposed
kubectl -n dsn-sandbox get svc
```

**Checklist**:
- [ ] 3 validators in active set
- [ ] Persistent volumes bound
- [ ] RPC endpoint reachable
- [ ] P2P peering established
- [ ] Explorer indexing blocks
- [ ] Faucet operational

### Port Reference

| Port | Service | Description |
|------|---------|-------------|
| 30333 | P2P | Validator P2P networking |
| 8545 | RPC | JSON-RPC API |
| 9090 | Metrics | Prometheus metrics endpoint |
| 26656 | Tendermint P2P | Internal consensus P2P |
| 26657 | Tendermint RPC | Internal RPC |

### Network Connectivity

```bash
# Test P2P connectivity between validators
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://dsn-validator-1.dsn-staging.svc.cluster.local:26657/status

# Test RPC accessibility
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://localhost:8545/status

# Test metrics endpoint
kubectl -n dsn-sandbox exec dsn-validator-0 -- curl -v http://localhost:9090/metrics
```

## Integration Test Reference

The following Go integration tests in `integration/` provide automated validation:

### Staging Tests (`integration/staging/`)
- `TestPVC_PersistenceAcrossRestart` - Validates state persists across pod restarts
- `TestPVC_WALReplayFromPersistedFiles` - Validates WAL replay from PVC
- `TestPVC_StateRootConvergence` - Validates state roots converge after restart
- `TestRollingRestart_Convergence` - Validates rolling restart maintains consensus
- `TestRollingRestart_QuorumMaintenance` - Validates quorum during restarts

### Resilience Tests (`integration/resilience_test.go`)
- `TestResilience_RollingRestart` - T4-1: Graceful rolling restart
- `TestResilience_CrashLoopRecovery` - T4-2: Crash-loop recovery
- `TestResilience_CorruptedSnapshot` - T4-3: Corrupted snapshot recovery
- `TestResilience_NetworkPartition` - T4-4: Network partition recovery
- `TestResilience_KillRestartLoop` - T4-5: Kill/restart loop (10 iterations)

Run tests:
```bash
go test -v -tags=integration ./integration/staging/...
go test -v -tags=integration ./integration/... -run TestResilience
```

## Troubleshooting

### Pods Not Starting

```bash
# Check events
kubectl -n dsn-sandbox describe pod dsn-validator-0

# Check storage class
kubectl get storageclass standard

# For local clusters, ensure WaitForFirstConsumer binding
kubectl get storageclass standard -o yaml | grep volumeBindingMode
```

### PVC Stuck in Pending

```bash
# Check storage class binding mode
kubectl get storageclass

# Verify storage provisioner is available
kubectl get pods -n kube-system | grep -i storage
```

### Network Policy Blocking Traffic

```bash
# Check network policies
kubectl -n dsn-sandbox get networkpolicy

# Test connectivity
kubectl -n dsn-sandbox exec -it dsn-validator-0 -- curl -v http://dsn-validator-1.dsn-staging.svc.cluster.local:8545
```

## Cleanup

```bash
# Delete staging namespace (removes all resources)
kubectl delete namespace dsn-staging
kubectl delete namespace dsn-monitoring

# Remove Helm release
helm uninstall dsn-sandbox
```

## Related Documentation

- [OPERATIONS.md](./OPERATIONS.md) - Day-to-day operational procedures
- [RECOVERY_RUNBOOK.md](./RECOVERY_RUNBOOK.md) - Detailed recovery procedures
- [PERSISTENCE.md](./PERSISTENCE.md) - Storage and state management
- [NETWORKING.md](./NETWORKING.md) - P2P networking details

### Mainnet Gap Analysis
# DSN Mainnet Gap Analysis

**Date**: 2026-05-19
**Based on**: Sandbox v0.1.0-sandbox

## Overview

The current sandbox implementation provides a functional deterministic settlement network. This document analyzes the gaps between the sandbox and a production mainnet.

## Resolved in Phase 8B

The following operational blockers were resolved in Phase 8B:

- **WAVE 4**: WASM VM enabled in devnet block production (`cmd/dsn/devnet.go`)
- **WAVE 5**: Real `transfer_token` host function implemented (`vm/host.go`)
- **WAVE 6**: FastSyncEngine P2P handlers wired to dispatch system (`network/fastsync.go`, `network/p2p.go`, `network/snapshot.go`)
- **WAVE 7**: Full economic test suite verified (100+ staking, rewards, slashing tests pass)
- **WAVE 8**: Full build verification complete (`go build ./...` passes, `go vet` clean)

---

## HIGH Priority (mainnet blockers)

These limitations block production mainnet deployment:

### Consensus & Security

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No formal verification** | Critical security risk | Consensus protocol has not been formally verified | Engage formal verification team (CertiK, Runtime Verification) or internal formal methods team |
| **Penetration testing pending** | Unknown vulnerabilities | No third-party security audit conducted | Commission comprehensive penetration test from reputable security firm |
| **Ed25519 only** | Limited key security | Single signature scheme | Implement threshold signatures, multi-sig support; consider transition to BLS or Schnorr |
| **No HSM support** | Validator key exposure | Keys stored on disk without hardware protection | Integrate with AWS KMS, Google Cloud HSM, or HashiCorp Vault |

### Economic & Governance

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No slashing automation** | Malicious validators go unpunished | Slashing requires manual evidence | Implement automatic detection and submission of slashing evidence |
| **No on-chain governance** | Protocol changes require hard forks | Parameter changes require coordinated manual updates | Design and implement on-chain governance module (token-based voting) |
| **No delegation** | Retail users cannot stake | Staking delegation not implemented | Implement delegation mechanism with commission rates |
| **No MEV protection** | Validator extracted value harms users | No MEV mitigation | Implement proposer-builder separation (PBS) or state-based MEV auctions |

### Operational & Networking

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No automatic failover** | Downtime on validator failure | Manual intervention required | Implement hot standby validators with automatic failover |
| **No NAT traversal** | Nodes behind NAT cannot participate | P2P connectivity fails for NAT'd nodes | Implement STUN/TURN, libp2p circuit relay, or NAT hole punching |
| **No peer scoring** | Bad actors not penalized | Peer reputation not tracked | Implement peer scoring/ reputation system with gossipsub validation |
| **No multi-DC deployment** | Single point of failure | Cross-DC not validated | Validate and document multi-datacenter deployment patterns |

---

## MEDIUM Priority (should have before mainnet)

These limitations significantly impact mainnet operation but can be addressed post-launch:

### State & Execution

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No state pruning** | Storage grows unbounded | Full archival nodes required | Implement state trie pruning with epoch-based cleanup |
| **No state rent** | State bloat attack vector | No economic mechanism | Implement state rent mechanism (storage deposits, per-byte fees) |
| **No parallel execution** | Throughput limited | Sequential transaction execution | Implement parallel transaction execution within blocks (Speculative/OPT) |

### Networking

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **DHT not implemented** | Limited peer discovery | Centralized bootstrap only | Implement Kademlia DHT for decentralized peer discovery |
| **No relay protocol** | NAT'd nodes cannot be reachable | No relay for inaccessible nodes | Implement libp2p circuit relay v2 |

### API & SDK

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **REST-only API** | Limited real-time data | No gRPC streaming | Implement gRPC API with bidirectional streaming |
| **No WebSocket pub/sub** | No real-time subscriptions | Limited subscription capabilities | Implement WebSocket-based event subscription |
| **No non-Go SDK** | Limited developer adoption | Only Go SDK available | Implement SDKs for TypeScript, Rust, Python |

### Storage

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **BoltDB only** | Single point of failure | No alternative storage backends | Implement PostgreSQL, RocksDB, or custom storage interface |
| **No backup automation** | Data loss risk | Manual backup procedures | Implement automated backup with redundancy (S3, GCS) |
| **No cold storage** | Cost inefficient | All data on hot storage | Implement tiered storage (hot/warm/cold) |

---

## LOW Priority (nice to have)

These limitations are enhancements that improve the system but are not blockers:

### Consensus

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **Single proposer** | Throughput limited | One proposer per round | Implement multiple proposers or leader rotation |
| **Fixed block time** | Limited responsiveness | 1 second block time | Implement dynamic block time based on load |
| **No light client** | Full node requirement | Light client verification unavailable | Implement stateless light client with sync committees |
| **No fast finality** | Linear finality | Linear with block confirmations | Implement finality gadget (Casper CBC or EigenDA) |

### WASM VM

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No floating point** | Limited contract capability | FP disabled for determinism | Implement deterministic floating point via soft-float |
| **No randomness host** | Limited gaming/simulation | Deterministic RNG not available | Implement verifiable random function (VRF) |
| **Single runtime** | Vendor lock-in | wazero only | Evaluate adding Wasmer or Wasmtime alternatives |

### Testing & Upgrade

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **24h soak only** | Unknown long-term behavior | 7+ day soaks not validated | Implement continuous soak testing infrastructure |
| **Single region testing** | Unknown geo-distributed behavior | Cross-region testing pending | Deploy testnet across multiple regions (AWS, GCP, Azure) |
| **No fuzzing** | Unknown edge cases | Fuzz testing pending | Implement fuzz testing for consensus and state machine |
| **No in-place upgrade** | Coordinated restarts required | Protocol upgrades need node restarts | Implement state migration and hot-upgrade mechanism |
| **No fork detection** | Manual monitoring required | Fork detection relies on manual monitoring | Implement automatic fork detection and alerting |

### Performance

| Limitation | Impact | Current State | Required Work |
|------------|--------|---------------|---------------|
| **No sharding** | Single shard throughput | All state on one shard | Implement sharding with cross-shard transactions |
| **No L2** | Limited scaling | No L2 solutions | Implement rollup orvalidium L2 |
| **No batch processing** | Limited optimization | No batching for high-throughput | Implement batch transaction submission |

---

## Summary

| Priority | Count | Key Blockers |
|----------|-------|--------------|
| HIGH | 11 | Formal verification, pentesting, HSM, slashing automation, governance, delegation, MEV, failover, NAT traversal, peer scoring, multi-DC |
| MEDIUM | 9 | State pruning, state rent, parallel execution, DHT, relay, gRPC, WebSocket, multi-SDK, backup automation |
| LOW | 14 | Multiple proposer, dynamic block time, light client, finality gadget, floating point, randomness, fuzzing, in-place upgrade, fork detection, sharding, L2, batch processing |

### Recommended Path Forward

1. **Pre-mainnet (HIGH only)**: Address all 11 HIGH priority items before any production mainnet launch
2. **Post-launch v1 (MEDIUM)**: Add MEDIUM items in subsequent releases
3. **Post-launch v2+ (LOW)**: Implement LOW items as system matures

### Notes

- Some HIGH items (formal verification, penetration testing) require external resources
- MEDIUM items can be addressed with internal engineering effort
- LOW items are nice-to-have based on user demand and system evolution

## Network Launch Checklist
# DSN Network Launch Checklist

## Pre-Launch Verification

### ✅ Critical Blockers (9A.1)
- [x] RPC returns real state root, pending txs, current height (no stubs)
- [x] Genesis init accepts --validator-addresses and --allocations flags
- [x] Node detects pending validators at startup
- [x] TUI send uses correct 28-byte payload and tracked nonce
- [x] HD wallet derivation integrated

### ✅ Sandbox Infrastructure (9A.2)
- [x] docker-compose.yml builds from source
- [x] Prometheus scrapes all validators
- [x] Grafana dashboards provisioned
- [x] Seed node config template present

### ✅ Validator Economics (9A.3)
- [x] Inflation tokens issued at epoch boundaries (10% treasury, 90% pool)
- [x] Rewards distributed proportionally by voting power
- [x] Transaction fees distributed per block (70% val, 20% burn, 10% treasury)
- [x] Jailed validators excluded from rewards
- [x] Reward balances queryable via dsn_getAccount

### ✅ Security (9A.4)
- [x] TLS/HTTPS support (DSN_TLS_CERT_FILE, DSN_TLS_KEY_FILE)
- [x] Optional API key auth (DSN_RPC_API_KEY)
- [x] Rate limiting enabled (100 req/s, burst 200)
- [x] Security headers applied (nosniff, XSS protection, HSTS)
- [x] Request body size limited (1 MiB)

### ✅ Launch Tooling (9A.5)
- [x] genesis_version field for forward compatibility
- [x] dsn genesis inspect command
- [x] Mainnet deploy guide (deploy/mainnet/README.md)
- [x] Mainnet genesis template (deploy/mainnet/genesis-template.json)
- [x] Example environment config (deploy/mainnet/.env.example)

## Launch Steps

### 1. Build
```bash
go build -o dsn ./cmd/dsn
```

### 2. Generate Genesis
```bash
./dsn genesis init --chain-id dsn-mainnet-1 --output genesis.json
./dsn genesis validate genesis.json
./dsn genesis inspect genesis.json
```

### 3. Configure Environment
```bash
cp deploy/mainnet/.env.example .env
# Edit .env with your settings
```

### 4. Start Node
```bash
./dsn node start --genesis genesis.json --datadir ~/.dsn-mainnet
```

### 5. Monitor
- RPC: http://localhost:8545
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000

## Post-Launch Verification
- [x] Node producing blocks
- [x] Validators active and signing
- [x] RPC responding to queries
- [x] Transaction submission works
- [x] Rewards accumulating over epochs

## Recovery Runbook
# Recovery Runbook

This runbook provides structured procedures for recovering from various incident levels affecting the DSN blockchain. Each incident classification has specific recovery steps, verification procedures, and escalation criteria.

## Incident Classification

Incidents are classified by severity and data loss potential:

| Level | Description | Impact | Recovery Time Target |
|-------|-------------|--------|---------------------|
| **L1**: Single Node Crash | Service interruption, no data corruption | Single validator offline | < 5 minutes |
| **L2**: Data Corruption | Database corruption on single node | Potential state inconsistency | < 30 minutes |
| **L3**: Network Partition | Multiple nodes affected, partition | Consensus delay, potential fork | < 1 hour |
| **L4**: Catastrophic Failure | Majority of nodes lost | Chain halt, potential rollback | < 4 hours |

## Recovery Procedures

### L1: Single Node Crash

Single node crashes are the most common incident type. The node process terminates unexpectedly but data remains intact.

**Symptoms:**
- `systemctl status dsn` shows failed or exited
- Node not responding to RPC requests
- Peer count drops to zero
- Missing blocks in monitoring

**Recovery Steps:**

1. **Identify cause** — Check logs to determine why the node crashed:
   ```bash
   journalctl -u dsn -n 200 --no-pager

   # Look for:
   # - Out of memory (OOM) kills
   # - Panic errors
   # - Port conflicts
   # - Disk space exhaustion
   ```

2. **Restart the node**:
   ```bash
   sudo systemctl restart dsn

   # Or for manual restart:
   dsn node start --config /etc/dsn/config.toml
   ```

3. **Verify reconnection**:
   ```bash
   # Watch logs for peer connections
   journalctl -u dsn -f | grep -E "peer|connected"

   # Check peer count
   curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'
   ```

4. **Check sync status**:
   ```bash
   # Verify block height matches network
   LOCAL=$(curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
     | jq -r '.result')
   echo "Local height: $LOCAL"

   # Compare with known network height
   curl -s https://dsn.example.com/eth/blockNumber | jq -r '.result'
   ```

5. **Confirm validator operation** (if validator):
   ```bash
   # Check validator is in active set
   curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"dsn_validators","params":[],"id":1}' \
     | jq '.validators[] | select(.pubkey == "<your-pubkey>")'

   # Watch for block proposals
   journalctl -u dsn -f | grep -i "propos"
   ```

**Root Causes and Prevention:**
- OOM: Increase memory limits, monitor memory metrics
- Panic: Review error logs, apply patches
- Port conflict: Ensure no other service uses ports 30333, 8545, 9090
- Disk full: Set up disk space monitoring and cleanup

### L2: Data Corruption

Data corruption occurs when the BoltDB database becomes inconsistent. This can happen due to crashes during writes, filesystem issues, or hardware failures.

**Symptoms:**
- Node fails to start with database errors
- State root mismatch with peers
- Panic during state load
- Database file size anomalies
- Checksum verification failures

**Recovery Steps:**

1. **Stop the node**:
   ```bash
   sudo systemctl stop dsn
   ```

2. **Run integrity check**:
   ```bash
   # Check BoltDB integrity (if bbolt tools available)
   bbolt verify /var/lib/dsn/chaindata/state.db

   # Check for database corruption
   dsn node check-db --data-dir /var/lib/dsn

   # Review system logs for filesystem errors
   dmesg | grep -i error | tail -20
   ```

3. **Restore from latest snapshot**:
   ```bash
   # List available snapshots
   ls -la /var/lib/dsn/snapshots/

   # Identify the latest complete snapshot
   # It should have: state.tar.gz, metadata.json

   # Stop node if running
   systemctl stop dsn

   # Backup corrupted database
   mv /var/lib/dsn/chaindata /var/lib/dsn/chaindata.corrupt.$(date +%Y%m%d)

   # Extract snapshot
   mkdir -p /var/lib/dsn/chaindata
   tar -xzf /var/lib/dsn/snapshots/latest/state.tar.gz -C /var/lib/dsn/chaindata/
   ```

4. **Replay WAL** (if applicable):
   ```bash
   # BoltDB automatically replays WAL on startup
   # If manual replay needed:
   dsn node replay-wal --data-dir /var/lib/dsn
   ```

5. **Start the node**:
   ```bash
   sudo systemctl start dsn
   ```

6. **Verify state root matches peers**:
   ```bash
   # Get local state root
   LOCAL_ROOT=$(curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", false],"id":1}' \
     | jq -r '.result.stateRoot')

   # Get peer state root (compare with multiple peers)
   PEER_ROOT=$(curl -s -X POST https://dsn-peer-1.example.com:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", false],"id":1}' \
     | jq -r '.result.stateRoot')

   echo "Local: $LOCAL_ROOT"
   echo "Peer: $PEER_ROOT"
   # These should match
   ```

**Root Causes and Prevention:**
- Crashes during writes: Ensure sync mode is enabled, use UPS
- Filesystem issues: Use reliable filesystems (ext4, xfs), check disk health
- Hardware failure: Replace failing drives, use RAID
- Enable regular snapshots for faster recovery

### L3: Network Partition

Network partitions occur when validator nodes cannot communicate due to network failures. The consensus protocol can handle partitions but may experience delays or temporary forks.

**Symptoms:**
- Multiple validators report low peer count
- Block height diverges between groups
- Consensus rounds taking longer than normal
- Alert: DSNPeerCountLow on multiple nodes

**Recovery Steps:**

1. **Identify partition scope**:
   ```bash
   # Check peer counts on all affected nodes
   for node in validator-1 validator-2 validator-3; do
     echo "=== $node ==="
     ssh $node "curl -s -X POST http://localhost:8545 \
       -H 'Content-Type: application/json' \
       -d '{\"jsonrpc\":\"2.0\",\"method\":\"net_peerCount\",\"params\":[],\"id\":1}'"
   done

   # Check which nodes can reach each other
   ssh validator-1 "nc -zv validator-2 30333"
   ssh validator-1 "nc -zv validator-3 30333"
   ```

2. **Restore network connectivity**:
   ```bash
   # Check network infrastructure
   # - Verify router/firewall configurations
   # - Check load balancer health
   # - Review network cables and switches

   # If using Kubernetes:
   kubectl get pods -n dsn -o wide
   kubectl describe svc -n dsn

   # Restart network-related pods if needed
   kubectl rollout restart deployment/network-proxy -n dsn
   ```

3. **Allow partition healing**:
   ```bash
   # DSN consensus automatically heals after partition
   # Nodes will re-sync and converge

   # Monitor for convergence
   sleep 60
   for node in validator-1 validator-2 validator-3; do
     ssh $node "curl -s -X POST http://localhost:8545 \
       -H 'Content-Type: application/json' \
       -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":1}' \
       | jq -r '.result'"
   done
   ```

4. **Verify convergence**:
   ```bash
   # All nodes should be at same height
   # Check for any missed blocks

   for node in validator-1 validator-2 validator-3; do
     echo "=== $node ==="
     ssh $node "curl -s http://localhost:9090/metrics | grep dsn_block_height"
   done
   ```

5. **Check for missed blocks**:
   ```bash
   # Review your validator's block proposals during partition
   # Missed blocks are normal during network issues
   # No action needed unless pattern persists
   journalctl -u dsn -g "missed\|skipped" --since "1 hour ago"
   ```

**Root Causes and Prevention:**
- Network infrastructure failures: Use redundant network paths
- Firewall misconfigurations: Regular audit of firewall rules
- DNS issues: Use static IPs or reliable DNS
- Kubernetes network policies: Review and test network policies

### L4: Catastrophic Failure

Catastrophic failure occurs when the majority of validator nodes are lost simultaneously. This requires full chain recovery from backups or genesis.

**Symptoms:**
- Network halt (no new blocks)
- Cannot reach quorum for consensus
- Majority of validators offline
- Data center or regional failure suspected

**Recovery Steps:**

1. **Restore from latest full backup**:
   ```bash
   # Check available backups
   ls -la /backup/dsn/

   # Select most recent full backup
   # Ensure backup is before the failure time

   # Stop all affected nodes
   kubectl delete statefulset dsn -n dsn-prod
   # Or for systemd:
   systemctl stop dsn

   # Restore data directory
   tar -xzf /backup/dsn/dsn-full-backup-20240115.tar.gz -C /var/lib/
   ```

2. **Bootstrap from genesis with trusted snapshot**:
   ```bash
   # Clear corrupted data
   rm -rf /var/lib/dsn/chaindata/*

   # Initialize from genesis
   dsn node init --genesis /var/lib/dsn/genesis.json

   # Download trusted snapshot
   curl -L https://trusted-snapshot.example.com/snapshot.tar.gz -o /tmp/snapshot.tar.gz
   tar -xzf /tmp/snapshot.tar.gz -C /var/lib/dsn/chaindata/

   # Verify snapshot integrity
   sha256sum -c /tmp/snapshot.sha256
   ```

3. **Rebuild validator set from genesis**:
   ```bash
   # Verify validator set via genesis
   cat /var/lib/dsn/genesis.json | jq '.validators'

   # If validators need to re-join:
   # - Each validator initializes fresh
   # - Re-stake if needed
   # - Wait for epoch transition
   ```

4. **Coordinate restart with other validators**:
   ```bash
   # Establish communication channel (emergency matrix/chat)
   # Coordinate block height for restart

   # Start validators in sequence:
   # 1. Start highest stake validator first
   systemctl start dsn

   # 2. Wait for it to sync
   sleep 60

   # 3. Start remaining validators
   for node in validator-2 validator-3; do
     ssh $node "systemctl start dsn"
     sleep 30
   done
   ```

5. **Verify chain integrity**:
   ```bash
   # Check all nodes agree on state root
   for node in validator-1 validator-2 validator-3; do
     ssh $node "curl -s -X POST http://localhost:8545 \
       -H 'Content-Type: application/json' \
       -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBlockByNumber\",\"params\":[\"latest\", false],\"id\":1}' \
       | jq -r '.result.stateRoot'"
   done

   # Verify consensus is producing blocks
   sleep 30
   for node in validator-1 validator-2 validator-3; do
     ssh $node "curl -s http://localhost:9090/metrics | grep dsn_block_height"
   done
   ```

**Root Causes and Prevention:**
- Data center failure: Deploy validators across multiple data centers
- Regional disaster: Geographic distribution of validators
- Software bug causing mass failure: Staged rollouts, canary testing
- Attack: Implement DDOS protection, monitoring, and incident response

## Post-Recovery Verification

After any recovery, verify system integrity:

1. **State root consistency check**:
   ```bash
   # Compare state roots across all nodes
   for node in "${NODES[@]}"; do
     ROOT=$(ssh $node "curl -s -X POST http://localhost:8545 \
       -H 'Content-Type: application/json' \
       -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBlockByNumber\",\"params\":[\"latest\", false],\"id\":1}' \
       | jq -r '.result.stateRoot'")
     echo "$node: $ROOT"
   done
   ```

2. **Validator set verification**:
   ```bash
   # Verify all expected validators are active
   curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"dsn_validators","params":[],"id":1}' \
     | jq '.validators | length'

   # Check validator status
   curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"dsn_staking_validatorInfo","params":["<validator-pubkey>"],"id":1}'
   ```

3. **Transaction replay verification**:
   ```bash
   # Verify recent transactions are confirmed
   # Check a few recent blocks contain expected transactions

   BLOCK=$(curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest", true],"id":1}')

   echo "$BLOCK" | jq '.result.transactions | length'
   echo "$BLOCK" | jq '.result.transactions[0]'
   ```

4. **Event log consistency check**:
   ```bash
   # Verify events/receipts are consistent
   # Check receipt root matches

   curl -s -X POST http://localhost:8545 \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"eth_getBlockReceipts","params":["latest"],"id":1}' \
     | jq '.result | length'
   ```

## Escalation

If recovery exceeds time targets or conditions worsen:

| Level | Contact | Escalation Time |
|-------|---------|-----------------|
| L1 | On-call engineer | > 10 minutes |
| L2 | Team lead | > 45 minutes |
| L3 | Engineering manager | > 30 minutes |
| L4 | CTO / Incident commander | Immediate |

## Practical Commands Reference

This section provides quick reference commands for common node operations.

### Node Binaries

| Binary | What | Use case |
|--------|------|----------|
| `dsn.exe` | Full node | Runs state, mempool, P2P, consensus, RPC server |
| `dsn-tui.exe` | Terminal UI client | Connects to any node via RPC for wallet ops + monitoring |

### Quick Start Commands

```bash
# Create a wallet + genesis node (first run)
# Creates validator_key.json with a funded account (100,000,000 DSN)
./dsn.exe -genesis

# Normal node start
./dsn.exe

# Launch the TUI (separate terminal)
./dsn-tui.exe --wallet validator_key.json
```

### TUI Usage (dsn-tui.exe)

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--rpc` | `http://localhost:8545` | DSN node RPC URL |
| `--wallet` | `""` | Path to wallet key file (optional for read-only) |

**Modes:**

| Mode | Command | What you can do |
|------|---------|-----------------|
| **Local wallet** | `./dsn-tui.exe --wallet validator_key.json` | Full access: check balance, send tx, view pending, monitor node |
| **Remote wallet** | `./dsn-tui.exe --rpc http://192.168.1.100:8545 --wallet mykey.json` | Same, but connecting to a remote node |
| **Read-only monitor** | `./dsn-tui.exe` | View node status and pending txs (no wallet operations) |

**Keybindings:**

| Key | Action |
|-----|--------|
| `Tab` / `→` | Next tab |
| `Shift+Tab` / `←` | Previous tab |
| `r` | Refresh current screen (Dashboard, Pending) |
| `Enter` | Confirm action (Balance query, Send) |
| `Esc` | Cancel confirmation |
| `q` / `Ctrl+C` | Quit |

### RPC Commands (via curl)

**Check Balance:**
```bash
curl -X POST http://localhost:8545 -H "Content-Type: application/json" \
  -d '{"method":"dsn_getBalance","params":["DSN1FvduKjC9B7Hj2FkHZmqZwY1mJHGxPN7N"]}'
```

**Get State Root:**
```bash
curl -X POST http://localhost:8545 -H "Content-Type: application/json" \
  -d '{"method":"dsn_getStateRoot","params":[]}'
```

**Get Account Details:**
```bash
curl -X POST http://localhost:8545 -H "Content-Type: application/json" \
  -d '{"method":"dsn_getAccount","params":["DSN1FvduKjC9B7Hj2FkHZmqZwY1mJHGxPN7N"]}'
```

**Get Pending Transactions:**
```bash
curl -X POST http://localhost:8545 -H "Content-Type: application/json" \
  -d '{"method":"dsn_getPendingTxs","params":[]}'
```

### Running a Multi-Node Network

**Two nodes, same machine:**
```powershell
# Terminal 1: First node
.\dsn.exe -rpc-port 8545 -p2p-port 26656

# Terminal 2: Second node (different ports)
.\dsn.exe -rpc-port 8546 -p2p-port 26657 -p2p-seed "127.0.0.1:26656"
```

### Common Troubleshooting

**Node won't start:**
- Check genesis file is valid JSON: `cat genesis.json | jq .`
- Verify wallet file exists and is decrypted
- Ensure P2P ports are not in use

**Transactions not being included:**
- Check mempool has transactions: `dsn_getPendingTxs`
- Verify your transaction has sufficient fee
- Check node is producing blocks (look at logs)

**Nodes not connecting:**
- Verify P2P seed addresses are correct
- Check firewall allows TCP on P2P ports

---

## Related Documentation

- [OPERATIONS.md](./OPERATIONS.md) — Deployment and monitoring
- [PERSISTENCE.md](./PERSISTENCE.md) — Data storage internals
- [CONSENSUS.md](./CONSENSUS.md) — Consensus protocol details

## Troubleshooting
# DSN v0.1.0-sandbox — Troubleshooting Guide

> Real issues based on actual code behavior. See [CLI_REFERENCE.md](./CLI_REFERENCE.md) for command docs.

## Genesis Mismatch

- **Symptom**: `genesis hash mismatch` fatal error on node start.
- **Cause**: Genesis file changed after initial node creation. On first start, genesis hash is stored in BoltDB. Every subsequent start compares genesis hash against stored value.
- **Fix**: Restore original genesis file, or delete data directory and re-sync from genesis.

## Validator Key Errors

**"validator key file not found"**
- File path doesn't exist. Verify `--validator-key` path. Default is `~/.dsn/validator.key`.

**"invalid validator key: public key does not match private key"**
- Key file is corrupted. Regenerate with `dsn validator init`.

**"validator not in active set"**
- Validator key loaded but address not found in active validator set. May be pending registration or not registered.

**"address mismatch"` (`validator.go` check)
- Public key doesn't derive the expected address. File corruption.

## Port Conflicts

- **Symptom**: `bind: address already in use` on node/devnet start.
- **Default ports**: RPC=8545, P2P=26656 (or 0=disabled), Metrics=9464, Explorer=8080.
- **Fix**: Use different ports via flags (`--rpc-port`, `--p2p-port`, `--metrics-port`), kill existing process, or check for zombie processes.

## Peer Connection Failures

- **P2P disabled**: `P2PPort=0` means no networking. Node runs in isolation.
- **Can't connect**: Verify bootstrap peer format. Must be `/ip4/1.2.3.4/tcp/26656/p2p/PeerID`.
- **Firewall**: P2P port must be reachable.
- **Devnet note**: `dsn devnet` starts a single in-memory node with P2P disabled (no `--p2p-port` flag).

## Replay Mismatches

- **Symptom**: "state root doesn't match checkpoint" on restart.
- **Cause**: State corruption or manual data directory tampering.
- **Recovery**: Node attempts automatic recovery —
  1. Checks for snapshot at checkpoint height → restores from snapshot if available.
  2. If tip height > checkpoint height → replays blocks from checkpoint+1 to tip.
  3. If no snapshot and no replay path → fatal error.
- **Manual fix**: Restore data directory from backup, or start fresh with new genesis.

## State Corruption

- **Symptom**: `loadAccounts: corrupt entry` (from `state/persistent.go`).
- **Cause**: BoltDB data corruption or invalid account entries.
- **Fix**: Restore from backup or start fresh. The node returns errors rather than silently skipping.

## Recovery Failures

- **Snapshot restore fails**: Verify snapshot file exists and hash matches checkpoint.
- **Block replay fails**: May indicate consensus logic bug or state corruption at specific height.
- **No snapshots available**: If `Snapshot.Enable=false` or `Snapshot.Interval` too high, recovery may have no fallback.

## RPC Unavailable

- **Symptom**: Connection refused on `:8545`.
- **Node not running**: Start devnet or node first.
- **Wrong port**: Verify `--rpc-port` matches. Devnet defaults to 8545.
- **Firewall**: Check local firewall rules.
- **Legacy note**: `dsn` without subcommand starts RPC but in legacy mode only.

## RPC Method Errors

- **"method not found"**: Method doesn't exist. See [CLI_REFERENCE.md](./CLI_REFERENCE.md) for valid methods.
- **"requires event indexer (Phase 6)"**: `dsn_getEvents` not implemented.
- **"requires active staking state synchronization"**: `dsn_getValidators` not implemented.
- **"requires block indexer"**: Block hash lookup requires `--indexer` enabled (devnet) or indexer running.

## Empty Mempool

- **Devnet Pending tab**: `dsn_tui` Pending tab always shows "No pending transactions" because `dsn_getPendingTxs` RPC stub returns empty array.
- **No txs flowing**: Verify RPC connection, wallet has non-zero balance, faucet was called.
- **Mempool config**: `MempoolMaxSize=10000` default. If exceeded, new txs rejected.

## Sync Stalls

- **"waiting for network sync"**: Fresh genesis with no peers. If solo devnet, use `dsn devnet` instead.
- **Fast sync not progressing**: Network fast sync is NOT implemented. Use local snapshot restore only.
- **No block production**: Verify validator is in active set. Single devnet produces blocks every 3s automatically.

## Explorer Issues

- **Symptom**: Frontend shows no data.
- **Cause**: Explorer REST API at `/api/v1` works, but `index.html` calls RPC methods that DON'T EXIST (`dsn_blockNumber`, `dsn_getBlockByNumber`, etc.). The frontend is broken for data display.
- **Workaround**: Use REST API directly via curl:
  - `curl http://localhost:8080/api/v1/blocks`
  - `curl http://localhost:8080/api/v1/accounts/0x...`
  - `curl http://localhost:8080/api/v1/supply`
  - `curl http://localhost:8080/health`

## Contract Execution Failures

- **Devnet**: VM is nil on devnet. Contracts CANNOT execute on `dsn devnet`. Use `dsn node start` with genesis + validator key.
- **"contract not found"**: Contract address may be wrong, or contract wasn't deployed.
- **"execution reverted"**: Contract runtime error (out of gas, invalid parameters, host function error).
- **"out of gas"**: Increase `--gas-limit`. Default deploy is 1,000,000.
- **Deploy fails silently**: Nonce defaults to 0 on RPC error, causing deployment to same address. Check RPC connectivity.

## Faucet Issues

- **"rate limit exceeded"**: Devnet faucet has 60-second cooldown per IP. Wait or use different IP.
- **"invalid address"**: Address must be valid 20-byte hex with `0x` prefix.
- **No response**: Verify devnet is running. Faucet is at `POST /api/v1/faucet` on explorer port (default: 8080).

## TUI Issues

- **Blank screen on Windows**: Alt screen may not render in some terminals. Try Windows Terminal or WSL.
- **Data races**: Known issue with goroutine state updates (non-critical for most operations).
- **Send shows wrong nonce**: Hardcoded nonce=1 in TUI. Use CLI for correct nonce.

## Build Issues

- **Go version**: Requires Go 1.21+. Check with `go version`.
- **Build fails**: `go build ./cmd/dsn` works. `make build` requires make (use `choco install make` on Windows, or WSL).
- **CGO errors**: DSN uses pure Go (BoltDB, wazero). No CGO required.

## General Debugging

```bash
# Node health
curl http://localhost:9464/health
curl http://localhost:8545 -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_getSupply","params":{},"id":1}'

# Validate genesis
dsn genesis validate --file genesis.json

# Set log level
dsn node start --log-level debug

# Reset devnet
dsn devnet --reset
```

## Validator Economics & Rewards
# DSN Validator Rewards

## Overview
Validators earn rewards through two mechanisms:
1. **Block rewards** — Inflation tokens minted at each epoch boundary
2. **Transaction fees** — Fees from transactions included in proposed blocks

## Reward Distribution Flow

### Block Rewards (Epoch Boundary)
At each epoch boundary, `consensus/system.go:BeginBlock()` executes:
1. `staking.IssueEpochTokens()` — Mints new tokens:
   - 10% goes to the treasury
   - 90% goes to the validator reward pool
2. `staking.DistributeValidatorRewards()` — Distributes pool proportionally:
   - Based on each validator's relative voting power
   - Uses a snapshot of validator set from the PRIOR epoch
   - Remainder (from integer division) goes to highest-powered validator
3. Rewards are credited to each validator's reward address (or operator address if not set)

### Fee Distribution
At each block, `FinalizeBlock()` distributes collected transaction fees to validators via `staking.DistributeRewards()`.
- 70% to validators (proportional to voting power)
- 20% burned
- 10% to treasury

### Example
From `staking/rewards_test.go` - TestDistributeValidatorRewards_Basic:

```go
// Three validators with stakes: 100k, 200k, 300k
// Validator pool: 1000 tokens

// Total power = 100k + 200k + 300k = 600k
// Distribution:
// - Validator 3 (300k): 1000 * 300000 / 600000 = 500
// - Validator 2 (200k): 1000 * 200000 / 600000 = 333
// - Validator 1 (100k): 1000 * 100000 / 600000 = 166
// Remainder = 1000 - 500 - 333 - 166 = 1 → goes to validator 3 (highest power)
// Final: v3 = 501, v2 = 333, v1 = 166
```

## Querying Rewards
- Account balance: `dsn_getAccount` — use `nonce` field for account state
- Validator info: `dsn_getValidator` — includes bonded stake
- Supply metrics: `dsn_getSupply` — total, circulating, staked
- Validator set: `dsn_getValidators` — all active validators with stakes

## Reward Formulas

### Validator Reward Calculation
```
validatorReward = poolAmount * validatorPower / totalPower
remainder = poolAmount - sum(allValidatorRewards)
// remainder goes to highest-powered validator
```

### Inflation
```
epochTokens = blocksPerEpoch * blockTime * inflationRate * totalSupply
treasuryShare = epochTokens * 10%
validatorPool = epochTokens * 90%
```

### Fee Distribution
```
validatorShare = totalFees * 70%
burnShare = totalFees * 20%
treasuryShare = totalFees * 10%
```

## Testing
Run reward verification:
```bash
go test ./staking/ -run TestDistribute -v
go test ./staking/ -run TestReward -v
go test ./consensus/ -run TestSlash -v
```

## Current Status
✅ Inflation token issuance at epoch boundaries
✅ Proportional reward distribution
✅ Treasury allocation (10%)
✅ Fee distribution per block
✅ Reward address support (operators can set separate reward address)
✅ Jailed validators excluded from rewards
✅ All economic tests passing (100+ tests)

## Known Limitations

# DSN v0.1.0-sandbox — Definitive Known Limitations

> This document is the authoritative registry of every known stub, placeholder,
> incomplete feature, and bug in the DSN v0.1.0-sandbox release. Every item is
> verifiable against the source code.

## Severity Classification

| Severity | Meaning |
|----------|---------|
| **CRITICAL** | Breaks a core user-facing or protocol-level feature |
| **WARNING** | Impacts usability or correctness, workaround may exist |
| **INFO** | Nice-to-have; does not block sandbox use cases |

---

## 1. CLI Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 1 | `genesis init --validator-addresses` flag is declared/parsed but **never referenced** in the execution flow (`runGenesisInit` ignores it entirely) | `cmd/dsn/genesis_cmd.go:95` | WARNING |
| 2 | `genesis init --allocations` flag is declared/parsed but **never referenced** in the execution flow | `cmd/dsn/genesis_cmd.go:96` | WARNING |
| 3 | `contract deploy --abi` flag is declared/parsed but **never used** in `runContractDeploy` | `cmd/dsn/contract_cmd.go:109`, `:179` | INFO |
| 4 | `validator register` submits to RPC method `dsn_submitTransaction` — server expects `dsn_sendTransaction`. Registration **will always fail** end-to-end. | `cmd/dsn/validator_cmd.go:407` vs `rpc/server.go:128` | CRITICAL |
| 5 | `validator status` queries RPC method `dsn_getValidator` — no such method exists on server (only `dsn_getValidators` exists). Status query **will always fail**. | `cmd/dsn/validator_cmd.go:424` vs `rpc/server.go:150` | CRITICAL |
| 6 | Template genesis mode (`dsn genesis init` without `--devnet`) generates all-zero placeholder validator keys (`0x0000...`). Not usable for real networks without manual editing. | `cmd/dsn/genesis_cmd.go:260-268` | WARNING |
| 7 | `ValidateGenesis()` errors are printed as non-fatal warnings in `genesis init`. Validation errors should be fatal — the genesis file is silently created even when invalid. | `cmd/dsn/genesis_cmd.go:153-154` | WARNING |

---

## 2. RPC Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 8 | `dsn_getEvents` always returns error `"event querying not available: requires event indexer (Phase 6)"` | `rpc/service/node_service.go:372` | WARNING |
| 9 | `dsn_getValidators` always returns an error (`"StateDB interface doesn't expose validator queries yet"`). Validator queries require active staking state synchronization. | `rpc/service/node_service.go:377-378` | CRITICAL |
| 10 | `dsn_getStateRoot` returns hardcoded `0x0000000000000000000000000000000000000000000000000000000000000000`. Legacy method, never updated to return real state root. | `rpc/server.go:369-372` | WARNING |
| 11 | `dsn_getPendingTxs` always returns empty array `[]`. Legacy stub method. | `rpc/server.go:375-377` | WARNING |
| 12 | Block hash lookup in `dsn_getBlock` returns error `"block lookup by hash not available: requires block indexer"` | `rpc/service/node_service.go:47` | WARNING |
| 13 | `dsn_getTransaction` searches mempool **only** — no historical transaction lookup available | `rpc/service/node_service.go:466-482` | WARNING |
| 14 | `dsn_getTransactionReceipt` only returns `"pending"` if tx is found in mempool; otherwise returns `ErrNotFound`. No historical receipt lookup. | `rpc/service/node_service.go:472-484` | WARNING |
| 15 | `GetCurrentHeight()` in node service always returns `0` — commented as `"Would be exposed from node in full implementation"` | `rpc/service/node_service.go:407-409` | WARNING |

### Missing RPC Methods

The following methods are called by the explorer frontend and/or TypeScript SDK but **do not exist** on the server. They will return `method not found (-32601)`:

| Method | Called From | Severity |
|--------|-------------|----------|
| `dsn_blockNumber` | `explorer/frontend/index.html:178`, `sdks/dsn-js/src/client.ts:85` | CRITICAL |
| `dsn_getBlockByNumber` | `explorer/frontend/index.html:187,213`, `sdks/dsn-js/src/client.ts:90` | CRITICAL |
| `dsn_getTransactionByHash` | `explorer/frontend/index.html:233`, `sdks/dsn-js/src/client.ts:97` | CRITICAL |
| `dsn_getNonce` | `explorer/frontend/index.html:258`, `sdks/dsn-js/src/client.ts:129` | CRITICAL |
| `dsn_sendRawTransaction` | `sdks/dsn-js/src/client.ts:107` | CRITICAL |
| `dsn_deployContract` | `sdks/dsn-js/src/client.ts:141` | CRITICAL |

---

## 3. VM Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 16 | `transfer_token` host function always returns success (0). Does **NOT** actually transfer tokens. Marked as TODO: `"full token transfer needs account state integration"`. | `vm/host.go:242-273` | CRITICAL |
| 17 | No per-WASM-instruction gas metering — execution relies on context timeout (30s default) rather than counting WASM opcodes | `vm/vm.go:19` | WARNING |
| 18 | Devnet block producer passes `nil` for VM. Contract execution **never occurs** on devnet — standard transactions only. | `cmd/dsn/devnet.go:157` | CRITICAL |

### Constructor Arguments

Constructor execution ignores any `ConstructorArgs` field — the WASM constructor is called without arguments. (No `ConstructorArgs` processing exists in the VM execution path.)

---

## 4. Explorer Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 19 | REST API returns `fmt.Sprintf("0x%032x", height)` as block hash — NOT the real block hash. True for both block list and block detail endpoints. | `explorer/handlers.go:73,165` | WARNING |
| 20 | Validators REST endpoint delegates to `dsn_getValidators` RPC which is a **stub** that always errors | `explorer/router.go` (delegates via service) | WARNING |
| 21 | Frontend `index.html` calls 6 RPC methods that **do not exist** on server (see table above). The entire explorer frontend data flow is broken. | `explorer/frontend/index.html` | CRITICAL |
| 22 | `handleAccount` doesn't return recent transactions — commented as `"Could implement GetTransactionsByAddress in indexer"` | `explorer/handlers.go:255-260` | INFO |

---

## 5. TypeScript SDK Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 23 | TS SDK calls RPC methods that **do not exist** on server: `dsn_blockNumber`, `dsn_getBlockByNumber`, `dsn_getTransactionByHash`, `dsn_getNonce`, `dsn_sendRawTransaction`, `dsn_deployContract`. **The SDK is broken for all data queries.** | `sdks/dsn-js/src/client.ts:85,90,97,107,129,141` | CRITICAL |

---

## 6. Node Stubs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 24 | `FastSync()` returns error `"fast sync from network not yet implemented; use FastSyncFromCheckpoint with a local snapshot file"` | `node/fastsync.go:35,75` | WARNING |
| 25 | `SyncFromNetwork()` only implements the initial snapshot query/response handshake. Chunk download, reassembly, and restoration are **not implemented** (marked TODO). Returns error `"P2P snapshot download not fully implemented"`. | `node/fastsync.go:191-226` | WARNING |
| 26 | Consensus engine init (Phase 9 of startup) is effectively a **no-op** — just copies params from genesis doc into node fields, no consensus engine is initialized. | `node/startup.go:456-471` | INFO |
| 27 | `isPending` in validator registry startup is **hardcoded to `false`** with TODO: `"TODO: check pending validator set"`. Pending validators are never detected. | `node/startup.go:400,410` | WARNING |
| 28 | `SyncModeReplay` (value 2) is declared but **never actively set** by any code path | `node/fastsync.go:19` | INFO |
| 29 | `evidence` in consensus block production loop is always `nil` — marked `"TODO: Get evidence from evidence pool"` | `node/node.go:467-468` | INFO |
| 30 | Metrics endpoint only exposes `dsn_block_height` gauge. No P2P, mempool, or consensus metrics on node startup. | `node/startup.go:717-719` | INFO |

---

## 7. Config Bugs

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 31 | `ErrCodeInvalidMaxPeers` error code reused for mempool size validation (`cfg.Chain.MempoolMaxSize`) and max tx/block validation (`cfg.Chain.MaxTxPerBlock`) — wrong semantic error code | `config/validation.go:26,66,73` | INFO |
| 32 | Snapshot validation comment says `"this is a warning but not an error — snapshots will just be disabled"` but **no actual warning is emitted** | `config/validation.go:60-61` | INFO |
| 33 | Genesis validation checks `ConsensusParams.MaxTxPerBlock < 0`, `ConsensusParams.MaxBytesPerBlock < 0`, etc. — all are `uint64` fields so these comparisons are **compile-time no-ops** (unsigned integers are never negative) | `genesis/validation.go:27-47` | INFO |
| 34 | `sortMapKeys()` and `canonicalizeMap()` in genesis/json.go are **dead code** — defined but never called from any exported function | `genesis/json.go:27,45` | INFO |

---

## 8. Validator / Staking Stubs

| # | Issue | Severity |
|---|-------|----------|
| 35 | No delegation — staking to someone else's validator is not implemented | INFO |
| 36 | No on-chain governance — protocol parameter changes require manual coordinated updates | INFO |
| 37 | No slashing automation — slashing requires manual evidence submission | INFO |
| 38 | No MEV protection | INFO |
| 39 | Pending validator tracking: `isPending` hardcoded `false` — pending validators are not tracked during node startup | WARNING |

---

## 9. TUI Limitations

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 40 | Dashboard always shows `0x0000...` for state root (relies on stub RPC `dsn_getStateRoot`) | `cmd/dsn-tui/screens/dashboard.go:72` | WARNING |
| 41 | Pending tab always shows empty (relies on stub RPC `dsn_getPendingTxs`) | `cmd/dsn-tui/screens/pending.go:29` | WARNING |
| 42 | Send tab uses **hardcoded nonce=1** regardless of actual account nonce | `cmd/dsn-tui/screens/send.go:122` | WARNING |
| 43 | Send tab payload is just recipient address bytes (not address+amount as the protocol expects) | `cmd/dsn-tui/screens/send.go:123` | WARNING |
| 44 | No auto-refresh — must press `r` manually to update data | `cmd/dsn-tui/model.go:94` | INFO |
| 45 | Addresses truncated to 16 characters in dashboard display | `cmd/dsn-tui/screens/dashboard.go:72,75,92` | INFO |
| 46 | Goroutine state updates (`m.stateRoot = root`, `m.balance = acc.Balance`, etc.) occur outside Bubbletea's update loop — **potential data races** on model fields | `cmd/dsn-tui/screens/dashboard.go:29-38,49-56`, `screens/wallet.go:26`, `screens/pending.go:28` | WARNING |

---

## 10. Networking Stubs

| # | Issue | Severity |
|---|-------|----------|
| 47 | No NAT traversal — nodes behind NAT may have connectivity issues | INFO |
| 48 | No peer scoring — peer reputation/scoring not implemented | INFO |
| 49 | No DHT — distributed hash table for peer discovery not available | INFO |
| 50 | No relay protocol — relay connections for inaccessible nodes not supported | INFO |

---

## 11. Governance / Economics

| # | Issue | Severity |
|---|-------|----------|
| 51 | No on-chain governance | INFO |
| 52 | No delegation | INFO |
| 53 | No slashing automation | INFO |
| 54 | No MEV protection | INFO |

---

## 12. Operational

| # | Issue | Severity |
|---|-------|----------|
| 55 | No automatic failover | INFO |
| 56 | No multi-DC validation | INFO |
| 57 | No hot standby validators | INFO |
| 58 | No SLA guarantees | INFO |
| 59 | No TLS/HTTPS in RPC server | WARNING |
| 60 | No RPC authentication/authorization | WARNING |
| 61 | No automatic backup tooling | INFO |
| 62 | Some Prometheus alerting rules may reference non-existent metrics | INFO |

### Sandbox-Specific Limitations
# DSN Sandbox — Known Limitations

## LEGAL DISCLAIMER

**THIS IS SANDBOX SOFTWARE.**
**IT IS NOT A PRODUCTION MAINNET.**
**IT HAS NO ECONOMIC GUARANTEES.**
**IT HOLDS NO REAL ASSETS.**

## Cryptographic & Security Limitations

- **Ed25519 only**: Only Ed25519 signatures are supported. Multi-signature and threshold schemes are not implemented.
- **No hardware security module (HSM) support**: Validator keys are stored on disk without HSM integration.
- **No formal verification**: The consensus protocol has NOT been formally verified.
- **Penetration testing pending**: No third-party security audit has been conducted.

## Consensus Limitations

- **Single proposer**: Block production uses a single proposer per round. No parallel block production.
- **Fixed block time**: Block time is fixed at 1 second. Dynamic block time is not implemented.
- **No light client support**: Light client verification is not available in this release.
- **No fast finality gadgets**: Finality is linear with block confirmation count.

## State Machine Limitations

- **Fixed state pruning**: Historical state is not pruned. Full archival nodes required.
- **No state rent**: No economic mechanism for state bloat prevention.
- **Single SMT implementation**: Only one SMT implementation. No alternative backends.
- **No parallel execution**: Transactions execute sequentially within a block.

## WASM VM Limitations

- **No floating point**: WASM floating point operations are disabled for determinism.
- **No host functions for randomness**: Deterministic random number generation is not available.
- **Single runtime**: wazero is the only supported WASM runtime.
- **Limited standard library**: WASM contracts have limited access to host functions.

## Networking Limitations

- **No NAT traversal**: Nodes behind NAT may have connectivity issues.
- **No peer scoring**: Peer reputation and scoring is not implemented.
- **DHT not implemented**: Distributed hash table for peer discovery is not available.
- **No relay protocol**: Relay connections for inaccessible nodes are not supported.

## Economic Limitations

- **No on-chain governance**: Protocol parameter changes require coordinated manual updates.
- **No delegation**: Staking delegation is not implemented.
- **No slashing automation**: Slashing requires manual evidence submission.
- **No MEV protection**: Maximal extractable value mitigation is not implemented.

## Operational Limitations

- **No automatic failover**: Validator failover requires manual intervention.
- **No multi-DC deployment**: Cross-datacenter deployment is not validated.
- **No backup validator**: Hot standby validators are not supported.
- **Limited monitoring**: Alerting rules are a starting point, not comprehensive.
- **No SLA guarantees**: This is sandbox software with no uptime guarantees.

## Performance Limitations

- **No sharding**: All state is on a single shard.
- **No layer 2**: No L2 scaling solutions are implemented.
- **No hardware acceleration**: No GPU or ASIC acceleration.
- **No batch processing**: No batching optimization for high-throughput scenarios.

## Storage Limitations

- **BoltDB only**: BoltDB is the only supported database backend.
- **No pruning**: Historical data grows unbounded.
- **No cold storage**: All data is stored on hot storage.
- **No backup automation**: Backup procedures are manual.

## API Limitations

- **REST-only API**: gRPC interface is not available.
- **No GraphQL**: GraphQL query interface not implemented.
- **No WebSocket pub/sub**: Limited subscription capabilities.
- **No SDK for non-Go languages**: Only Go SDK is available.

## Testing Limitations

- **24h soak only**: Longer duration soaks (7+ days) have not been validated.
- **Single region testing**: Cross-region and geo-distributed testing pending.
- **No fuzzing**: Fuzz testing of consensus and state machine pending.
- **Limited adversarial testing**: Byzantine scenarios are a subset of possible attacks.

## Upgrade Process

- **No in-place upgrade**: Protocol upgrades require coordinated node restarts.
- **No fork detection**: Fork detection relies on manual monitoring.
- **No version negotiation**: No protocol version handshake between nodes.

## Final Note

This sandbox release is intended for:
- Testing DSN's deterministic execution guarantees
- Learning the DSN protocol and APIs
- Building and testing WASM smart contracts
- Understanding validator operations
- Providing feedback on the developer experience

It is NOT intended for:
- Real asset transfers
- Production workloads
- Economic guarantees
- Regulatory compliance

We welcome your feedback at https://github.com/dsn/dsn/issues