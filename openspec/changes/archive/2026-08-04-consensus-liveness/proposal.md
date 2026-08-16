# Proposal: Consensus Liveness — Multi-Round + View Change

## Intent

- **Single-round protocol.** `Round: 0` hardcoded at `node/node.go:953` and `node/node.go:760`; `BlockHeader` has no round (`types/block.go:10-23`).
- **Deterministic halt.** Stuck/missing proposer → no view change anywhere → chain stalls forever (exploration §2). #1 hardening item (`docs/product-strategy.md:262`).
- **Code vs design.** Precommits sent without prevote-majority gate (`node/node.go:1000-1001`) contradict `ARCHITECTURE.md:406` + harness (`bft_harness.go:323-330`); evidence-pool slashing never fires (`node.go:682` TODO).

## Why Now

v0.1.0, zero tags, no prod data → chain reset affordable now, not later; liveness gates every product thesis (§7-8).

## Scope

### In Scope
- `Round uint32` in `BlockHeader` + `HeaderHash`; wire + bbolt layout 244→248 (`persist.go:185`).
- `WeightedProposerAtHeightAndRound` = `(height+round) % totalPower`; round 0 ≡ today.
- Round-aware validation: proposer check (`block_validator.go:153`); precommit round == header round.
- Node pending-round state machine: higher-round replacement mirroring `replace`/`votingHasExternalVotes` guards; timeout advancement with backoff.
- Precommit gating on 2/3 prevotes.
- Round metrics; doc/config alignment (2-phase, 16 methods — fix stale `config.yaml:7,10`).
- Tests: stuck-proposer view change, equivocation→advance, cross-round fork, round-0 compat.

### Out of Scope
BLS · locking/amnesia · cross-round evidence · evidence-pool wiring/live slashing · RPC round exposure · P2P identity · delegation/governance · sharding · consensus rewrite · DB migration (chain reset instead).

## Approach

Extend the 2-phase scheme: header round → round-aware proposer schedule → node tracks `pendingRound`, advances after `ProposerTimeout × (1+r)` without finalization → new proposer via existing snapshot-validate-revert (`node.go:679-785`). Single round source; no new wire type.

## Decision Points (user-owned)

1. **DECISION 1 — Chain reset (CRITICAL).** Header round changes every block hash; all chains re-sync. **(A)** header field → clean, reset (recommended); **(B)** round in `CommitProof` + new `MsgTypeProposal` envelope → no hash break, but new wire type + split round sources. **Requires explicit user sign-off — recommend (A).**
2. **DECISION 2 — Timeout.** Linear `base × (1+r)` (recommended) vs capped `base << r`; add `MaxRound` guard? Minimum must exceed ±5s skew (`timestamp.go:8`).
3. **DECISION 3 — Precommit gating.** In v1 (recommended — aligns code+harness, shrinks dual-commit window) vs deferred.
4. **DECISION 4 — Metrics.** Wire `telemetry` into prod `/metrics` (fixes gap; today only `dsn_block_height`, `startup.go:767-773`) vs hand-roll.

## Capabilities

> No consensus spec exists (`openspec/specs/`: bip39-mnemonic, sdk-compat, wallet-cli, wallet-keystore) → new domain.

- **New — `consensus`**: header round, round-aware schedule + validation, view-change state machine, precommit gating, round metrics.
- **Modified — none.**

## Affected Areas

| Area | Impact |
|------|--------|
| `types/block.go` | Modified — `Round` + hash |
| `consensus/persist.go` | Modified — 244→248 |
| `consensus/proposer.go` | Modified — round-aware schedule |
| `consensus/block_validator.go` | Modified — proposer/precommit-round checks |
| `node/node.go` + `node/config.go` | Modified — state machine, loop, gating, knobs |
| `telemetry/metrics.go`, `node/startup.go` | Modified — round gauges |
| Tests (loop/proposer/node/voting/harness/persist) | Modified — round-0 + new cases |
| `docs/*`, `openspec/config.yaml` | Modified — alignment |

## Risks

| Risk | Level | Mitigation |
|------|-------|------------|
| Chain reset (Dec 1) | CRITICAL | Explicit decision + re-sync procedure |
| Cross-round dual-commit | WARNING | Prevote gating + k=6 docs |
| Regression: stabilized loop logic | WARNING | 4 loop tests green; view-change tests first (TDD) |
| Clock determinism | WARNING | Shared base > skew window |
| Schedule determinism | WARNING | Same ordering as `GetActiveValidators` |
| Vote-persist key lacks round (`persist.go:301-305`) | INFO | Deferred |
| Stale docs (3-phase/17 methods) | INFO | Fixed in scope |

## Review Forecast

Plausibly **>400 authored lines** → chained PRs: **(1)** types/wire/validation plumbing; **(2)** node round state machine + loop; **(3)** metrics + docs. `sdd-tasks` MUST emit guard lines (Decision needed / Chained PRs / 400-line budget).

## Production-Readiness Impact

- **Enables:** liveness under proposer failure; view-change observability (`dsn_consensus_round`).
- **Operators:** chains re-sync under new header layout; new knobs (round timeout base, `MaxRound`).

## Rollback Plan

Git revert alone is insufficient post-deploy: 248-byte-header blocks are unreadable by reverted code — rollback = revert code AND re-sync again. Acceptable at v0.1.0. Pre-deploy: snapshot genesis/config for deterministic re-sync.

## Dependencies

Decision 1 sign-off + Decisions 2-4. Strict TDD: `make test`, `make test-integration`.

## Success Criteria

- [ ] Stuck proposer → round ≥1 finalizes (new test green)
- [ ] Round-0 schedule: existing proposer tests pass unmodified
- [ ] Precommit only after 2/3 prevotes (new test)
- [ ] Equivocation → round advance; cross-round forks single-valued
- [ ] `make test` + `make test-integration` green; loop suite green
- [ ] `dsn_consensus_round` on prod `/metrics`

## Decisions (resolved 2026-08-04)

| # | Decision | Resolution |
|---|----------|------------|
| 1 | Chain reset | **(A) Round in header — chain reset APPROVED** (v0.1.0, zero tags, re-sync accepted) |
| 2 | Timeout | Linear `base × (1+r)` + `MaxRound` guard |
| 3 | Precommit gating | In v1 |
| 4 | Metrics | Wire `telemetry` into prod `/metrics` |
