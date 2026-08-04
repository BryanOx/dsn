# DSN Protocol Specification v2
## Deterministic Programmable Financial Infrastructure

---

### 1. Abstract

The DSN Protocol is a deterministic programmable financial infrastructure designed to provide secure, auditable, and operationally resilient settlement for digital assets. Built on a BFT (Byzantine Fault Tolerant) consensus mechanism with deterministic state execution via WebAssembly (WASM), DSN ensures that any validator can independently verify the state of the system by replaying all transactions from genesis. The protocol employs a Sparse Merkle Tree (SMT) for state commitment, a deterministic proposer selection mechanism, and an economic model with built-in slashing for misbehavior. This document specifies the complete protocol architecture, consensus rules, economic mechanisms, and security guarantees.

---

### 2. Motivation

Traditional financial infrastructure relies on centralized intermediaries to provide settlement finality, auditability, and operational resilience. These intermediaries introduce single points of failure, opaqueness in decision-making, and systemic risk. Decentralized blockchain protocols aim to eliminate these intermediaries but often sacrifice determinism—meaning that different validators may reach different state conclusions from the same input, violating the fundamental requirement of verifiable consensus.

DSN addresses this gap by enforcing deterministic execution at every layer of the protocol. Every state transition must be reproducibly verifiable by any participant with access to the blockchain data. This property enables:
- **Independent Verification**: Any node can recompute the state from genesis without trusting other nodes.
- **Auditability**: All state changes are traceable through cryptographic proofs.
- **Operational Resilience**: The system continues to function correctly even when a portion of validators behave arbitrarily (within the BFT threshold).
- **Replay Safety**: Transactions cannot be replayed across sessions or across forks.

---

### 3. Design Philosophy

The DSN Protocol is founded on four core principles:

**3.1 Determinism**
Every function in the system must produce the same output given the same input. Non-determinism is the primary failure mode in distributed systems. DSN eliminates all sources of non-determinism: the WASM runtime uses entropy exclusively from the BlockHeader.Hash, external system calls are prohibited, and the Sparse Merkle Tree uses a fixed hash function (SHA-256).

**3.2 Accountability**
Every action taken by a validator is cryptographically signable and verifiable. Votes, proposals, and blocks all carry signatures that can be used as evidence for slashing. The protocol maintains a complete audit trail of all consensus messages.

**3.3 Auditability**
State transitions are verified via Merkle proofs generated from the Sparse Merkle Tree. Any account balance, contract storage value, or historical transaction can be proven with a logarithmic-size proof. The combination of deterministic execution and cryptographic state commitment ensures that the state is always auditable.

**3.4 Operational Resilience**
The system must continue to function correctly under adverse conditions: network partitions, validator crashes, adversarial behavior within the BFT threshold, and state database corruption. This is achieved through snapshot-based crash recovery, fast synchronization, peer reputation scoring, and clear fork handling rules.

---

### 4. System Architecture

The DSN Protocol is organized into logical layers, each with well-defined responsibilities:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Application Layer                         │
│  ┌──────────┐  ┌──────────────┐  ┌───────────┐  ┌─────────────┐ │
│  │   SDK    │  │   Indexer    │  │  Explorer │  │    RPC     │ │
│  │ (Go SDK) │  │  (BoltDB)    │  │ (REST API)│  │ (JSON-RPC) │ │
│  └──────────┘  └──────────────┘  └───────────┘  └─────────────┘ │
├─────────────────────────────────────────────────────────────────┤
│                        Execution Layer                          │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              WASM Virtual Machine (wasmer-go)              ││
│  │   - Deterministic execution                                 ││
│  │   - Gas schedule enforcement                                 ││
│  │   - Host functions: storage, transfer, emit_event          ││
│  └─────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│                        Consensus Layer                           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              BFT Consensus Engine                           ││
│  │   - Block production pipeline                               ││
│  │   - Proposer selection (deterministic hash-based)          ││
│  │   - Vote aggregation and commit                             ││
│  │   - Evidence handling and slashing                          ││
│  └─────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│                        State Layer                               │
│  ┌──────────────┐  ┌─────────────────────────────────────────────┐│
│  │  StateDB    │  │         Sparse Merkle Tree (32-level)     ││
│  │ (In-memory  │  │         - SHA256 hash function             ││
│  │   accounts) │  │         - Root in BlockHeader.StateRoot    ││
│  └──────────────┘  └─────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│                        Network Layer                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              P2P Network                                    ││
│  │   - Gossip protocol (flood-fill with TTL)                  ││
│  │   - Fast sync (snapshot chunks + state root)               ││
│  │   - Peer reputation scoring                                ││
│  │   - Anti-spam and rate limiting                            ││
│  └─────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│                        Core Types Layer                         │
│  ┌────────┐ ┌─────────┐ ┌────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ Hash   │ │Address  │ │Amount  │ │  Block   │ │Transaction │  │
│  │[32]byte│ │[20]byte │ │*big.Int│ │ Header   │ │            │  │
│  └────────┘ └─────────┘ └────────┘ └──────────┘ └─────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**4.1 Package Structure**

| Package | Purpose |
|---------|---------|
| `types` | Core data structures: Hash, Address, Amount, Block, Transaction, Receipt, Validator, Epoch |
| `state` | Sparse Merkle Tree implementation, StateDB (in-memory accounts), Checkpoint, Snapshot |
| `vm` | WASM runtime using wasmer-go, gas schedule, contract storage, host functions |
| `consensus` | BFT engine: block production, voting, commit, proposer selection, evidence handling |
| `network` | P2P: peer management, gossip, fast sync, reputation scoring, anti-spam |
| `node` | Node lifecycle, block production, recovery, fast sync integration |
| `mempool` | Transaction pool with validation (nonce, balance, gas) |
| `staking` | Validator staking, rewards, treasury, economics, slashing |
| `wallet` | ECDSA key generation, signing (secp256k1) |
| `rpc` / `service` | JSON-RPC 2.0 server, 11 methods, middleware (CORS, validation, rate limit) |
| `indexer` | BoltDB: 5 buckets (blocks, transactions, events, validators, state) |
| `explorer` | REST API: 8 endpoints |
| `telemetry` | Prometheus metrics, slog logging, request ID tracing |
| `sdk` | Go SDK: client, typed methods, txbuilder, wallet, WebSocket, retry |
| `cmd/dsn` | CLI (Cobra): devnet, wallet, contract subcommands |

---

### 5. Consensus Architecture

#### 5.1 BFT State Machine Replication

DSN implements a Byzantine Fault Tolerant (BFT) consensus mechanism inspired by practical Byzantine fault tolerance (PBFT). The protocol guarantees safety (no two correct validators commit conflicting blocks) and liveness (the system continues to produce blocks if less than 1/3 of validators are Byzantine) under the assumptions specified in Section 8.

The consensus process follows this flow:

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Epoch     │───▶│  Proposer   │───▶│   Block     │───▶│   Commit    │
│   Start     │    │  Selection  │    │  Proposal   │    │   (Quorum)  │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       │                  │                  │                  │
       ▼                  ▼                  ▼                  ▼
 Validator Set      Deterministic       Vote by all       Finalize block
 from StateDB       hash-based          validators        into chain
```

#### 5.2 Block Structure

Each block consists of a BlockHeader, a list of Transactions, a list of Receipts, and optionally Evidence for slashing.

**BlockHeader Structure:**

| Field | Type | Description |
|-------|------|-------------|
| ParentHash | Hash (32-byte) | SHA-256 of the previous block's header |
| StateRoot | Hash (32-byte) | Sparse Merkle Tree root after executing all transactions |
| TxRoot | Hash (32-byte) | Merkle root of all transactions in the block |
| ReceiptRoot | Hash (32-byte) | Merkle root of all receipts in the block |
| Number | uint64 | Block height |
| Timestamp | uint64 | Unix timestamp (seconds) |
| GasLimit | uint64 | Maximum gas for the block |
| GasUsed | uint64 | Total gas consumed by transactions |
| Proposer | Address (20-byte) | Validator who produced this block |
| Signature | [65]byte | ECDSA secp256k1 signature of the header |

**Block Structure:**

```
Block {
    Header: BlockHeader
    Transactions: []Transaction
    Receipts: []Receipt
    Evidence: []Evidence  // optional
}
```

#### 5.3 Block Production Pipeline

The block production pipeline executes as follows:

1. **Transaction Selection**: The proposer selects transactions from the mempool, ordered by gas price (highest first).
2. **State Execution**: Each transaction is executed in the WASM VM. The StateDB is updated in-memory.
3. **SMT Update**: After each state mutation, the Sparse Merkle Tree is updated: `SMT.Update(key, value)`.
4. **Root Computation**: After all transactions execute, the SMT root is computed and stored in BlockHeader.StateRoot.
5. **Block Assembly**: The proposer packages transactions, receipts, and evidence into a Block.
6. **Signing**: The proposer signs the BlockHeader using ECDSA secp256k1, storing the signature in BlockHeader.Signature.

```
pseudocode
function produceBlock(proposer, parentBlock, transactions, stateDB):
    stateDB = loadState(parentBlock.Header.StateRoot)
    SMT = buildSMT(stateDB)
    executedTxs = []
    gasUsed = 0

    for tx in transactions:
        if gasUsed + tx.GasLimit > parentBlock.Header.GasLimit:
            break
        result = executeTransaction(tx, stateDB, SMT)
        if result.Success:
            executedTxs.append(tx)
            gasUsed += result.GasUsed
            emitReceipt(tx, result)
        else:
            // Transaction reverted, still emit receipt with failure status

    stateRoot = SMT.Root()
    txRoot = merkleRoot(executedTxs)
    receiptRoot = merkleRoot(receipts)

    header = BlockHeader{
        ParentHash: parentBlock.Header.Hash,
        StateRoot: stateRoot,
        TxRoot: txRoot,
        ReceiptRoot: receiptRoot,
        Number: parentBlock.Header.Number + 1,
        Timestamp: currentTimestamp(),
        GasLimit: parentBlock.Header.GasLimit,
        GasUsed: gasUsed,
        Proposer: proposer.Address,
    }

    header.Hash = SHA256(rlp.encode(header))
    header.Signature = sign(header.Hash, proposer.PrivateKey)

    return Block{Header: header, Transactions: executedTxs, Receipts: receipts}
```

#### 5.4 Voting Protocol

Each validator that receives a valid block proposal must cast a vote. The vote is a signed message attesting to the validity of the block.

**Vote Structure:**

| Field | Type | Description |
|-------|------|-------------|
| Round | uint64 | Consensus round number |
| BlockHash | Hash (32-byte) | Hash of the block being voted for |
| Validator | Address (20-byte) | Validator casting the vote |
| Signature | [65]byte | ECDSA secp256k1 signature |

**Voting Rules:**
- A validator must verify the block: proposer signature, state root computation, transaction root, receipt root.
- A validator must not vote for conflicting blocks in the same round.
- A validator must not vote for a block that extends a fork not matching the canonical chain.

#### 5.5 Commit and Finality

A block is considered committed when ≥2/3 of the total voting power signs a Commit message for that block. The Commit message aggregates signatures from quorum validators.

**Commit Structure:**

| Field | Type | Description |
|-------|------|-------------|
| Round | uint64 | Consensus round |
| BlockHash | Hash (32-byte) | Hash of the committed block |
| Signatures | [][65]byte | Aggregated validator signatures |

**Finality Guarantee:**
Under BFT assumptions (≤1/3 Byzantine validators), a block that achieves a quorum commit is final. No conflicting block can achieve quorum without the Byzantine validators colluding, which would be detected and slashed.

#### 5.6 Proposer Selection

Proposer selection is deterministic and based on a verifiable random function derived from the epoch and round number.

```
pseudocode
function selectProposer(validators, epochNumber, roundNumber):
    // Hash input: epoch number || round number
    input = concat(uint64ToBytes(epochNumber), uint64ToBytes(roundNumber))
    hash = SHA256(input)

    // Convert hash to integer and take modulo
    index = bytesToUint64(hash) % len(validators)

    return validators[index]
