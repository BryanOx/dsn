# Archive Report: wallet-recovery — BIP-39 Mnemonic + Encrypted Keystore

**Status**: success — archived with warnings (intentional, non-critical)
**Archived to**: `openspec/changes/archive/2026-08-04-wallet-recovery/`
**Mode**: openspec (strict TDD; gentle-ai CLI NOT installed — manual archive)
**Date**: 2026-08-04

## Executive Summary

The wallet-recovery change is closed. Four new capabilities — `bip39-mnemonic`, `wallet-keystore`, `wallet-cli`, `sdk-compat` — shipped on `feat/wallet-recovery/sdk` (9 commits `e2f3e34`…`b45e180` over `development`), were verified PASS WITH WARNINGS (21/21 requirements, 47/47 scenarios, `make test` + `make test-integration` green, 0 CRITICAL), and are now synced into `openspec/specs/` as the main specs (no pre-existing main specs existed, so each delta spec was copied verbatim). The change folder is moved to the archive with all artifacts intact. No source code was modified; this phase is docs/bookkeeping only.

## Final-State Authority Notes

The archive reflects the state of the change AT CLOSE. Commit facts below are taken from repository evidence (`git log --oneline development..HEAD`), which outranks both `apply-progress.md` and `verify-report.md` snapshots.

- `verify-report.md` reported W1 — stale unchecked task checkboxes (7.1, 7.2, 7.3, 7.5, 8.1, 8.2) — as a warning that "must be reconciled before archive". **RECONCILED**: `tasks.md` and `apply-progress.md` now carry `[x]` for all 40 tasks; grep confirms zero unchecked implementation-task checkboxes in either file. The remaining `[ ]` in `proposal.md` are planning success criteria, not implementation tasks, and are satisfied by the verified outcomes.
- Launch prompt stated "7 commits over development"; `git log` shows **9 commits** (PR1 libs `e2f3e34, 2974d39, 032c029, b0ad404`; PR2 CLI+TUI `7ac7321, 5d7ece7`; PR3 SDK+integration+docs `29b6ce1, 4f1325e, b45e180`). Resolved in favor of repository evidence; no functional discrepancy.

## Commits (ground truth, `git log --oneline development..HEAD`)

| Slice | Commits | Scope |
|-------|---------|-------|
| PR1 — libs | `e2f3e34` bip39 wordlist/mnemonic/seed; `2974d39` AES-256-GCM keystore; `032c029` key derivation from mnemonics; `b0ad404` dual-format `LoadKeyFile` | `internal/bip39`, `wallet/` |
| PR2 — CLI + TUI | `7ac7321` TTY-only passphrase prompting; `5d7ece7` encrypted-keystore CLI + TUI prompt | `internal/passphrase`, `cmd/dsn`, `cmd/dsn-tui` |
| PR3 — SDK + docs | `29b6ce1` BIP-39 SDK surface + legacy deprecation; `4f1325e` wallet-recovery integration tests; `b45e180` docs (GUIDES/WHITEPAPER) | `sdk/`, `integration/`, `docs/` |

## Specs Synced (delta → main)

No main capability specs existed (`openspec/specs/` was empty), so per sdd-archive merge rules each delta spec IS a full spec and was copied verbatim — no ADDED/MODIFIED/REMOVED merge required, no destructive merge, no destructive-delta warning needed (per `openspec/config.yaml` `rules.archive`).

| Domain | Main spec | Requirements | Scenarios | Action |
|--------|-----------|-------------:|----------:|--------|
| `bip39-mnemonic` | `openspec/specs/bip39-mnemonic/spec.md` | 4 | 11 | Created (verbatim copy) |
| `wallet-keystore` | `openspec/specs/wallet-keystore/spec.md` | 8 | 15 | Created (verbatim copy) |
| `wallet-cli` | `openspec/specs/wallet-cli/spec.md` | 5 | 13 | Created (verbatim copy) |
| `sdk-compat` | `openspec/specs/sdk-compat/spec.md` | 4 | 8 | Created (verbatim copy) |
| **Total** | — | **21** | **47** | — |

Counts match the verify-report (`21/21` requirements, `47/47` scenarios).

## Verification State (carried from `verify-report.md`, PASS WITH WARNINGS)

