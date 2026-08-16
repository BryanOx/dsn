# Design: Wallet Recovery via BIP-39 Mnemonic + Encrypted Keystore

## Technical Approach

Three-layer strategy: `internal/bip39` pure logic (official-vector-tested, `go:embed` wordlist); `wallet/` keystore (AES-256-GCM + scrypt, versioned JSON, dual-format `LoadKeyFile`, atomic migrate); thin CLI/SDK surfaces. Legacy SHA-256 derivation is frozen, not repointed — compat pins guarantee zero key change. Delivered in 3 chained PRs (libs → CLI+TUI → SDK+docs+integration).

## Architecture Decisions

| # | Decision | Choice | Alternatives considered | Rationale |
|---|----------|--------|-------------------------|-----------|
| D1 | Unified loader | `LoadKeyFile(path, getPassphrase PassphraseFunc)`; `LoadKey(path)` = `LoadKeyFile(path, nil)` | `(path, passphrase string)`; `KeyLoader` interface | Lazy callback — invoked only when sniff finds a keystore; `LoadKey` signature unchanged → PR1 breaks zero call sites; `nil` → `ErrKeystorePassphraseRequired` |
| D2 | Passphrase supply | CLI/TUI pass TTY-prompt closure from `internal/passphrase`; daemon (`main.go`) passes `nil` | env var; argv; piped stdin | No secret-in-env convention (node `env.go` is config; validator keys are files). Spec bans argv/env/pipes. Daemon wallet is optional read-only — must not hang |
| D3 | Load sites | 5 real sites: `main.go` (daemon), `tx_sign.go`, `contract_cmd.go` (deploy), `wallet_cmd.go` (sign), `cmd/dsn-tui/model.go`. `wallet nonce`/`tx create`/contract nonce lookups load **no key** (verified, RPC-only) | treating nonce as a site | Open item 1 resolved with evidence |
| D4 | Package layout | `internal/bip39` + `wallet/keystore.go` + `wallet/mnemonic.go` + `internal/passphrase` (shared CLI/TUI); sdk = thin re-exports | all-in-`wallet`; sdk-owned crypto | Follows existing `internal/txfile` precedent; TUI needs prompts too |
| D5 | Keystore format | 9-field versioned JSON; checksum = `sha256(derivedKey)[:4]`; AES-256-GCM 12-byte nonce; scrypt N=2^17 r=8 p=1 dkLen=32 stored in-file | Ethereum-style `version/address` | Ecosystem-familiar, sniffable, per-file tunable params; checksum before GCM open → no key release on wrong passphrase |
| D6 | KDF guard | Reject non-pow2 N and `N*r*128 > 1 GiB` before any derivation; typed `ErrKDFParams` | trusting file params | Spec: reject absurd params pre-derivation (2^30, non-pow2) |
| D7 | Migrate order | temp 0600+fsync → rename `path`→`path.bak` → rename temp→`path` → verify `DecryptKey`; failure after backup names `.bak`; 2nd run → `ErrAlreadyEncrypted`; existing `.bak` → `ErrBackupExists` | verify before rename; auto-migrate on load | Spec mandates verify-as-success-gate and `.bak` naming; `.bak` = recovery; no auto-migrate |
| D8 | Prompt deps | Add `golang.org/x/term` + `golang.org/x/crypto` (scrypt); PBKDF2 = stdlib `crypto/pbkdf2` (Go 1.24+) | `charmbracelet/x/term` (indirect-only) | `x/term.ReadPassword` is canonical no-echo read; promoting a TUI indirect dep couples CLI to bubbletea |
| D9 | CLI shape | `wallet {generate,restore,export-mnemonic,migrate,sign,nonce}`; `--mnemonic` on restore; `--legacy-plaintext` on generate; `--passphrase` on generate **rejected with explanation** | hidden flags | Spec scenario requires explicit rejection message |
| D10 | Mnemonic lifecycle | Shown once on generate/restore; kept in package-level in-memory var in `cmd/dsn` for `export-mnemonic`; never on disk; fresh process → typed error | persist encrypted | Closed decision: in-process only; export-after-fact impossible by construction |
| D11 | TUI | Prompt once at startup (before `tea.NewProgram`) via `internal/passphrase`; failure → wallet `nil` (current behavior) | prompt in wallet tab | No new bubbletea textinput machinery; TUI stays read-only |
| D12 | Atomicity | `writeAtomic(path, data, 0600)`: `os.CreateTemp` in target dir → chmod → write → fsync → rename; best-effort dir fsync | bare `os.WriteFile` (current `SaveKey`) | Spec: failed write must never leave partial file; parent dir ≤ 0700 |

