# Design: State Sync — Automatic Catch-Up and Snapshot Initial Sync

## Technical Approach

Wire four dead seams into one automatic pipeline: (1) new `MsgTypeSyncHeight` so peers advertise tips; (2) read-loop dispatch of block-range frames into the existing single-consumer `blockCh`, with a Node-side `(bool, error)` adapter that re-adds the stripped type byte and applies via `ValidateBlock → applyAcceptedBlock`; (3) complete snapshot serve/fetch (serve handlers, `SyncFromNetwork`, full restore, replay target) with hash-verified reassembly and range-replay fallback; (4) height-based progress, resume from `max(on-disk tip, persisted)+1`. Reuse `state.ChunkSnapshot/VerifyChunk/ReassembleSnapshot`, `ReplayBlocks`, `PeerRecord.SyncHeight`, and the FastSyncEngine query/select/download skeleton.

## Architecture Decisions

| # | Option | Tradeoff | Choice |
|---|--------|----------|--------|
| D1 | `0x43` vs piggyback on ping | Breaking window accepted; piggyback pollutes ping | New `MsgTypeSyncHeight=0x43`, 8-byte BE payload |
| D2 | Adapter at engine boundary vs widening P2P handler | Engine contract stays spec'd; gossip handler untouched; type-byte re-add stays in one place | Node registers `applySyncedBlock` via `blockSync.SetBlockHandler` at construction; `decodeSyncedBlock` prepends `BlockMessageType` — engine never sees wire type |
| D3 | Progress = `LoadTip().Height` vs count/decode | `AtomicStoreBlockAndTip` persists tip per block → crash-safe floor | After each accepted block: `lastSyncedHeight = LoadTip().Height`; persist per batch (≤100) |
| D4 | Resume = `max(tip, progress)+1` | Tip atomic with block | Never re-apply ≤ tip; covers crash-after-apply-before-persist |
| D5 | Range responses via `blockCh` vs inline goroutine | Concurrent applies break determinism | Enqueue on `blockCh`; processor dispatches by type — serialized with gossip blocks |
| D6 | Serve handlers as Node closures vs p2p state access | p2p lacks persistent handle | `SetSnapshotQueryHandler`/`SetSnapshotRequestHandler` closures over Node state; cache latest chunk set; drop nil stubs (p2p.go:654-666) |
| D7 | Full restore vs `RestoreFromSnapshot` only | Restore alone leaves memory empty, tip stale | Restore → rebuild accounts/KV → Commit → verify root vs `LoadCheckpoint` → `setTip` → `StoreTip` |
| D8 | Replay target = max peer SyncHeight vs `n.currentHeight` (bug) | currentHeight=0 on fresh node | `ReplayBlocks(snapshotHeight+1, blockSync.MaxPeerHeight())` |
| D9 | Auto trigger gated on `FastSyncEnabled` | Flag inverted vs auto intent (default false) | `stateRoot==0 && p2p && FastSyncEnabled` → snapshot; else block-range (automatic regardless) |

## Data Flow

```
setTip → publishSyncHeight() → Broadcast [0x43][8B]
registerConn → SendTo SyncHeight (provider)
[0x43]→readLoop→ UpdateSyncHeight (monotonic, persist) → NotifyPeerHeight
[0x30]→readLoop→ go HandleBlockRangeRequest(payload,PeerID)      (serve ≤100)
[0x31]→readLoop→ blockCh→ processor→ HandleBlockRangeResponse→ applySyncedBlock each
applySyncedBlock: decodeSyncedBlock→ reject ≤finalized→ ValidateBlock(snapshot/revert)→ applyAcceptedBlock→(true,nil)|(false,err)
```

Catch-up (async; 30s ticker + `NotifyPeerHeight` wakeup): `resume=max(tip,persist)+1; target=max(peer heights)` → windowed request `[resume,resume+99]` → apply serially, advance → ≥target → clear progress → publish. 10s tick re-requests timed-out pending; invalid block → `ReportFailure(from,5)`, stop, resume narrowed; partial → narrowed re-request.

Snapshot: `SyncFromNetwork` → `fastSync.Start()` (Query→Select→Download→Verify→Restore→Replay→`SyncLive`) → simple seam: `blockSync.Start()`+`StartConsensus` (exists). Failure → `fallBackToLiveSync` (−3) → range replay from on-disk tip.

## File Changes

