# Consensus — Implementation Tasks

## Task 1: Validator Set Helpers
**Package**: consensus/validator_set.go  
**Test**: consensus/validator_set_test.go  
**Description**: Helper functions for validator set management: compute deterministic root hash from validator list, sort validators consistently.  
**Acceptance**:
- [ ] `ValidatorRoot(validators []types.Address) types.Hash` returns deterministic hash regardless of input order
- [ ] `SortedValidators(validators []types.Address) []types.Address` returns validators sorted by byte value
- [ ] Empty validator list returns empty root (zero hash)
- [ ] Single validator returns correct root
- [ ] Root is deterministic across multiple calls with same input

**Edge Cases**:
- Empty validator slice
- Duplicate addresses in input
- Unsorted input produces same root as sorted

**Verification**: Unit tests compare computed root against golden values.

---

## Task 2: Leader Selection
**Package**: consensus/leader.go  
**Test**: consensus/leader_test.go  
**Description**: Round-robin proposer selection based on block height and validator set.  
**Acceptance**:
- [ ] `ProposerAtHeight(height uint64, validators []types.Address) types.Address` returns correct proposer
- [ ] Height 0 uses first validator (sorted) — validator index = height % len(validators)
- [ ] Higher heights rotate correctly through validator list
- [ ] Empty validator list returns zero address (should not happen in practice)
- [ ] Single validator is always selected regardless of height

**Edge Cases**:
- Empty validator set returns types.Address{}
- Height wraps correctly with large numbers
- Validators sorted before selection

**Verification**: Unit tests verify rotation pattern for 3+ validators across multiple heights.

---

## Task 3: Config Extension
**Package**: node/config.go  
**Test**: node/config_test.go (or existing tests)  
**Description**: Extend node configuration with consensus parameters.  
**Acceptance**:
- [ ] Add `Validators []types.Address` field to Config struct
- [ ] Add `MaxTxPerBlock int` field to Config (default: 1000)
- [ ] Add `ProposerTimeout time.Duration` field to Config (default: 5s)
- [ ] Add corresponding env var parsing: DSN_VALIDATORS (comma-separated hex), DSN_MAX_TX_PER_BLOCK, DSN_PROPOSER_TIMEOUT
- [ ] Config validation: at least 1 validator required for consensus mode

**Edge Cases**:
- Empty validators list (node runs but cannot produce blocks)
- Invalid address in validators list
- Zero timeout (should default to reasonable value)

**Verification**: Load config from env, verify fields populated correctly.

---

## Task 4: Block Builder
**Package**: consensus/block_builder.go  
**Test**: consensus/block_builder_test.go  
**Description**: Assemble a block from mempool transactions: pull txs, compute merkle roots, calculate fee summary, sign block.  
**Acceptance**:
- [ ] `BuildBlock(proposer types.Address, parentHash types.Hash, height uint64, stateRoot types.Hash, txs []*types.Transaction, validatorRoot types.Hash, signer Signer) *Block` returns complete block
- [ ] Block header fields: Version=1, Height correct, PreviousHash matches parent, StateRoot passed in, TxRoot computed from txs, ValidatorRoot passed in, Timestamp set to current time, Proposer is proposer address
- [ ] TxRoot: deterministic hash of transaction hashes in merkle tree
- [ ] FeeSummary: total fees = sum(tx.MaxFee), shares computed (70/20/10)
- [ ] Signature: sign header hash using proposer private key via ed25519
- [ ] Empty block (no txs) builds successfully with empty tx list and zero tx root

**Edge Cases**:
- Zero transactions (genesis or empty block)
- Transactions sorted by fee in mempool, builder takes top N
- Duplicate txs in input (should not happen, mempool deduplicates)

**Verification**: Compare built block fields against expected values, verify signature verifies.

---

## Task 5: Block Validator
**Package**: consensus/block_validator.go  
**Test**: consensus/block_validator_test.go  
**Description**: Validate a received block: height, previous hash, proposer, signature, merkle roots, fees.  
**Acceptance**:
- [ ] `ValidateBlock(block *types.Block, parentHeader *types.BlockHeader, validators []types.Address, signer Signer) error` returns nil for valid block
- [ ] Height: block.Height == parentHeader.Height + 1
- [ ] PreviousHash: block.Header.PreviousHash == parentHeader.HeaderHash()
- [ ] Proposer: block.Header.Proposer == leader.ProposerAtHeight(block.Height, validators)
- [ ] Signature: ed25519.Verify(proposerPubKey, block.HeaderHash(), block.Signature) passes
- [ ] ValidatorRoot: block.Header.ValidatorRoot == validator_set.ValidatorRoot(validators)
- [ ] TxRoot: computed merkle root of tx hashes matches header
- [ ] FeeSummary: total == sum(tx.MaxFee), shares sum to total
- [ ] Timestamp: block.Header.Timestamp > parentHeader.Timestamp

**Edge Cases**:
- Invalid signature rejected
- Wrong proposer rejected
- Incorrect previous hash rejected
- Tampered tx list (tx root mismatch) rejected

**Verification**: Positive tests with valid blocks, negative tests with tampered blocks.

---

