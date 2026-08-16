# Ecosystem Strategy — Goal B (Developer-First Infrastructure)

> Status: DRAFT v3 (supersedes the Goal-A-oriented `product-strategy.md` v2 for developer adoption; v2 stays as the later enterprise layer).
> Date: 2026-08-04
> Companion: `docs/developer-adoption.md` (adoption playbook — patterns still valid, product anchor there is stale; this document supersedes its product choice).

## 1. Objective

Goal A (a profitable company) is **secondary**. Goal B is **primary**: maximize network usage, developer adoption, token utility, ecosystem growth, and long-term value — infrastructure that only exists because DSN exists.

Audience: developers, startups, indie hackers, OSS maintainers, creators, and AI builders. NOT enterprise, NOT compliance, NOT regulation, NOT procurement.

**The question answered here:** *What product could only exist because DSN exists?* — with exact sources for transaction volume, a <20-function SDK, a 30-minute integration, an invisible blockchain, and natural token consumption.

### 10 constraints (carried from the research direction)

1. <30-minute integration
2. SDK with <20 public functions
3. Blockchain almost invisible
4. Users never need to understand crypto
5. Token consumed naturally
6. Every new app increases network usage
7. Third parties can build businesses on DSN
8. Ecosystem strengthens with more developers
9. Strong network effect
10. Must make sense even if token speculation disappeared

### Rejected outright

Another wallet, DEX, bridge, NFT marketplace, explorer, exchange, DAO framework; generic DeFi; meme coins; blockchain-as-database; anything that requires users to care about crypto.

## 2. Method

Three research tracks ran in parallel on 2026-08-04, all with fresh market evidence:

| Track | Scope | Key sources |
|---|---|---|
| 1. Adoption study | 19 infrastructure companies (Cloudflare, Vercel, Supabase, Stripe, Twilio, GitHub, PlanetScale, Firebase, Railway, Fly.io, Render, Netlify, Clerk, Neon, Convex, Replicate, HuggingFace, OpenRouter, Resend) | docs/developer-adoption.md |
| 2. API economy | OpenRouter fees/expiry, MCP market, x402, Tempo (Stripe+Paradigm L1), MPP, RapidAPI, Replicate pricing, API call volumes | OpenRouter terms, MCP registry, x402/Stripe announcements 2026 |
| 3. Dev primitives | OSS funding (Tidelift, thanks.dev, GitHub Sponsors, Polar), webhook receipts (Svix, Hookdeck), licensing (Keygen, Cryptlex), machine identity/agent payments (x402, Cloudflare Wallets, agent frameworks) | provider sites, 2026 releases |

**The synthesis is one platform primitive plus wedges, not a list of unrelated products.** All three tracks converged on the same open slot.

## 3. The verdict

### Winner: CredRail — a portable, self-custodied, programmable developer-credit ledger with neutral metered settlement and verifiable receipts

One sentence: **a neutral credit + receipt layer for the machine economy — the anti-OpenRouter — where devs hold non-expiring, refundable, budget-capped credits that any API, MCP tool, or agent can accept, and every charge is a verifiable receipt.**

Why it can only exist because DSN exists: a centralized "portable credits" company would BE a bank — custodial, expiring, fee-taking, opaque. That is exactly OpenRouter's model and exactly why every existing credit system fails devs (see §7). Self-custody, neutrality, near-zero fees, 1-second finality, and account-model budget enforcement are not product features you can build in a SaaS — they require a settlement layer no single company owns. DSN is that layer.

## 4. TOP 20 ranked (Goal B lens)

Scores are Goal-B weighted: developer adoption, network usage/tx/day, SDK adoption, token utility, defensibility, platform potential. Wedges marked `[W]` are market entry points of the same CredRail primitive.

