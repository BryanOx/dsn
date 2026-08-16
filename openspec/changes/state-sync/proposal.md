# Proposal: State Sync — Automatic Catch-Up and Snapshot Initial Sync

## Intent

A validator that joins late or falls behind is stuck at height 0: peers advertise `SyncHeight` 0 (never written), block-range messages are dropped (`network/p2p.go:378-380`), `BlockSyncEngine.SetBlockHandler` never registered, snapshot serve handlers nil, and `node/fastsync.go:74` unimplemented — recovery needs manual operator action today.

## Scope

**In**: `SyncHeight` propagation; block-range wiring (read-loop dispatch, handler adapter, progress fix); automatic trigger (height gap / missed proposal → request + replay); snapshot sync (serve + fetch + restore + replay); replay determinism; late-joiner + convergence tests.

**Out** (deferred to a separate breaking change): P2P crypto identity/handshake/encryption (PeerID = TCP-address trust model); RPC auth; snapshot→live handoff seam (follow-up; both mechanisms in scope); consensus liveness, metrics, RPC, CI.

## Non-Goals

Security items + handoff seam (above). Breaking wire window accepted: new message types land without backward compat (nothing deployed).

## Capabilities

All new — `openspec/specs/` empty:

- `peer-height-propagation`: tip-height wire transport + `SyncHeight` maintenance.
- `block-range-sync`: serve/fetch ranges; catch-up wiring; auto trigger.
- `snapshot-sync`: chunked serve + fetch/verify/restore + replay handoff.
- `sync-progress-resume`: persisted progress; restart-resume without double-apply.

## Approach

1. Write `Peer.SyncHeight` from tip on-block; publish via wire transport (piggyback vs new message — design decision).
2. Wire `p2p.go:378-380` to `BlockSyncEngine`; adapter returns `(bool, error)`, re-adds stripped type byte, applies sequentially via `ValidateBlock → applyAcceptedBlock`; fix count-based `lastSyncedHeight`.
3. Auto-trigger: `startCatchUp` fires once peers report heights; add missed-proposal detection.
4. Serve side (chunk `state.LoadSnapshot`) + finish fetch (`SyncFromNetwork`); failures fall back to range replay.
5. Determinism: reuse `ReplayBlocks` shared `ApplyTransaction`; verify root per block.

## Edge Cases

Join mid-epoch (replay from 1 / last snapshot); restart-resume (skip applied heights); fall-behind (extend-only, no k=6 reorg); partial responses (narrow, retry, penalize).

## Risks

1. Consensus regression on stabilized pipeline (f34dd20/6bec8a6) — Med — convergence + epoch tests gate; single-consumer apply.
2. Replay nondeterminism forks nodes — Med — extend `replay_cert_test`; per-block root verify; CI.
3. Scope exceeds 400-line budget — High — chained PR slices; ask-on-risk per slice.
4. Breaking wire types; mixed-version peers — Low (nothing deployed) — accepted; documented rollback.
5. Snapshot path is dead code — Med — hash-verified reassembly; fallback to range replay.

## Slicing (chained PRs, ≤400 lines each)

1. Live catch-up: SyncHeight + range wiring + auto trigger + progress fix (late-joiner repro).
2. Snapshot sync: serve chunking + fetch completion + restore/replay handoff.
3. Resilience: restart-resume, partial ranges, fall-behind tests.

Each slice keeps convergence green; `Decision needed` per slice.

## Rollback Plan

Revert slice commits; restart nodes. No DB migration — progress buckets additive; old binaries drop unknown types.

## Dependencies

None external: bbolt, `state.LoadSnapshot`, `ReplayBlocks`.

## Acceptance Criteria

- [ ] 3-node convergence test stays green.
- [ ] Late-joiner fixed: node at height ≥1 reaches tip, no operator action.
- [ ] Identical state roots across nodes post-catch-up.
- [ ] Fresh node completes snapshot restore + replay (or verified fallback).
- [ ] Restart mid-sync resumes without double-apply.
