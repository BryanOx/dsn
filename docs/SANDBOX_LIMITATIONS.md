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