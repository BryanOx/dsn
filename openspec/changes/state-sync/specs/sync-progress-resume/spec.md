# Sync Progress Resume Specification

## Purpose

Persist sync progress with height-based semantics and resume cleanly after restart without double-applying blocks. Today `BlockSyncEngine` tracks progress as a block count (`network/blocksync.go:183` — `lastSyncedHeight += successCount`) instead of the applied tip height, so a restart can resume from the wrong height.

## Requirements

### Requirement: Height-based progress tracking

The sync engine MUST track progress as the height of the last applied block, not as a count of received blocks. Progress MUST be persisted under the existing `block_sync` bucket.

#### Scenario: Progress equals tip

- GIVEN a node applies blocks 41..80 in one batch
- WHEN the batch completes
- THEN persisted progress is 80, regardless of how many blocks the batch contained

### Requirement: Crash-safe resume without double-apply

On restart, the engine MUST resume from the height of the last applied block as recorded by the on-disk tip and the persisted sync progress, and MUST NOT re-apply any block at or below that height. Resume requests MUST start from max(tip height, persisted progress) + 1.

#### Scenario: Restart mid-sync

- GIVEN a node that synced to height 120 before restart
- WHEN the node restarts and peers advertise height 500
- THEN the node requests blocks from 121
- AND no block ≤ 120 is re-applied

#### Scenario: Crash after apply before persist

- GIVEN a crash after block 80 was applied and its tip stored but before progress was persisted
- WHEN the node restarts
- THEN it resumes from the on-disk tip 80, re-fetching from 81
- AND blocks ≤ 80 are never double-applied

### Requirement: Persistence cadence

Progress MUST be persisted with the applied blocks' tip so a crash loses at most the in-flight batch; the existing 100-block cadence MAY be retained, and per-batch persistence SHOULD be used.

#### Scenario: Cadence bound

- GIVEN a node applying blocks in batches of 100
- WHEN a crash occurs mid-batch
- THEN the node re-fetches only the unpersisted tail of the batch on restart

### Requirement: Sync state cleanup on completion

When catch-up or snapshot sync completes and the node is live, persisted sync state MUST be cleared so a later restart does not resume from stale progress.

#### Scenario: Clean completion

- GIVEN a node that finished catching up to the tip
- WHEN sync completes
- THEN the `block_sync`/`sync_state` buckets no longer hold stale progress
- AND a restart stays in live mode