```

This ensures:
- **Fairness**: Every validator has a probability proportional to their stake.
- **Predictability**: Any validator can compute who will be the proposer for a given epoch and round.
- **Unpredictability**: The hash is sufficiently distributed to prevent predicting future proposers.

#### 5.7 Fork Handling Rules

When a fork is detected, validators follow these rules:

1. **Longest Chain Rule**: In the absence of a commit, validators follow the chain with the highest block number.
2. **Commit Finality**: Once a block is committed (≥2/3 quorum), it is final and cannot be reverted.
3. **Evidence Slashability**: Proposing or voting for conflicting blocks in the same round constitutes slashable evidence.
4. **Fork Choice**: On startup, validators replay the canonical chain from the last snapshot.

#### 5.8 Invariants

**Invariant 5.1**: For any two committed blocks B₁ and B₂ in the same epoch, if B₁.Number < B₂.Number then B₁.ParentHash = B₂.ParentHash.

**Invariant 5.2**: A block is committed only if ≥2/3 of total voting power has signed.

**Invariant 5.3**: The Proposer field in any block must be a member of the active validator set for that epoch.

**Invariant 5.4**: The BlockHeader.ParentHash must equal the hash of the immediately preceding block in the canonical chain.

**Invariant 5.5**: The BlockHeader.StateRoot must equal the Sparse Merkle Tree root computed from the StateDB after executing all transactions in the block.

---

### 6. Validator Lifecycle

#### 6.1 Registration and Staking

To become a validator, a node must submit a registration transaction that includes:
- **Public Key**: The validator's secp256k1 public key.
- **Stake**: A deposit of tokens (≥ MinStake).
- **Metadata**: Optional information (node endpoint, identity).

The stake is locked in the StateDB and cannot be withdrawn until the unbonding period ends.

```
pseudocode
function registerValidator(publicKey, stake):
    if stake < MinStake:
        return error("Insufficient stake")

    validator = Validator{
        Address: deriveAddress(publicKey),
        PubKey: publicKey,
        Stake: stake,
        Power: stake,  // Power proportional to stake
        Active: false,
    }

    stateDB.SetValidator(validator)
    // Staked tokens are moved to the staking contract
    return validator
```

#### 6.2 Active Set Selection

At the beginning of each epoch, the active validator set is computed from StateDB. Validators are sorted by stake (or a deterministic tie-breaking rule), and the top N validators form the active set. The active set determines:
- Proposer eligibility
- Voting power for quorum
- Block production rights

#### 6.3 Responsibilities

Active validators must:
- Participate in consensus (propose or vote on blocks).
- Execute all transactions in proposed blocks to verify StateRoot.
- Submit evidence of misbehavior by other validators.
- Maintain network connectivity for P2P communication.
- Produce and store snapshots for fast sync.

Failure to fulfill responsibilities may result in slashing (Section 9).

#### 6.4 Unbonding and Exit

A validator may voluntarily exit by submitting an unbond transaction. The unbonding process:

1. **Initiate Exit**: Validator signals intent to exit. Staking is marked as "unbonding."
2. **Unbonding Period**: A fixed number of epochs must pass before tokens can be withdrawn.
3. **Exit Complete**: After the unbonding period, the stake is returned to the validator's account.

During the unbonding period, the validator:
- May still be part of the active set (if still in top N by stake).
- Cannot propose new blocks (if explicitly removed from active set).
- Is subject to slashing for any evidence of misbehavior prior to exit.

```
pseudocode
function unbondValidator(validatorAddress):
    validator = stateDB.GetValidator(validatorAddress)
    validator.UnbondingStart = currentEpoch
    stateDB.SetValidator(validator)

    // After unbonding period
    if currentEpoch >= validator.UnbondingStart + UnbondingPeriod:
        transfer(validatorAddress, validator.Stake)
        stateDB.RemoveValidator(validatorAddress)
```

---

### 7. Epoch System

#### 7.1 Epoch Definition

An epoch is a fixed number of blocks (EpochLength). Each epoch has:
- **Epoch Number**: Monotonically increasing identifier.
- **Start Block**: First block of the epoch.
- **Validator Set**: Active validators for the entire epoch.

#### 7.2 Epoch Transition

At the epoch boundary:
1. The final block of epoch N includes the validator set for epoch N+1.
2. The validator set is recomputed based on current stakes in StateDB.
3. New validators are activated, and exiting validators are removed.

```
pseudocode
function processEpochTransition(stateDB, currentEpoch):
    // Compute new validator set from StateDB
    allValidators = stateDB.GetAllValidators()
    activeValidators = sortByStake(allValidators)[0:MaxActiveValidators]

    // Create new epoch
    newEpoch = Epoch{
        Number: currentEpoch.Number + 1,
        StartBlock: currentEpoch.StartBlock + EpochLength,
        ValidatorSet: activeValidators,
    }

    stateDB.SetEpoch(newEpoch)
    return newEpoch
```

#### 7.3 Validator Set Reconfiguration

Validator set changes are applied atomically at the epoch boundary. No mid-epoch reconfiguration occurs. This ensures:
- Predictable consensus behavior during an epoch.
- Simplified fork handling (no competing validator sets).
- Deterministic proposer selection for the entire epoch.

#### 7.4 Reward Distribution at Epoch Boundary

At each epoch boundary, accumulated rewards are distributed:
- **Proposer Reward**: A portion (e.g., 70%) of the epoch's block rewards goes to block proposers.
- **Validator Reward**: A portion (e.g., 30%) is distributed proportionally to voting power among all validators.
- **Treasury**: A fixed fraction (TreasuryRate) of all rewards goes to the protocol treasury.

---

### 8. Byzantine Fault Model

#### 8.1 Threat Assumptions

DSN assumes the following threat model:
- **Asynchronous Network**: Messages may be delayed arbitrarily, but eventually delivered.
- **Byzantine Validators**: Up to 1/3 of validators may exhibit arbitrary (including malicious) behavior.
- **Static Validator Set**: The set of validators is fixed within an epoch (no dynamic joining/leaving mid-epoch).
- **Computational Bounds**: Adversaries have polynomial-time computational power.

#### 8.2 Adversarial Model

The adversary may:
- Delay or withhold messages.
- Send conflicting messages to different validators.
- Produce invalid blocks or votes.
- Attempt to cause the network to fork.

The adversary cannot:
- Break cryptographic primitives (ECDSA, SHA-256).
- Forge signatures of honest validators.
- Manipulate the randomness used for proposer selection (except by influencing the block hash).

#### 8.3 Safety Guarantees

**Safety**: No two correct validators will commit conflicting blocks at the same block height.

**Theorem 8.3.1**: Under the BFT assumptions (≤1/3 Byzantine), if a block B is committed by ≥2/3 quorum, then no block B' with B'.Number = B.Number and B'.Hash ≠ B.Hash can also be committed by any correct validator.

*Proof Sketch*: To commit B, ≥2/3 of validators must have voted for B. To commit B', ≥2/3 must have voted for B'. Since >1/3 are honest, at least one honest validator would need to vote for both B and B', which is prohibited by the voting protocol. This honest validator would produce slashable evidence (Section 9). ∎

#### 8.4 Liveness Guarantees

**Liveness**: If the network is eventually synchronous and <1/3 validators are Byzantine, new blocks will be produced and committed indefinitely.

*Proof Sketch*: Under synchrony, a correct proposer will be selected and will propose a block. All honest validators will vote for valid blocks. Since ≥2/3 are honest, quorum will be reached and the block will be committed. ∎

#### 8.5 Network Assumptions

- **Eventual Delivery**: Every message sent by an honest validator to another honest validator will eventually be delivered.
- **Partial Synchrony**: There exists a bound Λ (unknown to the protocol) such that all messages are delivered within Λ time after some unknown Global Stabilization Time (GST).
- **Broadcast Channel**: Validators communicate via a reliable broadcast channel (implemented via P2P gossip).

---

### 9. Evidence and Slashing

#### 9.1 Evidence Types

DSN defines the following slashable offenses:

1. **Double Sign**: A validator signs two different blocks at the same height (equivocation).
2. **Double Vote**: A validator votes for two different blocks in the same consensus round.
3. **Invalid Proposal**: A validator proposes a block that fails validation (invalid state root, signature, etc.).
4. **Invalid Vote**: A validator votes for a block that fails validation.
5. **Downtime**: A validator fails to participate in consensus for a sustained period.

#### 9.2 Evidence Submission and Verification

Evidence is submitted as a transaction or as part of a block. The evidence includes:
- The conflicting messages (proposals or votes).
- Proof that both messages were signed by the same validator.
- The block context (height, round, epoch) showing the conflict.

```
pseudocode
function submitEvidence(evidence, stateDB):
    // Verify both signatures
    if not verifySignature(evidence.Message1, evidence.Validator):
        return error("Invalid signature on message 1")
    if not verifySignature(evidence.Message2, evidence.Validator):
        return error("Invalid signature on message 2")

    // Verify conflict
    if evidence.Message1.Round == evidence.Message2.Round:
        if evidence.Message1.BlockHash == evidence.Message2.BlockHash:
            return error("Not a conflict")
    else:
        return error("Different rounds not slashable")

    // Slash the validator
    slashValidator(evidence.Validator, evidence.Type)
```

#### 9.3 Slashing Conditions

| Offense | Condition | Evidence Required |
|---------|-----------|-------------------|
| Double Sign | Signed two different blocks at same height | Two BlockHeaders with different hashes, same signer |
| Double Vote | Signed two different votes in same round | Two Votes with different BlockHash, same signer |
| Invalid Proposal | Proposed block with invalid StateRoot | Block + StateRoot mismatch proof |
| Invalid Vote | Voted for block with invalid StateRoot | Vote + StateRoot mismatch proof |
| Downtime | Missed N consecutive blocks | Validator absence in last N block proposals |

#### 9.4 Penalty Calculation

When a validator is slashed:
1. **Stake Burn**: A portion (e.g., 10–100% depending on severity) of the validator's stake is burned (removed from circulation).
2. **Penalty Transfer**: A portion is transferred to the reporter (if evidence submitted by a third party).
3. **Removal**: The validator is removed from the active set immediately.

```
pseudocode
function slashValidator(validatorAddress, offenseType):
    validator = stateDB.GetValidator(validatorAddress)
    penalty = calculatePenalty(validator.Stake, offenseType)

    // Burn a portion
    burnAmount = penalty * BurnRatio
    protocol.Burn(burnAmount)

    // Reward reporter (if any)
    if evidence.Reporter != nil:
        transfer(evidence.Reporter, penalty * ReporterReward)

    // Remove validator
    validator.Stake -= penalty
    validator.Active = false
    stateDB.SetValidator(validator)
```

#### 9.5 Evidence Lifecycle

1. **Detection**: Any validator can detect evidence of misbehavior by another validator.
2. **Submission**: Evidence is submitted as a transaction or included in a block.
3. **Verification**: All validators verify the evidence independently.
4. **Slashing**: If valid, the offending validator is slashed immediately.
5. **Persistence**: Evidence is stored in the block as part of the chain history.

---

### 10. Economic Model

#### 10.1 Token Economics Overview

DSN uses a native token (DSN) with the following characteristics:
- **Fixed Supply**: Maximum token supply is bounded (inflation decays over time).
- **Utility Token**: Used for staking, fees, and rewards.
- **Deflationary Mechanism**: Slashing burns a portion of staked tokens.

#### 10.2 Staking Mechanism

Validators must stake a minimum amount (MinStake) to participate in consensus. Stake determines:
- **Voting Power**: Proportional to staked amount.
- **Proposer Probability**: Proportional to stake.
- **Slashing Risk**: Larger stakes face larger penalties.

```
pseudocode
function calculatePower(stake):
    // Power is proportional to stake
    return stake / TotalStake * TotalVotingPower
```

#### 10.3 Reward Distribution

Block rewards are distributed as follows:

| Recipient | Share | Calculation |
|-----------|-------|--------------|
| Proposer | 70% | (BlockReward × 0.7) × ProposerShare |
| Validators | 30% | (BlockReward × 0.3) distributed by voting power |
| Treasury | TreasuryRate | (BlockReward × TreasuryRate) |

The proposer receives an additional share based on the proportion of votes received.

```
pseudocode
function distributeBlockReward(blockReward, proposer, validators, votesReceived):
    // Proposer reward (70%)
    proposerReward = blockReward * 0.7
    transfer(proposer.Address, proposerReward)

    // Validator pool (30%)
    validatorPool = blockReward * 0.3
    totalVotingPower = sum(v.Power for v in validators)
    for v in validators:
        share = v.Power / totalVotingPower
        transfer(v.Address, validatorPool * share)

    // Treasury
    treasuryReward = blockReward * TreasuryRate
    treasury.Add(treasuryReward)
