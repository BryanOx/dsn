# Apply Progress: Consensus Liveness — PR1 (Wire + Validation)

**Change**: consensus-liveness
**Scope**: Phase 1, tasks 1.1–1.11 (PR1 of chained PRs)
**Mode**: Standard (no test runner requirement for strict TDD; project runs `go test` — RED→GREEN evidence preserved per task)
**Status**: COMPLETE — 11/11 tasks, all gates green
**Branch**: `feat/consensus-liveness/pr1`

## Completed Tasks

- [x] 1.1 RED `consensus/persist_test.go`: 248B round-trip Round=7; 244B decode fails (:220). S1
- [x] 1.2 RED `types/block_test.go`: `HeaderHash` differs iff Round differs. S1
- [x] 1.3 GREEN `types/block.go`: `Round uint32` after `Epoch` (:19); hash Epoch→Timestamp (:92-95)
- [x] 1.4 GREEN `consensus/persist.go`: len 244→248 (:185); Round between Epoch (:208)/Timestamp; comment `Epoch(8)+Round(4)+Timestamp(8)+Proposer(20)`
- [x] 1.5 GREEN `consensus/gossip.go`: Round after Epoch Encode (:36)/Decode (:155); position pinned, ADR7 superseded
- [x] 1.6 RED `consensus/proposer_test.go`: `WeightedProposerAtHeightAndRound(h,0)==WeightedProposerAtHeight(h)`; r0≠r1. S2
- [x] 1.7 GREEN `consensus/proposer.go`: new fn `(height+round)%totalPower` (:28); old fn = round-0 alias
- [x] 1.8 RED `consensus/block_validator_test.go`: round-1 proposal, round-0 proposer → `ErrWrongProposer`. S3
- [x] 1.9 GREEN `consensus/block_validator.go:153`: use `WeightedProposerAtHeightAndRound(h, Header.Round, …)`
- [x] 1.10 RED `consensus/block_validator_test.go`: round-r block + r−1 precommits → `ErrInvalidCommitProof`; round-0 passes. S4
- [x] 1.11 GREEN `consensus/block_validator.go:309-314`: reject `vote.Round != block.Round`; 0==0 keeps legacy

## Work Unit Evidence

| Evidence | Required value | Result |
|---|---|---|
| Focused test command and exact result | Smallest command proving each unit | `go test ./types/... ./consensus/... -count=1` — `ok github.com/BryanOx/dsn/types`, `ok github.com/BryanOx/dsn/consensus` (11 proposer tests, 16 block-validation tests, persist/gossip round-trips) |
| Runtime harness command/scenario and exact result | Real integration/runtime path | `make test-integration` — `ok github.com/BryanOx/dsn/integration 17.7s`, `ok github.com/BryanOx/dsn/integration/soak`, `ok github.com/BryanOx/dsn/integration/staging`; plus `node` loop regression `go test ./node/... -run TestConsensus_` — 9/9 PASS incl. `LoopProducesConsecutiveHeights`, `RebroadcastsPendingProposal`, `RefreshesStaleProposal`, `RefreshesStaleBoundaryProposal` |
| Rollback boundary | Exact files/behavior revertable without removing unrelated work | `git revert` of GREEN commits `3b26f98` (wire+persist), `451263e` (schedule), `656badb` (proposer check), `7463167` (precommit check) — round-0 alias keeps legacy behavior; no node/ changes in PR1 |

## TDD Cycle Evidence

| Task | RED (test written first) | GREEN (implementation passes) | REFACTOR |
|---|---|---|---|
| 1.1 persist 248B/244B layout | `ada7fa9` — 244B decode + Round=7 round-trip fail (build: unknown field) | `3b26f98` — persist.go len 244→248, Round encode/decode | — |
| 1.2 header hash Round-sensitive | `ada7fa9` — `TestBlockHeaderHash_RoundAffectsHash` fail (build) | `3b26f98` — types/block.go hash Epoch→Timestamp | — |
| 1.3–1.5 wire GREEN | (covered by 1.1/1.2 RED) | `3b26f98` — types/block.go, persist.go, gossip.go + gossip_test Round=3 | ADR7 "after Proposer" wording superseded by pinned position |
| 1.6 proposer schedule | `a628682` — 5 RED tests fail (build: undefined fn) | `451263e` — proposer.go `(h+round)%totalPower`, round-0 alias | `8c19bf6` — gofmt proposer_test.go |
| 1.8 round-aware proposer validation | `7de03e6` — round-1 proposal by r0 proposer passes → test fails `ErrWrongProposer` expected | `656badb` — block_validator.go:153 `WeightedProposerAtHeightAndRound(h, Header.Round, …)` | — |
| 1.10 precommit round mismatch | `f55fc2f` — round-1 block + r0 precommits passes → test fails `ErrInvalidCommitProof` expected | `7463167` — block_validator.go per-vote `vote.Round != block.Round` reject; 0==0 legacy | — |

