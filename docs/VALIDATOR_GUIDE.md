# Validator Guide

This guide covers validator setup, operation, and maintenance for the DSN blockchain. Validators are critical infrastructure participants who produce blocks and secure the network through the BFT consensus protocol.

## Prerequisites

### Hardware Requirements

| Tier | CPU | RAM | Storage | Network |
|------|-----|-----|---------|---------|
| Minimum | 4 cores | 8 GB | 100 GB SSD | 100 Mbps |
| Recommended | 8+ cores | 16+ GB | 500 GB SSD | 1 Gbps |

The minimum requirements support basic validation on a testnet. Production validators should use the recommended configuration to handle higher transaction throughput and ensure reliable block production.

Storage requirements scale with chain history. Plan for at least 100 GB initially with growth of approximately 10-20 GB per month depending on transaction volume.

### Software Requirements

- **Go**: Version 1.22 or later
- **Operating System**: Linux (Ubuntu 20.04+, Debian 11+), macOS 12+, or Windows Server 2019+
- **Dependencies**:
  - Git for source retrieval
  - Make for build automation
  - Systemd (Linux) for service management
  - Kubernetes (optional) for containerized deployment

### Network Requirements

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| 30333 | TCP | Inbound/Outbound | P2P block and vote gossip |
| 8545 | HTTP | Inbound | JSON-RPC API |
| 8546 | WebSocket | Inbound | WebSocket API |
| 9090 | HTTP | Inbound | Prometheus metrics |

Ensure these ports are accessible through any firewall. For validator nodes, inbound access on all ports is required for peer connections and RPC access.

Minimum bandwidth of 100 Mbps is sufficient for testnet participation. Mainnet validators should have dedicated 1 Gbps connections with low latency to other validator nodes.

## Setup

### 1. Install DSN Binary

Clone and build from source:

```bash
git clone https://github.com/dsn/dsn.git
cd dsn
make build

# Verify installation
./build/dsn version
```

Alternatively, download pre-built binaries from releases:

```bash
curl -L https://github.com/dsn/dsn/releases/latest/download/dsn-linux-amd64.tar.gz -o dsn.tar.gz
tar -xzf dsn.tar.gz
sudo mv dsn /usr/local/bin/
```

### 2. Generate Validator Key

Create a new validator keypair:

```bash
dsn validator init --key-dir /var/lib/dsn/keystore

# Output includes:
# - Public key (validator ID)
# - Node ID for P2P networking
# - Key file path
```

Store the private key securely. The key file is encrypted with a password. Backup this file—loss means losing validator status.

### 3. Configure Node

Create the configuration file:

```toml
# /etc/dsn/config.toml
[chain]
id = "dsn-1"
genesis = "/var/lib/dsn/genesis.json"

[p2p]
port = 30333
seeds = "seed1@dsn-seed-1.example.com:30333,seed2@dsn-seed-2.example.com:30333"
persistent_peers = "validator1@dsn-validator-1.example.com:30333"
max_peers = 50

[rpc]
port = 8545
cors_allowed_origins = ["*"]
api = ["eth", "net", "web3", "dsn"]

[metrics]
enabled = true
port = 9090

[consensus]
timeout_propose = "3s"
timeout_precommit = "1s"

[validator]
enabled = true
key_path = "/var/lib/dsn/keystore/validator.key"
```

Download the genesis file for your network:

```bash
curl -L https://dsn.example.com/genesis.json -o /var/lib/dsn/genesis.json
```

### 4. Initialize Data Directory

Initialize the node:

```bash
dsn node init \
  --config /etc/dsn/config.toml \
  --data-dir /var/lib/dsn

# Creates:
# /var/lib/dsn/chaindata/
# /var/lib/dsn/keystore/
# /var/lib/dsn/snapshots/
```

### 5. Start Node

Systemd service (recommended):

```bash
sudo cp deploy/systemd/dsn.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dsn
sudo systemctl start dsn
```