```

#### 10.4 Fee Market

Transactions include a gas price (GasPrice), denominated in the native token. The fee for a transaction is:

`Fee = GasUsed × GasPrice`

Proposers select transactions from the mempool based on gas price (highest first), creating a fee market. The protocol does not impose a fixed gas price; it is determined by market demand.

#### 10.5 Deterministic Integer Arithmetic

DSN requires all monetary calculations to use **deterministic integer arithmetic**. The `Amount` type wraps `*big.Int` and enforces:

1. **No Floating-Point**: All amounts are integers (smallest unit: e.g., 10⁻¹⁸ DSN).
2. **Overflow Checking**: All arithmetic operations check for overflow.
3. **Precision Preservation**: Operations preserve full precision; rounding is explicit.

*Why this matters*: Floating-point arithmetic is inherently non-deterministic across platforms and compilers. The same calculation may produce different results on different architectures, breaking deterministic replay verification.

**Invariant 10.1**: The sum of all account balances in StateDB after any block must equal the total supply (genesis supply + all minted rewards - all burned penalties).

#### 10.6 Replay-Safe Accounting

Every state change that involves token transfer must be recorded in the Receipt. The Receipt includes:
- The transaction hash.
- The status (success/failure).
- The gas used.
- Any log events.

This ensures all transfers can be audited and verified during replay.

---

### 11. Inflation and Treasury

#### 11.1 Inflation Schedule

DSN implements an exponentially decaying inflation schedule. The reward per block decreases each epoch:

```
R_n = R_0 × decay^n
```

Where:
- `R_n`: Reward per block in epoch n.
- `R_0`: Initial reward per block.
- `decay`: Decay factor (e.g., 0.99).
- `n`: Epoch number.

This ensures that the total token supply approaches a finite limit over time.

#### 11.2 Treasury Accumulation

The treasury collects:
- **TreasuryRate** portion of all block rewards.
- **Slashing penalties** not awarded to reporters.
- **Transaction fees** (optional, depending on protocol configuration).

Treasury funds are controlled by the governance mechanism (outside the scope of this specification).

#### 11.3 Treasury Rate Parameter

The TreasuryRate is a protocol parameter (e.g., 5%). It is fixed at genesis but may be changed via governance (future specification).

#### 11.4 Parameter Table

| Parameter | Symbol | Default Value | Description |
|-----------|--------|---------------|-------------|
| Initial Block Reward | R₀ | TBD | Token reward per block at genesis |
| Decay Factor | decay | 0.99 | Exponential decay per epoch |
| Treasury Rate | τ | 5% | Fraction of rewards going to treasury |
| Min Stake | - | TBD | Minimum stake to become a validator |
| Unbonding Period | - | TBD | Epochs before stake can be withdrawn |
| Epoch Length | - | TBD | Number of blocks per epoch |
| Max Active Validators | - | TBD | Maximum validators in active set |

---

### 12. Deterministic State Execution

#### 12.1 Principle of Determinism

A function is deterministic if, given the same input, it always produces the same output. In distributed systems, non-determinism causes validators to disagree on state, breaking consensus.

**Determinism Requirement**: Given a Block (including BlockHeader) and a StateDB, every validator must produce the identical StateRoot after executing all transactions.

#### 12.2 Execution Model

DSN executes transactions in a WebAssembly (WASM) virtual machine. The WASM runtime uses wasmer-go and enforces:
- **No External Entropy**: Randomness comes only from BlockHeader.Hash.
- **No System Calls**: Contracts cannot make OS-level calls.
- **No Network I/O**: Contracts cannot access the network.
- **Gas Enforcement**: Execution aborts when gas is exhausted.

#### 12.3 State Transition Function

The state transition function `δ(StateDB, Block) → StateDB'` is defined as:

```
pseudocode
function applyBlock(stateDB, block):
    // Load state from parent StateRoot
    currentState = loadState(block.Header.ParentStateRoot)
    smt = buildSMT(currentState)

    for tx in block.Transactions:
        // Execute transaction
        result = executeTransaction(tx, currentState, smt)

        // Emit receipt
        receipt = Receipt{
            TxHash: tx.Hash(),
            Status: result.Status,
            GasUsed: result.GasUsed,
            Logs: result.Logs,
        }
        receipts.append(receipt)

    // Compute new StateRoot
    stateRoot = smt.Root()

    // Verify computed root matches header
    if stateRoot != block.Header.StateRoot:
        return error("StateRoot mismatch")

    return currentState
```

#### 12.4 Sources of Non-Determinism (and How They Are Eliminated)

| Source | Non-Deterministic Behavior | DSN Solution |
|--------|---------------------------|---------------|
| Random Number Generation | Different seeds produce different results | Entropy from BlockHeader.Hash only |
| Current Time | System clock varies across validators | Block timestamp from header, not wall clock |
| Network Access | External requests may succeed/fail differently | No network I/O in VM |
| File System | Different filesystems have different states | No filesystem access in VM |
| Floating-Point | Different hardware produces different results | Integer-only Amount type |
| Uninitialized Memory | Different initial values | WASM zero-initializes all memory |
| Non-Deterministic Libraries | Different library versions | Pre-approved deterministic library set |

#### 12.5 Replay Verification

Any validator can independently verify the state by:
1. Starting from the genesis state.
2. Applying each block in sequence using `applyBlock()`.
3. Comparing the resulting StateRoot with the block's StateRoot.

If all blocks produce matching StateRoots, the chain is valid. This is the core mechanism for achieving auditability and trustlessness.

---

### 13. Sparse Merkle Tree Architecture

#### 13.1 Tree Structure

The Sparse Merkle Tree (SMT) is a cryptographic data structure that provides:
- **Key-Value Storage**: Mapping from 256-bit keys to values.
- **Deterministic Ordering**: Keys are sorted by their hash value.
- **Merkle Proofs**: Logarithmic-size proofs for any key.

**Specifications:**
- **Levels**: 32 (accommodating 2²⁵⁶ possible keys)
- **Hash Function**: SHA-256
- **Key Encoding**: Keys are hashed (SHA-256) before insertion
- **Value Encoding**: Values are SHA-256 hashed before storage

```
              [Root]
                 │
        ┌────────┴────────┐
       [32]              [31]
        │                 │
    ┌───┴───┐         ┌───┴───┐
   [16]   [15]       [15]   [14]
    ...     ...        ...     ...
   [0]    [1]        [1]    [0] (leaves)
```

#### 13.2 Update Algorithm

```
pseudocode
function SMT.Update(key, value):
    // Hash the key
    keyHash = SHA256(key)

    // Determine the path from root to leaf
    path = binaryRepresentation(keyHash)  // 32 bits

    // Update the leaf
    leaf = Node{Key: keyHash, Value: SHA256(value)}
    pathNodes[0] = leaf

    // Compute new hash for each level going up
    for i from 0 to 31:
        leftChild = pathNodes[i]
        // Sibling depends on the bit at position i
        rightChild = getSibling(path[i])
        if path[i] == 0:
            newHash = SHA256(leftChild.Hash || rightChild.Hash)
        else:
            newHash = SHA256(rightChild.Hash || leftChild.Hash)
        pathNodes[i+1] = Node{Hash: newHash}

    // Update all nodes in the path
    updatePath(pathNodes)

    // Update root
    root = pathNodes[32]
    return root
```

#### 13.3 Root Computation

The SMT root is computed by:
1. Starting at the leaf level (depth 31).
2. For each level, hashing the concatenation of the two children.
3. The root is the hash at level 32.

The root is stored in BlockHeader.StateRoot, providing a commitment to the entire state.

#### 13.4 Merkle Proof Generation and Verification

A Merkle proof proves that a key-value pair exists in the SMT rooted at a given root.

**Proof Structure:**
- **Path**: The bits indicating the position of the key.
- **Siblings**: The sibling hashes at each level.
- **Value**: The value at the leaf (or nil if key not present).

```
pseudocode
function SMT.Prove(key):
    keyHash = SHA256(key)
    path = binaryRepresentation(keyHash)

    proof = []
    for i from 0 to 31:
        sibling = getSiblingNode(path[i])
        proof.append(sibling.Hash)

    return MerkleProof{
        Key: key,
        Value: leaf.Value,
        Path: path,
        Siblings: proof,
    }

function verifyMerkleProof(root, proof):
    computedHash = SHA256(proof.Value)
    currentHash = computedHash

    for i from 0 to 31:
        if proof.Path[i] == 0:
            currentHash = SHA256(currentHash || proof.Siblings[i])
        else:
            currentHash = SHA256(proof.Siblings[i] || currentHash)

    return currentHash == root
```

#### 13.5 SMT Root Consistency Invariant

**Invariant 13.1**: The StateRoot in BlockHeader must equal the root of the Sparse Merkle Tree computed from StateDB after applying all transactions in the block.

**Invariant 13.2**: Two SMTs with identical key-value sets must have identical roots.

**Invariant 13.3**: Any change to any key-value pair in StateDB must result in a different SMT root.

---

### 14. Replay Guarantees

#### 14.1 Replay Attack Definition

A replay attack occurs when a valid transaction is resubmitted to the network, causing the same state change to occur multiple times. This could result in:
- Double spending of tokens.
- Multiple executions of the same contract call.
- Theft of funds from the original sender's account.

#### 14.2 Nonce Mechanism

Each account maintains a **nonce**, a monotonically increasing counter. A transaction is valid only if its nonce equals the account's current nonce.

```
pseudocode
function validateNonce(tx, account):
    if tx.Nonce != account.Nonce:
        return error("Nonce mismatch")
    return valid
```

After a transaction is executed, the account's nonce is incremented:

```
pseudocode
function incrementNonce(account):
    account.Nonce = account.Nonce + 1
```

**Invariant 14.1**: For any account, the nonce is strictly increasing with each executed transaction.

#### 14.3 Chain ID Protection

Every transaction includes a ChainID field that identifies the specific blockchain. A transaction from chain A cannot be executed on chain B.

```
pseudocode
function validateChainID(tx, currentChainID):
    if tx.ChainID != currentChainID:
        return error("Chain ID mismatch")
    return valid
```

#### 14.4 Deterministic Execution Guarantees

As specified in Section 12, the execution model is deterministic. Given the same input (Block, StateDB), the output (StateRoot) is identical across all validators. This ensures that:
- Any validator can independently verify the state.
- Replaying the chain from genesis produces the same state.
- No hidden state changes can occur.

#### 14.5 Cross-Chain Replay Prevention

DSN prevents cross-chain replay through:
1. **ChainID**: Embedded in every transaction.
2. **Domain Separation**: Signatures include the ChainID, so a signature valid on one chain cannot be used on another.
3. **Replay Contract**: Optional mechanism to track used transaction hashes.

---

### 15. Snapshot and Fast Sync

#### 15.1 Motivation

When a new validator joins the network or an existing validator recovers from a crash, they must acquire the current state. Replaying all blocks from genesis is slow. Fast sync allows downloading a recent snapshot and verifying it cryptographically.

#### 15.2 Snapshot Structure

A snapshot contains:
- **StateRoot**: The SMT root at the snapshot height.
- **BlockHash**: The hash of the block at that height.
- **Epoch**: The epoch number at that height.
- **Timestamp**: When the snapshot was created.
- **Chunks**: State data broken into chunks (e.g., 1MB each).

```
Snapshot {
    StateRoot: Hash
    BlockHash: Hash
    BlockNumber: uint64
    Epoch: uint64
    Timestamp: uint64
    Chunks: []Chunk
}
```

#### 15.3 Snapshot Signing and Verification

Snapshots are signed by the validator who produces them. The signature is included in the snapshot metadata.

```
pseudocode
function signSnapshot(snapshot, validator):
    data = rlp.encode(snapshot.Header)
    signature = signECDSA(data, validator.PrivateKey)
    snapshot.Signature = signature
    return snapshot
```

On receiving a snapshot, validators verify:
- The signature is valid.
- The StateRoot matches the downloaded state.
- The BlockHash matches the canonical chain.

#### 15.4 Chunk Transfer Protocol

State data is transferred in chunks:
1. **Request**: Validator requests chunks for a given StateRoot.
2. **Response**: Peer sends signed chunks with Merkle proofs.
3. **Verification**: Validator verifies each chunk against the StateRoot.
4. **Assembly**: Chunks are assembled into the full StateDB.

```
pseudocode
function requestChunks(stateRoot, peer):
    chunks = []
    for chunkIndex in range(numChunks):
        chunk = peer.getChunk(stateRoot, chunkIndex)
        if verifyChunkMerkle(chunk, stateRoot):
            chunks.append(chunk)
        else:
            // Request from different peer
            peer = getDifferentPeer()
            chunk = peer.getChunk(stateRoot, chunkIndex)
    return chunks
```

#### 15.5 Chunk Merkle Verification

Each chunk includes a Merkle proof that the chunk data is part of the StateRoot. This ensures:
- Data integrity (no tampering in transit).
- Correctness (data matches the claimed StateRoot).
- Non-repudiation (peer cannot deny sending invalid data).