## Data Flow

Generate:
```
TTY ──PromptTwice──> passphrase (≥12)
rand16 ──bip39.EntropyToMnemonic──> phrase ──MnemonicToSeed──> seed ──seed[:32]──> KeyPair
KeyPair ──EncryptKey──> keystoreJSON ──writeAtomic 0600──> wallets/wallet.json
phrase printed ONCE (stdout) + warning (stderr); held in process memory for export-mnemonic
```

Restore:
```
--mnemonic phrase ──ValidateMnemonic──✗→ error before any prompt/file
✓ ──> PromptTwice ──> seed ──> KeyPair ──> EncryptKey ──writeAtomic──> file + address printed
```

Migrate:
```
plaintext file ──sniff(no version)──> KeyPair ──EncryptKey──> temp(fsync) ──rename──> path.bak
──rename──> path ──DecryptKey verify──✓> success   |  ✗ after .bak → error names .bak
```

Load-at-sign (all 5 sites):
```
LoadKeyFile(path, provider) ──read+sniff──> legacy: hex JSON → kp + stderr warning (file untouched)
                                          └> keystore: provider() ──TTY──> prompt once
                                              non-TTY → ErrKeystorePassphraseRequired
                                              ──> kdfparams guard ──> scrypt ──> checksum
                                                  ✗ mismatch → ErrWrongPassphrase (no GCM)
                                                  ✓ ──> GCM open ──> kp | ErrCorruptKeystore
```

## Interfaces / Contracts

```json
{"version":1,"cipher":"aes-256-gcm","kdf":"scrypt",
 "kdfparams":{"n":131072,"r":8,"p":1,"dkLen":32},
 "salt":"<hex 16B>","iv":"<hex 12B>","ciphertext":"<hex 80B>","checksum":"<hex 4B>"}
```

```go
// internal/bip39
func GenerateMnemonic() (string, error)
func EntropyToMnemonic(entropy []byte) (string, error) // official-vector path
func ValidateMnemonic(phrase string) error             // 12 words, wordlist, checksum; typed errors
func MnemonicToSeed(phrase string) ([]byte, error)     // PBKDF2-HMAC-SHA512, 2048, salt "mnemonic"

// wallet
type PassphraseFunc func() (string, error)
func KeyFromMnemonic(phrase string) (*KeyPair, error)  // validate → seed[:32] → Ed25519
func EncryptKey(passphrase string, kp *KeyPair) ([]byte, error)
func DecryptKey(passphrase string, data []byte) (*KeyPair, error)
func SaveKeystore(path, passphrase string, kp *KeyPair) error // atomic 0600
func LoadKeyFile(path string, getPassphrase PassphraseFunc) (*KeyPair, error)
func LoadKey(path string) (*KeyPair, error)            // LoadKeyFile(path, nil)
func Migrate(path, passphrase string) error
// errors: ErrKeystorePassphraseRequired, ErrWrongPassphrase, ErrCorruptKeystore,
//         ErrUnsupportedVersion/Cipher/KDF, ErrKDFParams, ErrAlreadyEncrypted, ErrBackupExists

// internal/passphrase
func IsTTY() bool
func PromptTwice(label string) (string, error)         // min 12, mismatch aborts
```

## File Changes — 3-PR Slices (verbatim for sdd-tasks)

**PR1 — libs** (authored ≈ 350 lines; `english.txt` 2048-word data asset excluded from authored count)

| File | Action | Description |
|------|--------|-------------|
| `internal/bip39/bip39.go` | Create | Generate/EntropyToMnemonic/Validate/MnemonicToSeed |
| `internal/bip39/wordlist.go` | Create | `go:embed english.txt`, index map |
| `internal/bip39/english.txt` | Create | Official BIP-39 English wordlist (data asset) |
| `internal/bip39/bip39_test.go` | Create | Official 128-bit vectors + validation failures |
| `wallet/keystore.go` | Create | Schema, Encrypt/Decrypt/Save/LoadKeyFile/Migrate/`writeAtomic` |
| `wallet/keystore_test.go` | Create | Roundtrip, wrong passphrase, truncation, KDF guard, atomicity, migrate, sniff |
| `wallet/mnemonic.go` | Create | `KeyFromMnemonic` |
| `wallet/mnemonic_test.go` | Create | Official vector → key; invalid phrase rejected |
| `wallet/wallet.go` | Modify | `LoadKey` = `LoadKeyFile(path, nil)`; legacy path + stderr warning; `SaveKey` untouched |
| `wallet/wallet_test.go` | Modify | Legacy tests stay green; add warning + sniff cases |
| `go.mod` / `go.sum` | Modify | + `golang.org/x/crypto` |

