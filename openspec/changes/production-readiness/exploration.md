# Exploration: Production Readiness

> Artifact: `sdd-explore` for change `production-readiness` · 2026-08-03
> Evidence-based assessment of DSN's current stage and what is missing for a production-ready version.
> All findings verified against source on branch `main` @ `6bec8a6` (46 commits ahead of `origin/main`, nothing pushed).

## Current State

DSN is a functional but **pre-production** BFT blockchain ("v0.1.0-sandbox", README:8). The core loop works: a 3-node
localnet converges and passes epoch transitions with 0 errors (fixes `f34dd20`, `6bec8a6`), staking/epochs/slashing
machinery exists, and there is a substantial test suite (105 `_test.go` files, unit + integration + soak). However, the
system has **no network catch-up path at all**, no end-to-end snapshot sync, an almost empty metrics surface on the
production path, a stubbed RPC layer, drifted CI, and no release pipeline. A validator that falls behind — or joins
late — is permanently stuck at genesis height.

### The late-joiner gap (P0 — verified, triple-confirmed dead end)

A node that misses the height-1 proposal has NO way to catch up:

1. **`Peer.SyncHeight` is never assigned anywhere.** It is declared (`network/peer.go:72,94`), serialized into
   `PeerRecord` (`peer.go:126,181`), and initialized to 0 in the constructor (`peer.go:347`), but no code path ever
   writes the node's actual chain height into it (`grep '\.SyncHeight\s*='` → zero matches). Block sync's catch-up
   trigger compares `p.SyncHeight > maxPeerHeight` (`network/blocksync.go:220`) → every peer reports 0 → "already
   synced" → `startCatchUp` never fires.
2. **Block range handlers are not wired.** The read loop explicitly drops both directions: `case MsgTypeBlockRangeRequest,
   MsgTypeBlockRangeResponse: // Valid but no block range handlers wired yet.` (`network/p2p.go:378-380`). A peer that
   does send a range request gets no answer; a range response is discarded.
3. **`BlockSyncEngine.SetBlockHandler` is never called in production code** (only in tests). Even if a response arrived,
   `HandleBlockRangeResponse` hits `if handler == nil { return }` (`network/blocksync.go:157`) and drops it.
4. **Fast sync from the network is explicitly unimplemented.** `node/fastsync.go:74` returns
   `"fast sync from network not yet implemented; use FastSyncFromCheckpoint..."` and `SyncFromNetwork` fails at
   `node/fastsync.go:224` (`"P2P snapshot download not fully implemented: peer tracking required"`).

Reproduction was observed once on the localnet (node2 stayed at 0 after a late connect) — consistent with the above.

### Snapshot sync: local creation works, serving/fetching dead (P0)

Snapshots are **created** locally at epoch boundaries (`node/node.go:968-980`, `1172-1183`) and stored via
`state.StoreSnapshot`. But the serving side is never registered: `SetSnapshotQueryHandler` / `SetSnapshotRequestHandler`
/ `SetSnapshotChunkHandler` have **zero call sites** (grep confirms only the setter definitions in `network/snapshot.go`).
`FastSyncEngine.HandleSnapshotInfo`/`HandleSnapshotChunk` are therefore unreachable — the engine's state machine
(`network/fastsync.go`) is dead code from the network's perspective. `FastSyncEngine.Start()` is only invoked when
`FastSyncEnabled && persistent != nil` (`node/node.go:458`, `startup.go:629`), after which it queries peers that never
answer. Snapshot sync does **not** work end-to-end and does **not** cover the late-joiner gap.

### RPC surface: 16 methods, several stubs, auth never wired (P1)

- Dispatch table has **16** methods (`rpc/server.go:160-207`): `dsn_getBlock, dsn_getTransaction, dsn_sendTransaction,
  dsn_getAccount, dsn_getBalance, dsn_getContract, dsn_callContract, dsn_estimateGas, dsn_getContractState,
  dsn_getTransactionReceipt, dsn_getEvents, dsn_getValidator, dsn_getValidators, dsn_getSupply, dsn_getStateRoot,
  dsn_getPendingTxs`. README/docs claim 17 — doc drift.
- Stubs confirmed in `rpc/service/node_service.go`: `GetEvents` hard-errors "requires event indexer (Phase 6)"
  (`:372`); `GetTransaction` is mempool-only (`:81-92`); `GetTransactionReceipt` mempool-only or not-found (`:525-554`);
  block-by-hash unavailable (`:46-48`); `GetBlock` requires the node's own block store (`:51-54`).
- HTTP surface is minimal: only `POST /`, `GET /ws`, `GET /health` (`rpc/router.go:99-109`). No `GET /status`, no REST
  block/validator endpoints — matches the localnet observation (root empty, `/status` 404).
- Router has rate limiting, body-size cap, security headers, and optional API-key auth — but the CLI starts the server
  with `rpc.NewWithService(svc)` → `DefaultRouterConfig()` → `ApiKey: ""` → **auth never applied** (`cmd/dsn/node_cmd.go:132`).
  `NewWithSecurityConfig` exists (`rpc/server.go:77`) but is unused. CORS is wildcard (`server.go:109`).