#### 15.6 Fast Sync Integration

Fast sync is integrated into the node startup process:

```
pseudocode
function startNode():
    if hasRecentSnapshot():
        // Fast sync
        snapshot = downloadLatestSnapshot()
        stateDB = restoreFromSnapshot(snapshot)
    else:
        // Full sync (replay from genesis)
        stateDB = replayFromGenesis()

    // Start consensus
    startConsensus(stateDB)
```

---

### 16. Crash Recovery

#### 16.1 Persistence Model

DSN persists data to disk at defined checkpoints:
- **Blocks**: All blocks are stored in the block store.
- **State**: StateDB snapshots are saved periodically.
- **Evidence**: Slashing evidence is persisted in blocks.
- **Mempool**: Uncommitted transactions are not persisted (they can be re-broadcast).

#### 16.2 State Restoration on Restart

On node restart:
1. Load the latest snapshot.
2. Verify the snapshot's StateRoot and signatures.
3. Replay blocks from the snapshot height to the current height.
4. Resume consensus.

```
pseudocode
function restoreState():
    snapshot = loadLatestSnapshot()
    if verifySnapshot(snapshot):
        stateDB = restoreFromSnapshot(snapshot)
    else:
        // Fallback to earlier snapshot or genesis
        snapshot = loadEarlierSnapshot()
        stateDB = restoreFromSnapshot(snapshot)

    // Replay recent blocks
    currentHeight = snapshot.BlockNumber
    for block in getBlocks(currentHeight + 1, currentHeight):
        stateDB = applyBlock(stateDB, block)

    return stateDB
```

#### 16.3 Replay of Uncommitted Blocks

If a node crashes while a block is being proposed but not yet committed:
- The block may or may not have achieved quorum.
- On restart, the node treats the block as not committed.
- The consensus protocol will re-propose in a later round.
- No state inconsistency occurs because the SMT root was never committed.

#### 16.4 Checkpoint Integrity

Snapshots include:
- **Version**: Monotonically increasing version number.
- **StateRoot**: The SMT root.
- **BlockHash**: The canonical block at that height.
- **Signatures**: Validator signatures (for multi-party snapshots).

This section describes the intended behavior; implementation details are pending.

#### 16.5 Crash Consistency Guarantees

**Invariant 16.1**: After a crash and restart, the node's state must be a valid prefix of the canonical chain.

**Invariant 16.2**: If a snapshot was committed, the StateRoot in the snapshot must match the StateRoot in the corresponding block header.

**Invariant 16.3**: No partial state updates are persisted. All state changes are atomic.

---

### 17. P2P Networking

#### 17.1 Network Topology

DSN uses a permissionless P2P network where any node can connect to any other node. The network topology is:
- **Gossip-based**: Messages are flooded to all connected peers.
- **Dynamic**: Peers can join and leave at any time.
- **Partially Connected**: Not all nodes connect to all other nodes (reduces O(n²) complexity).

#### 17.2 Peer Discovery

Nodes discover peers through:
- **Bootstrap Nodes**: Hardcoded list of known peers.
- **Peer Exchange**: Asking connected peers for their list of peers.
- **Address Book**: Persistent storage of previously known peers.

#### 17.3 Message Propagation (Gossip)

Messages are propagated using a flood-fill protocol with TTL (Time To Live):

```
pseudocode
function broadcast(message, sender):
    for peer in connectedPeers:
        if peer != sender and peer not in message.Seen:
            send(message, peer)

    message.TTL -= 1
    if message.TTL > 0:
        // Propagate to neighbors
        for peer in connectedPeers:
            forward(message, peer)
```

**Message Types Propagated:**
- Proposals (new block candidates)
- Votes (consensus messages)
- Commits (finality messages)
- Transactions (pending transactions)
- Evidence (slashing evidence)

#### 17.4 Message Types and Encoding

| Message Type | Encoding | Purpose |
|--------------|----------|---------|
| Proposal | RLP | Block proposal from proposer |
| Vote | RLP | Validator vote for a block |
| Commit | RLP | Quorum certificate for a block |
| Transaction | RLP | User-submitted transaction |
| Evidence | RSL | Slashing evidence |
| Snapshot | RLP | State snapshot (metadata only) |
| SnapshotChunk | RLP | State data chunk |

All messages are RLP (Recursive Length Prefix) encoded for efficient wire format.

#### 17.5 Connection Management

- **Handshake**: On connection, peers exchange protocol version and node ID.
- **Keep-Alive**: Peers send periodic keep-alive messages.
- **Disconnection**: Peers disconnect gracefully or on timeout.
- **Max Peers**: Each node limits the number of active connections (e.g., 50).

#### 17.6 NAT Traversal

This section describes the intended behavior; implementation details are pending. The protocol may support:
- **UPnP**: Universal Plug and Play for port forwarding.
- **NAT-PMP**: NAT Port Mapping Protocol.
- **Relaying**: Using a relay server when direct connection is impossible.
- **Traversing**: Interactive Connectivity Establishment (ICE) style negotiation.

---

### 18. Peer Reputation and Anti-Spam

#### 18.1 Scoring Function

Each peer maintains a reputation score based on its behavior. The score ranges from 0 (blacklisted) to 100 (trusted).

```
pseudocode
function updatePeerScore(peer, message):
    if isValidMessage(message):
        peer.Score = min(100, peer.Score + ScoreIncrease)
    else:
        peer.Score = max(0, peer.Score - ScorePenalty)
```

| Behavior | Score Impact |
|----------|--------------|
| Valid Proposal | +1 |
| Valid Vote | +1 |
| Valid Commit | +1 |
| Valid Transaction | +1 |
| Invalid Signature | -10 |
| Invalid Message | -5 |
| Spam / DoS | -20 |

#### 18.2 Message Validation

Every received message is validated before processing:
- **Signature Verification**: ECDSA secp256k1 signature check.
- **Format Validation**: RLP decoding success.
- **Semantic Validation**: Message content is valid (e.g., vote has correct round).

Invalid messages are:
- Dropped silently.
- Used to decrement the sender's reputation.

#### 18.3 Rate Limiting

Each peer is subject to rate limits:

```
pseudocode
function checkRateLimit(peer, messageType):
    count = peer.MessageCount[messageType]
    if count > MaxMessagesPerSecond:
        dropMessage()
        peer.Score -= RateLimitPenalty
        return false
    return true
```

Rate limits are applied per message type:
- Proposals: Max 10/second
- Votes: Max 50/second
- Transactions: Max 100/second

#### 18.4 Penalty Thresholds

| Score Range | Status | Behavior |
|-------------|--------|----------|
| 80–100 | Trusted | Full message processing, priority in mempool |
| 40–79 | Normal | Standard message processing |
| 20–39 | Warning | Reduced message limits, monitoring |
| 0–19 | Probation | Severely limited, may be disconnected |
| 0 | Blacklisted | No messages accepted |

#### 18.5 Blacklisting and Recovery

- **Blacklisting**: If a peer's score reaches 0, it is disconnected and added to the blacklist for a duration (e.g., 1 hour).
- **Recovery**: After the blacklist period, the peer may reconnect with a neutral score (50). Subsequent good behavior increases the score.

```
pseudocode
function maybeBlacklist(peer):
    if peer.Score <= 0:
        peer.BlacklistedUntil = currentTime + BlacklistDuration
        disconnect(peer)
```

This whitepaper provides the foundational specification for the DSN Protocol. Future documents will cover governance, smart contract development, RPC API specifications, and testnet implementation details.

---

*End of Part 1 (Sections 1–18)*

---

### 19. Fork Handling and Finality

**19.1 Fork Classification**

Forks are classified by their temporal distance from the canonical chain:

- **Short-Range Fork**: A fork where the conflicting chain tip is within `ReorgDepthLimit` blocks of the canonical tip. Short-range forks are resolved via the fork choice rule defined in Section 19.6.
- **Long-Range Fork**: A fork where the conflicting chain tip exceeds `ReorgDepthLimit` blocks from the canonical tip. Long-range forks are considered invalid and are rejected by validation rules.

**19.2 Fork Detection and Resolution**

Fork detection occurs during block validation. A node identifies a fork when:
1. It receives a block whose parent hash does not match its current chain tip's hash.
2. The block's block number is greater than or equal to the current chain tip's block number.

The node maintains a fork pool containing candidate chains. Resolution follows the safety-first fork choice rule (Section 19.6).

**19.3 Finality Rule**

A block is considered **final** when it has been committed with a quorum of ≥2/3 of total validator voting power.

```
Invariant: Once a block is committed with ≥2/3 power, it is final. No reorg can revert it.
```

The commit condition is satisfied when:
- The block has received Prepare messages from ≥2/3 of validators.
- The block has received Commit messages from ≥2/3 of validators.
- The Commit phase has completed and the block is included in the local chain.

**19.4 Orphan Block Handling**

Orphan blocks (blocks with unknown parents) are held in an orphan pool for up to `OrphanTimeout` (5 seconds). If the parent arrives within the timeout, the orphan is attached to the chain. Orphan blocks exceeding the timeout are discarded.

**19.5 Reorg Depth Limit**

The maximum reorganization depth is defined as `ReorgDepthLimit = 12` blocks. Chains deeper than 12 blocks from the current tip are considered invalid and are rejected.

```
Configuration Parameter: ReorgDepthLimit = 12
```

**19.6 Safety-First Fork Choice (Pseudocode)**

```
function forkChoice(localChain, forkPool):
    // Start with the locally validated chain tip
    candidate <- localChain.Tip

    // Iterate through all forks in the pool
    for fork in forkPool:
        // Compute the total stake supporting each fork
        forkStake <- calculateStake(fork)
        candidateStake <- calculateStake(candidate)

        // Prefer the chain with higher accumulated validator support
        if forkStake > candidateStake:
            candidate <- fork

    // Return the fork with highest stake, ensuring finality is respected
    if isFinalized(candidate):
        return candidate
    else:
        return localChain.Tip  // Fallback to local chain if no fork is finalized

function calculateStake(chain):
    total <- 0
    for block in chain from genesis to tip:
        // Sum the voting power of validators who voted for each block
        total <- total + block.VoteWeight
    return total
```

---

### 20. WASM Runtime Architecture

**20.1 Virtual Machine Selection**

The DSN Protocol uses **wasmer-go** as the WebAssembly runtime. Wasmer provides a lightweight, sandboxed execution environment with near-native performance and supports the WebAssembly 1.0 MVP specification.

```
Runtime Selection: wasmer-go v2.x
Justification: Go-native binding, deterministic execution, stable API
```

**20.2 WASM Specification Compliance**

The runtime complies with:
- **WebAssembly 1.0 (MVP)**: Core instruction set, linear memory, function imports/exports.
- **WASI (WebAssembly System Interface)**: Subset for sandboxed system interactions (disabled by default in DSN).
- **Non-standard extensions**: Disabled to ensure deterministic behavior.

**20.3 Execution Engine Architecture**

```
┌─────────────────────────────────────────────────────────────────────┐
│                      WASM Runtime Architecture                      │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐         ┌──────────────────────────────────┐     │
│  │ Host Functions│────────▶│        VM Core (wasmer)         │     │
│  │              │         │  ┌─────────────────────────────┐  │     │
│  │ storage_get  │         │  │   Instruction Dispatcher   │  │     │
│  │ storage_set  │         │  │   - Const, Block, Loop     │  │     │
│  │ transfer     │         │  │   - Call, CallIndirect     │  │     │
│  │ emit_event   │         │  │   - Load, Store            │  │     │
│  │ call_contract│         │  │   - I32/I64/F32/F64 ops    │  │     │
│  │ deploy_      │         │  └─────────────────────────────┘  │     │
│  │   contract   │         │                                  │     │
│  └──────────────┘         │  ┌─────────────────────────────┐  │     │
│         │                 │  │     Linear Memory            │  │     │
│         ▼                 │  │     - 64KB initial           │  │     │
│  ┌──────────────┐         │  │     - Growable              │  │     │
│  │  StateDB     │         │  └─────────────────────────────┘  │     │
│  │  (Sparse     │◀────────│                                  │     │
│  │   Merkle)    │         │  ┌─────────────────────────────┐  │     │
│  └──────────────┘         │  │   Gas Metering Unit        │  │     │
│                            │  │   - Per-instruction costs  │  │     │
│                            │  │   - Budget enforcement      │  │     │
│                            │  └─────────────────────────────┘  │     │
│                            └──────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

**20.4 Contract Deployment Flow**

```
1. Sender submits transaction with Code field populated
2. VM initializes with empty state
3. WASM bytecode is validated (structural, type, bounds)
4. WASM module is instantiated
5. init function (if present) is executed with gas budget
6. Contract address is computed: keccak256(sender || nonce)[0:20]
7. Code is stored at contract address in StateDB
8. Deployment succeeds: receipt with contract address
```

**20.5 Contract Call Flow**

```
1. Sender submits transaction with To (contract address) and Data fields
2. VM loads contract code from StateDB at To address
3. WASM module is instantiated with fresh linear memory
4. Call function is resolved (by function selector from Data[0:4])
5. Function is executed with gas budget and provided arguments
6. State changes (storage writes) are staged
7. On success: staged changes are committed to StateDB
8. On failure: all staged changes are reverted atomically
```

**20.6 Runtime Initialization**

```
function InitializeRuntime() -> *Runtime:
    config <- DefaultConfig:
        - GasLimit: 10,000,000
        - MemoryPages: 16 (64KB)
        - MaxStackSize: 1024
    compiler <- NewJITCompiler(config)
    engine <- NewEngine(compiler)
    return &Runtime{
        Engine: engine,
        GasMeter: NewGasMeter(config.GasLimit),
        Memory: nil,
    }
