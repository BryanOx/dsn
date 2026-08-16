# Proposal: Wallet Recovery via BIP-39 Mnemonic + Encrypted Keystore

## Intent

DSN wallets are unrecoverable: `wallet.json` is plaintext and `sdk.NewKeyFromMnemonic` SHA-256-hashes the phrase (no BIP-39 validation). This change adds BIP-39 recovery (12 words) + passphrase-encrypted keystores.

## Scope

**In**
- English BIP-39 wordlist (`go:embed`), generate/validate/seed (PBKDF2 2048 iters)
- Encrypted keystore: versioned JSON, AES-256-GCM + scrypt; default-on for new wallets
- CLI: generate/restore/export-mnemonic/migrate; TTY prompts; dual-format load (sign, tx-sign, contract, TUI)
- Deprecate `sdk.NewKeyFromMnemonic`; add `sdk.NewKeyFromBIP39Mnemonic`; docs update

**Out**
- Validator keys (unchanged, plaintext); HD/SLIP-0010, non-English wordlists, hardware wallets; lost-passphrase recovery (by design)

## Capabilities

**New**: `bip39-mnemonic` (generate/validate/seed/restore/export); `wallet-keystore` (encryption, passphrase policy, dual-format load, migrate)
**Modified**: None — `openspec/specs/` empty

## Approach

- **A. Legacy `NewKeyFromMnemonic`**: keep + `Deprecated`, add BIP-39 entry — public API, 0 internal callers; repointing changes derived keys (fund loss); removal = v2 break
- **B. Keystore format**: versioned JSON `{version, cipher aes-256-gcm, kdf scrypt, kdfparams, salt, iv, ciphertext, checksum}` — sniffable loader, tunable params, ecosystem-familiar
- **C. CLI**: `generate` (encrypted default, `--legacy-plaintext`), `restore --mnemonic`, `export-mnemonic`, `migrate`; TTY prompts twice, never argv
- **D. Plaintext files**: dual-format read + warning; explicit `migrate` (atomic, `.bak`); no auto-migrate — 5 load sites keep working
- **E. Layout**: `internal/bip39` (logic + wordlist embed); `wallet.KeyFromMnemonic` (seed[:32]→Ed25519); sdk re-exports
- **F. Posture**: min 8-char passphrase (≥12 advised); scrypt N=2^17 r=8 p=1; mnemonic never persisted; atomic 0600 writes

PBKDF2: stdlib `crypto/pbkdf2`; only scrypt adds `golang.org/x/crypto`.

## Affected Areas

- `internal/bip39/` — new: logic, wordlist, vectors
- `wallet/wallet.go`, `wallet/keystore.go` — encrypt/decrypt, unified `Load`
- `sdk/wallet.go` — deprecate + BIP-39 entry
- `cmd/dsn/wallet_cmd.go` — 4 commands
- `cmd/dsn/tx_sign.go`, `contract_cmd.go`, `cmd/dsn-tui/model.go` — passphrase prompts
- `go.mod`, `docs/GUIDES.md`, `docs/WHITEPAPER.md`
- tests: wallet/, internal/bip39, cmd (`make test` / `make test-integration`)

## Risks

- Silent key-derivation change (Med): old fn untouched + vector tests
- Lost passphrase → funds locked (Med): warning banners, `.bak`, docs
- Scrypt latency on sign (Low): tunable in-file params
- 900–1300 lines vs 400-line budget (High): chained PRs (libs → CLI+TUI → docs)

## Rollback

Revert: plaintext loaders untouched, old binaries read old files. `migrate` keeps `.bak`; funds recoverable via mnemonic.

## Success Criteria

- [ ] Official BIP-39 vectors pass (English, 128-bit + seed)
- [ ] `generate` → `restore` roundtrip: identical address
- [ ] Legacy plaintext wallet signs unchanged
- [ ] Wrong passphrase fails fast (checksum)
- [ ] `make test`/`make test-integration` green; mnemonic never on disk

## Open Questions

- Scrypt cost: 2^17 vs 2^15 (faster) / 2^18 (stronger)?
- Passphrase minimum: 8 (recommended) vs 12?
- Chained PRs to respect the 400-line guard? (ask-on-risk)
