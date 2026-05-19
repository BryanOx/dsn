# BFT Consensus

## Overview

DSN uses a practical Byzantine Fault Tolerant (BFT) consensus protocol with pipelined voting to achieve deterministic finality. The consensus engine ensures that all honest validators agree on the same block sequence, even in the presence of up to 33% malicious validators. The protocol is designed for deterministic replay — given the same state and transactions, any validator will produce identical results.

The consensus system is composed of several integrated modules:

- **consensus/system.go** — Block lifecycle management (BeginBlock, FinalizeBlock, CreateCheckpoint)
- **consensus/voting.go** — Vote collection and finality detection (VotingState)
- **consensus/validator_set.go** — Validator set management and hashing
- **consensus/leader.go** — Proposer selection (round-robin)
- **consensus/proposer.go** — Weighted proposer selection based on voting power
- **consensus/slashing.go** — Evidence processing and validator punishment
- **consensus/gossip.go** — Vote and block propagation
- **consensus/block_builder.go** — Block construction from transactions
- **consensus/block_validator.go** — Block validation and state transition

---

## Key Concepts

- **Round**: A single attempt to produce a block. Each round has a designated proposer and a voting period. If the proposer fails or the block doesn't reach finality, the protocol moves to the next round.

- **View**: A view represents the current state of the consensus for a specific block height. Each view has a round number and a proposer. Views change when the protocol needs to retry block production.

- **Proposal**: A block proposed by the designated proposer for a given round. The proposal includes transactions, state root, and metadata required for validation.

- **Pre-vote**: The first voting step in the two-phase commit. Validators broadcast a pre-vote for the proposed block after validating it. A block reaches pre-vote majority when 2/3+ of total voting power pre-votes.

- **Pre-commit**: The second voting step. After seeing pre-vote majority for a block, validators broadcast a pre-commit. A block reaches pre-commit majority (finality) when 2/3+ of total voting power pre-commits.

- **CommitProof**: A cryptographic proof that a block has reached finality, containing the block hash and pre-commits from 2/3+ validators.

- **ValidatorSnapshot**: A frozen snapshot of the validator set at a specific epoch, used for deterministic vote validation. This ensures that vote counting is stable even if the validator set changes mid-epoch.

---

## Architecture

### Block Lifecycle

The consensus system manages blocks through three main phases:

1. **BeginBlock** (consensus/system.go): Executed BEFORE user transactions. Handles:
   - Epoch boundary detection and transition
   - Validator set rotation at epoch boundaries
   - Epoch inflation token issuance
   - Validator reward distribution
   - Validator snapshot creation for the current epoch
   - Evidence processing (double-sign slashing)

2. **Transaction Execution**: User transactions are applied via the state engine, producing state transitions and receipts.

3. **FinalizeBlock** (consensus/system.go): Executed AFTER user transactions. Handles:
   - Fee distribution to validators and treasury
   - Block finalization with CommitProof

### Voting State Machine

The VotingState (consensus/voting.go) tracks vote collection for each block height/round:

```
                    +--------------+
                    |   Propose    |
                    +-------+------+
                            |
                    +-------v-------+
                    |   Pre-vote    |
                    +-------+------+
                            |
              2/3+ pre-vote v
                    +-------v-------+
                    |  Pre-commit   |
                    +-------+------+
                            |
              2/3+ pre-commit v
                    +-------v-------+
                    |   Finality    |
                    +--------------+
```

Key operations:
- **AddPrevote**: Records a pre-vote, validates signature, checks for duplicates
- **AddPrecommit**: Records a pre-commit with same validation rules
- **HasPrevoteMajority**: Returns true if pre-vote power >= 2/3 total
- **HasPrecommitMajority**: Returns true if pre-commit power >= 2/3 total
- **BuildCommitProof**: Creates finality proof from collected pre-commits

### Proposer Selection

Two proposer selection algorithms are implemented:

1. **Round-Robin** (consensus/leader.go): Simple height-based rotation
   ```go
   ProposerAtHeight(height, validators) = validators[height % len(validators)]
   ```

