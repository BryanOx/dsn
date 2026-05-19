# DSN Protocol Specification v1
## Deterministic Settlement Network

Version: 1.0 Draft
Status: Protocol Design Specification

---

# 1. Introduction

This document defines the technical protocol specification for the Deterministic Settlement Network (DSN).

The specification describes:

- node behavior
- transaction structure
- consensus behavior
- state transition rules
- validator coordination
- deterministic execution
- fee accounting
- networking rules
- storage semantics
- anti-rug primitives
- idempotency guarantees

This specification is intended to be sufficient for implementation of:

- validator nodes
- full nodes
- light clients
- SDKs
- block explorers
- indexers
- execution runtimes

---

# 2. Design Goals

## 2.1 Primary Goals

DSN prioritizes:

1. Deterministic execution
2. Idempotent settlement
3. Constraint-safe transactions
4. Predictable fees
5. Parallel execution
6. Economic safety
7. High throughput
8. Fast finality
9. Low-cost validation
10. Anti-rug infrastructure

---

## 2.2 Non-Goals

DSN does not prioritize:

- arbitrary unrestricted execution
- maximum theoretical TPS at all costs
- anonymous execution
- unlimited smart contract flexibility
- speculative tokenomics

---

# 3. Reference Implementation Language

## 3.1 Recommended Language

Primary recommendation:

Go

---

## 3.2 Why Go

Go is recommended because:

- strong networking support
- mature concurrency primitives
- excellent tooling
- fast compilation
- simpler validator development
- strong operational ecosystem
- easier onboarding
- lower implementation complexity

---

## 3.3 Rust Consideration

Rust is also suitable.

Advantages:

- memory safety
- higher low-level control
- performance

Disadvantages:

- increased development complexity
- longer implementation time
- more difficult protocol iteration

---

## 3.4 Final Recommendation

For DSN v1:

- Core protocol → Go
- Performance-critical cryptography → Rust/C bindings optional later

Go provides the best balance between:

- development speed
- reliability
- ecosystem maturity
- protocol iteration

---

# 4. Network Topology

## 4.1 Node Types

DSN defines four node types.

---

### Validator Node

Responsibilities:

- participate in consensus
- validate blocks
- execute transactions
- maintain full state
- sign finality proofs
- produce blocks

---

### Full Node

Responsibilities:

- validate chain state
- replicate blockchain
- serve RPC queries
- execute transactions

Cannot:

- participate in consensus

---

### Light Client

Responsibilities:

- verify block headers
- verify Merkle proofs
- submit transactions

Does not maintain full state.

---

### Indexer Node

Responsibilities:

- event indexing
- analytics
- explorer APIs
- historical queries

---

# 5. Cryptography

## 5.1 Hashing

Hashing algorithm:

BLAKE3

Reasoning:

- high speed
- modern security properties
- efficient parallelism

---

## 5.2 Signatures

Signature algorithm:

Ed25519

Validator aggregation (future optional):

BLS signatures

---

## 5.3 Address Format

Addresses are:

20-byte account identifiers.

Encoding:

Base58Check

---

# 6. Block Structure

## 6.1 Block Layout

Each block contains:

| Field | Type |
|---|---|
| version | uint32 |
| height | uint64 |
| previous_hash | bytes32 |
| state_root | bytes32 |
| tx_root | bytes32 |
| receipt_root | bytes32 |
| validator_root | bytes32 |
| timestamp | uint64 |
| proposer | address |
| fee_summary | FeeSummary |
| signature | bytes |

---

## 6.2 Timestamp Rules

Timestamp drift tolerance:

±5 seconds.

Invalid timestamps are rejected.

---

## 6.3 Block Size

Initial max block size:

8 MB

Dynamic expansion may occur via governance.

---

# 7. Transaction Model

## 7.1 Intent-Based Transactions

Transactions describe desired state transitions.

Transactions are not unrestricted imperative commands.

---

## 7.2 Transaction Structure

