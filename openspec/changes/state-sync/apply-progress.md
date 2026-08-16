# Apply Progress: State Sync — PR1b (tasks 1.4, 1.5, 2.3, 2.5) + PR1c (transport fix) + PR2 (snapshot sync) + PR3 (resilience)

## Status

`applyState: all_done` — PR1b/PR1c committed; PR2 committed (size:exception); **PR3 committed (10 commits, size:exception approved) + gate-verified + gatekeeper correction applied**. All tasks 1.x–9.1 closed. `next_recommended: sdd-verify` (full change across PR1/PR1b/PR1c/PR2/PR3 slices).

## Slice record

| Slice | Branch | Commits | Lines (changed) | Forecast | Decision |
|-------|--------|---------|-----------------|----------|----------|
| PR1 | feat/state-sync/catchup → wiring | 55dbe92, ff4ae28, 133e6bb | 581 (504+/77−) | 330–400 | re-sliced (Option B) |
| PR1b | feat/state-sync/wiring | 911c02e | 462 (450+/12−) | ~300 | **size:exception approved** (tests with code) |
| PR1c | feat/state-sync/transport-fix (off wiring) | a1f7409 | 155 (148+/7−) | ~15+tests | noted in summary |
| PR2 | feat/state-sync/snapshot | UNCOMMITTED → committed PR2 | ~969 (836+/133−) | 250–310 | **size:exception REQUIRED — ask-on-risk** |
| PR3 | feat/state-sync/resilience | 10 commits (facd19c → 4f4bc5d) | 1,245 (1166+/79−) | 190–260 | **size:exception approved** (tests stay with code; 2 pre-existing bugs + 1 shutdown race fixed) |

Chain topology: `PR1 (closed) → PR1b (closed) → PR1c (closed) → PR2 → PR3`

---

## PR1b — Completed Tasks (commit 911c02e)

- [x] 1.4 node_test.go: decodeSyncedBlock re-adds type byte; root mismatch aborts
- [x] 1.5 integration/replay_cert_test.go: A mines 5, fresh B catches up; roots match
- [x] 2.3 network/p2p.go: dispatch 0x30/0x31/0x43; responses→blockCh; type-dispatch (single consumer)
- [x] 2.5 node/node.go: applySyncedBlock adapter + SetBlockHandler; publishSyncHeight setTip/connect; missed-proposal → NotifyPeerHeight

### PR1b Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `network/p2p.go` | Modified | kind-tagged block jobs; `syncHeightProvider`; connect-time 0x43 announce; 0x30/0x31/0x43 dispatch; single-consumer `startBlockProcessor` |
| `network/blocksync.go` | Modified | range request reframed to `[type][payload]` (was double-framed) |
| `node/node.go` | Modified | adapter + height-provider wiring; missed-proposal kick; `finalizeLocalBlock` full-block persistence; `publishSyncHeight` |
| `node/sync.go` | Created | `decodeSyncedBlock`, `loadParentHeader`, `applySyncedBlock` |
| `node/node_test.go` | Modified | helpers + 1.4 tests |
| `integration/replay_cert_test.go` | Modified | `TestReplayCert_CatchUpSync` |

## PR1c — Transport Double-Framing Fix (commit a1f7409)

**Root cause**: `FrameMessage` returns `[len][type][payload]`; `SendTo`/`Broadcast` prepend their own length prefix → call sites passing `FrameMessage` output emitted `[len][len][type][payload]`; the read loop parsed the inner length prefix as the type byte (`0x00`), hit the legacy transaction branch and dropped every message. **Plus a dispatch gap**: the read loop had no case for `MsgTypePeerListRequest` (0x21)/`MsgTypePeerListResponse` (0x22), so PEX was dropped even when single-framed.

### PR1c Tasks

- [x] 1c.1 gossip_test.go: GossipTransaction tx reaches peer — RED (2.10s timeout, dropped) → GREEN (0.10s)
- [x] 1c.2 peer_discovery_test.go: PEX round-trip through read loop — RED (2.10s timeout, dropped) → GREEN (0.11s)
- [x] 2c.1 network/gossip.go: `[0x02][payload]` via SendTo
- [x] 2c.2 network/discovery.go: `[0x21]`/`[0x22][payload]` via SendTo
- [x] 2c.3 network/p2p.go: 0x21/0x22 read-loop dispatch + `SetBootstrapDiscovery`
- [x] 3c.1 full regression green (below)