## Task 6: Block Gossip
**Package**: consensus/gossip.go  
**Test**: consensus/gossip_test.go  
**Description**: P2P message type for blocks, seen-set to avoid re-processing, encode/decode.  
**Acceptance**:
- [ ] `BlockMessage` struct contains `Block *types.Block` and `From string` (peer ID)
- [ ] `Encode(w io.Writer)` writes message with block data
- [ ] `Decode(r io.Reader)` reconstructs BlockMessage
- [ ] `SeenSet` type: Add(blockHash), Has(blockHash) bool, Remove(blockHash)
- [ ] Seen-set uses in-memory map with TTL or size limit (max 10000 entries)
- [ ] P2P handler registration: node.p2p.SetBlockHandler(func(BlockMessage))
- [ ] Gossip: broadcast block to all peers except sender

**Edge Cases**:
- Duplicate blocks from multiple peers filtered by seen-set
- Very large block (encode fails if > 1MB)
- Malformed message decode returns error

**Verification**: Two-node test: node A produces block, node B receives via gossip.

---

## Task 7: Chain Persistence
**Package**: persistence/chain.go  
**Test**: persistence/chain_test.go  
**Description**: BoltDB storage for block headers and chain tip.  
**Acceptance**:
- [ ] `ChainStore` struct wraps bbolt.DB with buckets: headers, tip
- [ ] `StoreBlock(block *types.Block) error` saves block header + tx hashes (not full txs) to bbolt
- [ ] `GetBlock(height uint64) (*types.Block, error)` retrieves block by height
- [ ] `GetTip() (height uint64, hash types.Hash, err)` returns current chain tip
- [ ] `SetTip(height uint64, hash types.Hash)` updates tip after valid block
- [ ] Genesis block (height 0) can be stored and retrieved
- [ ] Blocks indexed by height, not by hash (secondary index for hash lookups optional)

**Edge Cases**:
- Database file doesn't exist → create new
- Corrupt data → return error, don't crash
- Height not found → return error with type error

**Verification**: Store 10 blocks, retrieve by height, verify tip updates.

---

## Task 8: Node Producer Goroutine
**Package**: node/node.go  
**Test**: node/node_test.go (integration)  
**Description**: Producer loop checks if node is leader, builds block, gossips, persists.  
**Acceptance**:
- [ ] `StartConsensus()` launches producer and consumer goroutines
- [ ] Producer runs in loop every ProposerTimeout interval
- [ ] Producer checks: am I leader for current height? (use current height from chain store)
- [ ] If leader: pull up to MaxTxPerBlock from mempool, build block, sign, gossip to peers, store to chain
- [ ] If not leader: do nothing until next interval
- [ ] Producer stops gracefully on node.Close()
- [ ] Non-leader doesn't produce blocks

**Edge Cases**:
- No transactions in mempool → produce empty block
- Multiple nodes think they're leader (clock skew) → only one succeeds based on block acceptance
- P2P disabled → skip gossip, only local validation

**Verification**: Run two nodes with same validators, only one produces block.

---

## Task 9: Node Consumer Goroutine
**Package**: node/node.go  
**Test**: node/node_test.go (integration)  
**Description**: Consumer receives blocks via P2P, validates, applies fork choice, persists.  
**Acceptance**:
- [ ] Consumer registered as P2P block handler
- [ ] On receive: check seen-set, add to seen-set
- [ ] Get parent block from chain store (or genesis if height 0)
- [ ] Run `ValidateBlock(block, parentHeader, validators, signer)`
- [ ] If valid: apply transactions to state (state.ApplyTransaction for each), update chain tip
- [ ] If invalid: discard, log reason
- [ ] Fork choice: if block extends current tip, accept; if block is at same height but different hash, ignore (no reorg in this version)
- [ ] Remove applied transactions from mempool

**Edge Cases**:
- Receive block with missing parent (future block) → buffer or ignore
- Invalid block → no state update
- Duplicate blocks → seen-set prevents reprocessing

**Verification**: Two nodes: producer builds/gossips, consumer receives/validates/applies.

---

## Task 10: Integration Test — Two-Node Consensus
**Package**: node/node_test.go  
**Test**: node/node_test.go (new test function)  
**Description**: End-to-end test: two nodes with same validator set, fill mempool, verify consensus.  
**Acceptance**:
- [ ] Create two nodes with same Validators list (including both node addresses)
- [ ] Node A submits 5 transactions to its mempool
- [ ] Node A gossips transactions to Node B
- [ ] Node B receives transactions, submits to its mempool
- [ ] Wait for producer interval → one node produces block
- [ ] Block gossiped to other node
- [ ] Consumer validates and applies block
- [ ] Both nodes have same chain tip height and hash
- [ ] Applied transactions removed from both mempool
- [ ] State roots match between nodes

**Edge Cases**:
- Network partition → both nodes have different tips (eventual consistency not required for PoA)
- Both nodes produce same height → first one wins, second ignored
- Transaction fails during apply → block still valid, failed tx doesn't update state

**Verification**: Run test, assert both nodes have identical tip after 3 block heights.

---

## Dependency Graph

```
Task 1 ──► Task 2 ──► Task 3
                │
                ▼
              Task 4 ──► Task 5 ──► Task 6 ──► Task 7 ──► Task 8 ──► Task 9 ──► Task 10
                          │           │           │
                          └───────────┴───────────┘
```

- Task 1 (validator set) → Task 2 (leader)
- Task 2 + Task 3 → Task 4 (block builder needs leader + config)
- Task 4 → Task 5 (validator needs builder output)
- Task 5 + Task 7 → Task 6 (gossip + chain store)
- Task 6 + Task 7 → Task 8 (producer needs gossip + chain)
- Task 8 → Task 9 (consumer reacts to producer gossip)
- Task 9 → Task 10 (integration test verifies both)