| Field | Type |
|---|---|
| version | uint16 |
| chain_id | uint32 |
| intent_id | bytes32 |
| sender | address |
| nonce | uint64 |
| payload | bytes |
| constraints | bytes |
| max_fee | uint64 |
| gas_limit | uint64 |
| timestamp | uint64 |
| signature | bytes |

---

## 7.3 Intent ID Generation

Intent ID:

intent_id =
HASH(sender + nonce + payload + constraints)

---

## 7.4 Idempotency Rules

A transaction is considered already executed if:

- identical intent_id exists
- settlement finalized

Repeated submissions return:

- original execution result
- original receipt

No duplicated state mutation occurs.

---

# 8. State Model

## 8.1 Hybrid Account Model

DSN combines:

- account-based state
- deterministic UTXO-like guarantees

---

## 8.2 Account Structure

| Field | Type |
|---|---|
| address | bytes20 |
| balance | uint128 |
| nonce | uint64 |
| storage_root | bytes32 |
| code_hash | bytes32 |
| permissions | bitset |

---

## 8.3 State Invariants

The following invariants MUST hold:

1. No negative balances
2. Total supply consistency
3. Deterministic execution
4. No double execution
5. No invalid minting
6. Valid signatures only
7. Immutable finalized history

Violation invalidates the block.

---

# 9. Execution Engine

## 9.1 Deterministic VM

DSN VM is deterministic.

Forbidden operations:

- floating-point math
- unrestricted recursion
- nondeterministic randomness
- unrestricted external I/O
- system clock access

---

## 9.2 Execution Metering

Every operation consumes execution units.

Execution units convert into fees.

---

## 9.3 Parallel Execution

Transactions may execute in parallel if:

- state access sets do not overlap
- contract dependencies are isolated

---

## 9.4 Conflict Resolution

Conflicting transactions enter deterministic serialization.

Ordering rule:

(priority_fee DESC, timestamp ASC, intent_id ASC)

---

# 10. Constraint Engine

## 10.1 Purpose

The constraint engine validates execution rules before settlement.

---

## 10.2 Constraint Types

Supported constraints:

| Constraint | Description |
|---|---|
| max_fee | maximum acceptable fee |
| min_balance | minimum remaining balance |
| settlement_deadline | required execution time |
| vesting_lock | token lock restrictions |
| liquidity_lock | LP lock requirements |
| treasury_cap | treasury restrictions |

---

## 10.3 Constraint Failure

If constraints fail:

- transaction rejected
- no state mutation
- fee partially charged

---

# 11. Fee Market

## 11.1 Fee Formula

Fee calculation:

fee =
base_cost +
execution_cost +
storage_cost +
congestion_multiplier

---

## 11.2 Base Cost

Base cost protects against spam.

---

## 11.3 Congestion Multiplier

Network congestion dynamically adjusts fees.

Multiplier updates every block.

Target block occupancy:

60%

---

## 11.4 Fee Distribution

| Destination | Share |
|---|---|
| Validators | 70% |
| Burn | 20% |
| Treasury | 10% |

---

# 12. Consensus Protocol

## 12.1 Deterministic Intent Consensus (DIC)

Consensus phases:

1. Intent collection
2. Intent ordering
3. Constraint validation
4. Execution
5. State root generation
6. Validator voting
7. Finalization

---

## 12.2 Validator Selection

Validators selected via:

weighted stake randomness.

---

## 12.3 Epochs

Epoch duration:

1 hour.

Validator set updates occur between epochs.

---

## 12.4 Finality Threshold

Block finalization requires:

>= 67% validator stake approval.

---

## 12.5 Slashing Conditions

Validators are slashed for:

- double signing
- invalid blocks
- invalid state roots
- censorship proof violations
- consensus equivocation

---

# 13. Mempool Rules

## 13.1 Local Validation

Transactions entering mempool must pass:

- signature verification
- nonce validation
- balance checks
- constraint validation
- fee validation

---

## 13.2 Duplicate Prevention

Duplicate intent IDs are rejected.

---

## 13.3 Transaction Expiration

Transactions expire after:

300 seconds by default.

---

# 14. Anti-Rug Protocol Primitives

## 14.1 Liquidity Locks