### PR1c Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `network/gossip.go` | Modified | `GossipTransaction` single-framed (was dead-but-broken: no production callers) |
| `network/discovery.go` | Modified | `sendPeerListRequest`/`sendPeerListResponse` single-framed |
| `network/p2p.go` | Modified | `bootstrapDisc` field + `SetBootstrapDiscovery`; read-loop cases 0x21/0x22 |
| `network/gossip_test.go` | Modified | `TestGossipEngine_TransactionReachesPeer` |
| `network/peer_discovery_test.go` | Modified | `TestDiscovery_PeerListExchangeReachesPeer` |

**Out of scope, recommended follow-up**: `P2PNode.GossipTransaction` (p2p.go:555) legacy `[len][raw-tx]` framing works only because a Version-1 tx starts with byte 0x00; Version ≥ 0x02 collides with known message types and misroutes. Align to `[len][0x02][tx]` in PR2/PR3.

---

## PR2 — Completed Tasks (4.1–4.4, 5.1–5.6, gate 6.1)

- [x] 4.1 network/snapshot_test.go: snapshot query answered; chunk+hash verified; unknown snapshot → no chunk
- [x] 4.2 node/fastsync_test.go: SyncFromNetwork download + verify; root mismatch → error (storeSnapshotOnNode pattern, lines 49–83)
- [x] 4.3 node/config_test.go: default assertion → FastSyncEnabled true (defaults flipped)
- [x] 4.4 integration/snapshot_sync_test.go: fresh B auto-discovers A's snapshot, downloads + verifies + restores, replays the tail, converges on A's state root
- [x] 5.1 network/snapshot.go: SnapshotInfo.StateRoot [32]byte (wire 60→92B)
- [x] 5.2 network/p2p.go: SetSnapshotQueryHandler/SetSnapshotRequestHandler serve closures (snapshot.go wiring), nil stubs dropped
- [x] 5.3 node/fastsync.go + node/node.go: startup step → SyncFromNetwork → fastSync.Start(); query failure → fallback to live sync; replay target = max(own, blockSync.MaxPeerHeight())
- [x] 5.4 node/node.go: restore handler rebuild → Commit → root verify → setTip(snap.Height,{}) → StoreTip{StateRoot,Timestamp}
- [x] 5.5 trigger: persistent.GetStateRoot()==(types.Hash{}) && FastSyncEnabled → snapshot sync; else block-range catch-up
- [x] 5.6 node/config.go:108 + config/config.go:268: FastSyncEnabled default → true
- [x] 6.1 gate: `TestSnapshotSync_FreshNodeAutoSync` PASS 5.6s (10× stability loop), `TestReplayCert_CatchUpSync` PASS 0.41s, full `make test` (18 pkgs ok) + `make test-integration` green

### PR2 Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `network/snapshot.go` | Modified | SnapshotInfo.StateRoot [32]byte; serve closures registered via p2p wiring |
| `network/snapshot_test.go` | Created | 4.1 query/chunk tests (227 lines) |
| `network/fastsync.go` | Modified | SyncFromNetwork + defaultSnapshotTimeout=30s; query retry w/ backoff; fallback to live sync; state-machine reentrancy fixes (Advance/proceedFromSelection/HandleSnapshotChunk/handleQueryTimeout/verifyDownloadedSnapshot/fallBackToLiveSyncLocked) |
| `network/p2p.go` | Modified | registerConn sets Peer.State=PeerConnected (outbound Connect + accept path); snapshot query/chunk dispatch; io.ReadFull framing in readLoop |
| `network/blocksync.go` | Modified | serve/recv range handling; no-op cleanup |
| `node/fastsync.go` | Modified | exported `StoreStateSnapshot()` (seed snapshot+checkpoint at live tip); 5.3 wiring |
| `node/fastsync_test.go` | Created | 4.2 SyncFromNetwork tests (150 lines) |
| `node/node.go` | Modified | restore handler (rebuild→commit→verify→setTip→StoreTip); state-change handler → blockSync.Start on SyncLive; publishSyncHeight on proposer path (finalizeLocalBlock); startup triggers |
| `node/sync.go` | Modified | applySyncedBlock → ValidateBlockForSync (stale-tolerant) |
| `node/startup.go` | Modified | startup phase wiring for fast-sync path |
| `node/config.go` + `config/config.go` | Modified | FastSyncEnabled default → true |
| `node/config_test.go` | Modified | 4.3 default assertion |
| `consensus/block_validator.go` | Modified | nil-parent guards; ValidateBlockForSync (drift-skipping variant) |
| `integration/snapshot_sync_test.go` | Created | 4.4 end-to-end gate test (112 lines) |

