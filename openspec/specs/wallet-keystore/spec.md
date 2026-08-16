# Wallet Keystore Specification

## Purpose

New capability `wallet-keystore`: passphrase-encrypted, versioned JSON keystores plus a unified dual-format loader so legacy plaintext wallets keep working.

## Requirements

### Requirement: Encrypted keystore format

A keystore MUST be self-describing versioned JSON: `version:1`, `cipher:"aes-256-gcm"`, `kdf:"scrypt"`, `kdfparams{N,r,p,dkLen}`, `salt`, `iv`, `ciphertext`, `checksum`. Unknown `version`, `cipher`, `kdf`, or missing fields MUST fail parsing.

#### Scenario: Well-formed keystore

- GIVEN a keystore from the implementation
- WHEN parsed
- THEN all nine fields are present and typed

#### Scenario: Unsupported version

- GIVEN a keystore with `version:2`
- WHEN loading is attempted
- THEN it fails with an unsupported-version error

### Requirement: Encryption

Encryption MUST derive a 32-byte key via scrypt (N=2^17, r=8, p=1, random salt), seal the 64-byte Ed25519 private key with AES-256-GCM (random IV), and store `checksum` = first 4 bytes of SHA-256(derived key). No mnemonic or plaintext MAY persist.

#### Scenario: Roundtrip fidelity

- GIVEN a passphrase and keypair
- WHEN the keypair is encrypted then decrypted
- THEN the recovered keypair is byte-identical

#### Scenario: Randomized ciphertext

- GIVEN the same passphrase and keypair
- WHEN encryption runs twice
- THEN the two files differ in `salt`, `iv`, and `ciphertext`

### Requirement: Decryption with fail-fast checksum

Decryption MUST verify the checksum before GCM open, releasing no key on mismatch.

#### Scenario: Wrong passphrase

- GIVEN a keystore and wrong passphrase
- WHEN decryption is attempted
- THEN a checksum error is returned and no key material leaks

### Requirement: KDF parameters honored from file

Decryption MUST use the file's `kdfparams`, rejecting non-power-of-two `N` or over-1-GiB memory demands before deriving.

#### Scenario: Non-default params decrypt

- GIVEN a keystore written with N=2^15
- WHEN decrypted with the correct passphrase
- THEN the keypair is recovered using the file's params

#### Scenario: Absurd params rejected

- GIVEN a keystore with N=2^30 or non-power-of-two N
- WHEN decryption is attempted
- THEN it fails before any derivation

### Requirement: Atomic 0600 writes

Keystore writes MUST be atomic (temp file, fsync, rename) and 0600; a failed write MUST NOT leave a partial file.

#### Scenario: Atomic write

- GIVEN a keystore save
- WHEN the rename completes
- THEN the target holds only the complete file, no temp file remains

#### Scenario: File permissions

- GIVEN a saved keystore
- WHEN its mode is inspected
- THEN it is 0600 and the parent directory is at most 0700

### Requirement: Unified dual-format loader

The wallet loader MUST detect format by content: JSON with a `version` field loads via decryption; legacy hex JSON loads directly, warning on stderr, never auto-migrating.

#### Scenario: Encrypted load

- GIVEN an encrypted keystore and passphrase
- WHEN the loader runs
- THEN the keypair is returned via decryption

#### Scenario: Legacy plaintext load

- GIVEN a legacy plaintext wallet
- WHEN the unified loader runs
- THEN the keypair returns, stderr warns, the file is unchanged

### Requirement: Migration

`migrate` MUST write the encrypted keystore atomically (0600), back up the original as `<path>.bak`, rename the new file into place, and verify it decrypts before success; failures after backup MUST name the `.bak` path.

#### Scenario: Successful migration

- GIVEN a legacy plaintext wallet
- WHEN migrate runs with a valid passphrase
- THEN the target is a valid encrypted keystore and `<path>.bak` holds the plaintext

#### Scenario: Migration interrupted after backup

- GIVEN a failure between the renames
- WHEN migrate exits
- THEN the error names the `.bak` path so the original stays recoverable

### Requirement: Failure modes

Corrupt, truncated, or non-JSON data MUST fail loading without yielding a partial keypair.

#### Scenario: Truncated ciphertext

- GIVEN a keystore with truncated `ciphertext`
- WHEN decrypted with the correct passphrase
- THEN GCM authentication fails with an error

#### Scenario: Malformed JSON

- GIVEN a keystore file containing invalid JSON
- WHEN loading is attempted
- THEN a parse error is returned