| # | Opportunity | Dev | Net | Token | Def | Plat | Score | Verdict |
|---|---|---|---|---|---|---|---|---|
| 1 | **CredRail** — portable dev credits + settlement + receipts | 10 | 9 | 9 | 8 | 10 | **87** | **WINNER** |
| 2 | **AgentReceipt** — verifiable authz/receipt layer for agent txs | 9 | 8 | 8 | 8 | 9 | 76 | Second product on same primitive |
| 3 | **MCPPay** `[W]` — monetize MCP servers | 9 | 8 | 8 | 6 | 8 | 74 | Wedge #1 (12,770 servers, <5% monetized) |
| 4 | **EvalLedger** — verifiable AI eval/benchmark attestation | 8 | 6 | 7 | 8 | 7 | 72 | Standalone attestation product |
| 5 | **DevBudget** `[W]` — protocol-enforced agent spending caps | 9 | 7 | 8 | 6 | 7 | 70 | Wedge #2 (denial-of-wallet) |
| 6 | **API402** `[W]` — HTTP 402 drop-in for any API | 9 | 8 | 8 | 6 | 8 | 68 | Wedge #3 (per-call API monetization) |
| 7 | **OpenRouterRail** `[W]` — no-expiry, refundable, portable LLM credits | 8 | 8 | 9 | 6 | 8 | 66 | Wedge #4 (headline demo) |
| 8 | **DataRail** `[W]` — pay-per-record data/vector API settlement | 7 | 7 | 7 | 6 | 7 | 63 | Wedge #5 |
| 9 | **ComputeCred** `[W]` — GPU/cloud credit settlement | 7 | 7 | 7 | 6 | 7 | 60 | Wedge #6 |
| 10 | **MeterAPI** — neutral usage metering + credits for SaaS billing | 7 | 6 | 7 | 6 | 7 | 58 | Overlaps Stripe/Orb; niche value |
| 11 | **AgentIdentity** — self-custodied agent identity + attestation | 8 | 5 | 6 | 7 | 7 | 55 | Contested; subsumed by AgentReceipt |
| 12 | **KeyRegistry** — machine identity/pubkey registry | 6 | 5 | 5 | 6 | 6 | 52 | Commodity; standard needed first |
| 13 | **SignVault** — developer artifact signing/notarization | 6 | 4 | 5 | 6 | 5 | 50 | Niche; low volume |
| 14 | **WebhookNotary** — adversarial-proof webhook delivery receipts | 5 | 5 | 4 | 6 | 5 | 47 | Narrow fintech niche (track 3: claimed) |
| 15 | **LicenseGuard** — on-chain licensing for indie devs | 5 | 4 | 5 | 5 | 5 | 45 | Contested (Keygen/Cryptlex); resale thin |
| 16 | **OSSustain** — usage-attributed OSS funding | 5 | 4 | 4 | 4 | 5 | 42 | Demand-side blocked (track 3) |
| 17 | **StreamPay** — per-second streaming settlement | 5 | 5 | 5 | 4 | 5 | 40 | Rail claimed (MPP/Tempo) |
| 18 | **ProvenanceAPI** — SBOM/provenance registry | 4 | 4 | 4 | 6 | 4 | 38 | Enterprise-ish; CRA-driven |
| 19 | **RateLedger** — cross-provider rate-limit/quota ledger | 4 | 4 | 4 | 4 | 4 | 35 | Low differentiation |
| 20 | **TimeStamp** — generic timestamping/attestation API | 4 | 3 | 3 | 3 | 3 | 32 | Commodity |

## 5. TOP 5 profiles

### 5.1 CredRail (winner) — see §6 for the full treatment

