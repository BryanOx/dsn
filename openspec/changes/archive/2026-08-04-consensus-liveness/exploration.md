# Exploration: Consensus Liveness — Multi-Round + View Change

> Artifact: `sdd-explore` for change `consensus-liveness` · 2026-08-04
> Read-only investigation. Every claim verified against source on the working tree.
> Artifact store: `openspec` (this file); strict TDD active (`make test` unit, `make test-integration`).
> Naming note: written as `exploration.md` per `openspec-convention.md` (the launch prompt asked for `explore.md`).

## Ground Truth (file:line verified)

### 1. The consensus round/vote path today

The protocol is a **single-round, two-phase (preVote → preCommit) BFT-lite** scheme. The wire and state types are
already round-capable; only the producer and the block itself are round-agnostic:

- **Vote carries a round on the wire.** `types.Vote` has `Round uint32` (`types/vote.go:36`), is included in
  `VoteHash` (`types/vote.go:52`) and in Encode/Decode (`types/vote.go:88`, `:119`). A round≠0 vote serializes fine.
- **VotingState is round-parameterized.** `VotingState` stores `round` (`consensus/voting.go:23`),
  `NewVotingState(height, round, blockHash, snapshot)` takes the round (`voting.go:33`), and `addVote` rejects
  any vote whose round ≠ the state's round (`voting.go:66-68`). `TestVotingState_WrongRound` covers rejection
  (`consensus/voting_test.go:464`).
