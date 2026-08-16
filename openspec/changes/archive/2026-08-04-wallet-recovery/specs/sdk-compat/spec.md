# SDK Compatibility Specification

## Purpose

SDK surface for wallet recovery with a hard compatibility guarantee: the legacy `NewKeyFromMnemonic` keeps its exact SHA-256 derivation (repointing it would silently change derived keys and lock funds), while a new BIP-39 entry point is added alongside new mnemonic/keystore helpers.

## Requirements

### Requirement: Legacy NewKeyFromMnemonic deprecated but unchanged

`sdk.NewKeyFromMnemonic` MUST remain exported and MUST derive keys exactly as today (SHA-256 of the raw phrase → 32-byte seed → Ed25519), for any input phrase. Its doc comment MUST carry `Deprecated` and MUST point to `NewKeyFromBIP39Mnemonic`. It MUST NOT be re-implemented on top of BIP-39.

#### Scenario: Compat pin test

- GIVEN a set of sample phrases and the keypairs they derived before this change
- WHEN `NewKeyFromMnemonic` is called for each phrase after the change
- THEN every keypair is byte-identical to the pre-change golden values

#### Scenario: Deprecation marker present

- GIVEN the exported symbol's documentation
- WHEN it is inspected via `go doc`
- THEN it is marked `Deprecated` and names `NewKeyFromBIP39Mnemonic` as the replacement

### Requirement: NewKeyFromBIP39Mnemonic

`sdk.NewKeyFromBIP39Mnemonic` MUST validate the phrase (12 words, English wordlist, checksum) and MUST derive the key via BIP-39 PBKDF2 (2048 iterations, salt `"mnemonic"`), mapping the seed's first 32 bytes to Ed25519. Invalid phrases MUST return an error; derivation MUST be deterministic.

#### Scenario: Official vector reproduces the key

- GIVEN the official 128-bit BIP-39 phrase `abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about`
- WHEN `NewKeyFromBIP39Mnemonic` is called
- THEN the public key equals `ed25519.NewKeyFromSeed(seed[:32])` for the vector seed `c55257c3…3495`

#### Scenario: Invalid phrase errors

- GIVEN a 12-word phrase with a bad checksum
- WHEN `NewKeyFromBIP39Mnemonic` is called
- THEN an error is returned and no keypair is produced

#### Scenario: Deterministic derivation

- GIVEN the same valid phrase
- WHEN `NewKeyFromBIP39Mnemonic` is called twice
- THEN both keypairs are identical

### Requirement: SDK mnemonic and keystore surface

The SDK MUST expose `GenerateMnemonic` (12-word English phrase) and `RestoreFromMnemonic` (validate + derive, equivalent to `NewKeyFromBIP39Mnemonic`). The SDK SHOULD expose `EncryptKey`/`DecryptKey` as thin wrappers over the wallet keystore (passphrase as parameter, never prompting).

#### Scenario: GenerateMnemonic shape

- GIVEN a call to `GenerateMnemonic`
- WHEN it returns
- THEN the phrase has 12 words and passes `NewKeyFromBIP39Mnemonic` validation

#### Scenario: Restore agrees with the new entry point

- GIVEN the same valid phrase
- WHEN both `RestoreFromMnemonic` and `NewKeyFromBIP39Mnemonic` are called
- THEN both return identical keypairs

### Requirement: No silent key change

No existing code path or persisted wallet MAY change which key it produces as a result of this change. New derivation behavior MUST only be reachable through new functions or explicit user action (`generate`, `restore`, `migrate`).

#### Scenario: Existing addresses unchanged

- GIVEN a set of wallets derived and saved before this change
- WHEN each is loaded after the change
- THEN every address and signature matches the pre-change values, with only the plaintext warning added
