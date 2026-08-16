# Block Range Sync Specification

## Purpose

Wire block-range request/response into the read loop and apply engine, fix the wire-format frame mismatch, and make catch-up automatic so a lagging node reaches the tip without operator action. Today the read loop drops both range types (`network/p2p.go:378-380`), the engine's block handler is registered only in tests (`network/blocksync.go:87`), and served block bytes lack the type byte `DecodeBlockMessage` requires (`network/blocksync.go:406`, `consensus/gossip.go:118`).

## Requirements

### Requirement: Read-loop dispatch of block-range messages

The P2P read loop MUST dispatch `MsgTypeBlockRangeRequest` to `BlockSyncEngine.HandleBlockRangeRequest` and `MsgTypeBlockRangeResponse` to `BlockSyncEngine.HandleBlockRangeResponse`, replacing the current no-op case.

#### Scenario: Incoming range request

- GIVEN a peer requests blocks 1..100
- WHEN the frame reaches the read loop
- THEN the request handler responds with the available blocks
- AND the frame is not dropped

#### Scenario: Incoming range response

- GIVEN a peer sends a range response
- WHEN the frame reaches the read loop
- THEN the response handler processes each block through the apply callback

### Requirement: Production block handler registration

The node MUST register a block-apply handler on the `BlockSyncEngine` at startup via `SetBlockHandler`; registration MUST NOT be limited to tests.

#### Scenario: Startup wiring

- GIVEN a node starting with the sync engines constructed (`node/node.go:174`)
- WHEN startup completes
- THEN the engine's apply callback is non-nil and validates then applies blocks

### Requirement: Wire frame-type preservation

The sync apply path MUST re-add the message type byte that `encodeBlockAsWireFromBlock` strips before passing payloads to `DecodeBlockMessage`, so replay decodes without a frame-type mismatch.

#### Scenario: Round-trip decode

- GIVEN a block encoded by `encodeBlockAsWireFromBlock` (type byte stripped)
- WHEN the sync handler re-adds the type byte and calls `DecodeBlockMessage`
- THEN the block decodes with its height and state root intact

### Requirement: Sequential deterministic apply

Sync blocks MUST be applied one at a time through `ValidateBlock` then `applyAcceptedBlock`, and replay MUST reuse the shared `ApplyTransaction` path with per-block state-root verification, so identical inputs produce identical state roots.

#### Scenario: Converged roots

- GIVEN three nodes syncing the same block sequence from the same genesis
- WHEN each node reaches the same tip height
- THEN all three report identical state roots

#### Scenario: Root mismatch aborts

- GIVEN a block whose applied state root differs from its header root
- WHEN the block is applied
- THEN application stops with an error and the offending peer is penalized

### Requirement: Automatic catch-up for lagging nodes

A node whose tip is below the highest advertised peer `SyncHeight` MUST start block-range catch-up automatically, for both joining and live nodes, without operator action. Catch-up MUST be extend-only: it MUST NOT induce a reorg of finalized (k=6) blocks.

#### Scenario: Late joiner reaches tip

- GIVEN a fresh node at height 0 connected to a peer at height 500
- WHEN the peer's SyncHeight is observed
- THEN the node requests ranges 1..500 and applies them sequentially
- AND the node reaches height 500 without operator action

#### Scenario: Fall-behind during steady state

- GIVEN a live node whose tip falls 50 blocks below a peer mid-epoch
- WHEN the gap is observed
- THEN the node catches up from its tip+1
- AND blocks below the finalized height are never re-fetched or reorged

### Requirement: Partial and invalid responses

On a malformed response or an invalid block, the engine MUST penalize the sender, stop the current range, and resume from the last accepted height. Partial responses SHOULD be re-requested narrowed to the missing heights.

#### Scenario: Invalid block in range

- GIVEN a range response containing one invalid block
- WHEN the handler hits it
- THEN the peer is penalized and the remaining blocks in the range are rejected
- AND retry continues from the last accepted height
