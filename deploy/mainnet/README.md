# DSN Mainnet Deployment

## Prerequisites

- Go 1.21+
- Access to a server with public IP

## Building

```bash
git clone <repo>
cd dsn
go build -o dsn ./cmd/dsn
```

## Genesis Generation

```bash
# Generate mainnet genesis with validators
./dsn genesis init --chain-id dsn-mainnet-1 \
  --validator-addresses "0xaddr1:1000000:0xpub1,0xaddr2:2000000:0xpub2" \
  --allocations "0xaddr1:1000000000,0xaddr2:2000000000" \
  --output genesis.json

# Validate the genesis
./dsn genesis validate genesis.json

# Inspect genesis details
./dsn genesis inspect genesis.json
```

## Running a Mainnet Node

```bash
# Initialize data directory
./dsn node start --genesis genesis.json --datadir ~/.dsn-mainnet

# With persistent state
./dsn node start --genesis genesis.json --datadir ~/.dsn-mainnet \
  --p2p 30303 --rpc 8545
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| DSN_DATA_DIR | Data directory | (in-memory) |
| DSN_RPC_PORT | RPC port | 8545 |
| DSN_P2P_PORT | P2P port | 0 (disabled) |
| DSN_TLS_CERT_FILE | TLS certificate | (disabled) |
| DSN_TLS_KEY_FILE | TLS private key | (disabled) |
| DSN_RPC_API_KEY | RPC API key | (no auth) |
| DSN_BOOTSTRAP_PEERS | Comma-separated peers | (none) |

## Backup

- BoltDB file in DataDir contains all state
- Genesis file should be backed up separately
- Validator private key must be stored securely