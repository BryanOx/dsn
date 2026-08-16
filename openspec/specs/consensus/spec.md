# Consensus Specification

## Purpose

Multi-round liveness for DSN's 2-phase (preVote→preCommit) BFT-lite consensus. Scenarios are strict-TDD tests, written before implementation.

## Requirements

### Requirement: Header Round and Chain Impact

`BlockHeader` MUST carry `Round uint32` hashed in `HeaderHash`; bbolt layout MUST grow 244→248 (persist.go:185). Existing chains MUST re-sync (rollback = revert + re-sync); round 0 MUST reproduce today's behavior.

#### Scenario: Existing chain re-syncs

- GIVEN a 244-byte header chain
- WHEN a new-code node starts
- THEN it MUST reject stale headers and re-sync

### Requirement: Round-Aware Proposer Schedule

`WeightedProposerAtHeightAndRound(height, round)` MUST use offset `(height+round) % totalPower` with `GetActiveValidators` ordering (power DESC, ConsensusID ASC), so all validators derive the same proposer. Round 0 MUST equal `WeightedProposerAtHeight`.

#### Scenario: Round-0 schedule unchanged

- GIVEN validator set V at height h
- WHEN computing proposer at (h, 0)
- THEN it equals `WeightedProposerAtHeight(h)`

### Requirement: Round-Aware Validation

Proposer check MUST use (height, round); unscheduled proposer MUST fail `ErrWrongProposer` (round 0 ≡ today; block_validator.go:153). `CommitProof` precommits MUST carry the header round; mismatches MUST be rejected (block_validator.go:309).

#### Scenario: Wrong proposer rejected

- GIVEN proposal at (h, r) from an unscheduled validator
- WHEN `ValidateBlockProposal` runs
- THEN it fails with `ErrWrongProposer`

#### Scenario: Precommit round mismatch rejected

- GIVEN a round-r block with round r−1 precommits
- WHEN `validateCommitProof` runs
- THEN the proof is rejected

### Requirement: Node Pending-Round State Machine

A validator MUST vote at most once per (height, round). Higher-round proposals MUST be accepted only if unvoted in the current round (mirrors `replace`/`votingHasExternalVotes`, node.go:725); otherwise dropped. New proposers MUST propose fresh via snapshot-validate-revert.

#### Scenario: Higher round accepted when unvoted

- GIVEN a validator unvoted in round r
- WHEN a round r+1 proposal arrives
- THEN the node votes once in round r+1

#### Scenario: One vote per round

- GIVEN a validator already voted in round r
- WHEN another round-r vote arrives
- THEN it is dropped

### Requirement: Timeout-Driven Advancement

Without finalization, the node MUST advance to r+1 after `ProposerTimeout × (1+r)` (linear backoff; node/config.go:101), MUST NOT advance past `MaxRound` (wait/rebroadcast at cap), and MUST keep timeout minimums above the ±5s skew window (timestamp.go:8).

#### Scenario: Stuck proposer → view change

- GIVEN stalled round-0 proposer, no finalization
- WHEN the round-0 timeout elapses
- THEN the node advances to round 1 and a round ≥1 block finalizes

#### Scenario: MaxRound caps advancement

- GIVEN the node at `MaxRound`, no finalization
- WHEN the round-`MaxRound` timeout elapses
- THEN the node MUST NOT advance further

#### Scenario: Clock-skew tolerance

- GIVEN a timeout base at or below the ±5s skew window
- WHEN configuration is validated
- THEN it MUST be rejected

### Requirement: Precommit Gating

A validator MUST precommit only after observing 2/3 prevotes for the block in the current round (aligns node.go:1000-1001 with ARCHITECTURE.md:406, bft_harness.go:323).

#### Scenario: Precommit withheld below majority

- GIVEN a proposal with fewer than 2/3 prevotes
- WHEN it is handled
- THEN the node sends only a prevote

#### Scenario: Precommit sent at majority

- GIVEN 2/3 prevotes for the block in round r
- WHEN the majority is observed
- THEN the node precommits that block at round r

### Requirement: Equivocation and Cross-Round Forks

Conflicting proposals for the same height MUST remain dropped (node.go:981). Double-vote detection MUST stay in-memory per `VotingState`; cross-round amnesia evidence is a documented NON-GOAL (evidence.go:66). Forks MUST stay single-valued via first-arrival + fork rules; k=6 remains the finality answer, caveat documented.

#### Scenario: Equivocation → round advance

- GIVEN conflicting proposals in round r, no precommit majority
- WHEN the round timeout elapses
- THEN the node advances to r+1 and votes once

#### Scenario: Cross-round fork single-valued

- GIVEN two blocks at 2/3 precommits in different rounds, same height
- WHEN a final block is applied
- THEN the chain stays single-valued; k=6 is the finality answer

### Requirement: Round Metrics

The production `/metrics` endpoint MUST expose `dsn_consensus_round` and `dsn_consensus_height` gauges via `telemetry` (startup.go:767; today only `dsn_block_height`).

#### Scenario: Gauges exposed

- GIVEN a running node
- WHEN `/metrics` is scraped
- THEN both gauges report the current (height, round)

### Requirement: Docs and Config Alignment

`docs/ARCHITECTURE.md` MUST document the implemented round/view-change semantics; `docs/dsn_protocol_spec_v_1.md` MUST gain consistent round content; `openspec/config.yaml` context MUST state 2-phase round semantics and 16 RPC methods (config.yaml:7, :10).

#### Scenario: Config and docs accurate

- GIVEN `openspec/config.yaml` and `docs/dsn_protocol_spec_v_1.md`
- WHEN both are reviewed
- THEN config states 2-phase round consensus and 16 methods
- AND the protocol spec covers rounds
