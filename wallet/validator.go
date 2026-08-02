package wallet

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsn/dsn/types"
)

// ErrInvalidKeyFile represents errors when loading validator key file
var ErrInvalidKeyFile = errors.New("invalid key file")

// ValidatorKey holds Ed25519 key pair and derived address for validator operations.
type ValidatorKey struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Address    types.Address
}

// SaveValidatorKey persists validator key to a three-line hex file.
// Format:
//
//	Line 1: hex(private key) - 64 bytes = 128 hex chars
//	Line 2: hex(public key)  - 32 bytes = 64 hex chars
//	Line 3: hex(address)     - 20 bytes = 40 hex chars
func SaveValidatorKey(path string, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) error {
	addr := deriveAddress(pubKey)

	data := hex.EncodeToString(privKey) + "\n"
	data += hex.EncodeToString(pubKey) + "\n"
	data += hex.EncodeToString(addr[:]) + "\n"

	// Write with secure permissions (0600), creating the parent directory if needed.
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(data), 0600)
}

// LoadValidatorKey reads and validates a validator key file.
// Returns the private key, public key, and derived address.
func LoadValidatorKey(path string) (*ValidatorKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	lines := splitLines(string(data))
	if len(lines) < 3 {
		return nil, fmt.Errorf("%w: expected 3 lines, got %d", ErrInvalidKeyFile, len(lines))
	}

	// Line 1: private key (128 hex chars = 64 bytes)
	privBytes, err := hex.DecodeString(lines[0])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid private key hex: %w", ErrInvalidKeyFile, err)
	}
	if len(privBytes) != 64 {
		return nil, fmt.Errorf("%w: private key must be 64 bytes, got %d", ErrInvalidKeyFile, len(privBytes))
	}

	// Line 2: public key (64 hex chars = 32 bytes)
	pubBytes, err := hex.DecodeString(lines[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid public key hex: %w", ErrInvalidKeyFile, err)
	}
	if len(pubBytes) != 32 {
		return nil, fmt.Errorf("%w: public key must be 32 bytes, got %d", ErrInvalidKeyFile, len(pubBytes))
	}

	// Line 3: address (40 hex chars = 20 bytes)
	addrBytes, err := hex.DecodeString(lines[2])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid address hex: %w", ErrInvalidKeyFile, err)
	}
	if len(addrBytes) != 20 {
		return nil, fmt.Errorf("%w: address must be 20 bytes, got %d", ErrInvalidKeyFile, len(addrBytes))
	}

	// Validate that public key derived from private key matches stored public key
	derivedPub := ed25519.PrivateKey(privBytes).Public().(ed25519.PublicKey)
	if len(derivedPub) != 32 {
		return nil, fmt.Errorf("%w: derived public key invalid", ErrInvalidKeyFile)
	}
	if hex.EncodeToString(derivedPub) != hex.EncodeToString(pubBytes) {
		return nil, fmt.Errorf("%w: public key does not match private key", ErrInvalidKeyFile)
	}

	// Validate that address = sha256(pubKey)[:20]
	computedAddr := deriveAddress(ed25519.PublicKey(pubBytes))
	var storedAddr types.Address
	copy(storedAddr[:], addrBytes)
	if computedAddr != storedAddr {
		return nil, fmt.Errorf("%w: address does not match public key", ErrInvalidKeyFile)
	}

	return &ValidatorKey{
		PrivateKey: ed25519.PrivateKey(privBytes),
		PublicKey:  ed25519.PublicKey(pubBytes),
		Address:    storedAddr,
	}, nil
}

// deriveAddress derives the DSN address from a public key (first 20 bytes of SHA256).
func deriveAddress(pubKey ed25519.PublicKey) types.Address {
	hash := sha256.Sum256(pubKey)
	var addr types.Address
	copy(addr[:], hash[:20])
	return addr
}

// splitLines splits text into non-empty lines
func splitLines(s string) []string {
	var lines []string
	for _, line := range []byte(s) {
		if line == '\n' {
			if len(lines) > 0 && len(lines[len(lines)-1]) > 0 {
				lines = append(lines, "")
			}
		} else if line != '\r' {
			if len(lines) == 0 {
				lines = append(lines, "")
			}
			lines[len(lines)-1] += string(line)
		}
	}
	// Remove trailing empty line
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// GenerateValidatorKey generates a new validator key pair.
func GenerateValidatorKey() (*ValidatorKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	addr := deriveAddress(pub)
	return &ValidatorKey{
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    addr,
	}, nil
}
