# Archive Report: consensus-liveness

**Change**: consensus-liveness — Multi-Round Consensus + View Change
**Archived**: 2026-08-04
**Archived from**: `openspec/changes/consensus-liveness/` → `openspec/changes/archive/2026-08-04-consensus-liveness/`
**Mode**: openspec (file-based)
**Archive type**: standard — complete artifact trail, no partial archive, no overrides required

## Change Intent

DSN's 2-phase (preVote→preCommit) BFT-lite consensus was single-round: `Round: 0` was hardcoded and a stuck or missing proposer stalled the chain forever (deterministic halt; #1 hardening item in `docs/product-strategy.md:262`). This change added per-round view change: `Round uint32` in `BlockHeader` (hashed, wired, bbolt 244→248), a round-aware weighted proposer schedule `(height+round) % totalPower` (round 0 ≡ today), a node pending-round state machine with linear `ProposerTimeout × (1+r)` backoff capped by `MaxRound`, precommit gating on a 2/3 prevote majority, and round/height gauges on the production `/metrics` endpoint. Delivered as 3 chained PRs (PR1 wire+validation, PR2 round state machine + gating, PR3 metrics + docs/config alignment).

## Decisions

| # | Decision | Resolution |
|---|----------|------------|
| D1 | Chain reset (CRITICAL, user-approved) | `Round` in header — chain reset APPROVED at v0.1.0 (zero tags; 248-byte headers unreadable by old code; no migration path; rollout requires re-sync) |
| D2 | Timeout | Linear `base × (1+r)` + `MaxRound` cap (default 8, `DSN_MAX_ROUND` env, lower bound ≥ 1) |
| D3 | Precommit gating | In v1 — precommit only after `HasPrevoteMajority()` |
| D4 | Metrics | Wire `telemetry` into prod `/metrics` (promhttp + hand-rolled `dsn_block_height` kept) |

## Artifact Trail

| Artifact | State | SHA-256 |
|----------|-------|---------|
| `exploration.md` | Complete | `d5d1cf5e810a07fff32dd57699a51ef3b876491d7d53ad1fba6bac437f8957d2` |
| `proposal.md` | Complete; D1–D4 resolved | `59b9d076858ceda469cd4df696629d25f0f74a64693fc74973c44eaad066150f` |
| `specs/consensus/spec.md` | Delta spec (9 requirements, 15 scenarios) — merged into baseline below | `bbd3f3cb58af1f58f033b95650fa1cf88f3d74dd911689086ea0e5fe320de79f` |
| `design.md` | Complete; ADR 1–8, 3-PR slices, testing strategy | `8be7f7bfbc2c473098662b6f1cc0d6e690c800beb29afc237d10b3cc92fb8d5b` |
| `tasks.md` | **25/25 tasks complete** (all checkboxes checked; Task Completion Gate passed) | `d55070de81b8a53918d35497d36f83d355b617ff46508b167ef562fe037c9892` |
| `apply-progress.md` | Complete — PR1 (11/11), PR2 (8/8 incl. 3 corrective commits), PR3 (6/6), full-change verify summary | `d1116d389c48885ef0968e0db1e6b1b61475c44b6e68cc4a1941ee77721d7550` |
| `verify-report.md` | **PASS** — admitted by `gentle-ai sdd-verify-validate` (`valid: true, verdict: pass`) | `7d7e3b76eea6197507ff708a703abd0644d84b227b34d69ea8dffd4183467291` |
| `archive.md` | This report | — |

Native review receipt gate: N/A in openspec mode — no Engram review transaction/ledger/receipt artifacts exist for file-based stores. Gate evidence for this archive is the verify-report verdict (`pass`, zero CRITICAL/WARNING) and the orchestrator's launch status.

## Spec Merge

`openspec/specs/consensus/` did not exist (no consensus spec in the pre-change baseline) — per the OpenSpec convention the delta spec IS a full spec and was copied verbatim to `openspec/specs/consensus/spec.md` as the new source of truth. SHA-256 of the baseline (`bbd3f3cb58af1f58f033b95650fa1cf88f3d74dd911689086ea0e5fe320de79f`) is identical to the delta spec. Non-goal statements are preserved (embedded in Requirement: Equivocation and Cross-Round Forks — BLS, locking/amnesia, evidence-pool wiring, RPC round, governance, DB migration).

Coverage of the merged baseline: round-in-header + chain impact (R1), weighted proposer (R2), round-aware validation (R3), pending-round state machine (R4), timeout/MaxRound advancement (R5), precommit gate (R6), equivocation/cross-round forks (R7), round metrics (R8), docs/config alignment (R9).

## Verification Summary (final state)

Per `verify-report.md` (verification-time snapshot, superseded only where noted): **9/9 requirements, 15/15 scenarios compliant, PASS** — no CRITICAL, no WARNING.

