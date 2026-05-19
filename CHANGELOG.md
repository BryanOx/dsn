# Changelog

## v0.1.0-sandbox (2026-05-18)

First public sandbox release of DSN — Deterministic Settlement Network.

### Added

#### Core Protocol
- Deterministic BFT consensus engine with pipelined voting
- Ed25519-based transaction signing and verification
- Sparse Merkle Tree (SMT) state management with proofs
- In-memory state with BoltDB persistent backend
- Deterministic WASM virtual machine (wazero runtime)
- On-chain staking with inflation-based rewards
- Treasury system for protocol fee management
- Validator lifecycle: registration, activation, jailing, slashing
- Evidence-based Byzantine fault detection (double-sign, equivocation)

#### Networking
- libp2p-inspired P2P networking with gossip protocol
- Peer discovery via bootstrap nodes
- Message propagation: transactions, blocks, votes, evidence
- Configurable peer limits with rate limiting

#### Persistence & Recovery
- BoltDB-backed persistent state
- Write-ahead log (WAL) for crash recovery
- Periodic state snapshots for fast sync
- FSync option for deterministic replay guarantees
- SMT rebuild on restart (LoadFromPersistent, Recover)

#### Developer Tooling
- Go SDK with transaction building and signing
- RPC API (HTTP + WebSocket)
- CLI client (cobra) and TUI (bubbletea)
- WASM smart contract interface
- Event subscription system

#### Testing & Certification
- Deterministic replay certification (tools/replaycert)
- Convergence testing (multi-node integration tests)
- Adversarial testing (Byzantine scenarios)
- Performance benchmarking (14 benchmarks across subsystems)
- Soak testing infrastructure (tools/soakrunner)
- Operational resilience tests (5 recovery scenarios)
- Goroutine leak detection (internal/leakdetect)
- Economic simulation engine

#### Operations
- Helm chart for Kubernetes deployment
- systemd unit for Linux deployment
- Prometheus metrics and alerting rules
- Grafana dashboard (block time, sync, validators)
- CI/CD pipeline (lint, test, build, release)
- Docker Compose for local development

#### Documentation
- Protocol specification (PROTOCOL.md)
- Consensus specification (CONSENSUS.md)
- Persistence architecture (PERSISTENCE.md)
- WASM VM design (WASM_VM.md)
- Networking specification (NETWORKING.md)
- Operations guide (OPERATIONS.md)
- Validator guide (VALIDATOR_GUIDE.md)
- Recovery runbook (RECOVERY_RUNBOOK.md)
- Soak testing guide (SOAK_TESTING.md)
- Benchmarks reference (BENCHMARKS.md)
- Architecture overview (ARCHITECTURE.md)
- Quick start guide (QUICKSTART.md)
- Staging validation procedures
- Sandbox deployment guide

### Changed
- N/A (first release)

### Fixed
- SMT siblingAt() bug: state roots now correctly computed with sibling lookups
- SMT rebuild on persistent recovery: state root no longer zero after restart
- ValidateBlock now calls FinalizeBlock for consistent fee distribution
- Integration test helper now creates proper genesis configuration
- Soak test nonce tracking fixed to read from state

### Known Limitations
See SANDBOX_KNOWN_LIMITATIONS.md