Or run directly:

```bash
dsn node start --config /etc/dsn/config.toml --data-dir /var/lib/dsn
```

### 6. Verify Sync Status

Check sync status via RPC:

```bash
# Check if syncing
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}'

# If syncing returns false, node is caught up
# If syncing returns object, node is behind
```

Check block height:

```bash
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

Compare with network height from peers:

```bash
# Query a public RPC endpoint
curl -s https://dsn.example.com/eth/blockNumber
```

Verify peer connectivity:

```bash
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'
```

A healthy validator should have at least 5-10 peers and be fully synced.

## Staking

### How to Stake Tokens

Stake tokens to become a validator:

```bash
# Create stake transaction
dsn staking stake \
  --amount 1000000 \
  --validator-pubkey <your-pubkey> \
  --from <your-address> \
  --keyring-backend file \
  --chain-id dsn-1 \
  --node http://localhost:8545

# Monitor stake status
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_staking_validatorInfo","params":["<your-pubkey"]}'
```

The minimum stake required varies by network. Check the network parameters for the current minimum.

### How to Delegate

Delegate tokens to an existing validator:

```bash
dsn staking delegate \
  --validator <validator-pubkey> \
  --amount 10000 \
  --from <your-address> \
  --keyring-backend file \
  --chain-id dsn-1
```

Delegators earn a portion of validator rewards minus commission. You can delegate to multiple validators to diversify risk.

### Reward Calculation

Rewards are calculated per epoch:

```
epoch_reward = base_reward * (total_staked / minimum_staked)
validator_share = epoch_reward * (validator_stake / total_staked)
delegator_share = validator_share * (1 - commission_rate)
```

Check your current rewards:

```bash
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_staking_rewards","params":["<your-address>"]}'
```

### Withdrawal Process

Unbond and withdraw staked tokens:

```bash
# Initiate unbonding
dsn staking unbond \
  --validator <validator-pubkey> \
  --amount <amount> \
  --from <your-address> \
  --keyring-backend file

# Unbonding has a 14-day cooldown period
# After cooldown, withdraw:
dsn staking withdraw \
  --from <your-address> \
  --keyring-backend file
```

During the unbonding period, the staked tokens are locked and do not earn rewards. The cooldown period allows the network to adjust the validator set.

## Maintenance

### Monitoring Health

Check node health via metrics:

```bash
# Prometheus metrics endpoint
curl http://localhost:9090/metrics | grep dsn_

# Key metrics to watch
dsn_block_height              # Current height
dsn_consensus_round           # Current round
dsn_p2p_peer_count           # Connected peers
dsn_consensus_votes           # Votes cast/received
```

System health checks:

```bash
# Check systemd service
systemctl status dsn

# Check disk space
df -h /var/lib/dsn

# Check memory usage
free -h

# Check CPU load
uptime
```

### Log Management

View logs:

```bash
# Systemd journal
journalctl -u dsn -f

# Filter by level
journalctl -u dsn -p err