Protocol-native LP locks supported.

Lock policies:

- fixed lock
- gradual unlock
- irreversible burn

---

## 14.2 Vesting Contracts

Vesting policies immutable after deployment.

---

## 14.3 Mint Restrictions

Minting policy immutable after token deployment.

Possible modes:

- fixed supply
- capped inflation
- scheduled emission

---

## 14.4 Treasury Constraints

Treasury policies may enforce:

- spending caps
- multisig
- cooldowns
- milestone releases

---

# 15. Validator Economics

## 15.1 Minimum Stake

Minimum validator stake:

100,000 DSN

---

## 15.2 Reward Sources

Validators earn:

- fees
- inflation rewards
- settlement bonds

---

## 15.3 Unstaking

Unstaking cooldown:

14 days.

---

# 16. Storage Layer

## 16.1 Database Recommendation

Recommended:

RocksDB

Alternative:

PebbleDB

---

## 16.2 State Commitment

State committed using:

Sparse Merkle Trees.

---

## 16.3 Event Log

All state transitions generate append-only events.

---

# 17. Networking Layer

## 17.1 P2P Stack

Recommended:

libp2p

---

## 17.2 Protocol Messages

Supported message types:

| Message | Purpose |
|---|---|
| TX_SUBMIT | transaction propagation |
| BLOCK_PROPOSAL | proposed block |
| BLOCK_VOTE | validator vote |
| STATE_SYNC | state synchronization |
| PEER_DISCOVERY | node discovery |
| FINALITY_PROOF | finalized block proof |

---

## 17.3 Compression

Network messages compressed using:

Snappy

---

# 18. RPC Interface

## 18.1 JSON-RPC

Nodes expose JSON-RPC APIs.

---

## 18.2 Core Methods

| Method | Purpose |
|---|---|
| sendTransaction | submit tx |
| getTransaction | fetch tx |
| getBalance | account balance |
| getBlock | fetch block |
| getReceipt | execution receipt |
| subscribeEvents | realtime events |

---

# 19. Smart Contracts

## 19.1 Runtime

Recommended runtime:

Deterministic WASM.

---

## 19.2 Gas Metering

Every instruction metered.

---

## 19.3 Contract Permissions

Contracts must declare:

- storage scope
- external access
- token permissions
- upgradeability

---

# 20. Governance

## 20.1 Governance Limits

Governance cannot:

- confiscate balances
- bypass locks
- arbitrarily mint
- rewrite finalized history

---

## 20.2 Governance Scope

Governance may modify:

- fee parameters
- validator thresholds
- block sizes
- protocol versions

---

# 21. Security Requirements

## 21.1 Required Audits

Before mainnet:

- cryptographic audit
- consensus audit
- VM audit
- economic audit

---

## 21.2 Byzantine Tolerance

DSN assumes:

< 33% malicious validator stake.

---

# 22. Initial Mainnet Targets

| Metric | Target |
|---|---|
| TPS | 5,000–20,000 |
| Finality | 2–5 seconds |
| Fee Target | <$0.01 |
| Validator Count | 100+ |

---

# 23. Suggested Repository Structure

```text
/cmd
/node
/consensus
/execution
/state
/storage
/network
/crypto
/vm
/rpc
/contracts
/mempool
/validator
/tests
/sdk
```

---

# 24. Recommended Development Roadmap

## Phase 1

- deterministic ledger
- mempool
- transaction model
- state engine

---

## Phase 2

- networking
- consensus
- validator coordination

---

## Phase 3

- WASM VM
- smart contracts
- constraint engine

---

## Phase 4

- anti-rug primitives
- SDKs
- explorer
- tooling

---

## Phase 5

- public testnet
- economic testing
- audits
- mainnet

---

# 25. Final Notes

This specification defines the architectural foundation for DSN.

Additional documents are recommended:

- VM instruction specification
- consensus proof specification
- binary serialization format
- formal state transition rules
- cryptographic proof specification
- validator networking specification
- economic attack analysis

The DSN architecture prioritizes correctness, predictability, and financial safety over unrestricted programmability.