## Fixes Beyond Spec (RED at integration level, root-caused during PR2 apply)

1. **nil-parent replay crash**: `validateBlock` dereferenced a nil parent header for fresh-node replay (block 1) / snapshot-restore replay (block 6) — `parentHeader.Height+1` panicked inside the read-loop goroutine and killed sync silently. Guard added (`parentHeader != nil`), consistent with the pre-existing nil-safe monotonicity check. Was latent because the prior catch-up tests always had the parent on disk.
2. **ErrWrongPreviousHash at the restore boundary**: `loadParentHeader` deliberately returns `ZeroHash` for a missing parent (node/sync.go doc), but the validator still enforced the link hash. Guard: skip the PreviousHash check when `expectedPrevHash == (types.Hash{})` and height > 0. Live path unaffected — `handleBlockMessage` does fork detection (`isFork`) before validation.
3. **Stale-timestamp replay rejection**: `types.ValidateTimestamp` enforces ±5s of NOW (live-gossip freshness). B replays blocks A produced seconds earlier; whenever handshake+download+restore exceeded 5s, block 6 was rejected (`invalid timestamp`) and B re-requested forever. Fix: `ValidateBlockForSync` skips only the wall-clock drift check; epoch, proposer, validator set, hashes, commit proof and re-executed state root are unchanged.
4. **Peer never marked Connected on outbound Connect**: `P2PNode.Connect` + the accept path bypass `ConnectToPeer`, so `AddPeer` left `State=PeerDisconnected` and the snapshot engine's query broadcast (`GetPeersByState(PeerConnected)`) never reached A. The integration test's `p.State = PeerConnected` poke was compensating for the production gap — removed once `registerConn` set the state itself.
5. **Proposer never announces its height**: only the receiver path (`applyAcceptedBlock`) published the tip; `finalizeLocalBlock` (single-validator networks always take this path) never did — B could converge to A-1 and never learn the final block. Added `publishSyncHeight()` at the end of `finalizeLocalBlock`.
6. **Epoch math vs test harness**: the shared `registerValidatorsInState` helper consumes the first epoch boundary (ProcessEpochTransition at height 100 + ActivationEpoch = currentEpoch+1), so a chain built with it can never cross a real boundary. The integration test does NOT mine across an epoch boundary — the snapshot is seeded explicitly via `StoreStateSnapshot()`.