## Deviations from Design

None — implementation matches design.md (ADRs 1, 2, 7, 8). Position of `Round` pinned between `Epoch` and `Timestamp` per ADR7 (superseded "after Proposer" wording); proposer check uses the round-aware fn while `node/node.go:600` loop stays on the height-only call (deliberate, PR2).

## Issues Found

- `consensus/proposer_test.go` was left unformatted by the authoring step; fixed with `gofmt -w` in `8c19bf6` (style commit).
- `node/config.go` and `docs/product-strategy.md` are pre-existing unformatted/untracked files unrelated to this change — left untouched.

## Files Changed

| File | Action | What Was Done |
|---|---|---|
| `types/block.go` | Modified | `Round uint32` after `Epoch`; hash Epoch→Timestamp; genesis Round:0 |
| `types/block_test.go` | Modified | RED: Round round-trip, 244B rejection, hash sensitivity |
| `consensus/persist.go` | Modified | expectedHeaderLen 244→248; Round between Epoch/Timestamp |
| `consensus/persist_test.go` | Modified | RED: 248B Round=7 round-trip; 244B decode fails |
| `consensus/gossip.go` | Modified | Round after Epoch in Encode(:36)/Decode(:155) |
| `consensus/gossip_test.go` | Modified | Round=3 wire assertion |
| `consensus/proposer.go` | Modified | `WeightedProposerAtHeightAndRound`; old fn = round-0 alias |
| `consensus/proposer_test.go` | Modified | RED schedule tests + gofmt |
| `consensus/block_validator.go` | Modified | round-aware proposer check (:153); precommit round check (:309-314) |
| `consensus/block_validator_test.go` | Modified | RED round-aware proposer + commit-proof round mismatch tests |

## Commit History (development..HEAD)

```
8c19bf6 style(consensus): gofmt proposer schedule tests
7463167 feat(consensus): require precommit round to match block round
f55fc2f test(consensus): add RED commit-proof round mismatch test
656badb feat(consensus): validate proposer by (height, round)
7de03e6 test(consensus): add RED round-aware proposer validation test
451263e feat(consensus): add round-aware weighted proposer schedule
a628682 test(consensus): add RED round-aware proposer schedule tests
3b26f98 feat(consensus): add Round to block header wire and storage
ada7fa9 test(consensus): add RED header round layout and hash tests
```

## Workload / PR Boundary

- Mode: chained PR slice (PR1 of 3) — `size:exception` NOT requested; this slice is the approved first work unit
- Current work unit: U1 (PR1) — wire + round validation
- Boundary: wire layout (types/persist/gossip) + proposer schedule fn + round-aware validation; node loop round logic (2.6/2.7), MaxRound config (2.5), metrics/docs (Phase 3) are out of scope
- Estimated review budget impact: ~373 inserted lines in 9 commits (diffstat `development...pr1`), well under the 400-line budget for this slice
- Chain strategy: `stacked-to-main` — PR1 branch is `feat/consensus-liveness/pr1`; PR2 branches from the PR1 tip; PR1 → PR2 → PR3 merge into `development` in order

## Next Steps

1. Orchestrator creates PR1 from `feat/consensus-liveness/pr1` → review
2. After merge: start PR2 (Phase 2, tasks 2.1–2.8) on `feat/consensus-liveness/pr2` targeting PR1 branch
3. PR3 (Phase 3, tasks 3.1–3.6) after PR2 merge

---

# Apply Progress: Consensus Liveness — PR2 (Round SM + Gating)

**Change**: consensus-liveness
**Scope**: Phase 2, tasks 2.1–2.8 (PR2 of chained PRs)
**Mode**: Standard RED→GREEN per task (project runs `go test`; RED evidence preserved per commit)
**Status**: COMPLETE — 8/8 tasks, all gates green
**Branch**: `feat/consensus-liveness/pr2` (stacked on PR1 tip)

## Completed Tasks

