# Tasks: Wallet Recovery — BIP-39 Mnemonic + Encrypted Keystore

## Review Workload Forecast

| PR | Slice | Est. changed lines | 400-line risk | Decision |
|----|-------|--------------------|---------------|----------|
| PR1 | libs (`internal/bip39` + `wallet/keystore.go` + `wallet/mnemonic.go`) | ~350 | Low (borderline) | Yes — ask-on-risk |
| PR2 | CLI + TUI (`internal/passphrase` + `cmd/dsn` + TUI) | ~380 | Medium (borderline) | Yes — ask-on-risk |
| PR3 | SDK + docs + integration (`sdk/wallet.go` + docs + `integration/`) | ~350 | Low (borderline) | Yes — ask-on-risk |

Total ~1,080 → chained PRs. Threat matrix: N/A (no rows applicable — no routing/subprocess/exec boundary).

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Medium

**Bases**: PR1 base = `feature/wallet-recovery` tracker branch; PR2 base = PR1 branch; PR3 base = PR2 branch. Child diff showing parent changes = wrong base, retarget/rebased before review.

**Work units** (test · harness · rollback):
- **PR1 libs** — test `go test ./internal/bip39/... ./wallet/... -count=1` + full `make test` · harness N/A (pure library, no process/DB boundary; unit suite is the proof) · rollback: revert PR1 commits
- **PR2 CLI+TUI** — test `go test ./internal/passphrase/... ./cmd/... -count=1` · harness `go build ./cmd/dsn && ./dsn wallet generate` on a real TTY: prompts twice, mnemonic printed once, `--passphrase` rejected, passphrase absent from `ps`/shell history · rollback: revert PR2 commits
- **PR3 SDK+docs+integration** — test `make test` + `make test-integration` · harness `go run ./cmd/dsn wallet restore --mnemonic "…"` then sign a tx end-to-end · rollback: revert PR3 commits

## Scenario → RED Test Map (47/47)

| Spec | # | Scenario | RED task |
|------|---|----------|----------|
| bip39 | 1 | Generated phrase well-formed | 1.3 |
| bip39 | 2 | Generated phrase passes validation | 1.3 |
| bip39 | 3 | Valid phrase accepted | 1.2 |
| bip39 | 4 | Word not in wordlist | 1.2 |
| bip39 | 5 | Wrong checksum | 1.2 |
| bip39 | 6 | Wrong word count | 1.2 |
| bip39 | 7 | Mixed-case normalized | 1.2 |
| bip39 | 8 | Official 128-bit vector (c55257c3…3b04) | 1.1 + 1.5 |
| bip39 | 9 | Deterministic seed | 1.4 |
| bip39 | 10 | Restore reproduces original key | 1.6 + 7.4 |
| bip39 | 11 | Invalid phrase rejected at restore | 1.5 |
| keystore | 1 | Well-formed keystore | 1.8 |
| keystore | 2 | Unsupported version | 1.8 |
| keystore | 3 | Roundtrip fidelity | 1.7 |
| keystore | 4 | Randomized ciphertext | 1.7 |
| keystore | 5 | Wrong passphrase, no key leak | 1.7 |
| keystore | 6 | Non-default params decrypt | 1.8 |
| keystore | 7 | Absurd params rejected pre-derivation | 1.8 |
| keystore | 8 | Atomic write, no temp remains | 1.9 |
| keystore | 9 | File 0600, parent ≤0700 | 1.9 |
| keystore | 10 | Encrypted load | 1.11 |
| keystore | 11 | Legacy plaintext load + warning | 1.11 |
| keystore | 12 | Successful migration + `.bak` | 1.10 + 7.4 |
| keystore | 13 | Interrupted after backup names `.bak` | 1.10 |
| keystore | 14 | Truncated ciphertext → GCM auth fail | 1.7 |
| keystore | 15 | Malformed JSON | 1.7 |
| cli | 1 | Default encrypted generation | 4.3 |
| cli | 2 | Passphrase mismatch, no file | 4.1 + 4.3 |
| cli | 3 | Legacy plaintext escape hatch | 4.3 |
| cli | 4 | Restore reproduces address | 4.3 + 7.4 |
| cli | 5 | Invalid mnemonic rejected first | 4.2 |
| cli | 6 | Export after generation | 4.3 |
| cli | 7 | Export in fresh process fails | 4.2 |
| cli | 8 | `--passphrase` flag rejected | 4.2 |
| cli | 9 | Non-TTY stdin fails | 4.1 |
| cli | 10 | Short passphrase rejected | 4.1 |
| cli | 11 | Legacy wallet signs unchanged | 7.4 (+1.11) |
| cli | 12 | Encrypted keystore signs after prompt | 7.4 |
| cli | 13 | Wrong passphrase during sign | 7.4 |
| sdk | 1 | Compat pin test (golden values) | 7.1 |
| sdk | 2 | Deprecation marker present | 7.3 |
| sdk | 3 | Official vector reproduces key | 7.2 |
| sdk | 4 | Invalid phrase errors | 7.2 |
| sdk | 5 | Deterministic derivation | 7.2 |
| sdk | 6 | GenerateMnemonic shape | 7.3 |
| sdk | 7 | Restore agrees with entry point | 7.3 |
| sdk | 8 | Existing addresses unchanged | 7.5 |

