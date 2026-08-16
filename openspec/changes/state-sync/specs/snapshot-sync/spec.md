# Snapshot Sync Specification

## Purpose

Complete the snapshot path: serve stored snapshots, fetch and verify them over the network, restore, and replay to the tip so a fresh node skips long range replay. Today the serve handlers return nil (`network/p2p.go:654-666`), network fetch is unimplemented (`node/fastsync.go:74`, and `SyncFromNetwork` at `node/fastsync.go:224` returns an error), while snapshot files exist on disk at epoch boundaries (`node/node.go:968-979`).

## Requirements

### Requirement: Snapshot serving

A node with stored snapshots MUST answer snapshot queries with the latest snapshot's metadata and MUST serve chunk requests from the stored snapshot data via `state.LoadSnapshot`/`state.LoadCheckpoint`. Serve handlers MUST NOT return nil when snapshots exist.

#### Scenario: Query answered

- GIVEN a node with a stored snapshot at height 100
- WHEN a peer broadcasts a snapshot query
- THEN the node replies with snapshot info for height 100

#### Scenario: Chunk served

- GIVEN a peer requests chunk 3 of the snapshot at height 100
- WHEN the chunk data exists locally
- THEN the node sends chunk 3 with its chunk hash

#### Scenario: Unknown chunk

- GIVEN a peer requests a chunk index beyond the snapshot's chunk count
- WHEN the request is handled
- THEN the node does not respond and no state is corrupted

### Requirement: Network fetch completion

`SyncFromNetwork` MUST implement the full download path — snapshot discovery, peer tracking, chunk download with re-request, reassembly, and verification — instead of returning the current "not fully implemented" error.

#### Scenario: Fresh node fetches snapshot

- GIVEN a fresh node with peers advertising a verified snapshot
- WHEN `SyncFromNetwork` runs
- THEN all chunks are downloaded, reassembled, and verified
- AND the reassembled hash matches the advertised `SnapshotHash`

### Requirement: Automatic trigger for fresh nodes

A node with empty state MUST automatically start snapshot sync when peers advertise snapshots. The `FastSyncEnabled` config flag MAY force-disable snapshot sync; live block-range catch-up MUST remain automatic regardless.

#### Scenario: Empty state auto-trigger

- GIVEN a fresh node with no state root connected to a peer with snapshots
- WHEN peers advertise snapshot info
- THEN the node starts snapshot sync without operator action

#### Scenario: Disabled by config

- GIVEN a node with `FastSyncEnabled=false`
- WHEN snapshot discovery would otherwise start
- THEN snapshot sync does not start
- AND block-range catch-up still runs

### Requirement: Hash-verified restore with replay handoff

The node MUST verify the snapshot hash and the restored state root before accepting a snapshot, then replay blocks from `snapshotHeight+1` to the tip through the shared replay path with per-block root verification, then transition to live. Seamless snapshot→live transition polish is out of scope for this change.

#### Scenario: Restore then replay

- GIVEN a verified snapshot at height 100 and a peer tip at 150
- WHEN restore completes and roots match
- THEN blocks 101..150 replay sequentially
- AND the node enters live mode at height 150

### Requirement: Fallback on snapshot failure

Any snapshot failure — query timeout, chunk verification failure, hash mismatch, or restore error — MUST fall back to block-range replay rather than stalling at the current height.

#### Scenario: Hash mismatch falls back

- GIVEN a reassembled snapshot whose hash differs from the advertised hash
- WHEN verification fails
- THEN the node falls back to block-range catch-up
- AND it still reaches the peer tip via range replay