**PR2 — CLI + TUI** (authored ≈ 380 lines)

| File | Action | Description |
|------|--------|-------------|
| `internal/passphrase/passphrase.go` | Create | IsTTY, PromptTwice (x/term, min 12, prompts→stderr) |
| `internal/passphrase/passphrase_test.go` | Create | Non-TTY error; mismatch; short; injected reader |
| `cmd/dsn/wallet_cmd.go` | Modify | restore/export-mnemonic/migrate; generate encrypted default + `--legacy-plaintext` + `--passphrase` rejection; sign→LoadKeyFile |
| `cmd/dsn/tx_sign.go` | Modify | LoadKeyFile + TTY provider |
| `cmd/dsn/contract_cmd.go` | Modify | LoadKeyFile + TTY provider |
| `cmd/dsn/main.go` | Modify | `nil` provider (read-only daemon continues) |
| `cmd/dsn-tui/model.go` | Modify | Startup prompt for keystores; nil on failure |
| `go.mod` / `go.sum` | Modify | + `golang.org/x/term` |

**PR3 — SDK + docs + integration** (authored ≈ 350 lines)

| File | Action | Description |
|------|--------|-------------|
| `sdk/wallet.go` | Modify | `Deprecated:` on `NewKeyFromMnemonic` (SHA-256 impl untouched); `NewKeyFromBIP39Mnemonic`, `GenerateMnemonic`, `RestoreFromMnemonic`, `EncryptKey`/`DecryptKey` wrappers (no prompting) |
| `sdk/wallet_test.go` | Create | Legacy compat pins (golden values below); BIP-39 official vector; determinism |
| `integration/wallet_recovery_test.go` | Create | generate→restore→sign; migrate then sign; wrong passphrase; legacy sign unchanged |
| `docs/GUIDES.md` | Modify | Replace "no recovery mechanism" (line 1480); wallet recovery section |
| `docs/WHITEPAPER.md` | Modify | Wallet security/recovery notes |

Legacy golden pins (computed from current pre-change code — freeze): `abandon … about` → priv `c557ee…e7fc7`; `legal winner … yellow` → priv `ecb0e7…00152`; `dsn demo … eight` → priv `f4ba61…511c`. BIP-39 vector: entropy `0000…00` → seed `c55257c3…3b04` → `ed25519.NewKeyFromSeed(seed[:32])`; plus `7f7f…`/`8080…`/`ffff…` 12-word vectors.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | bip39: official 128-bit vectors; word-count/wordlist/checksum/case failures | `internal/bip39/bip39_test.go` |
| Unit | keystore: roundtrip; randomized salt/iv; wrong passphrase (checksum, no GCM); truncated ciphertext (GCM auth); N=2^30 & non-pow2 rejected pre-derivation; atomic write (no temp, 0600); migrate idempotency + `.bak` naming | `wallet/keystore_test.go` |
| Unit | `KeyFromMnemonic` vector + invalid-phrase; sdk legacy pins + deprecation (`go doc` shows `Deprecated`) | `wallet/`, `sdk/wallet_test.go` |
| Integration | generate→restore same address; legacy tx signs identically; migrate→sign; wrong passphrase fails fast | `integration/wallet_recovery_test.go` (`make test-integration`) |

All gated by `make test` / `make test-integration`; strict TDD — RED tests per scenario before implementation.

## Threat Matrix

N/A — no routing, shell command, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. (TTY passphrase reads are direct fd I/O within one process.)

## Migration / Rollout

No data migration outside `dsn wallet migrate` (user-invoked, in-place, `.bak` retained). Additive: old binaries read legacy files; rollback = revert PR chain; funds recoverable via `.bak` or mnemonic.

## Open Questions

None — all spec-agent open items resolved (D1–D12).