```

**20.7 Instance Management**

Each contract call creates a transient WASM instance. Instances are not reused across calls to prevent state leakage.

```
function NewInstance(code []byte, hostFuncs *HostFunctions) -> *Instance:
    module <- Compile(code, wasmFeatureSet)
    instance <- module.Instantiate()
    instance.Exports["memory"] <- memory
    instance.Exports["call"] <- contractCallHandler
    return instance

// Instance lifecycle:
// 1. Create -> 2. Execute -> 3. Commit/Revert -> 4. Destroy
// No instance reuse across calls
```

---

### 21. Deterministic VM Constraints

**21.1 Sources of Non-Determinism in WASM**

The following categories introduce non-determinism and are explicitly prohibited:

| Source | Description | Risk |
|--------|-------------|------|
| Floating Point | F32/F64 operations have implementation-defined rounding | State divergence |
| Threads | Shared memory, mutexes, atomic operations | Race conditions |
| I/O | Filesystem, network sockets, stdin/stdout | Non-reproducible |
| System Calls | syscalls, time, random | External dependency |
| Date/Time | Current time queries | Temporal dependency |

**21.2 Banned Operations**

The following WASM operations are banned in DSN contracts:

```
Banned Operations:
- Memory.grow with unspecified behavior
- Unaligned atomic operations
- Reference types (ref.func, ref.null)
- SIMD instructions (v128.*)
- Bulk memory operations (memory.copy, memory.fill) beyond specified limits
- Float-to-int conversions with undefined overflow behavior
```

**21.3 Entropy Source**

All entropy in DSN contracts is derived exclusively from the current `BlockHeader.Hash`. No other entropy sources are permitted.

```
function getEntropy() -> [32]byte:
    return BlockHeader.Hash  // Deterministic, block-specific
```

**21.4 Floating Point Prohibition**

All floating-point operations (F32, F64) are prohibited in contract WASM code. The gas schedule does not include FPU operation costs.

```
Invariant: Contracts must use integer arithmetic only. F32/F64 instructions result in validation failure.
```

**21.5 Time/Clock Access Prohibition**

Contracts cannot access the current time. Time-dependent logic must use block number as the temporal reference.

```
// Banned: current_timestamp()
// Allowed: block.number
```

**21.6 Threading Prohibition**

WASM threads are disabled. The runtime does not support shared memory or atomic operations across threads.

**21.7 Determinism Verification at Block Validation**

During block validation, the validator re-executes all transactions in the block. The resulting state root must match the proposer's state root.

```
Invariant: All WASM execution is fully deterministic and reproducible given the same BlockHeader.
```

---

### 22. Gas Accounting

**22.1 Gas Model**

Gas is a computational resource that prevents infinite loops and abuse. Every WASM instruction consumes a fixed amount of gas. The gas model ensures that any validator can compute the exact gas consumption for any transaction.

**22.2 Gas Schedule**

| Operation | Cost (Gas) |
|-----------|------------|
| NOP | 1 |
| I32_LOAD/I64_LOAD | 10 |
| I32_STORE/I64_STORE | 10 |
| I32_ADD/I64_ADD | 3 |
| I32_MUL/I64_MUL | 5 |
| I32_DIV_S/I64_DIV_S | 20 |
| I32_REM_S/I64_REM_S | 20 |
| I32_AND/I64_AND | 3 |
| I32_OR/I64_OR | 3 |
| I32_XOR/I64_XOR | 3 |
| I32_SHL/I64_SHL | 3 |
| I32_SHR_S/I64_SHR_S | 3 |
| I32_EQ/I64_EQ | 3 |
| I32_EQZ/I64_EQZ | 3 |
| I32_CLZ/I64_CLZ | 5 |
| I32_POPCNT/I64_POPCNT | 5 |
| BR/BR_IF | 2 |
| CALL | 10 |
| CALL_INDIRECT | 12 |
| DROP | 1 |
| SELECT | 3 |
| Memory.init (per byte) | 1 |
| Host function call | 50–5000 (per call) |

**22.3 Gas Metering Implementation**

Gas metering is enforced by the WASM runtime through an injected gas counter that decrements before each instruction execution.

```
Pseudocode: Gas Metering Hook

functionmeterInstruction(opcode OpCode):
    cost <- GasSchedule[opcode]
    GasRemaining <- GasRemaining - cost
    if GasRemaining < 0:
        revert all state changes
        raise OutOfGasError
```

**22.4 Gas Limit Enforcement**

The block gas limit is enforced at the consensus level. The proposer sets the gas limit for the block, and validators verify that `sum(tx.gasLimit) ≤ block.gasLimit`.

```
Configuration Parameter: BlockGasLimit = 10,000,000
```

**22.5 Out-of-Gas Handling**

When gas is exhausted during execution:
1. All state changes made during the transaction are reverted atomically.
2. The transaction is marked as failed.
3. No gas is refunded.
4. A receipt is generated with `status = 0` and `gasUsed = transaction.gasLimit`.

```
Invariant: Gas exhaustion reverts ALL state changes atomically.
```

**22.6 Gas Refund Policy**

Gas is not refunded for failed transactions. For successful transactions, unused gas (up to 50% of the gas limit) is refunded to the sender.

```
Refund Formula: gasRefunded = (tx.gasLimit - gasUsed) * 0.5 (capped)
```

**22.7 Fee Calculation**

Transaction fees are calculated as:

```
Fee = GasUsed × GasPrice

where:
- GasUsed = sum of gas consumed by all instructions + host function calls
- GasPrice = transaction.gasPrice (specified by sender)
```

---

### 23. Contract Storage Model

**23.1 Storage Architecture**

Contract storage is organized hierarchically:

```
Storage Architecture:
┌────────────────────────────────────────────────────────────────────┐
│                     Contract Storage Model                         │
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Account Address ───────────────────────────────────────────────► │
│  (20 bytes)                                                        │
│       │                                                            │
│       ▼                                                            │
│  ┌────────────────┐      ┌─────────────────────────────────────┐  │
│  │ Contract Code  │      │         Contract State Trie         │  │
│  │ (immutable)    │      │                                     │  │
│  │                │      │   Key (storage key) ──────────────► │  │
│  │ - WASM bytecode│      │        │                        │  │
│  │ - init func    │      │        ▼                        │  │
│  │ - entry points │      │   Value (storage value)          │  │
│  └────────────────┘      │                                     │  │
│                          └─────────────────────────────────────┘  │
│                                                                     │
└────────────────────────────────────────────────────────────────────┘
```

**23.2 Key-Value Storage Interface**

Contracts interact with storage through host functions:

```
storage_set(key: [32]byte, value: []byte):
    contractStorage[key] <- value
    chargeGas(StorageWriteCost)

storage_get(key: [32]byte) -> []byte:
    return contractStorage[key]
    chargeGas(StorageReadCost)
```

**23.3 Storage Gas Costs**

| Operation | Cost (Gas) |
|-----------|------------|
| Storage Read (key exists) | 100 |
| Storage Read (key missing) | 50 |
| Storage Write (new key) | 500 |
| Storage Write (update) | 200 |
| Storage Delete | 100 |

**23.4 Contract Metadata**

Each contract maintains metadata:

```
Contract Metadata:
- CodeHash: keccak256(code)
- CodeSize: len(code)
- StorageRoot: root of the contract's state trie
- Nonce: number of calls since deployment
```

**23.5 Storage Proofs**

Storage proofs are generated using the Sparse Merkle Tree. Each storage slot can be proven with a log-sized proof.

```
function generateStorageProof(contract Address, key [32]byte) -> Proof:
    value <- StateDB.GetStorage(contract, key)
    path <- computeSMTPath(key)
    siblings <- collectSiblings(path)
    return Proof{
        Key: key,
        Value: value,
        Siblings: siblings,
    }
```

**23.6 Code Immutability**

Contract code cannot be modified after deployment. Only the contract's storage (state) is mutable.

```
Invariant: Contract code is fixed after deployment. Only contract storage is mutable.
```

---

### 24. Host Function Architecture

**24.1 Host Function Registration**

Host functions are registered during VM initialization and are exposed to WASM contracts through the import namespace `env`.

```
Host Function Registration Flow:
1. Initialize Runtime
2. Register all host functions in importObject
3. Compile contract WASM module
4. Instantiate with importObject
5. Execute contract code
```

**24.2 Available Host Functions**

| Function | Signature | Purpose | Gas Cost |
|----------|-----------|---------|----------|
| `storage_get` | `(address, key) → value` | Read contract storage | 100 |
| `storage_set` | `(address, key, value)` | Write contract storage | 200–500 |
| `transfer` | `(from, to, amount)` | Transfer tokens | 200 |
| `emit_event` | `(topics, data)` | Emit event log | 100 |
| `call_contract` | `(address, data) → result` | Call another contract | 500 |
| `deploy_contract` | `(bytecode) → address` | Deploy new contract | 1000 |
| `get_block_number` | `() → number` | Get current block | 50 |
| `get_caller` | `() → address` | Get transaction sender | 50 |
| `get_call_value` | `() → amount` | Get attached value | 50 |
| `get_tx_origin` | `() → address` | Get original sender | 50 |

**24.3 Host Function Determinism Guarantees**

All host functions are deterministic:
- No system time access.
- No randomness (except block hash-derived entropy).
- No external I/O.
- All state changes go through StateDB.

**24.4 Gas Costs for Host Calls**

Host function calls are metered separately from WASM instructions. Each host function call incurs a base cost plus any internal gas consumed.

```
Host Call Gas = BaseCost + InternalGas
Example: transfer() = 200 (base) + gas for balance update
```

**24.5 Security Validation**

Each host function performs input sanitization:

```
function storage_set(address, key, value):
    // Validate address is not zero
    if address == 0:
        panic(InvalidAddress)
    // Validate key length
    if len(key) != 32:
        panic(InvalidKeyLength)
    // Validate value size
    if len(value) > MaxStorageValueSize:
        panic(StorageValueTooLarge)
    // Proceed with storage write
    StateDB.SetStorage(address, key, value)
```

---

### 25. Event Architecture

**25.1 Event Structure**

Events are emitted by contracts during execution and are recorded in the block.

```
type Event struct {
    Contract   Address   // Emitting contract address
    Topics     []byte    // Indexed topics (max 4)
    Data       []byte    // Non-indexed data
    BlockNumber uint64   // Block containing the event
    Index      uint64    // Event index within block
}
```

**25.2 Event Emission Flow**

```
1. Contract calls emit_event(topic, data) host function
2. Host function validates topic count (≤4) and data size
3. Event is created and added to transaction's event list
4. After transaction execution, events are included in receipt
5. Block producer includes all transaction events in block
```

**25.3 Event Indexing**

Events are indexed by:
- Contract address (primary index)
- Topic[0] (secondary index)
- Block number range (time-filtered index)

```
Indexer Buckets:
- events_by_contract: contract → [event indices]
- events_by_topic: topic0 → [event indices]
- events_by_block: block_number → [event indices]
```

**25.4 Event Proof Generation**

Events can be proven using the transaction receipt Merkle tree:

```
Event Proof:
- ReceiptMerkleProof: path from event to receipt root
- TransactionProof: path from receipt to block transactions root
- HeaderProof: transactions root to block header
```

**25.5 Light Client Event Verification**

Light clients verify events by:
1. Fetching block header
2. Fetching transaction receipt
3. Verifying receipt against receipt root
4. Verifying event against receipt

---

### 26. Transaction Types

**26.1 Transaction Structure**

```
type Transaction struct {
    From     Address    // Sender address
    To       Address    // Recipient (nil for deployment)
    Value    *big.Int   // Native token amount
    GasPrice *big.Int   // Gas price (wei)
    GasLimit uint64     // Maximum gas
    Nonce    uint64     // Sender nonce
    Data     []byte     // Payload (calldata or bytecode)
    Signature []byte    // ECDSA secp256k1 signature
}
```

**26.2 Transaction Types**

| Type | `To` Field | `Data` Field | Description |
|------|------------|--------------|-------------|
| Value Transfer | recipient address | empty | Simple token transfer |
| Contract Deployment | `nil` | WASM bytecode | Deploy new contract |
| Contract Call | contract address | function selector + args | Invoke contract method |

**26.3 Transaction Validation Rules**

1. Signature must be valid (ECDSA secp256k1 recovery).
2. `GasLimit` must be ≤ block gas limit.
3. `Nonce` must equal sender's account nonce.
4. Sender balance must cover `GasLimit × GasPrice + Value`.
5. `Data` size must be ≤ 64 KB.

**26.4 Transaction Lifecycle**

```
Wallet → RPC → Mempool → Block Production → Execution → Receipt

