```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:a69c83fceba75ee08702a63f0fe5aac5b10dd0c9c832cd7157f07037692e9324
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 21/21
scenarios: 47/47
test_command: go test ./... -cover -count=1 -timeout=120s
test_exit_code: 0
test_output_hash: sha256:4fbf7f51c168d677c945fd8ba406e81db48fdc3d53fcce8b89a4ff2a35c3dde5
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# SDD Verify Report — wallet-recovery (full change)

**Change**: wallet-recovery (9 commits on the chain `e2f3e34`…`b45e180`: PR1 libs `e2f3e34,2974d39,032c029,b0ad404`; PR2 CLI+TUI `7ac7321,5d7ece7`; PR3 SDK+docs+integration `29b6ce1,4f1325e,b45e180`; base `6811624` module rename)
**Version**: tasks.md — 34/40 checkboxes ticked, 6 stale-unchecked (see W1); apply-progress closed as `all_done`
**Mode**: Strict TDD (runner `make test` / `make test-integration`; gentle-ai CLI unavailable — manual status contract, counts hand-verified against retrieved specs, envelope vocabulary self-declared per state-sync precedent)

**Verification scope**: FULL change at HEAD `b45e180` (working tree clean except untracked `openspec/`). All 21 requirements and 47 scenarios verified against source on the branch, executed unit + integration suites, frozen golden pins independently recomputed, deviations and call sites checked.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 40 |
| Tasks complete | 40 (implemented + passing; 6 checkboxes stale in tasks.md) |
| Tasks incomplete | 0 |
| Requirements (4 specs) | 21/21 |
| Scenarios (4 specs) | 47/47 |
| CRITICAL / blockers | 0 |
| WARNING | 4 |
| SUGGESTION | 3 |

### Build & Tests Execution

**Build**: ✅ Passed — `go build ./...` exit 0 (empty output, `sha256:e3b0c442…`); `go vet ./...` exit 0 (empty output, `sha256:e3b0c442…`).

**Tests (full unit suite)**: ✅ `make test` (`go test ./... -cover -count=1 -timeout=120s`) exit 0 — every package `ok`. Output hash `sha256:4fbf7f51…`. Relevant packages: `internal/bip39 0.020s / 96.1%`, `wallet 8.259s / 79.1%`, `internal/passphrase 0.026s / 71.4%`, `cmd/dsn 1.275s / 14.3%`, `sdk 1.636s`. Full suite includes the SDK wallet tests (legacy pins + BIP-39 vectors) — no integration-tag leakage.

**Integration suite**: ✅ `make test-integration` (`go test -tags=integration -count=1 -timeout=300s ./integration/...`) exit 0 — `integration 17.346s`, `soak 6.344s`, `staging 0.576s`, `testutil 0.013s`. Output hash `sha256:275b6d51…`. The `make test-integration` target exists and runs the tagged integration tests including `wallet_recovery_test.go`.

**Frozen golden pins (independently recomputed by verifier, all PASS in suite)**: TREZOR 128-bit vector seed `c55257c3…e7463b04` (`TestMnemonicToSeed_OfficialVector`, `sdk` vector tests) — salt `"mnemonicTREZOR"` (passphrase `"TREZOR"`) per the official vector convention, documented deviation; legacy `abandon…about` → priv `c557ee…e7fc7` ✅; legacy `legal winner…yellow` → priv `220a43…a7eff` ✅ (**CORRECTED** — the `220a434e…` value is the true SHA-256→Ed25519 pin, recomputed and matched; design.md/tasks.md carried the wrong truncated `ecb0e7…00152`); legacy `dsn demo…eight` → priv `4b171f…f6bf3c` ✅. All three legacy pins byte-match an independent `sha256(phrase) → ed25519.NewKeyFromSeed` recomputation.

### Spec Compliance Matrix — bip39-mnemonic (4 req / 11 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Mnemonic generation | Generated phrase is well-formed | `internal/bip39/bip39_test.go > TestGenerateMnemonic_WellFormed` (12 words, each in wordlist) | ✅ COMPLIANT |
| Mnemonic generation | Generated phrase passes validation | same test (calls `ValidateMnemonic` on the result) | ✅ COMPLIANT |
| Mnemonic validation | Valid phrase accepted | `TestValidateMnemonic` case `valid` | ✅ COMPLIANT |
| Mnemonic validation | Word not in wordlist | `TestValidateMnemonic` case `word_not_in_wordlist` — `xyzzy`, error names the word (spec's `zebra` is word #2048, corrected; see W2) | ✅ COMPLIANT |
| Mnemonic validation | Wrong checksum | `TestValidateMnemonic` cases `bad_checksum` + `zebra_is_valid_but_bad_checksum` (`ErrChecksum`) | ✅ COMPLIANT |
| Mnemonic validation | Wrong word count | `TestValidateMnemonic` cases `eleven_words` / `thirteen_words` (`ErrWordCount`) | ✅ COMPLIANT |
| Mnemonic validation | Mixed-case input normalized | `TestValidateMnemonic` case `uppercase` (accepted after lowercasing) | ✅ COMPLIANT |
| Seed derivation | Official BIP-39 test vector (128-bit) | `TestMnemonicToSeed_OfficialVector` (seed `c55257c3…e7463b04` verbatim) + `TestMnemonicToSeed_MoreOfficialVectors` (7f7f/8080/ffff) + `wallet/mnemonic_test.go > TestKeyFromMnemonic_OfficialVector` (pubkey == `NewKeyFromSeed(seed[:32])`) | ✅ COMPLIANT |
| Seed derivation | Deterministic seed | `TestMnemonicToSeed_Deterministic` | ✅ COMPLIANT |
| Restore from mnemonic | Restore reproduces the original key | `wallet/mnemonic_test.go > TestKeyFromMnemonic_RestoreReproducesAddress` + `integration > TestWalletRecovery_GenerateRestoreSign` (generate→restore same address) | ✅ COMPLIANT |
| Restore from mnemonic | Invalid phrase rejected at restore | `TestKeyFromMnemonic_InvalidChecksumRejected` (error, no key) | ✅ COMPLIANT |

### Spec Compliance Matrix — wallet-keystore (8 req / 15 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Encrypted keystore format | Well-formed keystore | `wallet/keystore_test.go > TestKeystoreSchema_WellFormed` (version/cipher/kdf/kdfparams/salt/iv/ciphertext/checksum typed) | ✅ COMPLIANT |
| Encrypted keystore format | Unsupported version | `TestDecryptKey_UnsupportedVersion` (`version:2` → `ErrUnsupportedVersion`) | ✅ COMPLIANT |
| Encryption | Roundtrip fidelity | `TestEncryptDecrypt_Roundtrip` (byte-identical private/public/address) | ✅ COMPLIANT |
| Encryption | Randomized ciphertext | `TestEncryptKey_RandomizedCiphertext` (salt, iv, ciphertext all differ) | ✅ COMPLIANT |
| Decryption with fail-fast checksum | Wrong passphrase | `TestDecryptKey_WrongPassphrase` (`ErrWrongPassphrase`, no keypair released) | ✅ COMPLIANT |
| KDF parameters honored from file | Non-default params decrypt | `TestDecryptKey_NonDefaultParams` (N=2^15 honored from file) | ✅ COMPLIANT |
| KDF parameters honored from file | Absurd params rejected | `TestDecryptKey_AbsurdParamsRejected` (N=2^30 / non-pow2 → `ErrKDFParams` pre-derivation) | ✅ COMPLIANT |
| Atomic 0600 writes | Atomic write | `TestSaveKeystore_AtomicWrite` (complete valid target, no `.keystore-*.tmp` remains) | ✅ COMPLIANT |
| Atomic 0600 writes | File permissions | `TestSaveKeystore_Permissions` (file 0600, parent ≤0700 via `MkdirAll(0700)`) | ✅ COMPLIANT |
| Unified dual-format loader | Encrypted load | `TestLoadKeyFile_KeystoreWithProvider` (provider decrypt) | ✅ COMPLIANT |
| Unified dual-format loader | Legacy plaintext load | `TestLoadKeyFile_LegacyWarningAndUnchangedFile` (keypair + stderr warning + file unchanged) | ✅ COMPLIANT |
| Migration | Successful migration | `TestMigrate_Success` (target valid keystore, `.bak` holds plaintext) | ✅ COMPLIANT |
| Migration | Interrupted after backup | `TestMigrate_InterruptedAfterBackup` (error names `.bak`; original preserved) + `TestMigrate_BackupExists` / `TestMigrate_AlreadyEncrypted` | ✅ COMPLIANT |
| Failure modes | Truncated ciphertext | `TestDecryptKey_TruncatedCiphertext` (`ErrCorruptKeystore`, GCM auth, no key) | ✅ COMPLIANT |
| Failure modes | Malformed JSON | `TestDecryptKey_MalformedJSON` (parse error wrapped in `ErrCorruptKeystore`) | ✅ COMPLIANT |

### Spec Compliance Matrix — wallet-cli (5 req / 13 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| wallet generate | Default encrypted generation | `cmd/dsn/wallet_cmd_test.go > TestWalletGenerate_EncryptedKeystore` (0600 keystore, mnemonic printed once `strings.Count==1`, warning, address roundtrip) | ✅ COMPLIANT |
| wallet generate | Passphrase mismatch | `TestWalletGenerate_PassphraseMismatchNoFile` (`ErrMismatch`, no file) | ✅ COMPLIANT |
| wallet generate | Legacy plaintext escape hatch | `TestWalletGenerate_LegacyPlaintextNoPrompt` (no prompt, no mnemonic, no `version` field) | ✅ COMPLIANT |
| wallet restore | Restore reproduces the address | `TestWalletRestore_ValidMnemonicWritesKeystore` (keystore + derived address printed) + `integration > TestWalletRecovery_GenerateRestoreSign` | ✅ COMPLIANT |
| wallet restore | Invalid mnemonic rejected | `TestWalletRestore_InvalidMnemonicBeforePromptOrFile` (error before prompt/file) | ✅ COMPLIANT |
| wallet export-mnemonic | Export after generation | `TestWalletGenerate_EncryptedKeystore` export block (prints once + warning) | ✅ COMPLIANT |
| wallet export-mnemonic | Export in a fresh process | `TestWalletExportMnemonic_FreshProcessFails` (error, prints nothing, explains never stored) | ✅ COMPLIANT |
| Passphrase prompting policy | Passphrase flag rejected | `TestWalletGenerate_PassphraseFlagRejected` (explicit error naming `--passphrase`, provider never invoked, no file) | ✅ COMPLIANT |
| Passphrase prompting policy | Non-TTY stdin | `internal/passphrase/passphrase_test.go > TestPromptTwice_NonTTYFailsWithoutPrompts` (`ErrTTYRequired`, nothing prompted on stderr) | ✅ COMPLIANT |
| Passphrase prompting policy | Short passphrase | `TestPromptTwice_TooShortStops` (`ErrTooShort`, no confirm prompt) | ✅ COMPLIANT |
| Existing commands keep working | Legacy wallet signs unchanged | `integration > TestWalletRecovery_LegacyPlaintextSignUnchanged` (identical signature via `LoadKey`) | ✅ COMPLIANT |
| Existing commands keep working | Encrypted keystore signs after prompt | `integration > TestWalletRecovery_GenerateRestoreSign` (provider load → identical signature, no legacy warning) | ✅ COMPLIANT |
| Existing commands keep working | Wrong passphrase during sign | `integration > TestWalletRecovery_WrongPassphrase` (`ErrWrongPassphrase`, file unchanged, still readable with correct passphrase) | ✅ COMPLIANT |

### Spec Compliance Matrix — sdk-compat (4 req / 8 scenarios)

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Legacy NewKeyFromMnemonic deprecated but unchanged | Compat pin test | `sdk/wallet_test.go > TestNewKeyFromMnemonic_LegacyPins` (3 frozen pins, byte-identical; independently recomputed) + `TestNewKeyFromMnemonic_SHA256` | ✅ COMPLIANT |
| Legacy NewKeyFromMnemonic deprecated but unchanged | Deprecation marker present | `TestDeprecatedDocExists` + verifier `go doc github.com/BryanOx/dsn/sdk NewKeyFromMnemonic` shows `Deprecated: Use NewKeyFromBIP39Mnemonic instead`; commit `29b6ce1` diff proves SHA-256 body untouched | ✅ COMPLIANT |
| NewKeyFromBIP39Mnemonic | Official vector reproduces the key | `TestNewKeyFromBIP39Mnemonic_AbandonVector` (pub == `NewKeyFromSeed(seed[:32])`) + `_AdditionalVectors` (7f7f/8080/ffff) + `_SeedConsistency` | ✅ COMPLIANT |
| NewKeyFromBIP39Mnemonic | Invalid phrase errors | `TestNewKeyFromBIP39Mnemonic_BadChecksum` (error names checksum) | ✅ COMPLIANT |
| NewKeyFromBIP39Mnemonic | Deterministic derivation | `TestNewKeyFromBIP39Mnemonic_Determinism` | ✅ COMPLIANT |
| SDK mnemonic and keystore surface | GenerateMnemonic shape | `TestGenerateMnemonic_12Words` (12 words + passes validation) + `_DifferentEachTime` | ✅ COMPLIANT |
| SDK mnemonic and keystore surface | Restore agrees with the new entry point | `TestRestoreFromMnemonic` (identical keypairs) + `TestEncryptKey_Roundtrip` / `TestDecryptKey_WrongPassphrase` (wrappers, passphrase param, never prompt) | ✅ COMPLIANT |
| No silent key change | Existing addresses unchanged | `TestLegacyWalletLoadedPostChange` + `TestNewKeyFromMnemonic_LegacyPins` + `integration > TestWalletRecovery_LegacyPlaintextSignUnchanged`; new derivation reachable only via new functions / `generate|restore|migrate` (verified in `wallet_cmd.go`, `sdk/wallet.go`) | ✅ COMPLIANT |

**Compliance summary**: 47/47 scenarios compliant (46 with dedicated passing runtime tests, 1 — deprecation marker — via test + `go doc` evidence).

### TDD Compliance (apply-progress cross-checked against executed tests)

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress has "TDD Cycle Evidence" tables (PR1 11 rows, PR2 3 rows, PR3 1 row) |
| All tasks have tests | ✅ | 40/40 implemented; every RED task maps to an existing executed test file |
| RED confirmed (tests exist) | ✅ | All test files present: bip39 (7 tests), keystore (16), mnemonic (3), wallet (8), passphrase (5), cmd/dsn wallet (7), sdk (16), integration wallet-recovery (4) |
| GREEN confirmed (tests pass) | ✅ | Full `make test` + `make test-integration` pass on execution (exit 0) |
| Triangulation adequate | ✅ | 7 validation cases, 4 official vectors, 3 legacy pins, 2 encrypt rounds, 4 KDF cases |
| Safety Net for modified files | ✅ | `wallet_test.go` (6/6 prior green), `wallet_cmd.go` (prior cmd tests green); PR1/PR3 test files new (N/A) |
| PR3 evidence completeness | ⚠️ | PR3 TDD table lists only task 7.4; rows for 7.1, 7.2, 7.3, 7.5, 8.1, 8.2 are implemented and passing but not tabulated (W1) |

**TDD Compliance**: 6/7 checks passed (1 ⚠️ bookkeeping).

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 62 | internal/bip39, wallet, internal/passphrase, cmd/dsn, sdk | go test |
| Integration | 4 (wallet recovery) | integration/wallet_recovery_test.go (`-tags=integration`) | make test-integration |
| E2E | 0 | — | not applicable (no browser/process boundary) |
| **Total** | **66** | **6** | |

### Changed File Coverage

| File | Line % | Uncovered Notes | Rating |
|------|--------|-----------------|--------|
| `internal/bip39/bip39.go` | 96.1% (pkg) | — | ✅ Excellent |
| `wallet/keystore.go` | 76.6% key fns (pkg 79.1%) | `createTempAndSync` 45% (error paths), `SaveKeystore` 71.4%, `Migrate` 76.5% | ⚠️ Acceptable |
| `wallet/mnemonic.go` | 90.0% | — | ✅ Excellent |
| `wallet/wallet.go` (modified) | `LoadKey` 100%, `loadLegacyKey` 76.9% | — | ✅ Excellent |
| `internal/passphrase/passphrase.go` | 71.4% (pkg) | TTY no-echo path via `x/term` (injected-reader seam covered) | ⚠️ Acceptable |
| `sdk/wallet.go` (new/changed fns) | 100% for all new + changed fns | pre-existing `KeyFromHex/Address/…` 0% (untouched) | ✅ Excellent |
| `cmd/dsn/wallet_cmd.go` | 14.3% (pkg) | CLI wiring paths via tests in-package; RunE surfaces exercised | ⚠️ Low (informational) |

**Average changed-file coverage**: ~79% (informational — coverage metrics never block).

### Assertion Quality

**Assertion quality**: ✅ All assertions verify real behavior. No tautologies, no ghost loops, no type-only assertions, no smoke-test-only assertions across the 6 changed test files. Banned-pattern scan clean.

### Quality Metrics

**Linter (`go vet ./...`)**: ✅ No errors (exit 0).
**Type Checker (go build)**: ✅ No errors (exit 0).
**staticcheck**: ➖ not installed (Makefile skips gracefully) — informational.

### Design Coherence

| Decision (design.md) | Implementation | Status |
|----------------------|----------------|--------|
| D1 unified `LoadKeyFile(path, getPassphrase)`; `LoadKey` = `LoadKeyFile(path, nil)` | `wallet/wallet.go:80-82`; zero broken call sites (grep `LoadKey(` → only defs + tests, all compile) | ✅ |
| D2 passphrase supply: TTY closure; daemon passes nil | `wallet_cmd.go` `walletPassphrase()`/`ttyWalletPassphrase`; `main.go:79` explicit nil; `tx_sign.go:54`, `contract_cmd.go:192` TTY provider | ✅ |
| D3 five load sites; nonce/tx-create don't load keys | main, tx_sign, contract deploy, wallet sign, TUI model confirmed; `wallet nonce`/`tx create` RPC-only | ✅ |
| D4 package layout `internal/bip39` + `wallet` + `internal/passphrase`, thin SDK | matches; SDK re-exports wrappers | ✅ |
| D5 9-field keystore, checksum `sha256(dk)[:4]`, AES-256-GCM 12B nonce, scrypt N=2^17 stored in-file | `keystore.go` `keystoreJSON`, `EncryptKey` | ✅ |
| D6 KDF guard pre-derivation, typed `ErrKDFParams` | `validateKDFParams`; tested | ✅ |
| D7 migrate order + `.bak` naming + verify + `ErrBackupExists`/`ErrAlreadyEncrypted` | `Migrate`; tested incl. interrupt seam | ✅ |
| D8 deps `x/term` + `x/crypto`, stdlib PBKDF2 | `golang.org/x/term v0.33.0`, `golang.org/x/crypto v0.39.0` in go.mod | ✅ |
| D9 CLI shape + `--passphrase` rejected with explanation | wallet_cmd.go; tested | ✅ |
| D10 mnemonic once, in-memory `lastMnemonic`, never on disk | `wallet_cmd.go:23`, printed once; tested | ✅ |
| D11 TUI prompt once at startup, nil wallet on failure | `model.go:38-54` | ✅ |
| D12 `writeAtomic` temp+fsync+rename, dir fsync, 0600 | `keystore.go:314-370`; tested | ✅ |
| Out-of-scope: HD derivation, non-English wordlists, hardware wallets, validator keystores | absent from 9-commit diff (verified by file inventory) | ✅ |

### Issues

**CRITICAL** — none.

**WARNING**
- **W1 — Stale task bookkeeping (tasks.md + apply-progress PR3 TDD table).** `tasks.md` still shows `[ ]` for 7.1, 7.2, 7.3, 7.5, 8.1, 8.2; the PR3 TDD Cycle Evidence table in apply-progress lists only 7.4. All six are implemented and their tests exist and PASS (verified by execution: `sdk/wallet_test.go` 16 tests, `integration/wallet_recovery_test.go` 4 tests), so this is bookkeeping, not missing work — but the checkbox state contradicts the `all_done` claim and must be reconciled before archive.
- **W2 — Two documented spec deviations (both required and documented).** (a) Seed salt is `"mnemonicTREZOR"` (fixed passphrase `"TREZOR"`), not the literal `"mnemonic"` in the bip39 spec/design text — REQUIRED because the spec pins the official TREZOR vector seed `c55257c3…3b04`, which is only reachable with the `"TREZOR"` passphrase; the spec text is internally inconsistent and was resolved in favor of the frozen pin (documented in `bip39.go:27-32` and apply-progress deviation 1). (b) Spec scenario "Word not in wordlist" names `zebra`, which IS word #2048; the test uses `xyzzy` for the wordlist-failure case and keeps an explicit `zebra_is_valid_but_bad_checksum` case (documented in apply-progress deviation 2). Both deviations break no other scenario; the scenario intent is preserved with corrected facts.
- **W3 — Changed-file coverage below the 80% bar.** `wallet` 79.1% (`createTempAndSync` 45%, `SaveKeystore` 71.4% — temp-file error paths), `internal/passphrase` 71.4% (real-TTY `x/term` path), `cmd/dsn` 14.3% (CLI wiring). Informational per strict module (never blocking), but the temp-file and TTY error paths would benefit from focused cases.
- **W4 — Size guard: PR1 1,243 + PR2 792 changed lines exceed the 400-line review budget.** apply-progress records `size:exception REQUIRED per session contract` and, per its slice record, PR1's commits were withheld and never landed as a separate reviewable PR before PR2/PR3 stacked onto them. Documented; to be acknowledged by the orchestrator at archive time.

**SUGGESTION**
- **S1 — `TestDeprecatedDocExists` is a source-file-content smoke check** (reads `wallet.go` and asserts the string `Deprecated`). Adequate only because the verifier independently confirmed via `go doc`; a test that shells out to `go doc` would pin the directive itself.
- **S2 — No real-TTY interactive harness was executed** (gate 6.1 automated only; no TTY in this environment). The no-echo `x/term.ReadPassword` path is exercised only through the injected-reader seam; a one-time manual `dsn wallet generate` on a real TTY would close the gap.
- **S3 — `wallet nonce` / `tx create` / contract nonce lookups don't load a key (D3)** — verified by grep (only `wallet.LoadKey` defs and tests reference the loader), but no test pins the "no key load on nonce paths" invariant; out of change scope.

### Verdict

**PASS WITH WARNINGS** — 40/40 tasks implemented and passing, 21/21 requirements, 47/47 scenarios with passing runtime evidence, 0 blockers, 0 CRITICAL. All four frozen pins byte-verified (including the corrected `legal winner` value), both documented deviations confirmed in code + apply-progress, zero broken `LoadKey` call sites, docs "no recovery mechanism" statements removed. The WARNINGs are task-bookkeeping (W1), documented spec deviations required by frozen pins (W2), informational coverage (W3), and the recorded size-exception (W4) — none block archiving, but W1 must be reconciled in tasks.md/apply-progress before or during archive.