| File | Action | What |
|------|--------|------|
| `network/message.go` | Modify | `MsgTypeSyncHeight=0x43` + `knownMessageTypes` + `maxPayloadByType` |
| `network/p2p.go` | Modify | Dispatch 0x30/0x31/0x43 in readLoop; responses on `blockCh`; processor type-dispatch; `syncHeightProvider`; remove nil snapshot stubs (654-666) |
| `network/peer.go` | Modify | `UpdateSyncHeight` monotonic setter; persist via 30s tick |
| `network/blocksync.go` | Modify | `HandleBlockRangeRequest(payload,PeerID)`; height progress; async `startCatchUp`; `NotifyPeerHeight`/`MaxPeerHeight`; timeout + narrowed re-request; completion clears progress |
| `network/snapshot.go` | Modify | `SnapshotInfo.StateRoot [32]byte` (wire 60→92 B) |
| `node/node.go` | Modify | Register adapter + serve handlers + provider; `publishSyncHeight` in `setTip`; missed-proposal → `NotifyPeerHeight`; full restore callback; `lastSnapshotHeight` tracking |
| `node/fastsync.go` | Modify | `FastSync()` step-3 → `SyncFromNetwork`; `SyncFromNetwork` delegates to `fastSync.Start()` (drop infoCh clobber) |
| `integration/replay_cert_test.go` | Modify | Catch-up determinism test (real block-range over P2P) |
| `integration/` (new sibling) | Create | Restart-resume + fall-behind tests |

## Interfaces / Contracts

```go
// Engine boundary — Node implements BlockSyncEngine's contract (blocksync.go:29):
//   onBlock func(data []byte) (bool, error)  // validate+apply; (accepted, err)
func (n *Node) applySyncedBlock(data []byte) (bool, error) {
    block, err := decodeSyncedBlock(data) // choke point: prepend BlockMessageType, then DecodeBlockMessage
    if err != nil || block.Header.Height <= n.finalizedHeight { return false, err }
    parent, prev := loadParentHeader(block) // expectedPrevHash from disk tip chain
    snap := n.state.Snapshot()
    if err := consensus.ValidateBlock(block, parent, prev, n.state, n.hasher, n.vm, n.cfg.BlockTimeSec); err != nil {
        n.state.RevertToSnapshot(snap)
        return false, err
    }
    n.applyAcceptedBlock(block) // same pipeline as live; root verified inside ValidateBlock
    return true, nil
}
```

New engine APIs: `NotifyPeerHeight(h uint64)`, `MaxPeerHeight() uint64`, `resumeFrom()`.

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | SyncHeight: malformed (≠8B)→−2; stale→keep; monotonic→update | peer/message tests |
| Unit | Range round-trip (request→serve ≤100→decode→apply); height progress; narrowed re-request | blocksync tests |
| Unit | Adapter: type-byte re-add decode; root mismatch → abort+penalize | node tests |
| Unit | Snapshot serve: query answered; chunk served; unknown chunk → no response | snapshot tests |
| Integration | Catch-up determinism: A mines 5, fresh B catches up over P2P; roots match per height | replay_cert extension |
| Integration | Late-joiner reaches tip; restart mid-sync resumes tip+1; crash-after-apply-before-persist | new sibling |
| Gate | 3-node `TestConvergence_ConsensusLoop` green; identical roots post-catch-up | existing |

## Threat Matrix

N/A — no shell/subprocess/VCS/PR-automation/executable-classification boundary touched. All rows inapplicable: documentation-like paths (nothing executed), git repo selection (no git invocation), commit state (no VCS automation), push state (none), PR commands (none). P2P frame dispatch is a network boundary, not a routing/shell boundary; no RED tests manufactured.

## Migration / Rollout — 3 chained PRs (≤400 authored lines each)

| Slice | Scope | Gates |
|-------|-------|-------|
| PR1 Live catch-up | message type; read-loop dispatch; adapter + registration; height progress; `startCatchUp`; `publishSyncHeight`; missed-proposal; unit + late-joiner + determinism tests | convergence green |
| PR2 Snapshot sync | `SnapshotInfo.StateRoot`; serve handlers; `SyncFromNetwork`; full restore; replay target; auto trigger; snapshot test | PR1 green |
| PR3 Resilience | restart-resume (block_sync + sync_state); partial/narrow + timeout re-request; stale/malformed hardening; fall-behind + resume tests | PR2 green |

Chain: PR1→feature branch; PR2→PR1 branch; PR3→PR2 branch; retarget until child diffs clean. `Decision needed before apply: Yes` per slice (ask-on-risk).

Rollback: revert slice commits; restart nodes. No DB migration — buckets/keys additive; only in-code key deletes. Old binaries treat 0x43 as legacy-tx → decode fail → −2 (accepted, nothing deployed).

## Open Questions

- [ ] `FastSyncEnabled` naming inverted vs auto-snapshot intent (default false → block-range only). Confirm intent: keep opt-in, or flip default?
- [ ] Chunk cache bound: latest snapshot only in memory — acceptable v1?