Stages:
1. Wallet creates and signs transaction
2. RPC validates and broadcasts to network
3. Mempool accepts valid transactions (ordered by gas price)
4. Proposer selects transactions for block
5. Validator executes transactions in order
6. Receipts are generated and included in block
```

**26.5 Fee Calculation and Prioritization**

Transactions are prioritized in the mempool by `GasPrice × GasLimit`. Higher-fee transactions are included first.

```
Priority Score = GasPrice × GasLimit
Mempool Sort: descending by Priority Score
```

**26.6 Transaction Signing and Verification**

Signing uses ECDSA over secp256k1 with SHA-256 hash:

```
Sign(tx, privateKey):
    txHash <- keccak256(rlpEncode(tx))
    sig <- ecdsaSign(txHash, privateKey)
    return sig

Verify(tx, signature):
    txHash <- keccak256(rlpEncode(tx))
    return ecdsaVerify(txHash, signature, tx.From)
```

---

### 27. RPC and Developer Platform

**27.1 JSON-RPC 2.0 Specification Compliance**

The DSN RPC server conforms to JSON-RPC 2.0. All requests use HTTP POST with JSON request/response bodies.

**27.2 Available RPC Methods**

| Method | Params | Returns | Description |
|--------|--------|---------|-------------|
| `dsn_blockNumber` | none | `number` | Current block height |
| `dsn_getBlockByNumber` | `(number)` | `Block` | Block by number |
| `dsn_getBlockByHash` | `(hash)` | `Block` | Block by hash |
| `dsn_getTransactionByHash` | `(hash)` | `Transaction` | Tx by hash |
| `dsn_getTransactionReceipt` | `(hash)` | `Receipt` | Receipt by tx hash |
| `dsn_sendRawTransaction` | `(signed_tx)` | `hash` | Submit signed tx |
| `dsn_sendTransaction` | `(tx)` | `hash` | Submit unsigned tx (devnet) |
| `dsn_getAccount` | `(address)` | `Account` | Account info |
| `dsn_getBalance` | `(address)` | `amount` | Account balance |
| `dsn_getNonce` | `(address)` | `nonce` | Account nonce |
| `dsn_deployContract` | `(sender, bytecode, fee, gas)` | `address` | Deploy contract |
| `dsn_callContract` | `(contract, data, sender)` | `CallResult` | Read-only call |
| `dsn_estimateGas` | `(contract, data, sender)` | `EstimateResult` | Gas estimation |
| `dsn_getEvents` | `(filter)` | `Event[]` | Query events |
| `dsn_getValidators` | none | `Validator[]` | Current validator set |
| `dsn_getSupply` | none | `Supply` | Token supply info |
| `dsn_health` | none | `ok` | Node health check |

**27.3 WebSocket Subscriptions**

Real-time subscriptions are available via WebSocket:

| Subscription | Event | Description |
|--------------|-------|-------------|
| `newHeads` | BlockHeader | New block produced |
| `logs` | Event | New events emitted |
| `pendingTransactions` | Transaction | New tx in mempool |

**27.4 Input Validation Middleware**

All RPC inputs are validated before processing:

```
Validation Rules:
- Address: 20 bytes, valid checksum
- Block number: 0 ≤ number ≤ current block
- Hash: 32 bytes, valid hex
- Gas limit: ≤ BlockGasLimit
- Data size: ≤ 64 KB
```

**27.5 Rate Limiting**

```
Rate Limiting Configuration:
- Algorithm: Token Bucket
- Rate: 100 requests/second
- Burst: 200 requests
- Scope: per IP address
```

**27.6 CORS Configuration**

```
CORS Headers:
- Access-Control-Allow-Origin: * (configurable)
- Access-Control-Allow-Methods: POST, OPTIONS
- Access-Control-Allow-Headers: Content-Type
```

**27.7 Error Codes**

| Code | Meaning |
|------|---------|
| `-32700` | Parse error |
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params |
| `-32603` | Internal error |
| `-32001` | Indexer not available |
| `-32002` | Execution rejected |

---

### 28. Explorer and Indexer

**28.1 Indexer Architecture (BoltDB-based)**

The indexer persists blockchain data in BoltDB for fast queries.

```
Indexer Storage: BoltDB (embedded key-value store)
Database File: indexer.db (per node)
Mode: Always-on (production), Optional (--indexer-enabled, dev/test)
```

**28.2 Indexer Buckets**

| Bucket | Key | Value | Purpose |
|--------|-----|-------|---------|
| `blocks` | `blockNumber` | `Block` | Block storage |
| `transactions` | `txHash` | `Transaction` | Transaction lookup |
| `receipts` | `txHash` | `Receipt` | Receipt storage |
| `events` | `blockNumber_index` | `Event` | Event storage |
| `accounts` | `address` | `Account` | Account state |
| `validators` | `epoch` | `Validator[]` | Validator history |
| `state` | `contract_key` | `value` | Contract storage |

**28.3 Indexing Pipeline**

```
Indexing Pipeline:
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Block     │───▶│   Parser    │───▶│   Writer    │
│  Producer   │    │ (extract    │    │ (BoltDB     │
│             │    │  txs,events)│    │  batch      │
└─────────────┘    └─────────────┘    │  writes)    │
                                      └─────────────┘
                                           │
              ┌────────────────────────────┘
              ▼
        Goroutine consumption:
        - Async: non-blocking
        - Ordered: by block number
        - Batched: 100 blocks per batch
```

**28.4 Graceful Degradation**

The indexer can be disabled for development or testing:

```
Configuration:
--indexer-enabled=true  (default, production)
--indexer-enabled=false (devnet, testing)
```

**28.5 Explorer REST API**

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/blocks` | GET | Block list (paginated) |
| `/api/v1/blocks/:id` | GET | Block detail |
| `/api/v1/transactions/:hash` | GET | Transaction detail |
| `/api/v1/accounts/:address` | GET | Account detail |
| `/api/v1/contracts/:address` | GET | Contract detail |
| `/api/v1/validators` | GET | Validator list |
| `/api/v1/events` | GET | Event query (filterable) |
| `/api/v1/supply` | GET | Token supply info |

**28.6 Web Frontend**

The explorer provides a web interface at:

```
Location: explorer/frontend/index.html
Features: Block list, transaction search, account lookup, event log
```

**28.7 Prometheus Metrics Integration**

The node exposes Prometheus metrics at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `dsn_rpc_request_total` | Counter | Total RPC requests |
| `dsn_rpc_duration_seconds` | Histogram | RPC request latency |
| `dsn_indexer_height` | Gauge | Current indexed block |
| `dsn_block_height` | Gauge | Current chain height |
| `dsn_mempool_size` | Gauge | Transactions in mempool |
| `dsn_validator_count` | Gauge | Active validator count |

---

### 29. Security Model

**29.1 Cryptographic Primitives**

The DSN Protocol relies on two core cryptographic primitives:

| Primitive | Algorithm | Application |
|-----------|-----------|-------------|
| Digital Signature | ECDSA secp256k1 | Transaction signing, block signing, consensus votes |
| Hash Function | SHA-256 | Block hashing, state root computation, transaction hashing |

The secp256k1 curve is used for its efficiency and widespread adoption in blockchain systems. SHA-256 provides a 256-bit security level suitable for all hash operations.

**29.2 Key Management**

Keys are managed through the wallet subsystem:

```
Key Management Flow:
1. Wallet generates Ed25519 key pair (private key → public key)
2. Public key is hashed to produce address (first 20 bytes)
3. Private key is stored in an encrypted keystore (scrypt + AES-256-GCM)
4. A 12-word BIP-39 mnemonic is generated for offline backup and recovery
5. Signing operations use the private key to produce signatures
```

The `wallet` package provides:
- Key generation: `wallet.GenerateKey()`
- Encrypted keystore: `wallet.SaveKeystore()` / `wallet.LoadKeyFile()`
- BIP-39 recovery: `wallet.KeyFromMnemonic()` derives keys from a mnemonic phrase
- Signing: `wallet.Sign(tx, hasher)`
- Migration: `wallet.Migrate()` encrypts legacy plaintext wallets in place

**29.3 Signature Verification**

All signatures are verified before acceptance:

```
function VerifySignature(txHash []byte, signature []byte, address Address) -> bool:
    publicKey <- ECDSARecover(signature, txHash)
    derivedAddress <- keccak256(publicKey)[12:]
    return derivedAddress == address
```

Signature verification is applied at multiple layers:
- Transaction reception (mempool admission)
- Block validation (block proposer signature)
- Consensus messages (vote signatures, commit certificates)

**29.4 Transaction Authentication**

Transaction authentication requires:

1. **Signature validity**: ECDSA secp256k1 signature over `keccak256(rlpEncode(tx))`
2. **Nonce correctness**: `tx.nonce == account.nonce`
3. **Balance sufficiency**: `account.balance >= tx.value + (tx.gasLimit * tx.gasPrice)`

```
function AuthenticateTransaction(tx Transaction) -> Error:
    if not VerifySignature(tx):
        return ErrInvalidSignature
    if tx.nonce != GetAccount(tx.from).nonce:
        return ErrInvalidNonce
    if not HasSufficientBalance(tx):
        return ErrInsufficientBalance
    return nil
```

**29.5 Block Authentication**

Blocks are authenticated through the BFT consensus mechanism:

```
Block Authentication:
- Proposer signs the block (header.signature)
- Validators sign prepare and commit messages
- Commit certificate contains ≥⅔ validator signatures
- Each signature is verified against the validator's public key
```

The block header structure includes:
- `Proposer`: Address of the block proposer
- `Signature`: Proposer's secp256k1 signature over the header hash
- `CommitCertificate`: Set of validator signatures (≥⅔ voting power)

**29.6 Consensus Message Authentication**

Consensus messages (prepare, commit, propose) are authenticated:

```
Consensus Message Authentication:
1. Each validator has a consensus key pair (distinct from transaction key)
2. All consensus messages are signed with the consensus private key
3. Validator set is derived from epoch state (authority list)
4. Signature verification uses the validator's consensus public key
5. Messages from non-validators are rejected
```

**29.7 Peer Identity and Authentication**

P2P peer identity uses libp2p with protocol encryption:

```
Peer Identity:
- Each node has a libp2p peer ID (derived from their private key)
- Connections are encrypted with Noise protocol
- Handshake exchange verifies peer ID matches their announced address
- Peers without valid identity are disconnected
```

**29.8 State Tamper Protection (SMT proofs)**

The Sparse Merkle Tree provides cryptographic state integrity:

```
State Integrity:
- StateRoot = SMT.Root(accounts, storage)
- Any modification to state produces a new root
- SMT proofs verify specific key-value mappings
- Proof size: O(log N) where N is state size
```

State proofs are generated via:
```
function GenerateStateProof(account Address) -> Proof:
    value <- SMT.Get(StateRoot, account)
    path <- SMT.Path(account)
    siblings <- SMT.Siblings(path)
    return Proof{Value: value, Siblings: siblings}
```

**29.9 Request Authentication (RPC)**

RPC requests are authenticated as follows:

| Method | Authentication | Notes |
|--------|---------------|-------|
| `dsn_sendRawTransaction` | Signature in payload | Full authentication via ECDSA |
| `dsn_sendTransaction` | None (devnet only) | Development only, not for production |
| `dsn_callContract` | Optional sender address | Read-only, no state modification |
| `dsn_get*` | None | Public read methods |

**Table: Signature Requirements by Message Type**

