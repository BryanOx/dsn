# Wallet CLI Specification

## Purpose

New CLI surface on `dsn wallet`: `generate`, `restore`, `export-mnemonic`, `migrate`, plus passphrase prompting policy; existing commands keep working against both keystore formats.

## Requirements

### Requirement: wallet generate

`dsn wallet generate` MUST create an encrypted keystore by default, prompting twice on the TTY; mismatched entries MUST abort without creating a file. The mnemonic MUST print exactly once with a back-up warning, never to disk. `--legacy-plaintext` MUST write the legacy format without prompting.

#### Scenario: Default encrypted generation

- GIVEN a TTY and matching passphrases ≥ 12 chars
- WHEN `dsn wallet generate` runs
- THEN an encrypted keystore (0600) is written, the mnemonic prints once, a warning shows

#### Scenario: Passphrase mismatch

- GIVEN a TTY where the two passphrase entries differ
- WHEN `dsn wallet generate` runs
- THEN an error is returned and no wallet file exists

#### Scenario: Legacy plaintext escape hatch

- GIVEN `dsn wallet generate --legacy-plaintext`
- WHEN it runs
- THEN no prompt and a legacy plaintext wallet is written

### Requirement: wallet restore

`dsn wallet restore --mnemonic "<phrase>"` MUST validate the phrase (wordlist, checksum, 12 words) before anything else, then prompt for a new passphrase twice, write an encrypted keystore, and print the derived address.

#### Scenario: Restore reproduces the address

- GIVEN a valid 12-word phrase from an earlier generated wallet
- WHEN restore runs with a new passphrase
- THEN an encrypted keystore is written and the address matches the original

#### Scenario: Invalid mnemonic rejected

- GIVEN a phrase with an invalid checksum
- WHEN restore runs
- THEN an error is returned before any prompt or file write

### Requirement: wallet export-mnemonic

`dsn wallet export-mnemonic` MUST print, exactly once, the mnemonic of a wallet generated or restored earlier in the same process, with a warning it cannot be shown again; without one in memory it MUST fail and print nothing.

#### Scenario: Export after generation

- GIVEN a wallet just generated in the same process
- WHEN `dsn wallet export-mnemonic` runs
- THEN the 12-word mnemonic prints once with the warning

#### Scenario: Export in a fresh process

- GIVEN a fresh process with only a keystore on disk
- WHEN `dsn wallet export-mnemonic` runs
- THEN it fails, explaining the mnemonic is never stored

### Requirement: Passphrase prompting policy

Passphrases MUST be read from the TTY with double entry, NEVER from argv, env, or piped stdin, and MUST be at least 12 characters. Commands needing one MUST fail without a TTY.

#### Scenario: Passphrase flag rejected

- GIVEN `dsn wallet generate --passphrase secret`
- WHEN the command runs
- THEN it fails, explaining passphrases are prompted only

#### Scenario: Non-TTY stdin

- GIVEN stdin that is not a TTY
- WHEN a passphrase-prompting command runs
- THEN it fails with a TTY-required error and prompts nothing

#### Scenario: Short passphrase

- GIVEN a first entry shorter than 12 characters
- WHEN the prompt flow runs
- THEN the entry is rejected and prompting stops

### Requirement: Existing commands keep working

`sign`, `tx-sign`, `contract`, `nonce`, and the TUI MUST load via the unified dual-format loader: encrypted keystores prompt on the TTY; legacy plaintext loads with the stderr warning, unchanged behavior.

#### Scenario: Legacy wallet signs unchanged

- GIVEN a legacy plaintext wallet that signed a transaction pre-change
- WHEN the same transaction is signed after the change
- THEN the signature is identical

#### Scenario: Encrypted keystore signs after prompt

- GIVEN an encrypted keystore and a TTY
- WHEN `dsn wallet sign` runs and the correct passphrase is entered
- THEN the transaction is signed and no warning is emitted

#### Scenario: Wrong passphrase during sign

- GIVEN an encrypted keystore
- WHEN `dsn wallet sign` runs with a wrong passphrase
- THEN a checksum error is returned and the transaction is unsigned
