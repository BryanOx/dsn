# Persistence Layer

## Overview

The persistence layer provides durable state storage for the DSN blockchain using BoltDB (embedded bbolt). It implements a hybrid storage model that combines account-based state with a Sparse Merkle Tree (SMT) for cryptographic state commitment. The layer ensures deterministic crash recovery and supports fast synchronization through periodic state snapshots.

The persistence system is composed of:

- **state/persistent.go** — BoltDB-based persistent state implementation
- **state/smt.go** — Sparse Merkle Tree for state root computation
- **state/snapshot.go** — State snapshot creation and restoration
- **state/checkpoint.go** — Checkpoint management for fast sync
- **state/restore.go** — State reconstruction from snapshots

---

## Key Concepts

- **State Root**: A cryptographic hash that commits to the entire blockchain state. The state root is computed from the SMT root after each block.

- **Sparse Merkle Tree (SMT)**: A Merkle tree optimized for sparse data structures. DSN uses a 256-bit path SMT where keys are hashed to derive paths, enabling efficient proof generation and verification.

- **Bucket**: BoltDB's term for a key-value collection. DSN uses multiple buckets for different data types.

- **WAL (Write-Ahead Log)**: BoltDB automatically provides durability through its write-ahead logging. Each write is logged before being applied to the main database.

- **Checkpoint**: A persisted snapshot of chain state at a specific block height, used for fast sync and crash recovery.

- **State Snapshot**: An in-memory representation of the state at a point in time, used for transaction execution and rollback.

---

## Architecture

### Storage Engine

DSN uses **bbolt** (Pure Go BoltDB) for embedded key-value storage:

```go
// PersistentState implements StateDB using BoltDB
type PersistentState struct {
    db     *bbolt.DB
    hasher types.Hasher
    cache  map[types.Address]*Account
    kvstore map[string][]byte
    root   types.Hash
}
```

### Data Layout

The database is organized into multiple buckets:

| Bucket | Purpose | Key Format |
|--------|---------|------------|
| `accounts` | Account state | Address (20 bytes) → Encoded Account |
| `kvstore` | Non-account state | String key → Binary value |
| `meta` | Metadata | state_root key |

#### Bucket Structure

```go
var (
    accountsBucket = []byte("accounts")
    kvstoreBucket  = []byte("kvstore")
    metaBucket     = []byte("meta")
    stateRootKey   = []byte("state_root")
)
```

### Account Storage

Accounts are stored in the `accounts` bucket:

- **Key**: 20-byte address
- **Value**: Encoded Account (using binary encoding)

The account cache is maintained in-memory for fast access during block processing.

### KVStore Storage

Non-account state (validator registry, staking ledger, epochs, contract storage) uses the `kvstore` bucket:

- **Key**: String key (e.g., "validator:...", "epoch:...")
- **Value**: Binary-encoded data

### State Root Management

The state root is stored in the `meta` bucket:

- **Key**: `state_root`
- **Value**: 32-byte hash

On database open, the state root is recovered and all accounts/kvstore entries are loaded into the cache.

---

## SMT Implementation

### Sparse Merkle Tree Structure

DSN implements a 256-bit path SMT (state/smt.go):

```go
const smtDepth = 256

type SMT struct {
    root     types.Hash
    hasher   types.Hasher
    siblings siblingsMap  // path → sibling hash (only non-zero)
    values   map[[32]byte][]byte // pathPrefix → raw value
}
```

### Key Operations

1. **Insert**: Add or update a key-value pair
   - Hash key to derive 256-bit path
   - Compute leaf hash: `Hash(0x00 || path || valueHash)`
   - Store value and recompute up the tree

2. **Get**: Retrieve a value by key
   - Hash key to derive path
   - Look up value in the values map

3. **Delete**: Remove a key
   - Remove from values map
   - Recompute tree with empty hash at leaf

4. **Root**: Get current Merkle root
   - Returns the topmost hash after all recomputations

### SMT Hashing

Two hash functions are used:

- **Leaf hash**: `Hash(0x00 || path || valueHash)` — for leaf nodes
- **Internal hash**: `Hash(0x01 || left || right)` — for internal nodes

The `0x00` and `0x01` prefixes distinguish leaf from internal nodes in the hash computation.

---

## Write-Ahead Log (WAL)

BoltDB provides WAL semantics automatically:

1. Each write transaction is logged before being applied
2. On crash recovery, the WAL is replayed to ensure durability
3. The `NoSync` option controls whether writes are synchronous (fsynced) or asynchronous (faster but less durable)

### Sync Mode Configuration