### Observability: ~24 metrics defined, 1 served on production (P1)

`telemetry/metrics.go` + `telemetry/defensive.go` define ~24 Prometheus metrics (RPC counters/histograms, mempool,
peers, consensus round, block processing, gossip drops, …) registered via `promauto` on the default registry. But the
production node's metrics server hand-rolls ONLY `dsn_block_height` as literal text (`node/startup.go:767-773`) and
never touches the registry. The full registry is served only in devnet mode via `promhttp.Handler()`
(`cmd/dsn/devnet.go:241`). The localnet runs `dsn node start` (`dev/localnet/docker-compose.yml:20`) — hence the
observed single metric. The Grafana dashboard and Prometheus rules (`monitoring/…`) reference metrics the node never
emits — the docs admit this (`docs/OPERATIONS.md:1309-1310`).

### CI/CD & release (P1)

- `.github/workflows/ci.yml:14,23,35` and `release.yml:12` pin Go **1.22**; `go.mod:3` requires **1.25.7**. Local Go is
  1.26.0. CI may limp through via `GOTOOLCHAIN=auto` download, but the pin is wrong, every run pays a toolchain
  download, and it fails outright if download is blocked. Unintentional drift — not a deliberate matrix.
- No coverage threshold, no staticcheck in CI (Makefile probes it, `which` gate), no integration/soak suite in CI, no
  TUI build (`cmd/dsn-tui` zero tests), no version stamping in release builds (Makefile's `-ldflags -X main.Version`
  is not used by the workflows).
- `release.yml` exists (tag-triggered cross-compile + GitHub release) but has never run: **zero tags, single `main`
  branch, 46 commits / 18,372 insertions ahead of origin, nothing pushed.**

### Consensus & finality (P2)

- Pipelined 3-phase voting (proposal → prevote/precommit → commit-proof block) with round-robin weighted proposer
  selection from the staking registry (`node/node.go:387-396`, `weightedProposerAtHeight`). Recent fixes make it
  converge on 3 nodes with retransmit + idempotent finalization — treat as working.
- **Single-round only**: `newVote` hard-codes `Round: 0` (`node/node.go:857-868`); there is no round increment, no
  view-change, no liveness mechanism if the designated proposer is down/Byzantine. Timeout-based re-broadcast
  (`rebroadcastPendingProposal`) helps a stalled proposer but cannot rotate.
- Finality is a **k=6 heuristic** (`node/node.go:79`, `991-993`), not BFT-finality from the commit proof.
- Slashing exists but is **double-sign only** (`consensus/slashing.go:63-67`); the proposer's evidence source is a
  TODO (`node/node.go:606` — `// TODO: Get evidence from evidence pool`); no equivocation detection or evidence
  propagation on the wire. Partition/byzantine behavior is unproven (staging docs describe tests that are not run).

### P2P resilience (P2)

Healthy in the happy path: bootstrap reconnect with backoff and idempotency (`network/peer_discovery.go`, fixes
`00a7fde`, `e29ba60`), ping health-checks with disconnect after 3 failures, PEX exchange, per-IP connection limiting
(5/s, 60s temp bans, max 50 peers) and per-message token buckets (`network/p2p.go:20-99,318-336`). But the resilience
story collapses once a peer is behind (see late-joiner gap) and there is **no cryptographic identity**: `PeerID` is
derived from the remote TCP address string (`p2p.go:210` — `PeerIDFromBytes([]byte(remoteAddr))`). No handshake, no
peer auth, no encryption. The `network/discovery.go` `Discovery` type (bootstrap PEX) is dead code — only
`NewPeerDiscovery` is instantiated (`node/node.go:196`).

### Persistence & recovery (P2)

- BoltDB gives ACID + WAL; `FSync` option for durability (`node.Config`, docs OPERATIONS.md:512). `AtomicStoreBlockAndTip`
  + `CommitState` per block with panic on failure (`node/node.go:1121-1127`).
- `node.Recover()` handles state-root-vs-checkpoint drift and local snapshot restore + replay (`node/recovery.go`),
  but recovery depends on snapshots/checkpoints that only exist if the node produced epoch boundaries locally.
- Startup verifies genesis hash against stored hash and rejects mismatches (`node/startup.go:228-254`) — good
  integrity guard. DB versioning (`meta/db_version`) is written but not enforced (always 0 / legacy).

### Operations (P2)

- `VERSION` = 0.1.0; README/CHANGELOG say v0.1.0-sandbox (last release 2026-05-18). Upgrade story is a binary swap +
  restart; no DB migration path, no rolling-upgrade validation (docs describe the procedure; nothing automated).
- Helm chart + systemd units exist but are unvalidated, use placeholder peer IDs, and are self-described as sandbox
  (docs/OPERATIONS.md:1365-1368).
- Validator keys are written plain (JSON, Ed25519) with 0600 perms / 0700 dirs (`wallet/validator.go:38-44`) — correct
  file permissions, but no at-rest encryption, no HSM/KMS.
- Indexer exists but is optional; RPC queries that need it (events, receipts, tx lookup, block-by-hash) are stubs
  without it.