- [x] 2.1 RED `node/consensus_loop_test.go`: stalled r0 proposer → deadline → round ≥1 finalizes. S5
- [x] 2.2 RED: higher round accepted when unvoted (S6); second r vote dropped (S7)
- [x] 2.3 RED: `MaxRound` cap → no advance (S8); config rejects `ProposerTimeout ≤ 5s` (S9)
- [x] 2.4 RED: precommit <2/3 withheld, ≥2/3 sent (S10,S11); equivocation → advance (S12); cross-round fork single-valued (S13)
- [x] 2.5 GREEN `node/config.go`: `MaxRound=8` default + `DSN_MAX_ROUND` env; reject base ≤5s
- [x] 2.6 GREEN `node/node.go`: round state machine + one-vote-per-(h,r) + gated precommit
- [x] 2.7 GREEN `node/node.go`: round-aware loop, `ProposerTimeout×(1+r)` deadline capped at MaxRound, rebroadcast
- [x] 2.8 GREEN `consensus/bft_harness.go` + `SetProposalRound`: round>0 harness, Round:0 sites kept valid

## Work Unit Evidence

| Evidence | Required value | Result |
|---|---|---|
| Focused test command and exact result | Smallest command proving each unit | `go test ./node/... -run 'TestConsensus_\|TestConfig' -count=1 -timeout=240s` — all pass (4 core SM tests ~1.6s; two-validator refresh tests fixed by own-prevote re-broadcast) |
| Runtime harness command/scenario and exact result | Real integration/runtime path | `make test` — `ok` all 22 packages, node 58.3% coverage; `make test-integration` — `ok github.com/BryanOx/dsn/integration 17.450s`, soak, staging, testutil |
| Rollback boundary | Exact files/behavior revertable without removing unrelated work | `git revert` of GREEN commits `9918a50` (harness+SetProposalRound), `5c6df9c` (config), `f4a7390` (node loop) — restores RED-failing state; round-0 behavior preserved via `pendingRound==0` paths |

## TDD Cycle Evidence

| Task | RED (test written first) | GREEN (implementation passes) | REFACTOR |
|---|---|---|---|
| 2.1 round advance finalizes | `8c8a118` — `TestConsensus_AdvancesToHigherRoundWhileUnvoted` fail (no round logic) | `f4a7390` — `maybeAdvanceRound` after the `ProposerTimeout` wait (timing fix: advance ran before the wait, aging round 0 at ~0ms; moved after so round r lives ≥(r+1)×timeout) | — |
| 2.2 higher-round acceptance / S7 | `8c8a118` — `TestConsensus_AcceptsHigherRoundProposalAfterVotingRoundZero`, `TestConsensus_RebroadcastsPendingProposal` | `f4a7390` — `handleProposalWithSnap` adopt policy; strict (height,round) vote match drops stale-round votes | — |
| 2.3 MaxRound cap / S9 | `c12e492` + `8c8a118` — `TestConsensus_RoundCappedAtMaxRound`, `TestConfig_MaxRoundDefaultsToEight`, `TestConfigFromEnv_RejectsTimeoutAtOrBelowSkewWindow` | `5c6df9c` — `cfg.MaxRound` cap; 6s default (rejects ≤5s env by keeping default) | — |
| 2.4 precommit gating (S10–S13) | `8c8a118` — `TestConsensus_PrecommitGatedOnPrevoteQuorum` | `f4a7390` — `tryPrecommitLocked` gated on `HasPrevoteMajority()` + `precommitSent`; v1's ungated immediate precommit removed | — |
| 2.8 round>0 harness | `8c8a118` — `consensus/bft_harness_round_test.go` fail (build) | `9918a50` — `RunRoundAt(round)`, round stamping before hashing, commit-proof attach | — |

## Key Design Decisions (GREEN)

- **Round stamping order**: `SetProposalRound` writes `Header.Round`, re-hashes, re-signs — must run BEFORE `HeaderHash`/`CommitProof` since Round is part of HeaderHash (commit `9918a50`).
- **Advance-after-wait timing**: `maybeAdvanceRound` runs AFTER the `time.After(ProposerTimeout)` select, giving round r a full `(r+1)×ProposerTimeout` lifetime; advancing before the wait made round 0 die at ~0ms and broke S5.
- **Precommit gating moved into the node**: v1 precommitted un-gated at proposal time; S7 now requires `HasPrevoteMajority()` (receiver-side `VotingState` created in `handleProposalWithSnap` when the epoch snapshot is non-nil) via `tryPrecommitLocked` → proposer-only `tryFinalizeLocked`.
- **Cross-node prevote liveness (S7)**: a stale, unvoted proposal is rebuilt fresh at the current pending round; the proposer's own prevote is re-broadcast alongside every proposal re-broadcast — a late-connecting peer otherwise never sees the 2/3 prevote majority its precommit is gated on and the round deadlocks. Vote re-sends are idempotent (duplicates dropped).
- **Adopt policy**: receiver adopts a proposal when (fresh height) or (no pendingBlock and `round >= pendingRound`) or (higher round and no external votes on current); same-round refresh blocked when external votes exist (`votingHasExternalVotes`).

