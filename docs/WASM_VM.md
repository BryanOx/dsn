# WASM Virtual Machine

## Overview

The DSN WASM VM provides a deterministic smart contract execution environment using [wazero](https://github.com/tetratelabs/wazero), a pure-Go WebAssembly runtime with no CGO dependencies. The VM ensures deterministic execution — given the same inputs at the same block height, all validators produce identical state roots, gas consumption, and events.

The VM implementation is based on the design document at `vm/DESIGN.md` and consists of:

- **vm/vm.go** — Main VM execution engine
- **vm/runtime.go** — wazero runtime configuration
- **vm/gas.go** — Gas metering and costs
- **vm/host.go** — Host function definitions
- **vm/storage.go** — Contract storage interface
- **vm/events.go** — Event logging
- **vm/contract.go** — Contract deployment helpers
- **vm/errors.go** — Error definitions

---

## Key Concepts

- **Contract**: A WASM module deployed to the blockchain that can be invoked via transactions. Each contract has a deterministic ContractID derived from deployer, nonce, and code hash.

- **Gas Metering**: A mechanism to limit computation by charging gas for each operation. If gas is exhausted, execution reverts with all state changes discarded.

- **Host Functions**: Functions provided by the VM to contracts, enabling state access, event emission, and token transfers. All host functions deduct gas before execution.

- **Determinism**: The property that same inputs → same outputs. This is achieved by:
  - No floating-point operations
  - No wall-clock time access
  - No randomness sources
  - Block context passed as parameters

- **Execution Result**: Contains gas used, return data, emitted events, and revert status.

---

## Architecture

### Runtime (wazero)

The VM uses wazero with deterministic configuration:

```go
func NewRuntime(ctx context.Context) (*wazero.Runtime, error) {
    config := wazero.NewRuntimeConfig().
        WithCloseNotifier(false)
    // Memory limit enforced at module instantiation
    return wazero.NewRuntime(ctx, config)
}
```

**Key configurations:**
- No filesystem access
- No network access
- No environment variables
- WASM memory capped at 64 KiB per contract
- No random number generation sources

### Execution Flow

```
Transaction → Execute() → [Deploy/Call] → Result
                                    ↓
                            State Snapshot
                                    ↓
                            [Execute/Commit]
                                    ↓
                            [Revert on Error]
                                    ↓
                            ExecutionResult
```

1. **Execute()** receives a transaction (DeployContractTx or CallContractTx)
2. **State snapshot** taken before execution for rollback capability
3. **Gas meter** created with transaction's gas limit
4. **Execution** proceeds via executeDeploy or executeCall
5. **Rollback** occurs on error or revert — state changes discarded
6. **Result** returned with gas used, events, return data

### Contract Deployment

DeployContractTx workflow (vm/vm.go):

1. Validate bytecode size (max 1 MiB)
2. Compute code hash (SHA256)
3. Derive ContractID: `SHA256(deployer || nonce || codeHash)`
4. Check contract doesn't already exist
5. Compile WASM module (rejects invalid WASM)
6. Validate imports (only "env" module with whitelisted functions)
7. Store bytecode and metadata
8. Execute constructor if exported ("init" or "constructor")
9. Return contract address

### Contract Execution

CallContractTx workflow (vm/vm.go):

1. Load contract code and metadata from state
2. Create new GasMeter with gas limit
3. Compile WASM module
4. Validate imports
5. Instantiate with host functions
6. Call exported function with calldata
7. Each host function deducts gas before execution
8. Capture events and return data
9. Rollback on revert

---

## Gas Model

### GasMeter

```go
type GasMeter struct {
    Limit    uint64
    Used     uint64
}
```

- **Deduct()**: Consumes gas, returns error if insufficient
- **Remaining()**: Returns gas left

### Gas Schedule

**WASM Instructions:**

| Operation | Cost | Notes |
|-----------|------|-------|
| Base instruction | 1 | Basic arithmetic, local get/set |
| Memory operation | 3 | Memory load/store |
| Control flow | 5 | Branches, calls |
| Non-deterministic | 100 | Operations needing extra verification |

**Host Functions:**

| Function | Cost | Description |
|----------|------|-------------|
| read_storage | 20 | Read from contract storage |
| write_storage | 50 | Write to contract storage |
| emit_event | 30 | Emit a typed event |
| read_caller | 10 | Get transaction sender |
| read_block_height | 10 | Get current block height |
| read_block_timestamp | 10 | Get block timestamp |
| transfer_token | 100 | Transfer tokens between accounts |

**Deployment:**

| Operation | Cost | Description |
|-----------|------|-------------|
| Base deployment | 1000 | Fixed overhead |
| Per code byte | 1 | Scaled by WASM size |
| Per storage byte | 2 | Scaled by value length |

### Gas Limit

Each transaction specifies a gas limit. If execution exceeds this limit:
- Execution reverts
- State changes are discarded
- Gas used is deducted from sender's balance

---

## Contract Interface

### DeployContractTx

```go
type DeployContractTx struct {
    Sender           types.Address
    Nonce            uint64
    WASMCode         []byte      // max 1 MiB
    ConstructorArgs  []byte
    MaxFee           uint64
    GasLimit         uint64
    Timestamp        uint64
    Signature        []byte
}
```

### CallContractTx

```go
type CallContractTx struct {
    Sender       types.Address
    Nonce        uint64
    ContractID   types.Hash    // target contract
    Function     string        // exported WASM function
    Args         []byte        // ABI-encoded arguments
    MaxFee       uint64
    GasLimit     uint64
    Timestamp    uint64
    Signature    []byte
}
```

### ExecutionResult

```go
type ExecutionResult struct {
    GasUsed         uint64
    ReturnData      []byte
    Events          []types.Event
    Reverted        bool
    ContractAddress *types.Address // Set for deploy operations
    GasLimit        uint64
}
```

---

## Storage

### ContractStore Interface

Contracts interact with state through the ContractStore interface:

```go
type ContractStore interface {
    GetCode(codeHash types.Hash) ([]byte, error)
    SetCode(codeHash types.Hash, code []byte) error
    GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error)
    SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error
    GetContractStorage(contractID types.Hash, key []byte) ([]byte, error)
    SetContractStorage(contractID types.Hash, key []byte, value []byte) error
}
```

### Key Namespacing

Contract data is namespaced in the SMT:

| SMT Key | Value | Description |
|---------|-------|-------------|
| `code:<hex(contractID)>` | WASM bytecode | Raw compiled WASM |
| `meta:<hex(contractID)>` | ContractMetadata | Deployment info |
| `storage:<hex(contractID)>:<hex(keyHash)>` | arbitrary bytes | Contract persistent storage |

---

## Events

Events are emitted during contract execution and included in blocks:

```go
type Event struct {
    ContractID  types.Hash
    Topic      string
    Data       []byte
    BlockHeight uint64
    TxIndex    int
}
```

**Event characteristics:**
- Deterministically serialized for inclusion in block
- NOT included in state root (no consensus impact)
- Replayable from genesis for audit/indexing
- Reverted on execution revert

---

## Limitations

The VM enforces strict limitations to ensure determinism:

| Limitation | Value | Rationale |
|------------|-------|-----------|
| Contract code size | 1 MiB | Prevent DoS via large deployments |
| Storage value size | 64 KiB | Prevent excessive storage |
| Memory pages | 1 page (64 KiB) | Prevent memory exhaustion |
| Call depth | 64 | Prevent stack overflow attacks |
| No floating point | enforced | Non-deterministic across platforms |
| No wall clock | enforced | Deterministic replay required |
| No randomness | enforced | Deterministic execution |
| Import whitelist | "env" only | Prevent system call access |

---

## Host Functions

The VM provides these host functions to contracts via the "env" module:

### read_storage

```go
read_storage(contractID_ptr, contractID_len, key_ptr, key_len, out_ptr) → error_code
// 0=ok, 1=gas limit, 2=out of bounds, 3=key not found
```

### write_storage

```go
write_storage(contractID_ptr, contractID_len, key_ptr, key_len, value_ptr, value_len) → error_code
// 0=ok, 1=gas limit, 2=out of bounds, 4=value too large, 5=write failure
```

### emit_event

```go
emit_event(topic_ptr, topic_len, data_ptr, data_len) → void
```

### read_caller

```go
read_caller(out_ptr) → error_code
// Writes 20-byte sender address to memory
```

### read_block_height

```go
read_block_height() → uint64
// Returns current block height
```

### read_block_timestamp

```go
read_block_timestamp() → uint64
// Returns block timestamp (not wall clock)
```

### transfer_token

```go
transfer_token(recipient_ptr, recipient_len, amount_hi, amount_lo) → error_code
// 128-bit amount: hi (upper 64) + lo (lower 64)
```

---

## Configuration

The VM accepts configuration parameters:

| Parameter | Description | Default |
|-----------|-------------|---------|
| maxDepth | Maximum call depth | 64 |
| executionTimeout | Execution timeout | 30 seconds |
| gasLimit | Default gas limit | Transaction-provided |
| Memory pages | WASM memory limit | 1 page (64 KiB) |

---

## Troubleshooting

### Common Issues

| Issue | Cause | Resolution |
|-------|-------|------------|
| Contract not found | Wrong ContractID or not deployed | Verify deployment transaction |
| Gas limit exceeded | Complex computation | Optimize contract or increase gas |
| Invalid import | Non-whitelisted function | Use only "env" module functions |
| Memory limit | Excessive memory allocation | Reduce memory usage |
| Call depth exceeded | Deep recursion | Refactor to iterative logic |
| Reverted execution | Contract logic error | Check contract code |

### Debugging

- Verify bytecode compiles with wazero
- Check all imports are from "env" module
- Ensure exported functions match entrypoint name
- Verify storage keys within size limits
- Check gas limit sufficient for execution

---

## Related Documentation

- [vm/DESIGN.md](../vm/DESIGN.md) — Detailed VM design
- [PERSISTENCE.md](./PERSISTENCE.md) — Storage layer for contract state
- [dsn_protocol_spec_v_1.md](./dsn_protocol_spec_v_1.md) — Protocol specification