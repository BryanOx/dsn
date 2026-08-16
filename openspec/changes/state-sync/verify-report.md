```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ba82c1134c6847da16f83a93b1a518591dadfac01c980c72473fe50c2bcd65ad
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 18/18
scenarios: 28/28
test_command: go test ./... -cover -count=1 -timeout=120s
test_exit_code: 0
test_output_hash: sha256:bef5a8e6eb3538d13968c88e9c5b121b59b0f7598d84a7fda9ebf34cb228337c
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# SDD Verify Report — state-sync (full change)

**Change**: state-sync (complete chain `55dbe92`…`4f4bc5d`, 18 commits: PR1 `55dbe92,ff4ae28,133e6bb` + PR1b `911c02e` + PR1c `a1f7409` + PR2 snapshot sync + PR3 resilience incl. `ca16d24` shutdown-race fix)
**Version**: tasks.md 39/39 tasks closed (PR2 11 + PR3 10 re-sliced and closed; apply-progress closed with size-exception records)
**Mode**: Strict TDD (test runner `make test` / `make test-integration`; gentle-ai CLI unavailable — manual status contract, counts hand-verified against retrieved specs, envelope vocabulary self-declared)

**Verification scope**: FULL change at HEAD `4f4bc5d`. This report supersedes the PR1/PR1b/PR1c-only report (verdict was `pass_with_warnings`, 13/18 requirements, 16/28 scenarios, evidence `sha256:6de23815…`).

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 39 |
| Tasks complete | 39 |
| Tasks incomplete | 0 |
| Requirements (4 specs) | 18/18 |
| Scenarios (4 specs) | 28/28 |
| CRITICAL / blockers | 0 |
| WARNING | 2 |
| SUGGESTION | 2 |

All 39 tasks checked `[x]`; every closed task maps to an existing test file that was executed and passed (see TDD Compliance). No unchecked core task remains.

### Build & Tests Execution

**Build**: ✅ Passed — `go build ./...` exit 0 (empty output, `sha256:e3b0c442…`); `go vet ./...` exit 0 (empty output).

**Tests (full unit suite)**: ✅ `go test ./... -cover -count=1 -timeout=120s` exit 0 — every package `ok` (network 7.17s/59.3% cov, node 25.33s/55.2% cov). Output hash `sha256:bef5a8e6…`.

**Integration suite**: ✅ `go test -tags=integration -count=1 -timeout=300s ./integration/...` exit 0 (integration 14.32s, soak 6.12s, staging 0.52s). Output hash `sha256:09f730b5…`.

**Regression gates (run by verifier)**:
- `TestConvergence_ConsensusLoop -count=5` — **5/5 PASS** (1.55s; run 5 logs a transient pre-convergence root-mismatch sample — eventual-consistency poll pattern, test PASSes, roots converge and `CompareStateRoots` passes)
- Full gate harness `-run 'TestRestartResume|TestSnapshotSync_FreshNodeAutoSync|TestReplayCert_CatchUpSync|TestConvergence_ConsensusLoop|TestFallBehind'` — **5/5 PASS** (11.69s); snapshot test logged `seeded snapshot at height 4`, `snapshot verified, 1157 bytes`, `state restored from snapshot at height 4`

**Focused pins (run by verifier, network + node, all PASS)**:
`TestReadLoop_MalformedSyncHeightPenalizes` (short −2 / oversized −4 / height kept / valid → 50), `TestBlockSyncEngine_ResumeTipFloorAfterCrash`, `TestBlockSyncEngine_CatchUpRequestStartsAtTipPlusOne` (wire start 81), `TestBlockSyncEngine_ResumePrefersPersistedProgress`, `TestBlockSyncEngine_InvalidBlockPenalizesStopsAndResumes`, `TestBlockSyncEngine_TimeoutReRequestsPending`, `TestBlockSyncEngine_PartialResponseNarrowsNextWindow`, `TestSnapshotServe_QueryAnswered/ChunkServed/UnknownChunkNoResponse`, `TestFastSyncEngine_ServeSnapshotCachesLatest`, `TestP2PNode_CloseJoinsBlockProcessor` (shutdown-race join), `TestP2PNode_GossipTransactionTypedFraming`, `TestP2PNode_OutboundReceivesBlock/Vote`, `TestDecodeSyncedBlock_ReaddsTypeByte`, `TestApplySyncedBlock_RootMismatchAborts`, `TestApplySyncedBlock_AlreadyFinalizedSkipped`, `TestSyncFromNetwork_HashMismatch`.

**Coverage**: package-level — network 59.3%, node 55.2% (`-cover`). Per-file coverage tooling not configured → changed-file coverage skipped (informational, not a failure).

### Spec Compliance Matrix

#### peer-height-propagation (3 req / 6 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Dedicated SyncHeight wire message | Height advertised over new type | `network/message_test.go > TestMsgTypeSyncHeightRegistered, TestParseMessageSyncHeight` + `integration/replay_cert_test.go > TestReplayCert_CatchUpSync` (B learns A=5 via 0x43) | ✅ COMPLIANT |
| Dedicated SyncHeight wire message | Malformed payload | `p2p_test.go > TestReadLoop_MalformedSyncHeightPenalizes` — wire read-loop path: short payload → score −2 height kept; oversized → −4 height kept; valid 8-byte → monotonic 50 (PR3 task 7.3 closure). Previously PARTIAL; now fully covered on the wire path | ✅ COMPLIANT |
| Height publication on tip advance and connect | On-block publication | `node/node.go:1290` in `applyAcceptedBlock` (after setTip) AND `node/node.go:1081` in `finalizeLocalBlock` (proposer path — previously SUGGESTION, now fixed); exercised in every convergence gate | ✅ COMPLIANT |
| Height publication on tip advance and connect | Fresh connection | `p2p.go` registerConn announce via provider; `TestReplayCert_CatchUpSync` — B's ONLY height source is A's connect-time 0x43 (A stopped consensus before B dialed) | ✅ COMPLIANT |
| Peer SyncHeight maintenance | Monotonic update | `network/peer_test.go > TestPeer_UpdateSyncHeight` | ✅ COMPLIANT |
| Peer SyncHeight maintenance | Stale height rejected | same test (stale 50 < 100 kept; equal no-op) | ✅ COMPLIANT |

#### block-range-sync (6 req / 9 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Read-loop dispatch of block-range messages | Incoming range request | `blocksync_test.go > TestBlockSyncEngine_RequestServeMax100`; `p2p.go` dispatch; end-to-end `TestReplayCert_CatchUpSync` | ✅ COMPLIANT |
| Read-loop dispatch of block-range messages | Incoming range response | `blocksync_test.go > TestBlockSyncEngine_HandleBlockRangeResponse`; `p2p.go` → blockCh → single-consumer processor | ✅ COMPLIANT |
| Production block handler registration | Startup wiring | `node/node.go` SetBlockHandler(n.applySyncedBlock) at construction; integration proves handler functional | ✅ COMPLIANT |
| Wire frame-type preservation | Round-trip decode | `node/node_test.go > TestDecodeSyncedBlock_ReaddsTypeByte` (re-encode byte-identity) | ✅ COMPLIANT |
| Sequential deterministic apply | Converged roots | `TestReplayCert_CatchUpSync` (B roots == A roots); gate 3.1; single-consumer `blockCh` (D5) | ✅ COMPLIANT |
| Sequential deterministic apply | Malformed payload | `blocksync_test.go > TestBlockRangeRequestInvalidData` | ✅ COMPLIANT |
| Request/response bounds | Max-100 window | `blocksync_test.go > TestBlockSyncEngine_RequestServeMax100` (serve cache latest-only, bounded) | ✅ COMPLIANT |
| Invalid-block resilience | Invalid block penalized + narrowed resume | `TestBlockSyncEngine_InvalidBlockPenalizesStopsAndResumes` (peer −5, stop, resume narrowed from persisted tip) | ✅ COMPLIANT |
| Invalid-block resilience | Timeout re-request | `TestBlockSyncEngine_TimeoutReRequestsPending`; `TestBlockSyncEngine_PartialResponseNarrowsNextWindow` | ✅ COMPLIANT |

#### snapshot-sync (5 req / 8 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Snapshot serving | Query answered | `snapshot_test.go > TestSnapshotServe_QueryAnswered` (PASS 0.11s) | ✅ COMPLIANT |
| Snapshot serving | Chunk served | `TestSnapshotServe_ChunkServed` (PASS 0.11s) | ✅ COMPLIANT |
| Snapshot serving | Unknown chunk | `TestSnapshotServe_UnknownChunkNoResponse` (PASS 1.10s) | ✅ COMPLIANT |
| Network fetch completion | Fresh node fetches snapshot | `node/fastsync_test.go > TestSyncFromNetwork_DownloadAndVerify` — download 2 chunks, reassemble, verify hash (PASS in full suite and 2/2 isolated runs; 1/8 flaky failure observed, see W1) | ✅ COMPLIANT (W1) |
| Automatic trigger for fresh nodes | Empty state auto-trigger | `integration/snapshot_sync_test.go > TestSnapshotSync_FreshNodeAutoSync` — B (fresh, FastSyncEnabled) auto-discovers, downloads+verifies+restores, replays tail 5..8, converges on A's root, equal heights | ✅ COMPLIANT |
| Automatic trigger for fresh nodes | Disabled by config | Gate `node/startup.go:581-610` (fresh genesis + FastSyncEnabled → SyncModeFastSync else SyncModeNormal); `node/config_test.go` asserts default `true`; `integration/sync_resume_test.go > TestRestartResume_AfterMidSyncGap` (FastSyncEnabled=false → B routed to block sync, converges via range) | ✅ COMPLIANT (S2) |
| Hash-verified restore with replay handoff | Restore then replay | `TestSnapshotSync_FreshNodeAutoSync` (restore at height 4 → replay 5..8 → live, `CompareStateRoots` equal) + `TestReplayCert_SnapshotReplay` (replay-from-snapshot, full suite PASS) | ✅ COMPLIANT |
| Fallback on snapshot failure | Hash mismatch falls back | `TestSyncFromNetwork_HashMismatch` — error contains "snapshot", state stays empty (`types.Hash{}`), no corrupt restore; range path to tip proven by `TestFallBehind_BlockSyncCatchUp` | ✅ COMPLIANT |

#### sync-progress-resume (4 req / 5 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Height-based progress tracking | Progress equals tip | `TestBlockSyncEngine_ResponseProgressEqualsTip` — 5-block batch → LastSyncedHeight 45 (not 5), persisted; fresh engine on same DB loads 45, `resumeFrom()` = 46 | ✅ COMPLIANT |
| Crash-safe resume without double-apply | Restart mid-sync | `TestBlockSyncEngine_CatchUpRequestStartsAtTipPlusOne` (wire-level: first request starts 81, ends 180; never ≤ 80) + `TestRestartResume_AfterMidSyncGap` (B restarts, resumes tip+1, converges, roots equal) | ✅ COMPLIANT |
| Crash-safe resume without double-apply | Crash after apply before persist | `TestBlockSyncEngine_ResumeTipFloorAfterCrash` — tip 80 on disk, no progress → floor 80, `resumeFrom()` = 81; `TestBlockSyncEngine_ResumePrefersPersistedProgress` — max(tip, progress): progress 120 beats tip 100 → 121 | ✅ COMPLIANT |
| Persistence cadence | Cadence bound | per-batch `persistProgress()` (blocksync.go:285, after each batch ≤ MaxBlocksPerRangeResponse); crash mid-batch re-fetches only unpersisted tail (tip-floor tests above) | ✅ COMPLIANT |
| Sync state cleanup on completion | Clean completion | `clearProgress()` (blocksync.go:304 on `last >= target`, impl 446-459) deletes the `block_sync` key; completion path exercised end-to-end by convergence/restart suites (nodes stay live, no stale resume) | ✅ COMPLIANT (S3) |

### TDD Compliance (apply-progress cross-checked against executed tests)

| Slice | RED test pins | GREEN (executed by verifier) | Status |
|-------|---------------|------------------------------|--------|
| PR1 (12 tasks) | `TestMsgTypeSyncHeightRegistered`, `TestParseMessageSyncHeight`, `TestPeer_UpdateSyncHeight`, `TestDecodeSyncedBlock_ReaddsTypeByte` | all PASS (full suite + focused) | ✅ |
| PR1b (4 tasks) | read-loop dispatch tests | `TestBlockSyncEngine_RequestServeMax100`, `HandleBlockRangeRequest/Response` PASS | ✅ |
| PR1c (2 tasks) | integration replay/catch-up | `TestReplayCert_CatchUpSync` PASS (gate 5/5) | ✅ |
| PR2 (11 tasks) | `TestSnapshotServe_*`, `TestSyncFromNetwork_DownloadAndVerify/HashMismatch`, `TestSnapshotSync_FreshNodeAutoSync`, `TestReplayCert_SnapshotReplay` | all PASS (focused + gate 5/5 + full suite) | ✅ |
| PR3 (10 tasks) | `TestReadLoop_MalformedSyncHeightPenalizes`, resume tests, `TestP2PNode_CloseJoinsBlockProcessor`, `TestP2PNode_GossipTransactionTypedFraming`, `TestFallBehind_BlockSyncCatchUp` | all PASS (focused pins + gate 5/5) | ✅ |

No RED-claimed test was found absent; every GREEN-claimed test function exists and passed in execution.

### Design Coherence

| Design decision (design.md) | Implementation | Status |
|------------------------------|----------------|--------|
| 0x43 dedicated wire type + wire-path penalty (D1) | `network/message.go` 0x43, read-loop validate+penalize; verified in `TestReadLoop_MalformedSyncHeightPenalizes` | ✅ |
| `blockCh` single-consumer apply (D5) | `p2p.go` processor goroutine; `TestP2PNode_CloseJoinsBlockProcessor` proves no apply after Close | ✅ |
| Progress = on-disk tip height (D3) | blocksync.go `lastSyncedHeight = tip.Height` after batch; `ResponseProgressEqualsTip` | ✅ |
| Tip-floor resume max(tip, persisted)+1 (D4/W5) | `resumeFrom()`; three dedicated tests PASS | ✅ |
| FastSync states Querying→Downloading→Verifying→Restoring→Replaying→Live | `node/fastsync.go`; fresh-node integration test exercises full cycle | ✅ |
| Serve cache latest-only, bounded window | blocksync.go serve path; `RequestServeMax100` | ✅ |
| Snapshot→live seam = simple blockSync.Start (polish out of scope) | `node/startup.go` Phase14 transition; no handoff-seam code leaked (diff touches only network/, node/, integration/, consensus/block_validator.go, config/config.go) | ✅ |
| Out-of-scope: P2P crypto identity, RPC auth | absent from 18-commit diff (verified by file inventory) | ✅ |

### Issues

**CRITICAL** — none.

**WARNING**
- **W1 — `TestSyncFromNetwork_DownloadAndVerify` flake (1/8 runs).** One isolated run failed with `write: connection reset by peer` between query and download (`peer not found` on re-request); passed in full `make test` and in 7/7 subsequent isolated runs (incl. `-count=5`). Likely root cause: duplicate-dial connection replacement in `registerConn` closing the live conn. Gate-validating, not blocking, but a CI-noise risk worth a targeted fix.
- **W2 — `docs/OPERATIONS.md` drift (change-introduced).** Lines 507 and 581 document `FastSyncEnabled` default `false`; the chain flips the default to `true` (`config/config.go:268`, `node/config.go:108`). Operators reading the docs get the wrong default.

**SUGGESTION**
- **S1 — disabled-by-config snapshot scenario has no dedicated gate unit test.** `startup_test.go` only contains `TestStartup_PersistsGenesisState`; the `FastSyncEnabled=false → no snapshot sync` branch is verified via the gate code + config default tests + `TestRestartResume_AfterMidSyncGap`. A direct startup-gate test would pin it.
- **S2 — no direct bucket-level assertion that `clearProgress()` deletes the `block_sync` key.** The completion branch is exercised end-to-end (convergence/restart suites stay live), but no unit test asserts `loadProgress() == 0` after clean completion.
- **S3 (informational)** — soft assertion at `blocksync_test.go:203-205` (log-only empty branch) is acceptable: companion non-empty test `TestBlockSyncEngine_RequestServeMax100` asserts real block serving with the same setup.

**Resolved since the previous report**: malformed-0x43 wire-path penalty (was PARTIAL), proposer-path SyncHeight publication (was SUGGESTION), typed-framing gossip (was SUGGESTION S2), shutdown-race on Close (was WARNING on pre-existing race), snapshot-sync 8/8 scenarios (were unimplemented/deferred).

### Final Verdict

**PASS WITH WARNINGS** — 39/39 tasks complete, 18/18 requirements, 28/28 scenarios with passing runtime evidence, 0 blockers, 0 CRITICAL. The two WARNINGs are a non-blocking test flake (W1) and a docs drift (W2); both are eligible for follow-up outside this change. No spec regression, no design deviation, no out-of-scope leak detected.