## Deviations from Design

- `node/config.go` and `docs/product-strategy.md` pre-existing formatting/untracked issues (noted in PR1) remain untouched.
- `openspec/` artifacts are local-only per repo history (openspec tracked files removed in `b45e180`); artifacts updated in place, not committed.

## Files Changed

| File | Action | What Was Done |
|---|---|---|
| `node/config.go` | Modified | `MaxRound uint32` (default 8) + doc; `DSN_MAX_ROUND` env (`strconv.ParseUint(v,10,32)`); `ProposerTimeout` default 5s→6s; S9 rejection of ≤5s env values |
| `node/node.go` | Modified | `consensusProposerAtHeightAndRound`, `currentRound`, `maybeAdvanceRound` (tick ≥ round+1, capped, clears votes/`precommitSent`, keeps pendingBlock); round-aware `proposeForHeight`/`runProposerRound`; `newVote` round param; `tryPrecommitLocked`; `handleProposal`→`handleProposalWithSnap`; `handleBlockMessage` snap capture before revert; `rebroadcastPendingProposal` stale rebuild + own-prevote re-send; loop uses round-aware proposer |
| `consensus/block_builder.go` | Modified | `SetProposalRound` helper (stamp Round, re-hash, re-sign) |
| `consensus/bft_harness.go` | Modified | `RunRoundAt(round)`; `RunRound()` = round-0 alias; round stamping before hashing; `simulateVoting(height, round, …)`; `NewVotingState(h, round, …)`; commit-proof attach on committed blocks |

## Commit History (development..HEAD)

```
f4a7390 feat(node): round-aware consensus loop with precommit gating
5c6df9c feat(config): add MaxRound knob and reject short proposer timeout
9918a50 feat(consensus): round-aware harness and SetProposalRound helper
8c8a118 test(consensus): RED round view change, MaxRound cap, precommit gating, higher-round acceptance, harness round>0
c12e492 test(config): RED reject timeout at/below skew window and add MaxRound knob
8c19bf6 style(consensus): gofmt proposer schedule tests
… (PR1 commits below)
```

## Workload / PR Boundary

- Current work unit: U2 (PR2) — `pendingRound`+timeout/gating — branch `feat/consensus-liveness/pr2` stacked on PR1 tip
- Boundary: node round state machine, MaxRound config, gated precommit, round>0 harness; metrics/docs/config.yaml (Phase 3 / PR3) out of scope
- Changed lines in PR2: 5 commits, +248/−51 node, +18/−3 config, +53/−15 consensus (≈389 net) — over the 400-line review budget at the margin; chain strategy `stacked-to-main` preserved

## Next Steps

1. Orchestrator runs PR2 review (chain PR2 → PR3 after merge)
2. Verify phase (sdd-verify) against S5–S13 before archive
3. PR3 (Phase 3, tasks 3.1–3.6) after PR2 merge

## Corrective Pass (phase-contract validator)

A phase-contract validator found 4 gaps after PR2 verification; all four are closed:

1. **S12 dedicated equivocation test (Gap 1)** — added `TestConsensus_EquivocationDroppedRoundAdvances` to `node/consensus_loop_test.go`: a same-round conflicting proposal for the same height is dropped at `handleProposalWithSnap` (pending hash/round unchanged, exactly one round-r prevote), and after the deadline the node advances to round 2 and votes exactly once. GREEN against the implementation — the drop lives in `handleProposalWithSnap` (`round >= pending round`), the advance in `maybeAdvanceRound`. Commit `e044880`.
2. **S7 literal same-round duplicate-vote test (Gap 2)** — added `TestConsensus_OneVotePerHeightAndRound` to `node/consensus_loop_test.go`: duplicate round-r prevotes (the peer's and the proposer's own re-broadcast) are dropped by `VotingState.addVote` (power never double-counted), A precommits once, and a duplicate round-r precommit after finalization is dropped. GREEN against the implementation. Commit `e044880`.
3. **apply-progress line fix (Gap 3)** — "3 commits" → "5 commits" (f4a7390, 5c6df9c, 9918a50, 8c8a118, c12e492) in Workload / PR Boundary; this subsection records the corrective commits. `openspec/` stays local-only per repo convention — the file is updated in place, not committed.
4. **DSN_MAX_ROUND lower bound (Gap 4)** — `node/config.go` now rejects `DSN_MAX_ROUND=0` (MaxRound must be >= 1) with a logged warning, keeping the default 8 — mirroring the ≤5s timeout rejection. RED test `TestConfigFromEnv_MaxRoundZeroRejected` in `node/config_test.go`; GREEN fix in `node/config.go`. Commits `d983d85` (RED), `666a63f` (GREEN).