| Message Type | Signed By | Signature Algorithm | Verification |
|-------------|-----------|---------------------|--------------|
| Transaction | Sender | ECDSA secp256k1 | Recover and verify address |
| Block Header | Proposer | ECDSA secp256k1 | Verify proposer signature |
| Prepare Vote | Validator | ECDSA secp256k1 | Verify validator set |
| Commit Vote | Validator | ECDSA secp256k1 | Verify validator set |
| Commit Certificate | Validators (≥⅔) | ECDSA secp256k1 | Aggregate threshold |
| Peer Message | Node | libp2p Noise | Handshake verification |

---

### 30. Threat Model

**30.1 Attack Surfaces**

The DSN Protocol has four primary attack surfaces:

| Surface | Description | Exposure |
|---------|-------------|----------|
| Network | P2P communication, RPC endpoints | External (internet) |
| Consensus | Block production, vote handling | Internal (validators) |
| Application | Contract execution, state transitions | External (users) |
| Host | Node infrastructure, key storage | Internal (operators) |

**30.2 Byzantine Threats**

Byzantine validators may exhibit arbitrary misbehavior:

| Threat | Description | Impact |
|--------|-------------|--------|
| Censorship | Validator refuses to include certain transactions | Transaction delay/exclusion |
| Equivocation | Validator signs two different blocks at same height | Fork creation |
| Junk Proposals | Validator proposes invalid or malformed blocks | Block production halt |
| Lazy Validation | Validator signs blocks without proper verification | Invalid state progression |

Mitigation:
- **Censorship**: Transaction inclusion is not guaranteed; users can submit to any validator
- **Equivocation**: Detected via double-signature evidence; results in slashing
- **Junk Proposals**: Blocks must pass validation rules; invalid blocks are rejected
- **Lazy Validation**: Validator must verify all transactions; failure triggers evidence

**30.3 Network Threats**

| Threat | Description | Impact |
|--------|-------------|--------|
| Eclipse Attack | Attacker isolates node from honest network | Node receives false view |
| Sybil Attack | Attacker creates many identities | Consensus manipulation |
| DoS Attack | Attacker floods node with requests | Service unavailability |
| Network Partition | Network splits into isolated components | Finality delay |

Mitigation:
- **Eclipse**: Use authenticated peer list; verify block hashes against known validators
- **Sybil**: Validator set is limited; peer scoring penalizes bad actors
- **DoS**: Rate limiting on RPC; connection limits on P2P
- **Partition**: Chain continues with remaining validators if ≥⅔ honest

**30.4 Economic Threats**

| Threat | Description | Impact |
|--------|-------------|--------|
| Long-Range Attack | Attacker rebuilds chain from genesis with old keys | History rewrite |
| Nothing-at-Stake | Validator votes on multiple forks simultaneously | Fork proliferation |
| Cartel Formation | ≥⅓ validators collude to censor or reorganize | Consensus capture |

Mitigation:
- **Long-Range**: Weak subjectivity; new validators sync from checkpoint
- **Nothing-at-Stake**: Slashing for double-signing; economic disincentive
- **Cartel**: No on-chain mitigation; social layer detection; community response

**30.5 Execution Threats**

| Threat | Description | Impact |
|--------|-------------|--------|
| Reentrancy | Contract calls back into caller before state update | Token drain |
| Gas Exhaustion | Contract consumes all available gas | Transaction failure |
| State Bloat | Contract stores excessive data | State database growth |

Mitigation:
- **Reentrancy**: Contracts must implement protection patterns; gas metering limits calls
- **Gas Exhaustion**: Hard gas limit per transaction; block gas limit
- **State Bloat**: Storage costs (gas) increase with write size; no free storage

**30.6 Mitigation Summary Matrix**

| Attack | Mitigation | Effectiveness | Residual Risk |
|--------|------------|---------------|---------------|
| Eclipse | Authenticated peer list | High | Low (peer selection) |
| Sybil | Validator set limit | High | Low (stake requirement) |
| DoS | Rate limiting, connection limits | Medium | Medium (resource bounds) |
| Partition | ≥⅔ honest assumption | High | Low (finality guarantee) |
| Censorship | Multi-proposer (random) | Medium | Medium (potential bias) |
| Equivocation | Slashing (double-sign) | High | Low (detection + penalty) |
| Long-Range | Weak subjectivity | High | Low (checkpoint sync) |
| Nothing-at-Stake | Slashing (double-vote) | High | Low (economic penalty) |
| Reentrancy | Contract-level patterns | Medium | Medium (developer error) |
| State Bloat | Storage gas costs | High | Low (economic disincentive) |

---

### 31. SDK Architecture

**31.1 Go SDK (`sdk/` package)**

The Go SDK provides a high-level interface for interacting with DSN:

```
SDK Package Structure:
sdk/
├── client.go          // DSNClient with typed RPC methods
├── config.go          // Client configuration
├── transaction.go    // TransactionBuilder
├── wallet.go         // Wallet and key management
├── types/
│   ├── block.go      // Block, BlockHeader types
│   ├── transaction.go// Transaction, Receipt types
│   ├── account.go    // Account type
│   └── errors.go     // Error hierarchy
└── websocket.go      // WebSocket subscription client
```

**Client construction and configuration:**

```
func NewClient(endpoint string, opts ...ClientOption) *DSNClient:
    config <- DefaultConfig()
    for opt in opts:
        opt.Apply(config)
    httpClient <- &http.Client{Timeout: config.timeout}
    return &DSNClient{
        endpoint: endpoint,
        http: httpClient,
        ws: NewWebSocketClient(config.wsEndpoint),
    }
```

**Typed RPC methods:**

The client provides type-safe wrappers for all RPC methods:

```
func (c *DSNClient) GetBlockByNumber(ctx context.Context, num uint64) (*types.Block, error):
    return c.call("dsn_getBlockByNumber", num)

func (c *DSNClient) SendRawTransaction(ctx context.Context, signedTx []byte) (common.Hash, error):
    return c.call("dsn_sendRawTransaction", signedTx)

func (c *DSNClient) GetAccount(ctx context.Context, addr common.Address) (*types.Account, error):
    return c.call("dsn_getAccount", addr)
```

All 13+ RPC methods are wrapped:
- `GetBlockByNumber`, `GetBlockByHash`
- `GetTransactionByHash`, `GetTransactionReceipt`
- `SendRawTransaction`, `SendTransaction` (devnet)
- `GetAccount`, `GetBalance`, `GetNonce`
- `DeployContract`, `CallContract`, `EstimateGas`
- `GetEvents`, `GetValidators`, `GetSupply`
- `Health`

**TransactionBuilder fluent API:**

```
tx := client.Transaction().
    To(recipient).
    Value(1000).
    GasLimit(21000).
    GasPrice(1e9).
    Build()

signed, _ := wallet.Sign(tx)
hash, _ := client.SendRawTransaction(ctx, signed)
```

**Wallet integration:**

```
wallet := wallet.NewWallet()

// Key generation
wallet.Generate()

// Sign transaction
signedTx := wallet.SignTransaction(tx, wallet.PrivateKey())

// Nonce management
nonce := wallet.NextNonce(senderAddress)
tx.SetNonce(nonce)
```

**WebSocket subscription support:**

```
ws := client.WebSocket()

// Subscribe to new blocks
ws.Subscribe("newHeads", func(header *types.BlockHeader) {
    fmt.Printf("New block: %d\n", header.Number)
})

// Subscribe to events
ws.Subscribe("logs", func(log *types.Log) {
    fmt.Printf("Event: %s\n", log.Topics)
})

// Subscribe to pending transactions
ws.Subscribe("pendingTransactions", func(tx *types.Transaction) {
    fmt.Printf("Pending tx: %s\n", tx.Hash)
})
```

**Retry middleware with exponential backoff:**

```
func WithRetry(maxRetries int, backoff time.Duration) ClientOption:
    return &retryOption{maxRetries, backoff}

func (c *DSNClient) callWithRetry(method string, params interface{}) (interface{}, error):
    var lastErr error
    for i := 0; i < c.retryMax; i++:
        result, err := c.call(method, params)
        if err == nil:
            return result, nil
        lastErr = err
        sleep(c.retryBackoff * 2^i)
    return nil, lastErr
```

**Typed error hierarchy:**

```
var (
    ErrNotFound      = errors.New("resource not found")
    ErrInvalidParams = errors.New("invalid parameters")
    ErrInternal      = errors.New("internal error")
    ErrInvalidNonce  = errors.New("invalid nonce")
    ErrInsufficientBalance = errors.New("insufficient balance")
    ErrInvalidSignature    = errors.New("invalid signature")
    ErrExecutionReverted  = errors.New("execution reverted")
)
```

**31.2 TypeScript SDK (`sdks/dsn-js/`)**

```
Package Structure:
sdks/dsn-js/
├── package.json
├── tsconfig.json
├── src/
│   ├── client.ts        // DSNClient class
│   ├── transaction.ts   // TransactionBuilder
│   ├── types.ts         // Type definitions
│   ├── wallet.ts        // Key management
│   └── index.ts         // Exports
└── dist/                // Compiled output
```

**DSNClient class:**

```typescript
export class DSNClient {
  constructor(endpoint: string, options?: ClientOptions);
  
  async getBlockByNumber(blockNumber: number): Promise<Block>;
  async getTransactionByHash(hash: string): Promise<Transaction>;
  async sendRawTransaction(signedTx: string): Promise<string>;
  async getAccount(address: string): Promise<Account>;
  async callContract(contract: string, data: string): Promise<CallResult>;
}
```

**Type definitions:**

```typescript
export interface Block {
  number: number;
  hash: string;
  parentHash: string;
  timestamp: number;
  transactions: string[];
  stateRoot: string;
  validator: string;
}

export interface Transaction {
  hash: string;
  from: string;
  to: string;
  value: string;
  gasLimit: number;
  gasPrice: string;
  nonce: number;
  data: string;
}

export interface Account {
  address: string;
  balance: string;
  nonce: number;
  codeHash: string;
}
```

**31.3 CLI Tooling (`cmd/dsn/`)**

```
CLI Commands:
dsn devnet              // Start single-node development network
dsn wallet generate     // Generate new keypair
dsn wallet sign         // Sign transaction with wallet
dsn wallet nonce        // Query account nonce
dsn contract deploy     // Deploy WASM contract
dsn contract call       // Call contract method
dsn contract estimate   // Estimate gas for call
dsn devnet --reset      // Reset devnet to genesis
```

**`dsn devnet` — single-command local development network:**

```
$ dsn devnet
Starting DSN devnet...
Genesis block: 0x0000000000000000000000000000000000000000
Validator: 0x1234567890abcdef...
RPC: http://localhost:8545
WebSocket: ws://localhost:8546
Explorer: http://localhost:8080
```

**`dsn wallet` — key management:**

```
$ dsn wallet generate
Address: 0xabcdef1234567890abcdef1234567890abcdef12
Private key: <encrypted>

$ dsn wallet sign --from 0xabcdef... --to 0x123456... --value 1000
Signature: 0xa1b2c3d4...

$ dsn wallet nonce --address 0xabcdef...
Nonce: 42
```

**`dsn contract` — contract interaction:**

```
$ dsn contract deploy --bytecode ./contract.wasm --from 0xabcdef...
Contract address: 0x9876543210abcdef...

$ dsn contract call --to 0x987654... --method "transfer" --args "0x123,1000"
Transaction hash: 0xdef456...

$ dsn contract estimate --to 0x987654... --method "transfer" --args "0x123,1000"
Estimated gas: 50000
```

**Hot restart (SIGHUP/SIGUSR1):**

```
Signal Handling:
- SIGHUP: Clean shutdown, save state, restart
- SIGUSR1: Reload configuration without restart
- SIGUSR2: Rotate log files

$ kill -SIGHUP $(pgrep dsn)
[DSN] Received SIGHUP, initiating graceful restart...
[DSN] State saved to snapshot
[DSN] Restarting services...
```

---

### 32. Consensus Invariants

The following invariants are formally guaranteed by the DSN consensus protocol:

**I₀ — Monotonic Block Growth with Validator Commitment**

```
∀ block B:
    B.Parent must be in the chain
    B.CommitCertificate contains signatures from validators with ≥⅔ total voting power
    ⇒ New blocks extend the canonical chain (no regression)
```

**I₁ — Deterministic Proposer Selection**

```
P(round) = ValidatorSet[ keccak256(epoch ‖ round)[0:8] % |ValidatorSet| ]

Where:
- epoch = current epoch number
- round = current consensus round
- ValidatorSet = sorted list of validator addresses
- |ValidatorSet| = validator count
```

