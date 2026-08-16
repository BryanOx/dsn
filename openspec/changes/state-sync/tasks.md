# Tasks: State Sync — Auto Catch-Up & Snapshot Initial Sync

## Review Workload Forecast

Changed lines est.: PR1 330–400 · PR2 250–310 · PR3 190–260
Over budget: PR1 borderline, PR2/PR3 within
Chained PRs: Yes — feature-branch chain per design (PR1→PR2→PR3)
Delivery: ask-on-risk · Chain: pending (confirm feature-branch-chain)
Decision per slice: PR1 Yes · PR2 Yes · PR3 Yes (ask-on-risk)

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: Medium

**Slice record — ACTUALS vs forecast:**
- PR1 closed 581 changed lines (504+/77−) vs 330–400 forecast → re-sliced (Option B)
- PR1b closed 462 changed lines (450+/12−) vs ~300 forecast → **size:exception approved** (tests stay with code)
- PR1c closed 155 changed lines (148+/7−) vs ~15+tests forecast → production 44 (dispatch gap found), tests 111
- PR2 closed ~969 changed lines (836+/133−) vs 250–310 forecast → **size:exception REQUIRED** (see PR2 slice record; deep fixes found during apply: nil-parent replay crash, stale-timestamp replay rejection, peer Connected state on outbound connect)
- PR3 closed 10 commits, 12 files, 1,245 changed lines (1166+/79−) vs 190–260 forecast → **size:exception approved** (tests stay with code; 2 pre-existing production bugs fixed + 1 pre-existing shutdown race fixed after gatekeeper review — see PR3 notes)
- Chain topology: PR1 (closed) → PR1b (closed) → PR1c (closed) → PR2 → PR3

**Work units** (test·harness·rollback):
- PR1 Live catch-up · PR2 Snapshot — test `go test ./network/... ./node/... -count=1` · harness `go test -tags=integration ./integration/ -run TestConvergence_ConsensusLoop` · rollback revert PR1/PR2
- PR3 Resilience — test `go test ./network/... -count=1` · harness `go test -tags=integration ./integration/ -run TestRestartResume` · rollback revert PR3

## PR1 — Live Catch-Up

**RED tests** (write first):
- [x] 1.1 message_test.go: 0x43 registered; 8B cap; malformed→err
- [x] 1.2 peer_test.go: SyncHeight monotonic (stale out, high kept)
- [x] 1.3 blocksync_test.go: request→serve≤100→apply; progress=tip
- [x] 1.4 node_test.go: decodeSyncedBlock re-adds type byte; root mismatch aborts
- [x] 1.5 integration/replay_cert_test.go: A mines 5, fresh B catches up; roots match

**GREEN production**:
- [x] 2.1 network/message.go: MsgTypeSyncHeight=0x43, knownMessageTypes, maxPayloadByType
- [x] 2.2 network/peer.go: UpdateSyncHeight monotonic setter, persist 30s tick
- [x] 2.3 network/p2p.go: dispatch 0x30/0x31/0x43; responses→blockCh; type-dispatch (single consumer)
- [x] 2.4 network/blocksync.go: startCatchUp (ticker+Notify); resume..+99 apply; ≥target → clear+publish; progress=LoadTip().Height, persist≤100
- [x] 2.5 node/node.go: applySyncedBlock adapter + SetBlockHandler (~174); publishSyncHeight setTip/connect; missed-proposal → NotifyPeerHeight

**Gates**:
- [x] 3.1 3-node TestConvergence_ConsensusLoop green, roots identical — **PASS 0.31s (PR1c commit)**
- [x] 3.2 catch-up determinism green (-tags=integration) — **PASS 0.41s (TestReplayCert_CatchUpSync)**

## PR1c — Transport Double-Framing Fix (added post-PR1b)

**Discovered during PR1b apply**: `FrameMessage` returns `[len][type][payload]` but `SendTo`/`Broadcast` prepend their own length prefix → call sites passing `FrameMessage` output double-frame; the read loop parses the inner length prefix as type `0x00` and drops the message.

**RED tests** (write first):
- [x] 1c.1 gossip_test.go: GossipTransaction tx reaches peer (RED: dropped → timeout)
- [x] 1c.2 peer_discovery_test.go: PEX request/response round-trips through read loop (RED: dropped → timeout)

**GREEN production**:
- [x] 2c.1 network/gossip.go: GossipTransaction sends [0x02][payload] via SendTo
- [x] 2c.2 network/discovery.go: sendPeerListRequest/Response send [0x21]/[0x22][payload] via SendTo
- [x] 2c.3 network/p2p.go: read-loop dispatch cases for 0x21/0x22 + SetBootstrapDiscovery seam (dispatch gap found — PEX dropped even when single-framed)

**Gate**:
- [x] 3c.1 network unit suite green; full regression (build/vet/make test/make test-integration) green