PR2 branch total after the corrective pass: 5 implementation commits + 3 corrective commits. Verification gates green: `go test ./...`, `make test-integration`, `go test ./node/... -run TestConsensus_`, `gofmt -l` clean.

---

# Apply Progress: Consensus Liveness — PR3 (Metrics + Docs/Config Alignment)

**Change**: consensus-liveness
**Scope**: Phase 3, tasks 3.1–3.6 (PR3 of chained PRs, final slice)
**Mode**: Strict RED→GREEN per task (project runs `go test`; RED preserved as its own commit)
**Status**: COMPLETE — 6/6 tasks, all gates green
**Branch**: `feat/consensus-liveness/pr3` (stacked on PR2 tip `e044880`)
**TDD**: RED test commit `edebdee`; GREEN commits `91e613f`, `c85f042`; docs commit `2224abf`

## Completed Tasks

- [x] 3.1 RED `node/startup_test.go`: `TestMetrics_ScrapeExposesConsensusGauges` scrapes `/metrics` via httptest server over the real `handleMetrics`, asserts `dsn_consensus_height`, `dsn_consensus_round`, and the legacy `dsn_block_height` line for two (height, round) states. S14
- [x] 3.2 GREEN `telemetry/metrics.go`: promauto gauges `dsn_consensus_round` (name matches `docs/OPERATIONS.md:161`) and `dsn_consensus_height`
- [x] 3.3 GREEN `node/startup.go` + `node/node.go`: `handleMetrics` serves `promhttp.HandlerFor(prometheus.DefaultGatherer, {DisableCompression: true})` then appends the hand-rolled `dsn_block_height` line; `recordConsensusMetrics(height, round)` recorded on round advance (`maybeAdvanceRound`), higher-round adopt (`handleProposalWithSnap`), boot (`StartConsensus`), and finalize (`finalizeLocalBlock` + `applyAcceptedBlock`)
- [x] 3.4 GREEN `docs/ARCHITECTURE.md` Key Concepts: Round/View/Pre-vote/Pre-commit rewritten to implemented semantics + new "Cross-round caveat" bullet (k=6 finality, no Tendermint locking)
- [x] 3.5 GREEN `docs/dsn_protocol_spec_v_1.md`: new `12.5 Rounds and View Change` (header round, `(h+r)%power` proposer, linear backoff + MaxRound, 2/3 prevote gate); old `12.5 Slashing Conditions` renumbered to `12.6`
- [x] 3.6 GREEN `openspec/config.yaml`: context line 7 → "2-phase (preVote→preCommit) BFT voting with per-round view change, round-aware weighted proposer"; line 10 → "16 methods" (verified against `rpc/server.go` switch: exactly 16 `dsn_*` methods). Updated in place — `openspec/` stays untracked per repo convention

## Work Unit Evidence