**I₂ — Safety: No Conflicting Final Blocks**

```
∀ block B1, B2:
    if Finalized(B1) and Finalized(B2) and Height(B1) == Height(B2):
        then B1.Hash == B2.Hash

No two different blocks at the same height can both receive ≥⅔ commits.
```

**I₃ — Finality Definition**

```
Finalized(B) ⟺
    B.CommitCertificate.Validators.PowerSum ≥ (2/3) × TotalValidatorPower
    ∧ B.CommitCertificate.Round == B.Round
```

**I₄ — Validator Set Change at Epoch Boundaries**

```
Epoch(e).ValidatorSet = Epoch(e+1).ValidatorSet
    ⇒ no change during epoch
    ⇒ change only at epoch transition (e → e+1)
```

**I₅ — Epoch Number Progression**

```
Epoch(e+1).Number = Epoch(e).Number + 1

No skip, no duplicate epoch numbers.
```

**I₆ — Vote Validity**

```
∀ vote V:
    V.Proposal must be in chain[V.Round]
    ∧ V.Proposer is in ValidatorSet[Epoch(V.Epoch)]
    ⇒ every vote references a valid proposal from the current round
```

**I₇ — Evidence Cryptographic Proof**

```
Evidence(e, signer, block1, block2):
    require ValidSignature(block1, signer)
    ∧ ValidSignature(block2, signer)
    ∧ block1.Height == block2.Height
    ∧ block1.Hash != block2.Hash
    ⇒ evidence must include cryptographic proof (signatures)
```

**I₈ — Atomic Slashing**

```
Slash(validator, evidence):
    ValidatorPower[validator] = 0
    StateDB.SetValidatorPower(validator, 0)  // atomic
    ⇒ slashing reduces validator power atomically at epoch boundary
```

**I₉ — State Transition Determinism**

```
StateRoot(block N+1) = SMT.Apply(StateRoot(block N), Txs(block N+1))

Where SMT.Apply is the deterministic sparse merkle tree update function.
```

**I₁₀ — Unique Transaction Nonce**

```
∀ tx1, tx2:
    if tx1.From == tx2.From ∧ tx1.Nonce == tx2.Nonce:
        then tx1.Hash == tx2.Hash

Each transaction has a unique nonce per sender account.
```

---

### 33. Determinism Invariants

The following invariants ensure deterministic execution across all validators:

**D₀ — WASM Execution Determinism**

```
∀ VM instance I, input (BlockHeader, Transaction):
    Result = Execute(I, BlockHeader, Transaction)
    Result is identical across all validators
```

**D₁ — Entropy Source**

```
∀ contract execution:
    getEntropy() = BlockHeader.Hash
    No other source of randomness is accessible
```

**D₂ — Floating Point Prohibition**

```
Contract bytecode must not contain F32 or F64 instructions.
F32/F64 in bytecode → validation failure → block rejected
```

**D₃ — System Call Prohibition**

```
System calls (clock, random, network I/O) are not available to contracts.
All I/O goes through deterministic host functions only.
```

**D₄ — Threading Prohibition**

```
WASM threads are disabled:
- No shared memory
- No atomics
- No parallel execution
```

**D₅ — Host Function Determinism**

```
∀ host function f, inputs x:
    f(x) = deterministic_result
    No side effects beyond StateDB
    No access to external state
```

**D₆ — Gas Accounting Determinism**

```
GasUsed(Tx) is identical across all validators:
    - Same bytecode → same instruction sequence
    - Same gas schedule → same total cost
    - No implementation-dependent variation
```

**D₇ — SMT Update Determinism**

```
∀ ordered inputs [(k₁,v₁), (k₂,v₂), ..., (kₙ,vₙ)]:
    SMT.Root = deterministic_hash_computation
    Identical across all validators
```

**D₈ — State Transition Determinism**

```
F(S, B) → S'
    Where:
    - S = pre-state (StateDB snapshot)
    - B = block with transactions
    - S' = post-state (deterministic)
```

**D₉ — Genesis Replay Determinism**

```
Replay from genesis:
    S₀ = GenesisState
    for each block B in chain[1..N]:
        Sᵢ = F(Sᵢ₋₁, B)
    Final state = S_N
    Identical across all validators
```

---

### 34. Operational Guarantees

**34.1 Safety (No Conflicting Finality)**

```
Guarantee: No two conflicting blocks can both reach finality.
Mechanism: BFT consensus with ≥⅔ threshold for finality.
Verification: If finality is violated, slashing evidence is reproducible.
```

**34.2 Liveness (Block Production Under Faults)**

```
Guarantee: Block production continues as long as ≤⅓ Byzantine validators.
Mechanism: Proposer rotation ensures fresh proposals; timeout rounds prevent deadlock.
Verification: Simulate ≤⅓ faults in test environment; observe continued production.
```

**34.3 Crash Recovery**

```
Guarantee: State is restored from the last signed snapshot after crash.
Mechanism: Snapshot includes StateDB + Block index + Validator set.
Verification: Kill node; restart; verify state matches pre-crash.
```

**34.4 Fast Sync**

```
Guarantee: New nodes catch up via snapshot + recent blocks.
Mechanism: 
    1. Download snapshot (StateDB dump)
    2. Verify snapshot hash against trusted checkpoint
    3. Fetch blocks from snapshot height to chain tip
    4. Replay verify locally
```

**34.5 Data Retention**

```
Guarantee: Historical data is persisted via indexer (when enabled).
Mechanism: BoltDB stores blocks, transactions, receipts, events.
Verification: Query historical block; verify against known values.
```

**34.6 Graceful Degradation**

```
Guarantee: Indexer can be disabled without affecting consensus.
Mechanism: Consensus does not depend on indexer; only RPC queries use indexer.
Verification: Disable indexer; verify block production continues.
```

**34.7 Monitoring**

```
Guarantee: Prometheus metrics for all subsystems.
Mechanism: Metrics exported at /metrics; structured JSON logging.
Verification: Query metrics endpoint; verify all subsystems report.
```

Metrics include:
- `dsn_rpc_request_total` — RPC request count
- `dsn_rpc_duration_seconds` — RPC latency
- `dsn_block_height` — current chain height
- `dsn_indexer_height` — indexer sync height
- `dsn_mempool_size` — pending transaction count
- `dsn_validator_active` — active validator count

**34.8 Hot Restart**

```
Guarantee: SIGHUP triggers clean restart without data loss.
Mechanism: 
    1. Receive SIGHUP
    2. Save state snapshot
    3. Close all file handles
    4. Reload configuration
    5. Resume from snapshot
```

**Table: Guarantee → Mechanism → Verification**

| Guarantee | Mechanism | Verification |
|-----------|-----------|--------------|
| Safety | ≥⅔ BFT threshold | Double-sign → slashing |
| Liveness | Proposer rotation, timeouts | Simulate fault injection |
| Crash Recovery | Snapshot persistence | Kill/restart test |
| Fast Sync | Snapshot + block fetch | Sync from scratch |
| Data Retention | BoltDB indexer | Query historical data |
| Graceful Degradation | Consensus/idx decoupling | Disable indexer test |
| Monitoring | Prometheus + structured logging | /metrics endpoint |
| Hot Restart | SIGHUP handler | Signal injection test |

---

### 35. Limitations

The following items are NOT fully implemented or are sub-optimal:

**35.1 RPC Placeholder Data**

The following RPC methods return placeholder data and require full node integration:
- `dsn_callContract`: Returns empty result; requires WASM execution
- `dsn_estimateGas`: Returns estimated value; requires execution simulation
- `dsn_getEvents`: Returns empty list; requires indexer
- `dsn_getValidators`: Returns static list; requires epoch state
- `dsn_getSupply`: Returns placeholder; requires state computation

**35.2 WASM Runtime Limitations**

- `wasmer-go` binding maturity: Some edge cases in memory management
- Host function completeness: Not all EVM-equivalent functions available
- No SIMD support: Performance limited compared to native

**35.3 Indexer Optional**

- Indexer is not required for consensus
- If disabled: historical queries return empty results
- Data loss possible if indexer is disabled and node restarts

**35.4 No Native Light Client Protocol**

- Currently requires full node for verification
- Merkle proof verification not exposed in client
- Resource-constrained devices cannot verify state independently

**35.5 WebSocket Hub Single Process**

- WebSocket hub operates in a single process
- No horizontal scaling
- Connection limit bounded by single-node resources

**35.6 TypeScript SDK Minimal**

- No HD wallet (BIP-39/44) implementation
- No bundler optimizations (tree-shaking, minification)
- Basic type definitions only

**35.7 Web Frontend Basic**

- Reference implementation only
- Not production-grade (UI/UX, error handling)
- Limited to block list, transaction search, account lookup

**35.8 Rate Limiter In-Memory**

- Uses in-memory token bucket
- Not persisted across restarts
- Resets on node restart

**35.9 Peer Discovery Manual**

- No DHT/Kademlia integration
- Static peer list or manual addition
- Bootstrap nodes required

**35.10 Governance Manual**

- No on-chain proposal/voting system
- Validator changes require manual coordination
- No automated upgrade mechanism

**35.11 Slashing Manual**

- Evidence submission is manual
- No automatic detection of double-signing
- Requires external monitoring to trigger slash

---

### 36. Future Extensions

**36.1 Anti-Rug Covenant Primitives**

Programmable vault safety features:
- Time-locked withdrawals (cooldown period)
- Multi-signature required for large transfers
- Rate limiting on withdrawals
- Emergency circuit breaker

**36.2 Light Client Support**

Merkle proof verification for resource-constrained devices:
- Compact block headers
- Proof generation for state queries
- SPV-style verification
- Mobile/embedded device support

**36.3 Zero-Knowledge Proof Integration**

zk-rollup settlement:
- ZK circuit definitions
- Proof generation and verification
- Rollup validator selection
- On-chain verification contract

**36.4 On-Chain Governance**

Proposal submission, voting, upgrade execution:
- Proposal creation and submission
- Voting mechanism (stake-based)
- Upgrade execution automation
- Governance token

**36.5 Account Abstraction**

User-defined transaction validation logic:
- Custom signature schemes
- Fee payment by third party (meta-transactions)
- Session keys with permissions
- Account recovery mechanisms

**36.6 Advanced Settlement Layers**

- Payment channels (state channel-based)
- Atomic swaps (cross-chain)
- HTLC-based transfers
- Lightning-style network

**36.7 DHT-based Peer Discovery**

Kademlia integration:
- Distributed hash table
- Automatic peer discovery
- NAT traversal
- Bootstrap redundancy

**36.8 Automatic Slashing Detection**

- Continuous block validation monitoring
- Double-sign detection algorithm
- Automatic evidence submission
- Slash broadcasting

**36.9 Horizontal Scaling for WebSocket Hub**

- Multi-process WebSocket hub
- Message broker (Redis/RabbitMQ)
- Connection sharding
- Load balancing

**36.10 WASM SIMD Support**

- Enable SIMD instructions in WASM
- Deterministic constraints
- Vectorized operations for performance
- Security sandboxing for SIMD

**36.11 Production Web Frontend**

- Production-grade UI/UX
- Error handling and validation
- Responsive design
- Comprehensive test coverage

---

### 37. Conclusion

The DSN Protocol provides deterministic settlement infrastructure for blockchain applications. Each layer is designed for verifiable correctness:

**Consensus Layer**: BFT consensus with deterministic proposer selection guarantees safety (no conflicting finality) and liveness (block production under ≤⅓ faults). The ≥⅔ threshold ensures that finality is cryptographic, not probabilistic.

**Execution Layer**: WASM-based smart contracts execute deterministically across all validators. The deterministic VM constraints (no floating point, no threads, no system calls, entropy from block hash) ensure that state transition functions produce identical results on every validator.

**State Layer**: Sparse Merkle Trees provide cryptographic integrity for all account and storage state. State proofs enable verification without requiring full state download.

**Network Layer**: libp2p provides secure peer-to-peer communication with authenticated identities and encrypted transport.

The protocol is implemented and testable today. The Go SDK, TypeScript SDK, and CLI tooling provide developer-ready interfaces. The devnet mode enables single-command local development. The indexer persists historical data for explorer and analytics.

Future work builds on this foundation:
- Light client support enables resource-constrained verification
- On-chain governance enables decentralized upgrade coordination
- Zero-knowledge proofs enable privacy-preserving computation
- Payment channels enable high-throughput settlement

**Call to Action**: Validators can join the network by running the `dsn` node with validator configuration. Developers can build applications using the Go or TypeScript SDKs. The protocol is open for extension and welcomes contributions.

---

*End of DSN Protocol Whitepaper v2*