## Work Unit Evidence

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./consensus/... ./network/... ./node/... -count=1` → all ok (consensus 0.2s, network 6.7s, node 25.6s). Snapshot gate: `go test -tags=integration ./integration/ -run TestSnapshotSync_FreshNodeAutoSync -count=1` → PASS 5.6s |
| Runtime harness command/scenario and exact result | `go test -tags=integration ./integration/ -run 'TestSnapshotSync_FreshNodeAutoSync\|TestReplayCert_CatchUpSync' -count=1 -v` → both PASS (5.6s / 0.41s). Debug trail (instrumented): 0x43(9) → query → snapshotInfo(5, 1158B) → chunk 1/1 → verify → restore at 5 → startCatchUp resume=6 target=9 → served [6,9] (4 blocks) → applied → B=9 roots match. Stability: 10× consecutive PASS (pre-cleanup), 8× post-cleanup |
| Rollback boundary | PR2 uncommitted: `git checkout -- <tracked>` + delete the three new test files restores HEAD. If committed: revert the PR2 commit; blocksync/p2p changes are additive (no migration) |

## Verification (full regression, run after all fixes + debug-print removal)

- `go build ./...` — clean
- `go vet ./...` — clean
- `make test` (`go test ./... -cover -count=1 -timeout=120s`) — 18 packages ok
- `make test-integration` (`go test -tags=integration -count=1 -timeout=300s ./integration/...`) — ok 8.8s
  - **6.1 `TestSnapshotSync_FreshNodeAutoSync` — PASS 5.6s (10× stability loop, post-cleanup 8×)** ✓
  - **3.2 `TestReplayCert_CatchUpSync` — PASS 0.41s** ✓ (regression baseline intact)
  - full integration suite green

## Notes / Deviations

- PR2 actual footprint ~969 changed lines vs 250–310 forecast — **3× over budget**. Root causes: (a) the four production bugs above each needed a fix + reasoning, (b) `StoreStateSnapshot` + three new test files, (c) the engine state-machine reentrancy fixes carried into this slice. Following the PR1b precedent (size:exception approved, tests stay with code), **PR2 requires the same explicit size:exception decision before committing** (ask-on-risk was pre-flagged).
- All DEBUG instrumentation removed after GREEN (12 prints across network/node + test poll log); verified with `grep -rn DEBUG` = 0 in non-test sources.
- Debugging artifacts /tmp/opencode/{fastsync.go,node.go}.bak (pre-instrumentation backups) — safe to delete.
- `go vet` clean; no AI attribution; conventional commits only.

---

## PR3 — Completed Tasks (7.1–7.5, 8.1–8.4, gate 9.1)

- [x] 7.1 blocksync_test.go: resume=max(tip,persist)+1; no double-apply
- [x] 7.2 blocksync_test.go: invalid → ReportFailure(from,5), stop, narrowed; partial → narrow
- [x] 7.3 p2p_test.go: malformed 0x43 → −2; height unchanged
- [x] 7.4 integration/sync_resume_test.go: restart mid-sync tip+1
- [x] 7.5 integration/fall_behind_test.go: **50**-block offline gap (spec scenario); no reorg ≤ finalized
- [x] 8.1 network/blocksync.go: resumeFrom=max(tip,persist)+1; per-batch persist; completion clears; timeout+partial narrow
- [x] 8.2 network/p2p.go: read-loop 0x43 hardening (−2, keep height); typed [0x02][tx] GossipTransaction (S2 fold-in)
- [x] 8.3 network/snapshot.go: chunk cache latest-only (OQ2)
- [x] 8.4 integration: restart-resume + fall-behind suites
- [x] 9.1 gate: 5 integration harness tests green (11.6s; convergence -count=10 → 10/10 PASS post-fix), full `make test` (18 pkgs ok) + `make test-integration` green

### PR3 Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `node/sync.go` | Modified | already-finalized blocks (≤ finalizedHeight) accepted silently, no peer penalty (S1) |
| `network/fastsync.go` | Modified | snapshot serve cache latest-only (8.3/OQ2); gofmt-aligned fields |
| `network/p2p.go` | Modified | 0x43 read-loop exact-8B penalty (−2, keep height) (7.3/W2); typed [0x02][tx] GossipTransaction (S2 fold-in) |
| `network/blocksync.go` | Modified | 10s timeout re-request + pending-window clearing; invalid-block ReportFailure(from,5), stop, persist, narrowed resume (8.1/W4/S3); oversized-range ReportFailure(from,2) (S4); partial → narrowed re-request |
| `network/blocksync_test.go` | Modified | 7.1/7.2 RED tests + pins (resume floor, no double-apply, persisted-progress-wins, partial→narrow) |
| `network/p2p_test.go` | Modified | 7.3 malformed-sync-height test |
| `network/fastsync_test.go` | Modified | 8.3 serve-cache test (latest-only, stale dropped) |
| `node/node_test.go` | Modified | S1 test: already-finalized synced block skipped, no penalty |
| `node/recovery.go` | Modified | **pre-existing bug fix**: recovery verifies state root against TIP header root, not last epoch-boundary checkpoint (see below) |
| `integration/sync_resume_test.go` | Created | 7.4 restart-mid-sync suite |
| `integration/fall_behind_test.go` | Created | 7.5 fall-behind suite (50-block gap, spec-aligned) |
| `integration/helpers.go` | Modified | corrected stale `MineBlock` doc comment (helper builds, does NOT store) |

### PR3 Commits (feature-branch-chain, targets PR2 head)

| Commit | Unit | Lines |
|--------|------|-------|
| `facd19c` fix(node): already-finalized sync blocks accepted (S1) | 31 (31+/0−) | |
| `533f868` feat(network): latest-only snapshot serve cache (8.3) | 188 (188+/0−) | |
| `326d973` fix(network): wire framing hardening (W2, S2 fold-in) | 207 (136+/71−) | |
| `f53dcbd` test(network): pins (7.1, W5, 7.2 partial) | 243 (243+/0−) | |
| `6aae867` feat(network): timeout re-request + narrowed invalid-block resume (8.1, W4, S3, S4) | 234 (231+/3−) | |
| `09a80d7` style(network): gofmt fastsync.go | 8 (4+/4−) | |
| `eb6b1bf` test(integration): restart-resume + fall-behind suites (7.4/7.5) | 236 (233+/3−) | |
| `d00d9d9` fix(node): recovery verifies state root against tip header (pre-existing bug) | 10 (10+/0−) | |
| `ca16d24` fix(network): join block processor before node close to prevent late db commit (gatekeeper correction) | 89 (89+/0−) | |
| `4f4bc5d` test(integration): align fall-behind gap with spec scenario (50 blocks) | 12 (7+/5−) | |

## Fixes Beyond Spec (found during PR3 apply — RED at integration level, root-caused, production fixes)

1. **`Recover()` misclassified clean restarts as corruption** (node/recovery.go, pre-existing at HEAD): `Recover()` compared the persistent state root against the *latest checkpoint* root, but checkpoints are only written at epoch boundaries while `CommitState` runs at every block. On any chain with transactions, the tip root legitimately differs from the last checkpoint root → "state root doesn't match checkpoint, and no snapshot available" → node failed to boot. Empty-block chains passed only because empty blocks keep the root identical. Fix: verify the persistent root against the **tip block header root** first (the authoritative check); the checkpoint/snapshot path now only applies when the tip check fails. Exposed by 7.5 (`TestFallBehind_BlockSyncCatchUp`), which restarts a node with a transaction-bearing chain.
2. **`MineBlockWithTxs` doc comment promised storing, implementation never stored** (integration/helpers.go, pre-existing at HEAD, first committed in 796fe0f): the helper only *builds* blocks (via `BuildBlock`, which commits state as a side effect) — it never calls `StoreBlockHeader`/`StoreTip`/`applyAcceptedBlock`. Every existing caller only compares state roots, so the gap was invisible. The aborted-run 7.5 test asserted `a.CurrentHeight()==12` after calling it → could never pass. Fixed the test to mine via the real consensus loop (`a.StartConsensus()` + `Eventually(height)`), matching the 7.4 pattern, and corrected the stale comment.
3. **7.5 test missing `b2.StartConsensus()` before `Connect`** (adopted from aborted run): the restarted node must start its engine before connecting so it reacts to the handshake announcement (`node.New` does not run the Phase14 startup transition; production calls it via `Run()`). The 7.4 suite already had this step; the 7.5 suite did not — added with the same comment.

## Work Unit Evidence (PR3)

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./network/... -count=1` → ok 6.9s; `go test ./node/... -count=1` → ok 24.9s (pre-commit-splitting full suite); `go build ./...` + `go vet ./...` clean before and after all commits |
| Runtime harness command/scenario and exact result | `go test -tags=integration ./integration/ -run 'TestRestartResume\|TestSnapshotSync_FreshNodeAutoSync\|TestReplayCert_CatchUpSync\|TestConvergence_ConsensusLoop\|TestFallBehind' -count=1` → PASS 11.6s (0.62s / 5.51s / 0.41s / 0.31s / 4.72s). RED→GREEN: S1 (node), W2 (0x43 penalty), S2 (GossipTransaction fold-in), the recovery fix, and the shutdown-race fix each demonstrated RED against the pre-fix code before the production change |
| Rollback boundary | Revert `d00d9d9` (recovery tip-root check) restores the pre-existing boot behavior; revert `eb6b1bf` removes both integration suites; revert `ca16d24` restores the pre-fix (racy) shutdown; `git revert` of the 10 commits in order restores `2c453ad` PR2 head. `openspec/` is untracked bookkeeping — never committed |