| Evidence | Required value | Result |
|---|---|---|
| Focused test command and exact result | Smallest command proving this unit | `go test ./node/... -run TestMetrics_ScrapeExposesConsensusGauges -count=1` — PASS (2 subtests: height 7/round 3, height 42/round 0); RED confirmed before GREEN via build failure `recordConsensusMetrics undefined` |
| Runtime harness command/scenario and exact result | Real integration/runtime path | `make test` — ok all 22 packages (node 59.0% coverage); `make test-integration` — ok integration 17.7s, soak, staging, testutil. S14 scrape-equivalent: httptest server over the production `handleMetrics` asserts both gauges + legacy line in the exposition body |
| Rollback boundary | Exact files/behavior revertable without removing unrelated work | `git revert` of `c85f042` (node wiring), `91e613f` (telemetry), `edebdee` (RED test), `2224abf` (docs) restores the PR2 state; `handleMetrics` reverts to the height-only hand-rolled output; no consensus behavior changed (gauges record values only) |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 3.1 `/metrics` gauges (S14) | `node/startup_test.go` | Unit (HTTP scrape) | ✅ `go test ./node/...` green before edits | ✅ `edebdee` — build fail (undefined `recordConsensusMetrics`) | ✅ `c85f042` + `91e613f` — 2/2 subtests pass | ✅ 2 cases: (7,3) and (42,0) — round-0 case forces real round reporting | ✅ Clean (gofmt) |
| 3.2 telemetry gauges | `telemetry/metrics.go` | Unit | ✅ package green | (covered by 3.1 RED) | ✅ `91e613f` — promauto gauges | ➖ Single output each; `telemetry_test.go` already covers gauge Set pattern | — |
| 3.3 promhttp wiring | `node/startup.go` | Unit (HTTP scrape) | ✅ `go test ./node/...` green | (covered by 3.1 RED) | ✅ `c85f042` — HandlerFor + append hand-rolled line | ➖ Covered by 3.1's two-state scrape | ✅ Clean (gofmt) |
| 3.4 docs semantics | `docs/ARCHITECTURE.md` | N/A (docs) | N/A | N/A | ✅ `2224abf` | ➖ Single | — |
| 3.5 protocol spec rounds | `docs/dsn_protocol_spec_v_1.md` | N/A (docs) | N/A | N/A | ✅ `2224abf` | ➖ Single | — |
| 3.6 config.yaml | `openspec/config.yaml` | N/A (config) | N/A | N/A | ✅ in-place (not committed) | ➖ Single | — |

### Test Summary
- **Total tests written**: 1 new test function (2 table-driven cases)
- **Total tests passing**: 2/2 subtests
- **Layers used**: Unit (2)
- **Approval tests**: None — no behavior-refactoring tasks in PR3
- **Pure functions created**: 0 (`recordConsensusMetrics` is a state-recording side-effect method by design)

## Deviations from Design

- **`handleMetrics` compression**: design said "promhttp in `handleMetrics`"; using bare `promhttp.Handler()` (gzip on) corrupts the appended hand-rolled line (gzip stream) and writing the line first triggers a stdlib `superfluous response.WriteHeader` log on every scrape. Implemented `promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true})` + append — same telemetry exposition, plain-text, no log noise, no behavior regression (the old handler never compressed either).
- **Initial record added**: `recordConsensusMetrics` also called in `StartConsensus` after tip load so a fresh boot reports its actual (height, 0) instead of 0/0 — makes S14 "current (height, round)" true from boot, not just after the first event.
- **Recording sites**: higher-round adopt (`handleProposalWithSnap`) added alongside `maybeAdvanceRound` as a round-change site; finalize recorded on both `finalizeLocalBlock` (proposer) and `applyAcceptedBlock` (receiver). Consensus logic untouched — gauges record values only.

## Issues Found

- `docs/dsn_protocol_spec_v_1.md` §12.2 still says "weighted stake randomness" for validator selection, contradicting the deterministic `(h+r)%power` schedule now documented in §12.5. Out of scope for task 3.5 (which added round content only); flagged for a future docs sweep.
- `docs/OPERATIONS.md` metric table lists some names that don't match the telemetry registry (`dsn_mempool_size` vs `dsn_mempool_tx_count`, `dsn_p2p_peer_count` vs `dsn_peer_count`). Pre-existing drift, untouched (only `dsn_consensus_round` was in scope and it matches).

## Files Changed

| File | Action | What Was Done |
|---|---|---|
| `node/startup_test.go` | Modified | RED: `/metrics` scrape asserts both consensus gauges + legacy `dsn_block_height` line (S14) |
| `telemetry/metrics.go` | Modified | `ConsensusHeight`, `ConsensusRound` promauto gauges |
| `node/startup.go` | Modified | `handleMetrics` serves telemetry exposition (HandlerFor, no compression) + appends hand-rolled `dsn_block_height` |
| `node/node.go` | Modified | `recordConsensusMetrics(height, round)` + call sites (advance, adopt, boot, both finalize paths) |
| `docs/ARCHITECTURE.md` | Modified | Key Concepts round/view/pre-vote/pre-commit → implemented semantics + cross-round caveat |
| `docs/dsn_protocol_spec_v_1.md` | Modified | Added §12.5 Rounds and View Change; renumbered slashing → §12.6 |
| `openspec/config.yaml` | Modified | Context: 2-phase + per-round view change; 16 RPC methods (in place, NOT committed) |

## Commit History (feat/consensus-liveness/pr2...pr3)