**Out of scope, noted for follow-up**: `P2PNode.GossipTransaction` (p2p.go:555) legacy `[len][raw-tx]` framing works only because Version 1's first byte is 0x00; Version ≥ 0x02 collides with known message types → recommend aligning to `[len][0x02][tx]` in PR2/PR3.

## PR2 — Snapshot Sync

**RED tests**:
- [x] 4.1 snapshot_test.go: query answered; chunk+hash; unknown → none
- [x] 4.2 fastsync_test.go: SyncFromNetwork download+verify; mismatch → error
- [x] 4.3 node/config_test.go: default assertion → FastSyncEnabled true
- [x] 4.4 integration: fresh node auto snapshot+replay

**GREEN production**:
- [x] 5.1 network/snapshot.go: SnapshotInfo.StateRoot [32]byte (wire 60→92B)
- [x] 5.2 network/p2p.go: SetSnapshotQuery/RequestHandler closures; drop nil stubs
- [x] 5.3 node/fastsync.go+node.go: step3 → SyncFromNetwork → fastSync.Start(); fail → fallback; replay=max(peer height)
- [x] 5.4 node/node.go: restore: rebuild→Commit→verify root→setTip→StoreTip
- [x] 5.5 trigger: stateRoot==0 && FastSyncEnabled → snapshot; else range
- [x] 5.6 node/config.go:108 + config/config.go:268: default → true

**Gate**:
- [x] 6.1 convergence + snapshot integration green — `TestSnapshotSync_FreshNodeAutoSync` PASS 5.6s ×10 (stability loop), `TestReplayCert_CatchUpSync` PASS 0.41s, full `make test` + `make test-integration` green

### PR2 Fixes Beyond Spec (found during apply — RED at integration level, see apply-progress)

- `consensus/block_validator.go`: nil-parent guards (height-continuity, prev-hash) for fresh-node/snapshot-restore replay — was a nil-pointer panic + ErrWrongPreviousHash on block 1/6
- `consensus/block_validator.go`: `ValidateBlockForSync` — skips the ±5s wall-clock drift check for replayed (historical) blocks; every chain-relative check unchanged
- `node/node.go`: `publishSyncHeight` on the proposer path (`finalizeLocalBlock`) — single-validator networks never announced past the connect-time poke
- `node/fastsync.go`: exported `StoreStateSnapshot()` (seeds snapshot + checkpoint at a node's live tip; test seam, mirrors unit-test helper)
- `network/p2p.go`: `registerConn` sets `Peer.State = PeerConnected` — outbound `Connect` and the accept path bypass `ConnectToPeer`; snapshot query broadcast filters on connected state
- `integration/snapshot_sync_test.go`: wire-level handshake wait (B-side SyncHeight ≥ 4) replaces the connect poke; no epoch-boundary mining (shared registration advances epoch counter — see apply-progress)

## PR3 — Resilience

**RED tests**:
- [x] 7.1 blocksync_test.go: resume=max(tip,persist)+1; no double-apply
- [x] 7.2 blocksync_test.go: invalid → ReportFailure(from,5), stop, narrowed; partial → narrow
- [x] 7.3 p2p_test.go: malformed 0x43 → −2; height unchanged
- [x] 7.4 integration/sync_resume_test.go: restart mid-sync tip+1; crash-after-apply-before-persist
- [x] 7.5 integration/fall_behind_test.go: live 50-behind; no reorg ≤ finalized

**GREEN production**:
- [x] 8.1 network/blocksync.go: resumeFrom=max(tip,persist)+1; per-batch persist; completion clears; timeout+partial narrow
- [x] 8.2 network/p2p.go+peer.go: stale/malformed hardening (−2, keep height)
- [x] 8.3 network/snapshot.go: chunk cache latest-only (OQ2 resolved)
- [x] 8.4 integration: late-joiner + fall-behind suites

**Gate**:
- [x] 9.1 convergence + resume green — **PASS 11.6s** (`TestRestartResume` 0.62s, `TestSnapshotSync_FreshNodeAutoSync` 5.51s, `TestReplayCert_CatchUpSync` 0.41s, `TestConvergence_ConsensusLoop` 0.31s, `TestFallBehind_BlockSyncCatchUp` 4.72s) + full `make test` (18 pkgs ok) + `make test-integration` green. **Shutdown-race fix (ca16d24) verified: `TestConvergence_ConsensusLoop -count=10` → 10/10 PASS** (was flaky at 2c453ad: block processor could apply after Close returned and the DB closed — see apply-progress PR3 gatekeeper section)

## Rollback

Revert slice commits; restart nodes. No DB migration (buckets additive). 0x43 → legacy decode fail → −2 (accepted). Out: P2P crypto identity, RPC auth, handoff seam.