# Last hour of logs
journalctl -u dsn --since "1 hour ago"
```

Set log level via config:

```toml
[log]
level = "info"  # debug, info, warn, error
format = "json"  # json or text
```

Consider configuring log rotation to prevent disk exhaustion:

```bash
# /etc/logrotate.d/dsn
/var/log/dsn/*.log {
    daily
    rotate 7
    compress
    delaycompress
    notifempty
    create 0640 dsn dsn
    postrotate
        systemctl reload dsn > /dev/null 2>&1 || true
    endscript
}
```

### Disk Space Management

Monitor disk usage:

```bash
# Check data directory size
du -sh /var/lib/dsn

# Check database sizes
ls -lh /var/lib/dsn/chaindata/

# Check available space
df -h
```

Clean up old data:

```bash
# Remove old snapshots (keep recent)
ls -t /var/lib/dsn/snapshots/ | tail -n +6 | xargs -r rm

# Prune old blocks (if enabled)
dsn node prune --keep-recent 10000
```

Set up monitoring for disk usage alerts at 80% capacity.

### Backup Procedures

Regular backups of critical data:

```bash
# Backup validator key
cp -r /var/lib/dsn/keystore /backup/keystore-$(date +%Y%m%d)

# Backup configuration
cp /etc/dsn/config.toml /backup/config-$(date +%Y%m%d).toml

# Full data backup (stop node first)
systemctl stop dsn
tar -czf /backup/dsn-data-$(date +%Y%m%d).tar.gz -C /var/lib dsn
systemctl start dsn
```

Test restoration procedures periodically:

```bash
# Verify backup integrity
tar -tzf /backup/dsn-data-*.tar.gz > /dev/null

# Test restore to temporary location
mkdir /tmp/dsn-restore
tar -xzf /backup/dsn-data-20240101.tar.gz -C /tmp/dsn-restore
```

### Software Updates

Apply security and feature updates:

```bash
# Check for updates
dsn version
git fetch origin
git log HEAD..origin/main

# Update binary
git pull origin main
make build
sudo systemctl stop dsn
sudo cp build/dsn /usr/local/bin/
sudo systemctl start dsn

# Verify version
dsn version
```

For cluster deployments, use rolling updates to maintain consensus. See [OPERATIONS.md](./OPERATIONS.md) for detailed upgrade procedures.

## Troubleshooting

### Common Issues and Fixes

| Issue | Cause | Solution |
|-------|-------|----------|
| Node won't start | Port conflict or corrupted data | Check logs, verify ports, restore from backup |
| Not syncing | Network connectivity or peer issues | Check firewall, add more seeds/peers |
| Not producing blocks | Not in active validator set | Check stake amount, wait for next epoch |
| High memory usage | Memory leak or large state | Restart node, consider state pruning |
| Peer count low | Network or firewall issues | Verify port 30333 is open, check peers |

### How to Check Sync Status

```bash
# Method 1: RPC syncing status
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}'

# Method 2: Compare block heights
LOCAL=$(curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq -r '.result')
REMOTE=$(curl -s https://dsn.example.com/eth/blockNumber | jq -r '.result')
echo "Local: $LOCAL, Remote: $REMOTE"

# Method 3: Check metrics
curl -s http://localhost:9090/metrics | grep dsn_block_height
```

A synced node returns `false` for the syncing method and has a block height matching the network.

### How to Verify Validator is Signing

```bash
# Check validator info
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_validators","params":[],"id":1}'

# Look for your validator in the active set
# Check last propose and vote timestamps

# Check consensus votes in metrics
curl -s http://localhost:9090/metrics | grep dsn_consensus_votes

# Check recent blocks proposed by your validator
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_blocks","params":[{"proposer": "<your-pubkey>"}],"id":1}'
```

### How to Troubleshoot Connectivity

```bash
# Test P2P port is listening
ss -tlnp | grep 30333

# Test outbound connectivity to seeds
nc -zv seed1.example.com 30333

# Check peer list
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'

# View peer connections
curl -s http://localhost:9090/metrics | grep dsn_p2p_peer_count

# Check for network errors in logs
journalctl -u dsn -g "connection\|peer\|network" --since "5m ago"

# Verify firewall rules (Linux)
iptables -L -n | grep 30333
# Or UFW:
ufw status | grep 30333
```

If peers are not connecting, check that your node ID is correct and that inbound port 30333 is accessible.

## Related Documentation

- [OPERATIONS.md](./OPERATIONS.md) — Deployment and operations
- [CONSENSUS.md](./CONSENSUS.md) — BFT consensus protocol
- [PERSISTENCE.md](./PERSISTENCE.md) — State storage and recovery
- [RECOVERY_RUNBOOK.md](./RECOVERY_RUNBOOK.md) — Incident response