```
2224abf docs: document implemented round and view-change semantics
c85f042 feat(node): expose telemetry gauges on /metrics and record on round/finalize
91e613f feat(telemetry): add consensus round and height gauges
edebdee test(node): add RED /metrics consensus gauge scrape tests (S14)
```

## Workload / PR Boundary

- Mode: chained PR slice (PR3 of 3, final) — `stacked-to-main`
- Current work unit: U3 — gauges + docs/config alignment
- Boundary: telemetry gauges, `/metrics` wiring, metrics recording, ARCHITECTURE + protocol spec docs, config.yaml context; no consensus/node logic changes
- Changed lines in PR3: 6 files, +124/−8 (≈132) — well under the 400-line budget for this slice
- `size:exception` NOT required and NOT requested

## Next Steps

1. Orchestrator runs PR3 review (chain PR3 → `development` after merge)
2. Verify phase (sdd-verify) against the full spec — all 9 requirements / 15 scenarios now implemented across PR1+PR2+PR3 (S14 covered by `TestMetrics_ScrapeExposesConsensusGauges`; S15 by config.yaml + protocol spec review)
3. Archive after verify

---

# Verify: Consensus Liveness — Full Change (PR1+PR2+PR3)

**Change**: consensus-liveness
**Scope**: Full verification — all 9 requirements (R1–R9) / 15 scenarios (S1–S15), 25/25 tasks
**Mode**: Strict TDD (openspec/config.yaml `strict_tdd: true`)
**Status**: PASS — no CRITICAL, no WARNING; 2 SUGGESTIONs + 2 INFOs (non-blocking)
**Date**: 2026-08-04
**Full report**: `openspec/changes/consensus-liveness/verify-report.md` (admitted by `gentle-ai sdd-verify-validate` — `valid: true, verdict: pass`)

## R1–R9 Requirement Map

| Req | Requirement | Status | Evidence |
|-----|-------------|--------|----------|
| R1 | Header Round and Chain Impact | ✅ | `types/block.go:20,97` Round hashed; `consensus/persist.go:185` len 248, `<248` rejected (:222) → re-sync |
| R2 | Round-Aware Proposer Schedule | ✅ | `consensus/proposer.go:31` `(height+round)%totalPower`; round-0 alias (node.go:458) |
| R3 | Round-Aware Validation | ✅ | `block_validator.go:153` round-aware proposer; per-vote `vote.Round != block.Round` reject |
| R4 | Node Pending-Round State Machine | ✅ | `node.go` `pendingRound`, one-vote-per-(h,r), higher-round adopt when unvoted, fresh propose |
| R5 | Timeout-Driven Advancement | ✅ | `maybeAdvanceRound` linear `×(1+r)`, MaxRound cap (node.go:513); `config.go` MaxRound=8, DSN_MAX_ROUND ≥1, ≤5s rejected |
| R6 | Precommit Gating | ✅ | `tryPrecommitLocked` gated on `HasPrevoteMajority()` + `precommitSent` |
| R7 | Equivocation and Cross-Round Forks | ✅ | equivocation dropped at `handleProposalWithSnap`; `forkWeight`/`handleFork` first-arrival; k=6 remains |
| R8 | Round Metrics | ✅ | `telemetry/metrics.go` gauges; `startup.go` `handleMetrics` promhttp + legacy line |
| R9 | Docs and Config Alignment | ✅ | config.yaml context; ARCHITECTURE.md:408; protocol spec §12.5 |

## S1–S15 Scenario Map