```go
func NewPersistentState(path string, hasher types.Hasher, sync bool) (*PersistentState, error) {
    opts := &bbolt.Options{
        Timeout: 1 * time.Second,
        NoSync:  !sync,  // sync=true means NoSync=false (fsync on every write)
    }
    // ...
}
```

| Mode | Performance | Durability |
|------|-------------|-------------|
| Sync (default for determinism) | Slower | Full durability |
| Async (for performance) | Faster | Crash may lose last few blocks |

---

## Snapshots and Checkpoints

### State Snapshots

The state module provides snapshot functionality for transaction execution:

```go
type StateDB interface {
    Snapshot() int
    RevertToSnapshot(id int) error
}
```

Snapshots are used for:
- Contract execution rollback on revert
- Failed transaction rollback
- Testing and development

### Checkpoints

Checkpoints (state/checkpoint.go) are persisted at epoch boundaries:

```go
type Checkpoint struct {
    Height           uint64
    BlockHash        types.Hash
    StateRoot        types.Hash
    SnapshotHash     types.Hash
    ValidatorSetHash types.Hash
    Epoch            uint64
    Timestamp        uint64
}
```

Checkpoint creation occurs in `consensus/system.go` via `CreateCheckpoint`:
- Called after BeginBlock at epoch boundaries
- Includes block height, state root, validator set hash, epoch number
- Stored via `state.StoreCheckpoint`

### Fast Sync

The network module provides fast sync capabilities (network/fastsync.go):
- Nodes can download state snapshots instead of replaying all blocks
- Snapshots are transferred in chunks for efficiency
- Validator set and state root provide integrity verification

---

## Recovery Procedures

### LoadFromPersistent

On startup, the node loads state from persistent storage:

1. Open BoltDB database
2. Read state root from meta bucket
3. If state root is non-zero, load all accounts and kvstore entries into cache

### State Recovery

For corrupted or missing state:

1. **From checkpoints**: Use the latest checkpoint to reconstruct state
2. **From genesis**: If no checkpoints, replay from genesis block
3. **Via fast sync**: Download state from a trusted peer

### SMT Rebuild

If the SMT becomes inconsistent with stored data:

1. Clear in-memory SMT structure
2. Re-insert all accounts (sorted by address)
3. Re-insert all kvstore entries (sorted by key)
4. Verify resulting root matches stored state root

---

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| Database Path | Location of BoltDB file | Configurable |
| Sync Mode | Synchronous vs asynchronous writes | Sync for determinism |
| Timeout | Database lock timeout | 1 second |
| Max Peers | Connection limits | 50 |

### Bucket Configuration

Buckets are created automatically on first open:

```go
if err := db.Update(func(tx *bbolt.Tx) error {
    if _, err := tx.CreateBucketIfNotExists(accountsBucket); err != nil {
        return err
    }
    if _, err := tx.CreateBucketIfNotExists(kvstoreBucket); err != nil {
        return err
    }
    if _, err := tx.CreateBucketIfNotExists(metaBucket); err != nil {
        return err
    }
    return nil
}); err != nil {
    // handle error
}
```

---

## Performance Considerations

### Cache Strategy

The PersistentState maintains in-memory caches:
- `cache`: Map of Address → Account
- `kvstore`: Map of string key → binary value

On `Commit()`, all cached data is written to BoltDB in a single transaction.

### Batch Writing

All state changes are batched into a single BoltDB transaction:
- Accounts are written in sorted address order
- KVStore entries are written in sorted key order
- State root is written last

### Memory Usage

- Cache size proportional to number of accounts and kvstore entries
- SMT stores only non-zero sibling hashes
- No automatic eviction (entire state kept in memory during operation)

---

## Troubleshooting

### Common Issues

| Issue | Cause | Resolution |
|-------|-------|------------|
| Database locked | Another process has the DB open | Ensure single writer |
| Corrupted database | Crash during write | Restore from checkpoint or genesis |
| State root mismatch | SMT rebuild needed | Reconstruct SMT from stored data |
| Slow commits | Large state, sync mode | Consider async mode for development |

### Recovery Procedures

1. **Database won't open**: Check file permissions, ensure no other processes
2. **State root mismatch**: Rebuild SMT from accounts/kvstore
3. **Missing checkpoints**: Use fast sync or genesis replay
4. **Slow performance**: Consider async sync mode (development only)

---

## Related Documentation

- [CONSENSUS.md](./CONSENSUS.md) — Block finality and checkpoint creation
- [WASM_VM.md](./WASM_VM.md) — Contract storage interface
- [NETWORKING.md](./NETWORKING.md) — Fast sync and state distribution