### 5.2 AgentReceipt (76) — second product on the same primitive
Problem: agents execute transactions on behalf of users (MCP calls, API usage, tool calls) and neither side can prove what was authorized or what happened. x402 moves money but leaves no neutral, non-repudiable authorization+receipt trail.
Why DSN: Ed25519 keys + cheap attestation = an agent signs an authorization intent, the charge is idempotent (IntentID), and the receipt is a verifiable record any party can check. 5-minute drop-in for agent frameworks; both sides gain.
Where txs come from: each authorized agent action anchors a receipt; volume = agent actions, the fastest-growing category in 2026.
Risk: identity layer is pre-consolidation; expect shakeout late 2026-2027 — being the neutral receipt layer, not another identity wallet, is the defensible position.

### 5.3 MCPPay (74) — wedge #1
Market evidence: 12,770+ MCP servers, 97M+ SDK downloads, <5% monetized, average price $0. The MCP economy is real and unfunded.
Why DSN: drop-in `@dsn/cred` — an MCP tool wraps its handler, the credit charge is idempotent, the receipt is verifiable. The server author monetizes without building payments; the agent user gets budget caps.
Where txs come from: paid MCP tool calls. 1% of 12,770 servers monetizing × 10-100 paying users each = 1.3K-13K paying devs; at 50-500 paid calls/dev/day → §6.5 volume math.

### 5.4 EvalLedger (72) — standalone attestation product
Problem: AI evals are copied, faked, and unverifiable; benchmark claims are trust-me.
Why DSN: tamper-evident attestation of eval runs, data hashes, and model responses — the same primitive family as v2's AuditAnchor but for the developer/benchmark audience, not enterprise compliance.
Where txs come from: each eval run attestation = 1 tx; volume is modest but every run adds network usage.
Why it ranks below CredRail: volume ceiling and weaker two-sided network effect.

### 5.5 DevBudget (70) — wedge #2
Problem: the #1 OpenRouter complaint is agent loops burning balances with no hard stop. Centralized credits cannot enforce a hard stop you trust.
Why DSN: account model = the vault balance IS the hard cap — a charge that would exceed the balance cannot settle, period. Per-call caps and per-provider budgets are SDK+protocol enforced. This property is the denial-of-wallet protection and it is what centralized credits structurally cannot offer.
Where txs come from: budget toggles, top-ups, per-call charges.

## 6. The winner, in full: CredRail

### 6.1 What it is

A developer-credit platform:

- **Self-custodied vaults.** A dev creates a vault (SDK manages the Ed25519 keypair invisibly; recovery via mnemonic). The credit balance lives on DSN, not in a company database.
- **Portable, non-expiring credits.** A credit is a claim on any participating API/MCP tool — not on one reseller's walled garden.
- **Metered settlement.** Every charge is idempotent (IntentID), settles in ~1s, and produces a **verifiable receipt** (attestation + amount + ref + counterparty) anyone can check without trusting either party.
- **Budget caps.** Per-day, per-call, per-provider limits enforced at the settlement layer; the hard stop is the vault balance itself — agents cannot overspend.
- **Refundable and programmable.** Refund = transfer; escrow/conditions are expressible in WASM for advanced cases.

### 6.2 Why it can only exist because DSN exists