| Scenario | Classification | Evidence (test name / reference) |
|----------|----------------|----------------------------------|
| S1 Existing chain re-syncs | ✅ Automated test | `TestHeaderDecode_RejectsLegacy244Bytes`, `TestHeaderEncodeDecode_RoundRoundTrip` (persist_test.go); `TestBlockHeaderHash_RoundAffectsHash` (types/block_test.go); `TestBlockMessage_EncodeDecode` (gossip_test.go) |
| S2 Round-0 schedule unchanged | ✅ Automated test | `TestWeightedProposerAtHeightAndRound_Round0EqualsAtHeight` (+ RoundRotates/ZeroTotalPower/SingleValidator/PanicsOnEmpty, proposer_test.go) |
| S3 Wrong proposer rejected | ✅ Automated test | `TestValidateBlockProposal_RoundAwareProposer` (block_validator_test.go) |
| S4 Precommit round mismatch rejected | ✅ Automated test | `TestValidateBlock_CommitProof_RoundMismatch` (block_validator_test.go) |
| S5 Higher round accepted when unvoted | ✅ Automated test | `TestConsensus_AcceptsHigherRoundProposalAfterVotingRoundZero`, `TestConsensus_AdvancesToHigherRoundWhileUnvoted` (consensus_loop_test.go) |
| S6 One vote per round | ✅ Automated test | `TestConsensus_OneVotePerHeightAndRound` (consensus_loop_test.go); `TestVotingState_DuplicatePrevote`, `TestVotingState_WrongRound` (voting_test.go) |
| S7 Stuck proposer → view change | ✅ Automated test | `TestConsensus_AdvancesToHigherRoundWhileUnvoted` (r0 stalls → round 2 ≥ 1 finalizes) |
| S8 MaxRound caps advancement | ✅ Automated test | `TestConsensus_RoundCappedAtMaxRound` (consensus_loop_test.go) |
| S9 Clock-skew tolerance | ✅ Automated test | `TestConfigFromEnv_RejectsTimeoutAtOrBelowSkewWindow`, `TestConfigFromEnv_TimeoutAboveSkewAccepted` (config_test.go) |
| S10 Precommit withheld below majority | ✅ Automated test | `TestConsensus_PrecommitGatedOnPrevoteQuorum` (zero precommit power below 2/3) |
| S11 Precommit sent at majority | ✅ Automated test | `TestConsensus_PrecommitGatedOnPrevoteQuorum` (precommit ≥ 100k once 2/3 crossed) |
| S12 Equivocation → round advance | ✅ Automated test | `TestConsensus_EquivocationDroppedRoundAdvances` (consensus_loop_test.go, corrective commit e044880) |
| S13 Cross-round fork single-valued | ✅ Automated test + code inspection | `TestConsensus_AdvancesToHigherRoundWhileUnvoted` S13 assertion (stored h=1 block = round-2 value); `TestHarness_RunRoundAt_RoundGreaterThanZero` (round-1 2/3 commit proof); `forkWeight`/`handleFork` (node.go:89, :1385) |
| S14 Gauges exposed | ✅ Automated test | `TestMetrics_ScrapeExposesConsensusGauges` (startup_test.go, 2 states (7,3)/(42,0) over real `handleMetrics`) |
| S15 Config and docs accurate | ✅ Doc review (per design) | config.yaml context (2-phase + per-round view change; 16 methods — 16 `dsn_*` cases in rpc/server.go); protocol spec §12.5; ARCHITECTURE.md:408 |

**Compliance summary**: 15/15 scenarios compliant.

## Test Runs (fresh, on `feat/consensus-liveness/pr3` @ 2224abf)

| Command | Result |
|---------|--------|
| `make test` (go test ./... -cover -count=1 -timeout=120s) | ✅ exit 0 — all packages ok (node 59.0%, consensus 77.2%) |
| `go test ./node/... -count=1 -run TestConsensus_ -timeout=240s` | ✅ 15/15 PASS (10 round/loop + 5 legacy) — 7.2s |
| `make test-integration` | ✅ exit 0 — integration 17.4s, soak, staging, testutil |
| Focused per-scenario suites (S1–S4, S9, S14 + harness) | ✅ all PASS |
| `make build` | ✅ exit 0 |
| `gofmt -l` on touched files | ✅ clean |
| `go vet ./...` | ✅ exit 0 |

## Gaps by Severity

**CRITICAL**: None
**WARNING**: None
**SUGGESTION**:
1. S13: a dedicated test literally racing two 2/3-precommit blocks (rounds 0/1, same height) through `handleFork` would strengthen the single-valued assertion (currently asserted post-finalization + harness round>0; `handleFork` code-inspected only).
2. Changed-file statement coverage < 80% on large pre-existing files (node.go 39.1%, persist.go 31.8%, startup.go 29.9%, types/block.go 22.9%); telemetry-package test Setting the gauges would close metrics.go in-package 0% artifact. Informational — `coverage_threshold: 0`.
**INFO (non-blocking docs drift, NOT verification failures)**:
1. `docs/dsn_protocol_spec_v_1.md` §12.2 "weighted stake randomness" contradicts §12.5 deterministic schedule (future docs sweep).
2. `docs/OPERATIONS.md` metric name drift (`dsn_mempool_size` vs `dsn_mempool_tx_count`, `dsn_p2p_peer_count` vs `dsn_peer_count`) — pre-existing.

## Non-Goals Confirmed Not Implemented

No BLS; evidence-pool TODO intact (node.go:765); no RPC round method (16 methods unchanged); no DB migration (chain reset per ADR 7); no governance; no Tendermint locking/amnesia.
