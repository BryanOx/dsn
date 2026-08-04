# DSN v0.1.0-sandbox Release Notes

## Summary
DSN v0.1.0-sandbox is the first publicly deployable release of the Deterministic Settlement Network — a financial-grade deterministic blockchain for settlement finality.

## What is DSN?
DSN is a Byzantine Fault Tolerant (BFT) blockchain that guarantees byte-identical execution across all nodes. It is designed for settlement networks where determinism and correctness are paramount.

## New in This Release

### Deterministic Execution
All nodes produce byte-identical state roots given the same transaction history. This is verified through:
- Deterministic replay certification (tools/replaycert)
- Cross-platform replay verification
- Convergence testing (8 test scenarios)

### Smart Contracts
WASM-based smart contracts using the wazero runtime. Contracts are executed deterministically with gas metering and non-determinism guards.

### Staking Economics
Validators stake tokens to participate in consensus. Rewards are distributed through inflation. Slashing penalizes Byzantine behavior.

### Developer Experience
- Go SDK for transaction building/signing
- RPC API for integration
- WASM contract deployment
- Event subscription

### Operational Readiness
- Helm chart for Kubernetes
- systemd for Linux
- Prometheus + Grafana monitoring
- CI/CD pipeline
- Recovery procedures documented

## Quick Start
```bash
# Clone and build
git clone https://github.com/BryanOx/dsn.git
cd dsn
go build -o dsn ./cmd/dsn

# Initialize and start
./dsn init --network sandbox
./dsn start
```

## Documentation
Full documentation is available in the repository's docs/ directory.

## Support
- GitHub Issues: https://github.com/BryanOx/dsn/issues