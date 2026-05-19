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