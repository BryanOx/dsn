# BIP-39 Mnemonic Specification

## Purpose

New capability `bip39-mnemonic` in `internal/bip39`: generate, validate, and derive seeds for 12-word English BIP-39 mnemonics. Enables wallet recovery: `dsn wallet restore` and `sdk.RestoreFromMnemonic` produce the same Ed25519 key from the same phrase, deterministically.

## Requirements

### Requirement: Mnemonic generation

`GenerateMnemonic` MUST produce exactly 12 words from the BIP-39 English wordlist (2048 words, vendored via `go:embed`) from 128-bit CSPRNG entropy: 128 entropy bits + 4 checksum bits split into 12 × 11-bit wordlist indices.

#### Scenario: Generated phrase is well-formed

- GIVEN a call to `GenerateMnemonic`
- WHEN it returns a phrase
- THEN the phrase has exactly 12 words joined by single spaces
- AND every word is present in the English wordlist

#### Scenario: Generated phrase passes validation

- GIVEN any result of `GenerateMnemonic`
- WHEN `ValidateMnemonic` is called on it
- THEN validation succeeds, confirming the checksum bits are correct

### Requirement: Mnemonic validation

`ValidateMnemonic` MUST reject a phrase unless it has exactly 12 words, every word is in the English wordlist, and the checksum (SHA-256 of entropy, first `entropy/32` bits) matches. It SHOULD normalize input by trimming surrounding whitespace and lowercasing before validating.

#### Scenario: Valid phrase accepted

- GIVEN the phrase `abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about`
- WHEN `ValidateMnemonic` is called
- THEN it succeeds

#### Scenario: Word not in wordlist

- GIVEN the phrase `abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon zebra`
- WHEN `ValidateMnemonic` is called
- THEN it fails with an error naming the invalid word

#### Scenario: Wrong checksum

- GIVEN a 12-word phrase whose last word is valid but does not match the checksum bits
- WHEN `ValidateMnemonic` is called
- THEN it fails with a checksum error

#### Scenario: Wrong word count

- GIVEN an 11-word or a 13-word phrase of valid words
- WHEN `ValidateMnemonic` is called
- THEN it fails with a length error

#### Scenario: Mixed-case input normalized

- GIVEN the official phrase written in UPPERCASE
- WHEN `ValidateMnemonic` is called
- THEN it succeeds after lowercasing

### Requirement: Seed derivation

Seed derivation MUST use PBKDF2-HMAC-SHA512 (stdlib `crypto/pbkdf2`) with 2048 iterations and salt `"mnemonic"` over the normalized phrase, producing the 64-byte BIP-39 seed. `wallet.KeyFromMnemonic` MUST map the seed's first 32 bytes to an Ed25519 keypair.

#### Scenario: Official BIP-39 test vector (128-bit)

- GIVEN entropy `00000000000000000000000000000000`
- WHEN the seed is derived from its mnemonic `abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about`
- THEN the 64-byte seed equals `c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04`
- AND the derived Ed25519 public key matches `NewKeyFromSeed(seed[:32])`

#### Scenario: Deterministic seed

- GIVEN the same normalized phrase
- WHEN seed derivation runs twice
- THEN both seeds are byte-identical

### Requirement: Restore from mnemonic

`wallet.KeyFromMnemonic` (and `sdk.NewKeyFromBIP39Mnemonic`) MUST validate the phrase before deriving; invalid phrases MUST fail without producing a key. The derivation MUST NOT fall back to any other algorithm.

#### Scenario: Restore reproduces the original key

- GIVEN a mnemonic whose wallet was generated earlier
- WHEN the mnemonic is restored
- THEN the restored keypair's address equals the originally generated address

#### Scenario: Invalid phrase rejected at restore

- GIVEN a phrase with an invalid checksum
- WHEN restore is attempted
- THEN an error is returned and no keypair is produced