2. **Weighted Proposer** (consensus/proposer.go): Voting-power-weighted selection
   - Validators sorted by VotingPower DESC, then ConsensusID ASC
   - Proposer determined by height offset into cumulative power range
   - Fallback to round-robin if total voting power is zero

### Validator Set Management

The validator set is managed through the staking module with these key operations:

- **ValidatorRoot**: SHA256 hash of sorted validator addresses (consensus/validator_set.go)
- **Epoch boundaries**: Configurable blocks per epoch (default from staking config)
- **ValidatorSnapshot**: Frozen at epoch start, used for deterministic voting throughout the epoch

### Evidence and Slashing

The slashing system (consensus/slashing.go) handles Byzantine behavior:

1. **Evidence types**: Currently supports DoubleSignEvidence
2. **Evidence validation**: Each evidence is validated before processing
3. **Double-slashing prevention**: Evidence hash tracked to prevent processing same evidence twice
4. **Slashing execution**: Via staking.SlashValidator with configurable penalty

Processing flow:
- Validate evidence structure and signatures
- Compute evidence hash and check for duplicates
- Resolve validator from evidence (VoteA.Validator)
- Apply slashing penalty via staking module
- Mark evidence as processed

---

## Configuration

The consensus system uses configuration from the staking module:

| Parameter | Description | Default |
|-----------|-------------|---------|
| BlocksPerEpoch | Number of blocks per epoch | Configurable |
| VotingThresholdNumerator | Numerator for 2/3 threshold | 2 |
| VotingThresholdDenominator | Denominator for 2/3 threshold | 3 |
| MaxPeers | Maximum connections per node | 50 |

### Storage Configuration

Consensus uses persistent storage from the state module:
- **Checkpoints**: Stored at epoch boundaries for fast sync
- **Validator snapshots**: Created per epoch, used for voting
- **Evidence tracking**: Processed evidence persisted to prevent double-slashing

---

## Validator Lifecycle

The validator lifecycle follows these stages:

1. **Join**: Validator submits stake via staking transaction
2. **Active Set**: Validator enters active set based on stake ranking
3. **Proposal**: Validator participates in block production as proposer
4. **Voting**: Validator participates in pre-vote/pre-commit voting
5. **Epoch Transition**: Validator set rotates at epoch boundaries
6. **Jail**: Validator misbehavior triggers temporary disablement
7. **Slashing**: Byzantine behavior (double-sign) triggers penalty
8. **Exit**: Validator voluntarily unbonds (14-day cooldown)

---

## Epochs

Epochs provide a mechanism for validator set rotation and reward distribution:

- **Epoch duration**: Configurable number of blocks per epoch
- **Epoch transition**: Triggered at block heights that are multiples of blocks per epoch
- **At epoch boundary**:
  - Issue inflation tokens (10% treasury, 90% validator pool)
  - Distribute validator rewards from prior epoch
  - Process epoch transition (validator set rotation)
  - Create validator snapshot for new epoch
  - Create checkpoint for fast sync

---

## Finality

DSN achieves finality through the two-phase voting protocol:

1. **Pre-vote phase**: Validators validate the proposed block and broadcast pre-votes
2. **Pre-commit phase**: After seeing 2/3+ pre-votes, validators broadcast pre-commits
3. **Finality**: After seeing 2/3+ pre-commits, the block is finalized

The finality threshold uses a 2/3+ supermajority:
```
HasPrecommitMajority = (precommitPower * 3) >= (totalPower * 2)
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Resolution |
|-------|-------|------------|
| Block not reaching finality | Proposer failure or network partition | Protocol moves to next round |
| Duplicate votes detected | Validator equivocation | Evidence processed, validator slashed |
| Validator not selected as proposer | Not in active set or low stake | Check staking position |
| Voting power mismatch | Validator set not synced | Verify validator snapshot |

### Debugging

- Check validator set matches across nodes
- Verify epoch transitions are synchronized
- Monitor evidence processing for slash events
- Review checkpoint creation at epoch boundaries

---

## Related Documentation

- [PERSISTENCE.md](./PERSISTENCE.md) — State persistence and checkpointing
- [NETWORKING.md](./NETWORKING.md) — P2P networking for consensus
- [dsn_protocol_spec_v_1.md](./dsn_protocol_spec_v_1.md) — Protocol specification