- **The round is hardcoded to 0 in exactly two producer sites:**
  1. `node/node.go:953` — `Round: 0` in `newVote` (the docs' cited line; **confirmed**).
  2. `node/node.go:760` — `consensus.NewVotingState(height, 0, hash, snap)` in `proposeForHeight`
     (**not cited in the docs**; a second hardcode).
- **The block does NOT carry a round.** `BlockHeader` has no round field (`types/block.go:10-23`); `HeaderHash`
  hashes every header field (`types/block.go:77-102`); the bbolt header layout is a fixed 244 bytes
  (`consensus/persist.go:185-216`). `CommitProof` has no round field either (`types/commit.go:12-19`) — only the
  precommit votes inside it carry rounds.

**Flow** (all in `node/node.go`):
- `runConsensusLoop` (`:579-632`): target `height := tip + 1`; selects the proposer via
  `consensusProposerAtHeight(height)` (`:600`) → `WeightedProposerAtHeight(height, active)` (`:452-461`,
  `consensus/proposer.go:11`) — voting-power-weighted round-robin, **height-only**; if self is proposer, runs
  `runProposerRound` → `proposeForHeight` (`:643-785`), which builds the block (`BuildBlock`,
  `consensus/block_builder.go:38`), self-validates via `ValidateBlockProposal`, creates `VotingState(height, 0, …)`,
  broadcasts the proof-less proposal + its own prevote AND precommit (`:760-777`), and finalizes when a 2/3 precommit
  majority exists (`tryFinalizeLocked`, `:902-936`).
- `ProposerTimeout` (default **5s**, `node/config.go:101`; 1s block time `:117`) only paces the idle wait between
  loop ticks (`node.go:619`). While a round is pending, `rebroadcastPendingProposal` (`:839-870`) re-sends the same
  block; a stale, unvoted proposal (timestamp past the ±5s window, `types/timestamp.go:8-17`) is rebuilt with a
  fresh timestamp (`proposeForHeight(…, replace=true)`, `:674-785`).
- Receivers: `handleBlockMessage` (`:1084-1169`) validates proof-less proposals via `ValidateBlockProposal`, then
  `handleProposal` (`:979-1002`) votes **prevote AND precommit immediately** (`:1000-1001`) — there is **no
  prevote-majority gate on precommits**, despite `docs/ARCHITECTURE.md:406` describing one and the BFT harness
  implementing one (`consensus/bft_harness.go:323-330`). Final blocks carry a `CommitProof` and are applied via
  `ValidateBlock` → `applyAcceptedBlock` (`:1216-1291`).
- Proposer validation in the block pipeline is **height-only**:
  `expectedProposer := WeightedProposerAtHeight(block.Header.Height, activeVals)` (`consensus/block_validator.go:153`)
  → a round>0 proposer fails `ErrWrongProposer` (`:154-156`). `validateCommitProof` checks each precommit's
  height/block-hash but **not its round** (`block_validator.go:309-314`).
- Legacy `ProposerAtHeight` (`consensus/leader.go:7-11`, simple `height % count`) is **not used** by the node runtime
  (only `docs/ARCHITECTURE.md:120` references it as intent; the node uses the weighted function).

### 2. Failure modes

- **Stalled proposer = deterministic chain halt.** Only the proposer collects votes (`handleVoteMessage`, `:875-896`;
  `tryFinalizeLocked` requires `n.pendingBlock.Header.Proposer == n.consensusID()`, `:907`). If the round's proposer
  is down or unreachable, the other validators' prevotes/precommits go nowhere, `rebroadcastPendingProposal` keeps
  re-sending (or rebuilding) forever, and no code path ever advances to another proposer. A proposer holding the
  majority stalls the chain with no recourse. **Confirmed: no round increment, no view change, no proposer rotation
  anywhere in the tree.**
- **Missed-proposal detection only covers "I'm behind", not "proposer is dead".** After the idle wait, the loop
  nudges block-sync if a peer advertises a longer chain (`node.go:626-630`). It cannot help when no peer has the next
  block because the proposer never produced it.
- **Equivocation / double-vote are not detected on the live path.** `handleProposal` drops any second proposal for
  the same height (`node.go:981-984`), so conflicting blocks are silently ignored, not evidenced. A validator that
  double-signs is rejected only within a single in-memory `VotingState` (`addVote` duplicate check, `voting.go:74-76`);
  the two conflicting votes are never assembled into `DoubleSignEvidence`. `proposeForHeight` passes
  `var evidence []types.Evidence` with `// TODO: Get evidence from evidence pool` (`node.go:682-683`) — the evidence
  pool is never populated. Slashing machinery exists (`consensus/slashing.go:42-87`; v1 supports only
  `DoubleSignEvidence`, `:64-67`; `staking.SlashValidator` at `staking/slashing.go:208`; blocks carry
  `Block.Evidence` validated at `block_validator.go:116-120`) but **never fires in production**.
- **Per-round state persistence: none.** Votes/proposals live only in memory (`VotingState` maps, `voting.go:26-29`).
  `StoreVote`/`LoadVotes`/`StoreProposal`/`StoreEvidence` exist (`consensus/persist.go:253-508`) but have **zero
  call sites outside tests** (`persist_test.go`, `adversarial_test.go`). The live path persists only final blocks +
  tip via `AtomicStoreBlockAndTip` (`persist.go:146-179`). Note: `StoreVote`'s key is
  `height(8)+voteType(1)+validator(20)` — **no round** (`persist.go:301-305`) — any future vote persistence must add
  round to the key.

### 3. Blast radius

- **bbolt schemas**: `blocks`/`full_blocks`/`chain(tip)`/`proposals`/`votes`/`commit_proofs`/`evidence`
  (`persist.go:14-22`). Header layout fixed at 244 bytes (`persist.go:185`). Adding a header round field → 248 bytes +
  every stored header invalid → **chain reset / re-sync** (see Risks).
- **RPC surface**: **16 methods** (`rpc/server.go:162-199`): `dsn_getBlock … dsn_getPendingTxs`. **None** expose
  votes, rounds, or consensus status. (Docs "16 methods" ✅ `docs/product-strategy.md:45`; `openspec/config.yaml:10`
  claims 17 ❌ — stale.)
- **Metrics**: production `/metrics` emits exactly one gauge, `dsn_block_height` (`node/startup.go:767-773`).
  `telemetry/metrics.go` defines 8 metrics, **none consensus/round-related** (the earlier
  `production-readiness/exploration.md` claim of a defined "consensus round" metric is stale for the current tree).
  Adding round metrics is net-new work.
- **Wire types** (`network/message.go:10-21`): 13 types; `MsgTypeVote 0x42` carries votes (`p2p.go:503-514`). No
  proposal type on the wire — a proposal IS a proof-less block frame (`consensus/gossip.go:18-111`).
- **Tests referencing rounds**: `node/consensus_loop_test.go` (hardcoded `Round: 0` at `:189`; loop/re-broadcast/
  stale-refresh/epoch-boundary regression tests), `node/proposer_test.go:156,162`, `node/node_test.go:545,548`,
  `consensus/voting_test.go` (~15 sites, plus `TestVotingState_WrongRound`), `consensus/bft_harness.go` (all rounds
  0, but already simulates proposer equivocation, voter double-vote, partition). Integration "round" references
  (`integration/convergence_test.go`) are transaction-generation loops, not consensus rounds. No consensus spec
  exists in `openspec/specs/` (domains: bip39-mnemonic, sdk-compat, wallet-cli, wallet-keystore) — this change
  introduces a new spec domain.
- **Docs**: `docs/ARCHITECTURE.md:398-406` documents the *intended* multi-round model ("If the proposer fails or the
  block doesn't reach finality, the protocol moves to the next round") — aspirational, not implemented. The protocol
  spec (`docs/dsn_protocol_spec_v_1.md`) has no round/view-change content.

### 4. Claim reconciliation (background facts vs code)

| Claim | Verdict |
|---|---|
| `Round: 0` hardcoded at `node/node.go:953` | ✅ Confirmed — plus a second hardcode at `node.go:760` |
| No view change; stuck proposer stalls the chain | ✅ Confirmed — no round advancement anywhere |
| BFT-lite **2-phase** weighted voting (preVote→preCommit) | ✅ Confirmed — exactly two `VoteType`s (`types/vote.go:14-17`); ARCHITECTURE.md:404-406 calls it two-phase |
| "Pipelined **3-phase** BFT voting" (`config.yaml:7`) | ⚠️ Loose/stale — it is a stage count (proposal + 2 vote types); there is no third vote phase |
| Round-robin proposer | ✅ Weighted round-robin by voting power (`proposer.go:11`) |
| Evidence-based slashing | ⚠️ Plumbing only — v1 = `DoubleSignEvidence` only; evidence pool is a TODO (`node.go:682`), slashing never fires on the live path |
| k=6 heuristic finality, 1s block, 5s proposer timeout | ✅ `finalityK=6` (`node.go:80`), `config.go:117`, `config.go:101` |
| RPC: 16 methods | ✅ (`rpc/server.go:162-199`); config.yaml's "17 methods" ❌ |

## Affected Areas

- `types/block.go` — add `Round uint32` to `BlockHeader` + `HeaderHash` (chain-hash impact, see Risks)
- `consensus/persist.go` — `encodeHeader`/`decodeHeader` 244→248 layout (`:185-250`)
- `consensus/gossip.go` — `EncodeBlockMessage`/`DecodeBlockMessage` header round field (`:24-42`, `:131-166`)
- `consensus/proposer.go` — `WeightedProposerAtHeightAndRound(height, round)` (offset `(height+round) % totalPower`; round 0 ≡ today)
- `consensus/block_validator.go` — round-aware proposer check (`:153`), require precommit round == block round (`:309-314`)
- `consensus/voting.go` — already round-ready; no change required (add round-aware helpers if needed)
- `types/commit.go` — optional explicit `Round` on `CommitProof` if not deriving from header
- `node/node.go` — `newVote` round (`:953`), `NewVotingState` round (`:760`), pending-round state machine (accept higher round when unvoted, `:979-1002`), loop round advancement on timeout (`:579-632`), rebroadcast/stale-refresh for round>0 (`:839-870`)
- `node/config.go` — round timeout base / max round
- `network/message.go`, `network/p2p.go` — only if a dedicated proposal wire type is chosen (not needed if round lives in the header)
- `telemetry/metrics.go`, `node/startup.go` — `dsn_consensus_round` / `dsn_consensus_height` gauges (net-new)
- Tests: `node/consensus_loop_test.go`, `node/proposer_test.go`, `node/node_test.go`, `consensus/voting_test.go`, `consensus/bft_harness.go`, `consensus/persist_test.go` — update round-0 hardcodes + add view-change tests
- `docs/ARCHITECTURE.md`, `docs/dsn_protocol_spec_v_1.md`, `openspec/config.yaml` — doc/spec alignment (2-phase, round semantics)

## Approaches

1. **Minimal liveness patch — timeout + "skip" the stalled proposer** — keep one proposer per height; on timeout,
   have the next validator produce the block for the same height.
   - Pros: smallest diff surface on the surface; reuses the existing rebroadcast/stale-refresh loop.
   - Cons: **not viable as described** — a block signed by any proposer other than `WeightedProposerAtHeight(height)`
     fails `ErrWrongProposer` (`block_validator.go:153-156`); height continuity forbids skipping the height
     (`block_validator.go:81-84`; blocks are height-keyed in bbolt and block-sync). "Skipping" *is* a round-aware
     proposer change → collapses into Approach 2.
   - Effort: n/a (subsumed)

2. **Round-based view change with per-round voting (recommended)** — extend the existing 2-phase scheme:
   - `WeightedProposerAtHeightAndRound(height, round)` with offset `(height+round) % totalPower`; round 0 preserves
     today's proposer exactly (backward-compatible schedule).
   - Round travels in the **block header** (new `Round uint32` field; mirrors how `Epoch` was added — `types/block.go:19`,
     `persist.go:206-207`). Cleanest of the two placement options: proposals, final blocks, sync/replay, fork handling
     all read the round from the same place; no new wire message type; `validateCommitProof` gains a
     precommit-round == header-round check.
     (Alternative placement — round only in `CommitProof` + a new `MsgTypeProposal` envelope + validation reorder —
     avoids the header-hash break but adds a wire type and splits round sources across two structures; not
     recommended.)
   - Node state machine: track `pendingRound`; a validator votes once per `(height, round)` and may replace its
     pending round with a **higher** round only if it hasn't voted in the current one (mirrors the existing
     `replace` + `votingHasExternalVotes` guards, `node.go:725-738`, `:789-805`). After `ProposerTimeout × (1+r)`
     (deterministic per-round backoff) without finalization, advance to round r+1; the new proposer proposes fresh
     (same snapshot-validate-revert pattern as today, `node.go:679-785`).
   - Safety tightening (cheap, aligns code with `ARCHITECTURE.md:406` + harness): **gate precommits on 2/3 prevote
     majority** instead of precommitting immediately (`node.go:1000-1001`).
   - Pros: closes the stuck-proposer gap with the existing 2-phase machinery; every type already carries or can carry
     a round; small, deterministic proposer-schedule delta; harness already simulates the Byzantine cases.
   - Cons: header round field changes every block hash → **chain reset / re-sync required** (acceptable at v0.1.0,
     zero tags, no prod data — but must be an explicit user decision); without Tendermint-style locking, two
     conflicting blocks *can* each reach 2/3 precommits at *different* rounds under adversarial partitions — the
     chain stays single-valued (first-arrival + fork rules, `node.go:1187-1189`) but a stale light client could see
     two valid commit proofs for the same height; k=6 heuristic finality remains the chain's finality answer.
   - Effort: **Medium-High** (types + consensus + node + wire + tests + doc alignment)

3. **Full PBFT-style multi-round with prepare/commit per round** — add a prepare vote phase, locked value/round
   (Tendermint-style `lockedValue`/`lockedRound`), +2/3-prevote round advancement, and amnesia/fork evidence.
   - Pros: eliminates the cross-round dual-commit caveat; production-grade liveness+finality semantics.
   - Cons: a consensus rewrite (new `VoteType`, voting state machine rework, locking rules, new evidence types,
     message changes); contradicts DSN's BFT-lite ethos (docs `product-strategy.md:40`); the most recently stabilized
     code path (convergence fixes) is the blast radius.
   - Effort: **High-Very High**

## Recommendation

**Approach 2 — round-based view change with per-round voting**, scoped as a v1 liveness change:

- Round lives in the block header (new `Round uint32` field, following the `Epoch` precedent), with the
  **explicit chain-reset consequence surfaced to the user in the proposal** (v0.1.0, no production data).
- Proposer schedule `(height+round) % totalPower` with round 0 ≡ today — deterministic, validator-set-order-safe
  (the ordering guarantee comes from `staking.GetActiveValidators`, the same input `WeightedProposerAtHeight` uses).
- Deterministic per-round timeout backoff (`ProposerTimeout × (1+r)`, capped) so all honest validators advance in
  lockstep.
- Precommit gating on 2/3 prevotes as part of v1 — it aligns the code with its own architecture doc and the harness,
  and materially reduces the cross-round dual-commit window.
- Include round/vote metrics (`dsn_consensus_round`, `dsn_consensus_height`) — they are net-new and cheap, and
  observability of view changes is precisely what ops needs.
- Defer locking, amnesia/fork evidence, and automatic evidence-pool slashing to a follow-up change; document the
  cross-round caveat.

## V1 Scope (in) / Non-Goals (out)

**In**: round field on header + hash + wire + bbolt layout; `WeightedProposerAtHeightAndRound`; round-aware block
validation (proposer + precommit-round checks); node pending-round state machine with higher-round replacement;
timeout-driven round advancement with backoff; prevote-majority precommit gating; round metrics; doc/config
alignment (2-phase wording, round semantics, 16 RPC methods); tests: stuck-proposer view change, equivocation →
round advance, cross-round fork resolution, round-0 backward-compat schedule.

**Out (non-goals)**: BLS aggregate signatures (per-vote Ed25519 stays); Tendermint-style locking / locked value;
cross-round double-sign detection (amnesia/ForkEvidence) — `DoubleSignEvidence` stays same-round
(`types/evidence.go:66-68`); evidence-pool wiring / automatic live slashing (`node.go:682` TODO — separate change);
RPC round/vote exposure (`dsn_getRound` etc.); P2P identity/encryption; delegation/governance; sharding;
consensus rewrite (Approach 3); DB *migration* for the header layout — accepted chain reset with re-sync instead.

## Risks

- **CRITICAL — chain reset.** Adding `Round` to `BlockHeader` changes `HeaderHash` (`types/block.go:77-102`), so
  every block hash changes and all existing chains/databases must re-sync; bbolt header layout goes 244→248
  (`persist.go:185`). Must be an explicit user decision in the proposal with a documented re-sync procedure. (This is
  the price of the clean placement; the `CommitProof`-only alternative avoids it at the cost of a new wire type and
  split round sources.)
- **WARNING — cross-round dual-commit without locking.** Under adversarial partitions, two different blocks at the
  same height can each reach 2/3 precommits at different rounds (quorum intersection: a validator precommits A@r0
  then, after round advance, B@r1). The chain remains single-valued via first-arrival + fork rules
  (`node.go:1187-1189`), but commit proofs are not absolute finality across rounds until k=6. Mitigation: prevote
  gating in v1 + explicit documentation; full fix is Approach 3 (non-goal).
- **WARNING — regression risk on freshly stabilized code.** The pending-round guards, rebroadcast, stale-refresh,
  and epoch-boundary logic were hard-won fixes (`consensus_loop_test.go` regression suite). Round state touches all
  of them. Mitigation: keep the four existing loop tests green as the regression gate; add view-change tests before
  implementation (strict TDD is active).
- **WARNING — clock determinism.** Round advancement is wall-clock based; with a ±5s timestamp skew window
  (`types/timestamp.go:8`), timeouts must be generous vs. clock skew or honest validators will advance
  desynchronized. Use a shared base (`ProposerTimeout`, 5s default) with documented minimums.
- **WARNING — proposer-schedule determinism.** All validators must derive identical proposers for `(height, round)`;
  the schedule must use the exact same validator ordering as `WeightedProposerAtHeight` (sorted by power DESC,
  ConsensusID ASC via `staking.GetActiveValidators` — `proposer.go:9`, `block_validator.go:129-132`).
- **INFO — vote persistence key lacks round** (`persist.go:301-305`). Not blocking (votes are not persisted on the
  live path), but any future evidence/audit persistence must add round to the key.
- **INFO — review budget.** This change likely exceeds the 400-line authored budget (types + consensus + node + wire
  + metrics + tests). `sdd-tasks` should forecast chaining: (1) types/wire/validation plumbing, (2) node round state
  machine + loop, (3) metrics + docs.

## Open Questions (for proposal/spec)

1. Chain reset acceptable? (Required for the header-field placement.) If not, the CommitProof-envelope variant must
   be chosen instead.
2. Round timeout policy: linear `base × (1+r)` vs capped exponential `base << r`; and a `MaxRound` guard (infinite
   rounds vs. give-up-and-wait)?
3. Should `ProposerTimeout` remain the single knob (base round timeout) or add a dedicated `RoundTimeoutBase`?
4. Precommit-gating change: in v1 (recommended) or deferred to a separate change to shrink this one?
5. Metrics: wire `telemetry` package metrics into the production `/metrics` handler (fixes the prod-path gap for
   these two gauges) or hand-roll like `dsn_block_height` (`startup.go:770-772`)?

## Ready for Proposal

**Yes.** The orchestrator should tell the user: (1) the docs are right — `Round: 0` is hardcoded (`node.go:953`
and also `:760`), the chain stalls deterministically on a stuck proposer, and there is no view change anywhere;
(2) the recommended fix is **round-based view change** extending the existing 2-phase scheme, with the round carried
in the block header; (3) that placement **changes every block hash — a chain reset/re-sync**, which must be approved
explicitly at the v0.1.0 stage; (4) locking/evidence-pool/full-PBFT are explicit non-goals for v1.