## Verification (full regression, run after final commits)

- `go build ./...` — clean
- `go vet ./...` — clean
- `make test` (`go test ./... -cover -count=1 -timeout=120s`) — 18 packages ok
- `make test-integration` (`go test -tags=integration -count=1 -timeout=300s ./integration/...`) — ok 13.6s (integration), 6.1s (soak), 0.5s (staging)
  - **9.1 gate harness — PASS 11.6s** (5 tests: TestRestartResume 0.62s, TestSnapshotSync_FreshNodeAutoSync 5.51s, TestReplayCert_CatchUpSync 0.41s, TestConvergence_ConsensusLoop 0.31s, TestFallBehind_BlockSyncCatchUp 4.72s) ✓
  - **post-fix stability: `TestConvergence_ConsensusLoop -count=10` → 10/10 PASS** (was intermittently flaky at 2c453ad — see gatekeeper section below)
  - full integration suite green

## Notes / Deviations (PR3)

- PR3 actual footprint 1,245 changed lines vs 190–260 forecast — **~4.8× over budget**, **size:exception approved** (same precedent as PR1b/PR2: tests stay with code; 2 pre-existing production bugs + 1 pre-existing shutdown race fixed). All 10 commits are individually within the 400-line review budget.
- S2 (GossipTransaction typed framing) was judged mechanical and folded into `326d973` — RED demonstrated (0.10s byte-shift corruption before, 0.10s clean after).
- Pre-existing gofmt misalignment in `network/p2p.go` (engine-references struct fields) exists at HEAD too — out of scope, noted, not fixed.
- The gate harness command from the orchestrator (4 tests) was extended with `TestFallBehind_BlockSyncCatchUp` (7.5) — the gate record above includes it since it exercises the 50-block offline-gap contract.
- `openspec/` remains untracked (bookkeeping only). No AI attribution; conventional commits only.

