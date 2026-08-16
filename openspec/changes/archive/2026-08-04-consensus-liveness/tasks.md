# Tasks: Consensus Liveness — Multi-Round + View Change

## Review Workload Forecast

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
Estimated changed lines: ~700
Delivery strategy: ask-on-risk

### Work Units

- **U1 (PR1)** Wire+round validation — test `go test ./types/... ./consensus/...`; runtime `make test-integration`; rollback revert wire commit (round-0 alias)
- **U2 (PR2)** pendingRound+timeout/gating — test `go test ./node/... -run 'TestConsensusLoop\|TestConfig'`; runtime harness round>0 sim; rollback revert PR2 (node round-0-correct)
- **U3 (PR3)** Gauges+docs/config — test `go test ./node/... -run TestMetrics` + `curl -s :9464/metrics \| grep dsn_consensus_round`; runtime devnet boot+scrape; rollback revert PR3

## Phase 1 — PR1: Wire + Validation

- [x] 1.1 RED `consensus/persist_test.go`: 248B round-trip Round=7; 244B decode fails (:220). S1
- [x] 1.2 RED `types/block_test.go`: `HeaderHash` differs iff Round differs. S1
- [x] 1.3 GREEN `types/block.go`: `Round uint32` after `Epoch` (:19); hash Epoch→Timestamp (:92-95)
- [x] 1.4 GREEN `consensus/persist.go`: len 244→248 (:185); Round between Epoch (:208)/Timestamp; comment `Epoch(8)+Round(4)+Timestamp(8)+Proposer(20)`
- [x] 1.5 GREEN `consensus/gossip.go`: Round after Epoch Encode (:36)/Decode (:155); position pinned, ADR7 superseded
- [x] 1.6 RED `consensus/proposer_test.go`: `WeightedProposerAtHeightAndRound(h,0)==WeightedProposerAtHeight(h)`; r0≠r1. S2
- [x] 1.7 GREEN `consensus/proposer.go`: new fn `(height+round)%totalPower` (:28); old fn = round-0 alias
- [x] 1.8 RED `consensus/block_validator_test.go`: round-1 proposal, round-0 proposer → `ErrWrongProposer`. S3
- [x] 1.9 GREEN `consensus/block_validator.go:153`: use `WeightedProposerAtHeightAndRound(h, Header.Round, …)` — same fn as loop (:600)
- [x] 1.10 RED `consensus/block_validator_test.go`: round-r block + r−1 precommits → `ErrInvalidCommitProof`; round-0 passes. S4
- [x] 1.11 GREEN `consensus/block_validator.go:309-314`: reject `vote.Round != block.Round`; 0==0 keeps legacy

## Phase 2 — PR2: Round SM + Gating

- [x] 2.1 RED `node/consensus_loop_test.go`: stalled r0 proposer → deadline → round ≥1 finalizes (FIRST). S5
- [x] 2.2 RED: higher round accepted when unvoted (S6); second r vote dropped (S7)
- [x] 2.3 RED: `MaxRound` cap → no advance (S8); config rejects `ProposerTimeout ≤ 5s` (S9)
- [x] 2.4 RED: precommit <2/3 withheld, ≥2/3 sent (S10,S11); equivocation → advance (S12); cross-round fork single-valued (S13)
- [x] 2.5 GREEN `node/config.go`: `MaxRound=8` default + `DSN_MAX_ROUND` env; reject base ≤5s
- [x] 2.6 GREEN `node/node.go`: `pendingRound`+reset; `newVote` round param; `NewVotingState(h,round,…)`; one-vote-per-(h,r); equivocation dropped; precommit gated (`tryPrecommitLocked`)
- [x] 2.7 GREEN `node/node.go`: loop round-aware proposer; deadline `ProposerTimeout×(1+r)` capped MaxRound; rebroadcast (stale rebuild + own-prevote re-send)
- [x] 2.8 GREEN `consensus/bft_harness.go`: extend `simulateVoting` round>0; keep Round:0 sites valid; `SetProposalRound` helper

## Phase 3 — PR3: Metrics + Docs

- [x] 3.1 RED `node/startup_test.go`: scrape `/metrics` → both gauges show (h,r). S14
- [x] 3.2 GREEN `telemetry/metrics.go`: promauto gauges `dsn_consensus_round`, `dsn_consensus_height` (OPERATIONS.md:161)
- [x] 3.3 GREEN `node/startup.go`: promhttp in `handleMetrics` (:767-773); record on round change/finalize; keep `dsn_block_height`
- [x] 3.4 GREEN `docs/ARCHITECTURE.md:398-406`: aspirational → implemented semantics + cross-round caveat
- [x] 3.5 GREEN `docs/dsn_protocol_spec_v_1.md`: round section: header round, `(h+r)%power`, backoff, MaxRound, 2/3 gate
- [x] 3.6 GREEN `openspec/config.yaml`: :7 → 2-phase + per-round view change; :10 → 16 methods. S15

## Non-Goals

BLS, locking/amnesia, evidence-pool wiring (node.go:682), RPC round, governance, DB migration — chain reset.