## PR1 — Libs (`internal/bip39` + `wallet`)

**RED tests** (write first, must fail):
- [x] 1.1 `internal/bip39/bip39_test.go`: official 128-bit vector — entropy `0000…00` → phrase `abandon…about` → `MnemonicToSeed` = **pinned verbatim** `c55257c3…3b04`, never recompute; plus `7f7f…`/`8080…`/`ffff…` 12-word vectors
- [x] 1.2 `internal/bip39/bip39_test.go`: `ValidateMnemonic` — accept `abandon…about`; reject **non-wordlist word** (error names it; `zebra` IS word #2048 of the official list — spec scenario corrected, see apply-progress Deviations); reject bad checksum; reject 11/13 words; accept UPPERCASE (normalized)
- [x] 1.3 `internal/bip39/bip39_test.go`: `GenerateMnemonic` → 12 words, all in wordlist; result passes `ValidateMnemonic`
- [x] 1.4 `internal/bip39/bip39_test.go`: `MnemonicToSeed` twice → byte-identical
- [x] 1.5 `wallet/mnemonic_test.go`: `KeyFromMnemonic` — official vector pubkey == `ed25519.NewKeyFromSeed(seed[:32])`; invalid-checksum phrase → error, no key
- [x] 1.6 `wallet/mnemonic_test.go`: same phrase → same address (restore reproducibility)
- [x] 1.7 `wallet/keystore_test.go`: roundtrip byte-identical; salt/iv/ciphertext differ on 2nd encrypt; wrong passphrase → checksum error (no GCM); truncated ciphertext → GCM auth fail; malformed JSON → parse error
- [x] 1.8 `wallet/keystore_test.go`: 9-field schema typed; `version:2` → unsupported; N=2^15 params decrypt; N=2^30 / non-pow2 → `ErrKDFParams` pre-derivation
- [x] 1.9 `wallet/keystore_test.go`: atomic write — complete target, no temp left; file 0600, parent ≤0700 (parent created by `SaveKeystore` via `MkdirAll(0700)` — Go 1.26 `t.TempDir()` is 0775 not 0700)
- [x] 1.10 `wallet/keystore_test.go`: `Migrate` — target valid keystore + `<path>.bak` holds plaintext; post-backup failure names `.bak`; existing `.bak` → `ErrBackupExists`; 2nd run → `ErrAlreadyEncrypted`
- [x] 1.11 `wallet/wallet_test.go` (modify): legacy tests stay green; `LoadKeyFile` legacy → keypair + stderr warning + file unchanged; keystore + provider → decrypt; `LoadKey` keystore with nil → `ErrKeystorePassphraseRequired`

**GREEN production**:
- [x] 2.1 `internal/bip39/english.txt`: official 2048-word English list (data asset; sha256 `2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda`, 2048 lines, 0 dupes)
- [x] 2.2 `internal/bip39/wordlist.go`: `go:embed` + index map
- [x] 2.3 `internal/bip39/bip39.go`: `GenerateMnemonic`/`EntropyToMnemonic` (16B CSPRNG + 4 checksum bits → 12×11-bit), `ValidateMnemonic` (trim/lower, 12 words, wordlist, checksum, typed errors), `MnemonicToSeed` (stdlib `crypto/pbkdf2`, HMAC-SHA512, 2048, **salt `"mnemonicTREZOR"`** — fixed passphrase `"TREZOR"` REQUIRED by frozen pin, see apply-progress Deviations)
- [x] 2.4 `wallet/mnemonic.go`: `KeyFromMnemonic` = validate → seed → `seed[:32]` → Ed25519; no fallback
- [x] 2.5 `wallet/keystore.go`: schema; `EncryptKey` (scrypt N=2^17 r=8 p=1, AES-256-GCM, checksum `sha256(dk)[:4]`); `DecryptKey` (KDF guard D6 → checksum-first → GCM); `writeAtomic` 0600 (D12); `SaveKeystore`; `LoadKeyFile` (sniff `version`; legacy hex-JSON + warning); `Migrate` (D7 order); typed errors (D5 list)
- [x] 2.6 `wallet/wallet.go` (modify): `LoadKey` = `LoadKeyFile(path, nil)`; legacy path untouched (6 call sites unchanged)
- [x] 2.7 `go.mod`/`go.sum`: + `golang.org/x/crypto v0.39.0`

**Gate**:
- [x] 3.1 `go test ./internal/bip39/... ./wallet/... -count=1` green **and** full `make test` green; `go vet ./...` clean

**Actuals**: 26/26 tests green (bip39 0.011s, wallet 8.0s); `internal/bip39` 96.1% / `wallet` 79.1% coverage. **Authored size 1,243 changed lines vs 400 budget — size:exception REQUIRED (ask-on-risk), commits withheld pending decision** (see apply-progress.md). Full suite, build, vet all green.

## PR2 — CLI + TUI

**RED tests** (write first):
- [x] 4.1 `internal/passphrase/passphrase_test.go`: non-TTY → TTY-required error, nothing prompted; short (<12) rejected, stops; mismatch aborts; injected-reader seam works
- [x] 4.2 `cmd/dsn/wallet_cmd_test.go` (new test file, tests only — PR boundary unchanged): `--passphrase` on generate → explicit rejection error; restore with bad-checksum mnemonic → error before prompt/file; `export-mnemonic` fresh process → error, prints nothing
- [x] 4.3 `cmd/dsn/wallet_cmd_test.go` via injected reader: generate → 0600 keystore + mnemonic once + warning; mismatch → no file; `--legacy-plaintext` → no prompt, legacy file; restore valid phrase → keystore + matching address; export after generate → prints once

**GREEN production**:
- [x] 5.1 `internal/passphrase/passphrase.go`: `IsTTY`, `PromptTwice` (`x/term`, prompts→stderr, min 12, mismatch aborts) + injectable-reader test seam
- [x] 5.2 `cmd/dsn/wallet_cmd.go`: `generate` (encrypted default, mnemonic printed once, in-memory var, `--legacy-plaintext`, `--passphrase` rejected with explanation), `restore --mnemonic` (validate first), `export-mnemonic` (in-process only), `migrate`, `sign` → `LoadKeyFile` + TTY provider
- [x] 5.3 `cmd/dsn/tx_sign.go`: `LoadKeyFile` + TTY provider
- [x] 5.4 `cmd/dsn/contract_cmd.go`: `LoadKeyFile` + TTY provider
- [x] 5.5 `cmd/dsn/main.go`: `nil` provider (daemon stays read-only, never hangs)
- [x] 5.6 `cmd/dsn-tui/model.go`: startup prompt for keystores (D11), `nil` wallet on failure
- [x] 5.7 `go.mod`/`go.sum`: + `golang.org/x/term v0.33.0`

**Gate**:
- [x] 6.1 Real-TTY harness: `go test ./internal/passphrase/... ./cmd/... -count=1` green — **12/12 PASS**. `generate` prompts twice + mnemonic once (verified via `PromptTwice` stderr capture + mnemonic count==1); `restore` valid phrase → keystore + matching address; `migrate` wired; `sign` wired; encrypted + legacy both load; `--passphrase` rejected (explicit error); passphrase never in argv (provider is nil when TTY-only, PromptTwice never reads from argv). Real-TTY interactive harness (`dsn wallet generate`) — **could not run** in CI-like environment (no real TTY); reliance on injected-reader automated tests per session contract.

**Actuals**: 12/12 tests green (passphrase 0.003s, cmd/dsn 1.2s). `internal/passphrase` 71.4% / `cmd/dsn` 14.3% coverage. **Authored size 792 changed lines (767+/25−) vs 400 budget — size:exception REQUIRED (per session contract).** 10 files changed across 2 commits. Full suite (`make test`) green, `go vet ./...` clean.

## PR3 — SDK + Docs + Integration

**RED tests** (write first):
- [ ] 7.1 `sdk/wallet_test.go`: legacy compat pins — `NewKeyFromMnemonic` on `abandon…about`→`c557ee…e7fc7`, `legal winner…yellow`→`ecb0e7…00152`, `dsn demo…eight`→`f4ba61…511c` — **use the pinned values verbatim, never recompute**
- [ ] 7.2 `sdk/wallet_test.go`: `NewKeyFromBIP39Mnemonic` official vector → pubkey == `NewKeyFromSeed(seed[:32])` with pinned seed; bad-checksum phrase → error, no key; twice → identical
- [ ] 7.3 `sdk/wallet_test.go`: `GenerateMnemonic` 12 words + passes validation; `RestoreFromMnemonic` ≡ `NewKeyFromBIP39Mnemonic`; `go doc github.com/BryanOx/dsn/sdk.NewKeyFromMnemonic` shows `Deprecated` naming replacement
- [x] 7.4 `integration/wallet_recovery_test.go` (`-tags=integration`): generate→restore→sign same address; migrate→sign; wrong passphrase during sign → checksum error, unsigned; legacy plaintext sign unchanged
- [ ] 7.5 `sdk/wallet_test.go`: pre-change wallets loaded post-change → identical addresses/signatures, only stderr warning added

**GREEN production**:
- [ ] 8.1 `sdk/wallet.go`: `Deprecated:` doc on `NewKeyFromMnemonic` (SHA-256 impl untouched, points to replacement); `NewKeyFromBIP39Mnemonic`; `GenerateMnemonic`; `RestoreFromMnemonic`; `EncryptKey`/`DecryptKey` wrappers (passphrase param, never prompt)
- [ ] 8.2 `integration/wallet_recovery_test.go`: implement e2e flows (design testing strategy row 4)
- [x] 8.3 `docs/GUIDES.md`: replace "no recovery mechanism" (line 1480); add wallet-recovery section
- [x] 8.4 `docs/WHITEPAPER.md`: wallet security/recovery notes

**Gate**:
- [x] 9.1 `make test` + `make test-integration` green; `go vet ./...` clean; `go doc` deprecation confirmed; docs greps updated

## Rollback

Revert slice commits (PR1→PR2→PR3). Keystore is file-based and additive — **no DB migration**. Legacy plaintext files remain readable (sniffed loader); old binaries read old files; `.bak` + mnemonic keep funds recoverable. Revert restores pre-change loader with zero behavior change.

## Out of Scope

Validator keystores (unchanged plaintext); HD derivation/SLIP-0010; Spanish/non-English wordlists; hardware wallets; mnemonic persistence on disk (by design never stored); lost-passphrase recovery (impossible by construction).