## Gatekeeper Correction (post-PR3 review, commit ca16d24)

**Failure caught**: `TestConvergence_ConsensusLoop` panicked intermittently (`panic: commit state failed: persistent commit: database not open at height 5`). Confirmed PRE-EXISTING at `2c453ad` (not a PR3 regression) but squarely a resilience bug in PR3 scope — it made gate 9.1 flaky.

**Root cause (verified against code)**: in `P2PNode`:
1. `startBlockProcessor` (single consumer) selects randomly between `<-n.stopCh` and `<-n.blockCh`; on stopCh it closes blockCh and returns — but it can pick a buffered job after stopCh fired, process it (block apply → DB commit), then exit.
2. `P2PNode.Close` closed stopCh and returned immediately — nothing waited for the processor.
3. `Node.Close` closes the persistent DB LAST, after `p2p.Close` returned → late apply hit the closed DB → panic.

**Fix (one scoped correction)**:
- Added `blockDone chan struct{}` closed by the processor goroutine (`defer close`) when it fully exits.
- `P2PNode.Close` closes stopCh, then joins on `blockDone` with a bounded `blockProcessorJoinTimeout = 2s` before returning — no apply can run after Close returns.
- Guarded `handleBlock`/`enqueueBlockRangeResponse` with a `stopCh` case: a read loop that passed its stop check could otherwise send on the processor-closed `blockCh` and panic ("send on closed channel") — same shutdown-race class, same work unit.
- Test: `TestP2PNode_CloseJoinsBlockProcessor` — enqueues a job, holds the handler in flight, asserts Close does NOT return while the apply is in flight, releases, asserts Close returns and no apply ran after. **RED without the join** (`Close returned while a block apply was still in flight`), GREEN with it.

**Proof**: `go test -tags=integration ./integration/ -run TestConvergence_ConsensusLoop -count=10` → **10/10 PASS** (0.31s each); full gate 11.6s; network suite 7.2s; `make test` 18 pkgs ok; `make test-integration` all ok.

**Follow-up (commit 4f4bc5d)**: aligned the fall-behind offline gap to the spec scenario (50 blocks below a peer, was 40) — spec-compliance fix found while auditing the correction scope.

---

## Remediation Pass — W1 (flake) + W2 (docs drift) — commits 14b3378 + e485cbd

**Trigger**: verify report on the PR3 head (`4f4bc5d`) found (a) `TestSyncFromNetwork_DownloadAndVerify` flaky (1-in-8 failures, "write: connection reset by peer" / "peer not found") and (b) docs drift (`docs/OPERATIONS.md` claiming `FastSyncEnabled` default `false`, actual `true`).

### W1 — Root cause (PROVEN, three layers)

