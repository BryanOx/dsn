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