- Requirements: 21/21; Scenarios: 47/47 (46 with dedicated passing runtime tests, 1 — deprecation marker — via test + `go doc` evidence).
- Gates: `make test` exit 0 (`sha256:4fbf7f51…`); `make test-integration` exit 0 (`sha256:275b6d51…`); `go build ./...` and `go vet ./...` exit 0.
- Tasks: 40/40 complete; all checkboxes reconciled at archive time.
- CRITICAL: 0. Blockers: 0. Verdict: **PASS WITH WARNINGS**.

## Warnings / Risks Carried Forward

- **W1 — Stale task bookkeeping** → **RESOLVED at archive**: tasks.md/apply-progress.md checkboxes reconciled to `[x]` (proof: grep for `[ ]` returns no implementation-task matches). apply-progress PR3 TDD Cycle Evidence table remains thin (lists only 7.4) but is historical snapshot, not a correctness gap.
- **W2 — Documented spec deviations (both required by frozen pins, both kept verbatim in main specs):**
  - (a) **Seed salt is `"mnemonicTREZOR"`** (fixed passphrase `"TREZOR"`), not the literal `"mnemonic"` written in the bip39/sdk spec text. Required because the official TREZOR vector seed `c55257c3…e7463b04` is only reachable with that salt. The main spec text is internally inconsistent with the implementation; resolved in favor of the frozen pin (`bip39.go` const comment + apply-progress deviation 1). **Future changes touching seed derivation MUST keep salt `"mnemonicTREZOR"`.**
  - (b) Spec scenario "Word not in wordlist" names `zebra`, which IS word #2048 of the official list; the test uses `xyzzy` and keeps an explicit `zebra_is_valid_but_bad_checksum` case (apply-progress deviation 2).
- **W3 — Changed-file coverage below 80% (informational, non-blocking)**: `wallet` 79.1% (`createTempAndSync` 45%, `SaveKeystore` 71.4% — temp-file error paths), `internal/passphrase` 71.4% (real-TTY `x/term` path), `cmd/dsn` 14.3% (CLI wiring). Coverage never blocks per strict module.
- **W4 — Size guard**: PR1 1,243 + PR2 792 changed lines exceed the 400-line review budget; `size:exception REQUIRED per session contract` recorded in apply-progress; PR1 commits were withheld as a separate reviewable PR and stacked under PR2/PR3. **Acknowledged at archive time**; no further action.

## Suggestions Carried Forward (non-blocking follow-ups)

- **S1** — `TestDeprecatedDocExists` is a source-content smoke check; a test shelling out to `go doc` would pin the directive itself (verifier confirmed independently via `go doc`).
- **S2** — No real-TTY interactive harness executed (gate 6.1 automated-only via injected-reader seam); a one-time manual `dsn wallet generate` on a real TTY would close the no-echo `x/term.ReadPassword` gap.
- **S3** — `wallet nonce` / `tx create` / contract-nonce paths load no key (D3, verified by grep) but the "no key load on nonce paths" invariant is not pinned by a test; out of change scope.

## Rollback

Revert the 9 commits in order (PR3 → PR2 → PR1). Keystore is file-based and additive — **no DB migration**. Legacy plaintext wallets remain readable (sniffed dual-format loader); old binaries read old files; `dsn wallet migrate` retains `<path>.bak`; funds recoverable via the backup or the BIP-39 mnemonic. Out of scope by design: validator keystores (plaintext), HD/SLIP-0010 derivation, non-English wordlists, hardware wallets, mnemonic persistence on disk, lost-passphrase recovery.

## Archive Contents

- `proposal.md` ✅ (success criteria noted; not implementation-task checkboxes)
- `specs/` ✅ (4 capability delta specs)
- `design.md` ✅
- `tasks.md` ✅ (40/40 `[x]`, no unchecked implementation tasks)
- `apply-progress.md` ✅
- `verify-report.md` ✅
- `archive-report.md` ✅ (this file)

Missing artifacts: `state.yaml` (DAG state file) was never created in this change; `openspec/changes/state-sync/` has none either — consistent with the repo precedent. No blocking omission; the change folder is self-contained without it.

## Archive Bookkeeping

- `openspec/` remains untracked in git (bookkeeping only, per repo convention — never committed).
- No code, tests, or documentation under version control were touched by this phase.