1. **Transport framing corruption** (the user-visible flake): `Broadcast`/`SendTo` (p2p.go) and `sendToPeer` (snapshot.go) emitted every frame as TWO `conn.Write` calls (4-byte big-endian length prefix, then payload). `Broadcast` serialized only against itself (connMu write-lock held across both writes), but `SendTo`/`sendToPeer` took only connMu-RLock for the lookup and no write lock, so a `SendTo`-style sender could interleave its length prefix between another sender's prefix and payload. The reader then parsed a corrupt length > `MaxPayloadSize` (1 MiB) and the read loop tore the connection down → reset-by-peer between snapshot query and download, then peer-not-found on the download manager's re-request. **Proven with /tmp/opencode/framing_repro**: unserialized (two-write) sends produced 38,974–77,380 bogus/desynced frames per run; single-write frames produced 599–600/600 clean. **Fix**: one buffer + one `conn.Write` per frame in all three send paths (net.Conn per-call atomicity; wire format bytes identical — no compat break).
2. **`Stop()` stopCh race** (found by `-race`): `FastSyncEngine.Stop()` did `close(e.stopCh); e.stopCh = make(...)` with no lock, racing goroutines reading `e.stopCh` (queryPeers/handleQueryRetry/launchDownloadManager). **Fix**: all `e.stopCh` field access under `e.mu`; channel recreation moved from `Stop()` into `Start()`; goroutines capture the channel once under RLock.
3. **Latent deadlocks unmasked by the (now locked) `Stop()`** — the old lock-free `Stop()` masked them for years:
   - `verifyDownloadedSnapshot` returned **without `e.mu.Unlock()`** on the no-chunks, reassembly-error, and hash-mismatch branches (the mismatch branch also re-entered the unlocked `fallBackToLiveSync()` in the no-chunks branch → self-deadlock). Fixed: all three branches use `fallBackToLiveSyncLocked()` + explicit unlock.
   - `handleQueryRetry` selected on `stopCh` **while holding `e.mu`** — lock-order inversion vs the locked `Stop()`. Fixed: backoff wait moved outside the lock.

### W1 Evidence

| Evidence | Value |
|---|---|
| Flake repro | verifier: 1-in-8 failures on `TestSyncFromNetwork_DownloadAndVerify`; framing_repro: unserialized 38,974–77,380 bogus frames/run vs single-write 599–600/600 clean |
| RED test | `TestP2PNode_ConcurrentWritesKeepFraming` (p2p_test.go): 4 goroutines × 150 frames via `SendTo` + 4 × 150 via `sendToPeer` on one conn, asserts 1200 clean frames, 0 corrupt lengths — FAIL on pre-fix code (`expected: 1200 actual: 19737367`), 10/10 GREEN after |
| HashMismatch hang | locked `Stop()` exposed the verify lock-leak: test hung >60s at `b.Close()` → `Stop()` blocked on `e.mu`; log showed "snapshot hash mismatch" printed then nothing. Base passed 5.1s only because lock-free `Stop()` bypassed the leaked lock. After the leak fix: 3/3, 5/5 `-race` PASS |
| Stress | `TestSyncFromNetwork_DownloadAndVerify -count=10` → 10/10 (was 1-in-8); `-count=5 -race` → 5/5 clean; `-race` network suite: base had 2 failing tests (TestAdversarial_ConcurrentStartStop, TestPeerDiscovery_StartIsIdempotent) → after fix **1** (TestAdversarial_ConcurrentStartStop now passes; TestPeerDiscovery_StartIsIdempotent remains, pre-existing, out of scope) |
| Full regression | `go build ./...` + `go vet ./...` clean; `make test` 18 packages ok (node 25.3s — the earlier 120s node timeout WAS the HashMismatch hang); full `make test-integration` ok 14.1s; gate harness 5 tests ok 6.2s |

### W2 — Docs drift (mechanical)

`docs/OPERATIONS.md` 4.2 table (line 507) and 4.5 TOML example (line 581) said `FastSyncEnabled` default `false`; actual default is `true` (config/config.go:268, node/config.go:108, asserted by node/config_test.go:104–147). Fixed both to `true` + "fresh node with empty state auto-triggers it". No other lines touched.

**Additional drift found (NOT in scope, flagged)**: OPERATIONS.md:831/844/3187 and ARCHITECTURE.md:837/901 still claim "network fast sync is NOT implemented" / `SyncFromNetwork` returns "fast sync from network not yet implemented" — contradicted by the implemented + verified `SyncFromNetwork` (node/fastsync.go:244). Recommend a follow-up docs pass.

### Commits (feat/state-sync/resilience, NOT pushed)

| Commit | Unit | Lines |
|---|---|---|
| `14b3378` | fix(network): serialize frame writes and fix fastsync stop/verify races (p2p.go, snapshot.go, fastsync.go, p2p_test.go) | 238 (194+/44−) |
| `e485cbd` | docs(operations): fast-sync default is now on (OPERATIONS.md) | 4 (2+/2−) |

Rollback boundary: revert `e485cbd` (docs) and/or `14b3378` (code) individually; both are self-contained. `openspec/` remains untracked bookkeeping.