1. **Neutrality.** A centralized portable-credit company is a custodian with a conflict of interest (OpenRouter's expiry+refund policy exists precisely because they hold the money). Only a chain is a neutral settlement layer no party owns.
2. **Self-custody at near-zero cost.** Ed25519 + cheap txs make per-vault self-custody viable; on expensive L1s the fee structure forces custodial aggregation.
3. **1-second finality fits the request/response lifecycle.** A charge must be settled and verifiable within a single API call. Slow-finality chains (minutes) cannot back real-time metered settlement.
4. **Idempotency (IntentID) is the agent-safe primitive.** Retries, parallel calls, and at-least-once delivery cannot double-charge — the nonce/intent dedup is in the protocol.
5. **The account model IS the budget cap.** No smart-contract execution needed for the core case; the chain's own balance semantics enforce the hard stop.

### 6.3 SDK surface — 12 public functions (constraint: <20)

```js
const dsn = require('@dsn/cred')

// vault (keys managed invisibly by SDK)
dsn.createVault()                       // → vaultId (prints recovery mnemonic)
dsn.importVault(mnemonic)

// credits
dsn.topUp(vaultId, { usd, source })     // fiat on-ramp or DSN transfer; → receipt
dsn.balance(vaultId)
dsn.transfer(from, to, amount)          // refunds, payouts, escrow release

// charging (provider side)
dsn.charge(providerVault, credId, amount, { ref, product, meta })  // idempotent
dsn.receipt(chargeId)                   // → verifiable receipt (JSON)
dsn.verifyReceipt(receipt)              // verify without trusting the provider
dsn.refund(chargeId)

// budgets (agent safety)
dsn.setBudget(vaultId, { perDay, perCall, perProvider })
dsn.budgetStatus(vaultId)

// events
dsn.subscribe(vaultId, onCharge)        // live stream of charges
```

### 6.4 The 30-minute integration

1. `npm i @dsn/cred` (2 min)
2. `vault = dsn.createVault()` — keys exist, mnemonic shown once (3 min)
3. Wrap the API/MCP handler: `dsn.charge(providerVault, credId, 0.001, { ref: reqId })` (10 min)
4. `receipt = dsn.receipt(chargeId)`; return it in the response; client verifies in 1 call (5 min)
5. Go live; devs who only *consume* credits never see a chain, a wallet, or a hash — they see a credit balance and receipts (10 min for docs + polish)

### 6.5 Where the transactions come from — exact math

Wedge population (from track 2): MCP ecosystem 12,770+ servers / 97M SDK downloads / <5% monetized / avg price $0 → targeting 1% of servers monetizing = ~128 server authors, each with 10-100 paying devs = **1.3K-13K paying devs** in the first year.

Per-dev call rates: a paid MCP tool or pay-per-call API is called 50-500 times/day per active dev (agent loops and CI workflows are the drivers).

Volume tiers (per-call, unbatched):

| Devs | Calls/dev/day | Total calls/day | On-chain txs/day (unbatched) | On-chain txs/day (batched ×100) |
|---|---|---|---|---|
| 100 | 200 | 20K | 20K | 200 |
| 1K | 200-1,000 | 200K-1M | 200K-1M | 2K-10K |
| 10K | 200-5,000 | 2M-50M | 2M-50M | 20K-500K |
| 100K | 500-10,000 | 50M-1B | — | needs session batching + sharding |

Design rule: **the network is batch-first.** Per-call accounting happens against an off-chain commit log; the on-chain settlement commit covers batches (MPP-style session settlement). Unbatched per-call charging is available for high-value calls. This keeps committed volume comfortably inside DSN's ~100 TPS ceiling (8.6M txs/day absolute) while per-call accounting scales to billions. The indexer (already built, not yet wired to RPC) becomes the receipt/verification backend.

### 6.6 Token utility (constraint 5 + 10)

- DSN is the settlement asset: fees and optional DSN-denominated credits.
- Every commit consumes a small DSN fee → natural consumption, proportional to usage, no speculation required.
- Credits are prepaid units with real purchasing power over real APIs — the token's value is its use, not its price.
- Inflation 5% + 70/20/10 split funds validators, ecosystem, and reserves; staking demand grows with usage.

### 6.7 Why the incumbents don't close this slot

| Player | What they do | Why the slot stays open |
|---|---|---|
| OpenRouter | Custodial, expiring (365d), non-refundable (24h), 5.5% fee credits | The product IS the walled garden; devs complain #1 about agent-loops burning balances with no hard stop |
| x402 | Per-request on-chain payment (140M tx cumulative) | Rail, not credit layer; expensive per call; no budget model; no neutral receipts |
| Tempo / MPP (Stripe) | Stablecoin L1 + session streaming | Stripe-owned rail; custodial settlement; credits and receipts are Stripe's, not portable |
| Cloudflare Wallets | Agent payments wallet (launched 2026-08-04) | Custodial wallet entry; complements a neutral credit layer, doesn't replace it |
| RapidAPI (failed) | Marketplace with 20-50% take | Providers bypassed the toll — the lesson: be the neutral settlement, not the toll |

**The open slot is the layer above the rails:** portable self-custodied credits + neutral metered settlement + verifiable receipts. Rails are claimed; the credit/receipt layer is not, and no incumbent can credibly own it because owning it means being the bank.

### 6.8 Honest risks

1. **TPS ceiling.** Solved by batch-first design (§6.5); the indexer must be wired into RPC first (production hardening P0, already listed in `product-strategy.md` §8).
2. **Self-custody UX.** The SDK must make keys invisible; loss of mnemonic = loss of funds. Mitigation: recovery mnemonic, testnet free tier, dead-man switch patterns in v2.
3. **Cold start.** Two-sided market (devs need providers to accept credits; providers need devs to hold them). Mitigation: the MCP wedge — supply and demand meet in one place; start with 100 curated servers + free testnet credits.
4. **Rail consolidation.** If Stripe/x402 absorb credits, the neutral layer still wins where both sides distrust the issuer (audit, compliance, cross-provider portability, agent authorizations). Don't fight the rails; settle above them.
5. **"Small chain" trust.** Devs trust self-custody + verifiable receipts, not the chain's brand; the receipt verification story must be verifiable by ANYONE, including non-DSN users (light client / stateless proof in v2).
6. **Regulatory.** Prepaid credits may attract money-transmitter scrutiny depending on on-ramp design. Mitigation: on-ramps are regulated partners; DSN settles credits, doesn't issue fiat.

### 6.9 What DSN must harden first (no consensus changes)

1. **Wire the indexer into RPC** — receipts/hash lookups are dead today (P0).
2. **Activate RPC auth/TLS** for public endpoints (P0).
3. **Batch charge API** (`cred.chargeBatch`) with idempotent commits (P0, new SDK method only).
4. **Light verification path** — stateless receipt verification for non-DSN users (P1).
5. **Keep the 10 constraints as product discipline**: every UI surface that exposes "wallet/transaction/hash" is a bug (P1, process).

### 6.10 What NOT to do (counter-patterns from the adoption study)

- Never kill the free tier (PlanetScale lesson) — permanent free testnet credits are the funnel.
- Don't market k=6 as "finality" to devs (Fly.io lesson) — say "settled in ~1-6 seconds."
- No framework capture (Vercel lesson) — first-class Go + JS + any MCP host.
- Don't be the gas toll (OpenRouter lesson) — near-zero fees; never a 5.5% credit rake.

## 7. Why this supersedes the Goal-A direction

`product-strategy.md` v2 chose AuditAnchor (82/100) for Goal A — enterprise multi-party tamper-evidence, EU AI Act driven. That analysis remains valid for the *later, enterprise* layer, but it scores poorly on the Goal-B criteria (developer adoption, tx/day, platform potential, invisible-blockchain constraint) and targets procurement, not developers.

CredRail is the **developer-first** answer to the same underlying primitive (cheap, fast, tamper-evident attestation + verification): instead of selling audits to enterprises, it gives devs a credit+receipt layer they adopt in 30 minutes, that generates transaction volume from day one, and that no centralized player can own. AuditAnchor and the rest of the v2 TOP 5 become the enterprise layer CredRail grows into (v3 attack order: CredRail wedges → AgentReceipt → EvalLedger → enterprise attestation).

## 8. Next steps

1. Convert CredRail into an SDD change: proposal → spec → design → tasks (openspec flow already established).
2. First slice: `@dsn/cred` SDK (12 fns) + `cred.chargeBatch` + indexer→RPC wiring + testnet free-tier credits + 100 curated MCP servers.
3. Track 2's OpenRouter-comparison data (§6.7) doubles as the public launch narrative.