- `make test` exit 0 (all packages ok; node 59.0%, consensus 77.2%)
- `go test ./node/... -run TestConsensus_ -count=1 -timeout=240s` — 15/15 PASS (10 round/loop + 5 legacy)
- `make test-integration` exit 0 (integration 17.4s, soak 6.3s, staging 0.5s, testutil)
- `make build` exit 0; `go vet ./...` exit 0; `gofmt -l` clean on touched files
- 14/15 scenarios covered by automated tests; S15 (config/docs accuracy) by documented review, as assigned in design
- Non-goals confirmed NOT implemented: no BLS; evidence-pool TODO intact; no RPC round method; no DB migration; no governance; no Tendermint locking/amnesia
- TDD compliance 6/6: RED/GREEN commit evidence per task, all tests exist, triangulation adequate, safety nets green

No post-verify work changed any of these numbers; the orchestrator's launch status and the verify report agree (25/25 tasks, PASS). No contradictions between sources were found — no unresolved claims to record.

## Deviations from Design (non-breaking)

1. **ADR 7 field position wording**: design said `Round` after `Proposer` in the bbolt layout; implementation pins `Round` between `Epoch` and `Timestamp` (final comment `Epoch(8)+Round(4)+Timestamp(8)+Proposer(20)=248`). Layout constant still 244→248; superseded wording documented in PR1. No spec broken.
2. **`handleMetrics` compression**: implemented `promhttp.HandlerFor(prometheus.DefaultGatherer, {DisableCompression: true})` + appended hand-rolled `dsn_block_height` line (design said bare "promhttp in `handleMetrics`"). Gzip-on would corrupt the appended plain-text line and trigger stdlib `superfluous response.WriteHeader` log noise; the old handler never compressed either. Same telemetry exposition, no behavior regression.

Both recorded in apply-progress at the time; neither breaks a spec requirement.

## Follow-ups (non-blocking, from verification)

1. **SUGGESTION** — S13 dual-commit race path: `handleFork`/`forkWeight` (node.go:1385/:89) are code-inspected only; no test literally races two 2/3-precommit blocks (rounds 0/1, same height) through `handleFork`. Single-valued invariant is asserted post-finalization + round>0 commit proofs via harness. A dedicated race test would strengthen R7.
2. **SUGGESTION** — Changed-file statement coverage < 80% on large pre-existing files (node.go 39.1%, persist.go 31.8%, startup.go 29.9%, types/block.go 22.9%); a telemetry-package test Setting the consensus gauges would close the metrics.go in-package 0% artifact. Informational — project `coverage_threshold: 0`; round-specific paths are directly covered.
3. **INFO** — `docs/dsn_protocol_spec_v_1.md` §12.2 "weighted stake randomness" contradicts the deterministic `(h+r)%power` schedule documented in §12.5 (flagged in PR3; future docs sweep).
4. **INFO** — `docs/OPERATIONS.md` metric table name drift: `dsn_mempool_size` vs `dsn_mempool_tx_count`, `dsn_p2p_peer_count` vs `dsn_peer_count` (pre-existing; `dsn_consensus_round` itself matches).

## Release Condition (required before deployment)

Chain reset (D1, user-approved). Procedure: stop all nodes → snapshot genesis/config for deterministic re-sync → remove data dirs → restart with new binary → nodes re-sync from genesis/snapshot. **No in-place DB migration exists** — 248-byte-header blocks are unreadable by reverted code, so rollback = revert the PR chain AND re-sync again. Acceptable at v0.1.0 (zero tags, no prod data). Feature-flag-free; knobs (`ProposerTimeout` default 6s, `MaxRound` default 8, `DSN_MAX_ROUND`) are config-only.

## Persisted Final State

- **HEAD**: `feat/consensus-liveness/pr3` @ `2224abf` (`docs: document implemented round and view-change semantics`)
- **Branch tips**: `feat/consensus-liveness/pr1` @ `8c19bf6` (9 commits), `feat/consensus-liveness/pr2` @ `e044880` (8 commits incl. 3 corrective: `d983d85`, `666a63f`, `e044880`), `feat/consensus-liveness/pr3` @ `2224abf` (4 commits) — stacked, base `development` @ `b45e180`; 21 commits total in the chain
- **Pushed**: nothing. All branches local; no upstreams created
- **Repo convention**: `openspec/` is intentionally UNTRACKED in git (tracked openspec files removed in `b45e180`). This archive is local-only until a future decision commits or pushes it. No source code, git refs, or branch state were touched by the archive phase
- **Next user handoff**: push the stacked chain and open PR1 → PR2 → PR3 (targeting `development` in order), then apply the release condition above

## Archive Checks

- [x] Main spec `openspec/specs/consensus/spec.md` created as new baseline (verbatim delta, non-goals preserved)
- [x] Change folder moved to `openspec/changes/archive/2026-08-04-consensus-liveness/`
- [x] Archive contains all artifacts (exploration, proposal, specs, design, tasks, apply-progress, verify-report, archive report)
- [x] Archived `tasks.md` has no unchecked implementation tasks (25/25)
- [x] Active `openspec/changes/` no longer contains this change
- [x] No destructive merge performed; no requirements removed or renamed
