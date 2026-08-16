# Peer Height Propagation Specification

## Purpose

Propagate each node's current tip height over a dedicated wire message so peers learn who is ahead and can trigger catch-up. Today `Peer.SyncHeight` is initialized to 0 and never updated (`network/peer.go:347`), so `BlockSyncEngine.startCatchUp` never sees a target (`network/blocksync.go:220`).

## Requirements

### Requirement: Dedicated SyncHeight wire message

The P2P protocol MUST define a new dedicated message type for tip-height transport, distinct from ping/PEX/block/vote/snapshot types. The payload MUST carry the sender's current tip height as an unsigned 64-bit value. This is a breaking wire addition; no backward compatibility is required (nothing is deployed).

#### Scenario: Height advertised over the new type

- GIVEN a connected pair of nodes A and B using the new message type
- WHEN A sends a SyncHeight message carrying its tip height 42
- THEN B decodes the message without touching ping/PEX handlers
- AND B records A's height as 42

#### Scenario: Malformed payload

- GIVEN a node receiving a SyncHeight frame whose payload is not 8 bytes
- WHEN the frame is dispatched
- THEN the node penalizes the sender and ignores the message
- AND the peer's recorded SyncHeight is unchanged

### Requirement: Height publication on tip advance and connect

A node MUST publish its tip height on every applied tip and SHOULD publish it immediately upon establishing a peer connection. Publication MUST occur after `applyAcceptedBlock` advances the tip so announced heights are never ahead of the applied state.

#### Scenario: On-block publication

- GIVEN node A has applied a block advancing its tip to height H
- WHEN the tip update completes
- THEN A sends its SyncHeight to all connected peers

#### Scenario: Fresh connection

- GIVEN node B dials node A
- WHEN the connection is established
- THEN A sends its current SyncHeight to B without waiting for the next block

### Requirement: Peer SyncHeight maintenance

A node MUST store the latest received SyncHeight on the corresponding `Peer` record and MUST reject non-monotonic updates (a lower height than the last recorded one). The stored height MUST survive restart via the existing `PeerRecord` persistence.

#### Scenario: Monotonic update

- GIVEN B has recorded A's SyncHeight as 42
- WHEN A sends SyncHeight 50
- THEN B updates A's record to 50

#### Scenario: Stale height rejected

- GIVEN B has recorded A's SyncHeight as 50
- WHEN A sends SyncHeight 30
- THEN B keeps 50