## Affected Areas

- `network/peer.go` — `SyncHeight` field dead (never written); catch-up trigger depends on it
- `network/blocksync.go` — `startCatchUp` dead; `onBlock` handler never set; range response path drops blocks
- `network/p2p.go:378-380` — block range messages explicitly unhandled in read loop; `PeerID` from TCP address (no crypto identity)
- `network/fastsync.go` — full FastSyncEngine state machine unreachable (handlers never wired to P2PNode)
- `network/snapshot.go` — serving handlers (`SetSnapshotQueryHandler`/`SetSnapshotRequestHandler`) never registered
- `node/fastsync.go` — `FastSync()`/`SyncFromNetwork()` return "not implemented"
- `node/node.go` — engines created but not connected to the message paths; evidence pool TODO (`:606`); k=6 finality (`:79`)
- `node/startup.go:767-773` — production `/metrics` emits only `dsn_block_height`
- `cmd/dsn/node_cmd.go:132` — RPC started without API key/TLS (`DefaultRouterConfig`)
- `rpc/router.go` — only `POST /`, `/ws`, `/health`; no `/status` or REST surface
- `rpc/service/node_service.go` — `GetEvents`/`GetTransaction`/`GetTransactionReceipt` stubs (indexer-gated)
- `telemetry/metrics.go`, `telemetry/defensive.go` — ~24 metrics defined, never exposed on production path
- `.github/workflows/ci.yml`, `.github/workflows/release.yml` — Go 1.22 vs go.mod 1.25.7; no coverage gate, no integration in CI
- `docs/OPERATIONS.md`, `monitoring/` — document metrics/endpoints/alerts that the node does not emit

## Approaches

1. **Incremental hardening, correctness-first (recommended)** — slice the gap: (1) catch-up/sync, (2) metrics
   exposure, (3) RPC completion + auth, (4) CI fix + release pipeline, (5) consensus liveness, (6) P2P identity.
   - Pros: each slice is testable and reviewable; preserves the hard-won convergence fixes; respects the 400-line
     review budget via chained PRs.
   - Cons: long runway; interim releases still not production-safe.
   - Effort: High (but the correct cost — this is a multi-change program, not one change).

2. **Adopt libp2p / replace the networking layer** — fix identity, handshake, encryption and sync in one move.
   - Pros: solves P2P security and gives battle-tested sync primitives.
   - Cons: throws away working convergence and discovery fixes; rewrites the most recently stabilized code; huge risk
     and review surface; a rewrite, not a hardening.
   - Effort: Very High.

3. **Stay sandbox-only** — document the gaps, gate the network, no production commitment.
   - Pros: zero risk; honest positioning.
   - Cons: fails the user's objective (production-ready version); every real deployment hits the same blockers.
   - Effort: Low.

## Recommendation

**Approach 1 — incremental hardening, correctness-first**, starting with the **block catch-up / state-sync** slice as
the first change: it is the single correctness blocker that makes every other production claim moot (a restarted or
late validator is permanently lost). That slice must (a) set `Peer.SyncHeight` from the node tip on a
schedule/on-block, (b) wire `MsgTypeBlockRangeRequest/Response` in the read loop to `BlockSyncEngine`, (c) register the
engine's `SetBlockHandler` to validate+apply via the existing block pipeline, and (d) keep the 3-node convergence
integration test as the regression gate. Snapshot serving/fetching can follow as the fast path for large gaps once
block range sync proves correct — snapshot-then-replay is riskier to get right and is not a prerequisite for catch-up.

## Risks

1. **Regression on the freshly stabilized consensus path.** The recent fixes (`f34dd20`, `6bec8a6`) made 3-node
   convergence work; catch-up code touches the same block-application pipeline. Mitigation: convergence + epoch
   transition integration tests as a mandatory gate for the first slice.
2. **Replay determinism.** Catch-up/replay must reproduce identical state roots; any nondeterminism (timestamps,
   mempool ordering, VM host calls) silently forks nodes. Mitigation: snapshot+replay certificate tests already exist
   (`integration/replay_cert_test.go`) — extend and run them in CI.
3. **Scope creep.** Production readiness spans 8 dimensions; without strict slicing the change balloons past the
   400-line review budget. Mitigation: chained PRs per slice; `Decision needed before apply` guard from sdd-tasks.
4. **Security gaps are architectural.** P2P identity from TCP address, no handshake/encryption, plaintext keys,
   no wired RPC auth — fixing these later implies protocol/API-breaking changes and deployment churn. Mitigation:
   treat them as separate explicit changes; document the trust model before external use.
5. **Documentation/observability drift.** Docs and dashboards already describe metrics/endpoints that don't exist;
   any hardening that adds surface will widen the drift unless docs are updated in the same change (work-unit commits).

## Ready for Proposal

**Yes.** The orchestrator should tell the user: stage = **alpha**; the first change should be **block catch-up /
state sync** (P0 correctness), followed by metrics exposure, RPC completion + auth, and CI/release fixes — each as a
separate SDD change. Recommended first slice scope: `SyncHeight` propagation, block-range handler wiring, engine
`onBlock` registration, plus convergence-regression gate.
