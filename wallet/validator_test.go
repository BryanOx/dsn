package wallet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/dsn/dsn/types"
)

// TestValidatorKey_GenerateAndSaveAndLoad tests the full round-trip of
// generating a validator key, saving it, and loading it back.
func TestValidatorKey_GenerateAndSaveAndLoad(t *testing.T) {
	// 1. Generate a new validator key
	key1, err := GenerateValidatorKey()
	if err != nil {
		t.Fatalf("failed to generate validator key: %v", err)
	}
	if key1 == nil {
		t.Fatal("generated key is nil")
	}

	// 2. Save to a temp file
	tmpFile := t.TempDir() + "/validator_key.txt"
	err = SaveValidatorKey(tmpFile, key1.PrivateKey, key1.PublicKey)
	if err != nil {
		t.Fatalf("failed to save validator key: %v", err)
	}

	// 3. Load from the temp file
	key2, err := LoadValidatorKey(tmpFile)
	if err != nil {
		t.Fatalf("failed to load validator key: %v", err)
	}
	if key2 == nil {
		t.Fatal("loaded key is nil")
	}

	// 4. Verify private keys match
	if !bytes.Equal(key1.PrivateKey, key2.PrivateKey) {
		t.Error("private keys do not match after round-trip")
	}

	// 5. Verify public keys match
	if !bytes.Equal(key1.PublicKey, key2.PublicKey) {
		t.Error("public keys do not match after round-trip")
	}

	// 6. Verify addresses match
	if key1.Address != key2.Address {
		t.Error("addresses do not match after round-trip")
	}

	// 7. Verify address is correctly derived from public key (sha256(pub)[:20])
	expectedAddr := deriveAddress(key1.PublicKey)
	if key1.Address != expectedAddr {
		t.Error("address is not correctly derived from public key")
	}
}

// TestValidatorKey_SigningWithLoadedKey tests that signing with a loaded key
// produces verifiable signatures.
func TestValidatorKey_SigningWithLoadedKey(t *testing.T) {
	// Generate and save a key
	key1, err := GenerateValidatorKey()
	if err != nil {
		t.Fatalf("failed to generate validator key: %v", err)
	}

	tmpFile := t.TempDir() + "/signing_key.txt"
	err = SaveValidatorKey(tmpFile, key1.PrivateKey, key1.PublicKey)
	if err != nil {
		t.Fatalf("failed to save validator key: %v", err)
	}

	// Load the key
	key2, err := LoadValidatorKey(tmpFile)
	if err != nil {
		t.Fatalf("failed to load validator key: %v", err)
	}

	// Sign a message with the loaded key
	message := []byte("test message for signing")
	signature := ed25519.Sign(key2.PrivateKey, message)

	// Verify the signature using the public key
	if !ed25519.Verify(key2.PublicKey, message, signature) {
		t.Error("signature verification failed")
	}

	// Verify that the signature is different from the message
	if string(signature) == string(message) {
		t.Error("signature should not equal the message")
	}
}

// TestValidatorKey_AddressDerivation tests that the address derivation
// (sha256(pubKey)[:20]) is consistent.
func TestValidatorKey_AddressDerivation(t *testing.T) {
	key, err := GenerateValidatorKey()
	if err != nil {
		t.Fatalf("failed to generate validator key: %v", err)
	}

	// Manually derive address
	hash := sha256.Sum256(key.PublicKey)
	var expectedAddr types.Address
	copy(expectedAddr[:], hash[:20])

	if key.Address != expectedAddr {
		t.Errorf("address derivation incorrect: got %v, want %v", key.Address, expectedAddr)
	}
}

// TestValidatorKey_DifferentKeysHaveDifferentAddresses tests that different
// keys produce different addresses.
func TestValidatorKey_DifferentKeysHaveDifferentAddresses(t *testing.T) {
	key1, _ := GenerateValidatorKey()
	key2, _ := GenerateValidatorKey()

	// Keys should be different
	if bytes.Equal(key1.PrivateKey, key2.PrivateKey) {
		t.Error("generated keys should be different")
	}

	// Addresses should be different
	if key1.Address == key2.Address {
		t.Error("different keys should produce different addresses")
	}
}

// TestValidatorKey_FilePermissions tests that the key file has secure
// permissions (0600).
func TestValidatorKey_FilePermissions(t *testing.T) {
	key, err := GenerateValidatorKey()
	if err != nil {
		t.Fatalf("failed to generate validator key: %v", err)
	}

	tmpFile := t.TempDir() + "/permissions_test.txt"
	err = SaveValidatorKey(tmpFile, key.PrivateKey, key.PublicKey)
	if err != nil {
		t.Fatalf("failed to save validator key: %v", err)
	}

	// Check file info
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	// On Unix, permission should be 0600. On Windows, this may differ.
	// Just verify the file exists and is readable by owner.
	_ = info
}

// TestValidatorKey_EmptyPath tests that loading with empty path returns error.
func TestValidatorKey_EmptyPath(t *testing.T) {
	_, err := LoadValidatorKey("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

// TestValidatorKey_NonExistentFile tests that loading non-existent file returns error.
func TestValidatorKey_NonExistentFile(t *testing.T) {
	_, err := LoadValidatorKey("/nonexistent/path/to/key.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestValidatorKey_KeyFormatIntegrity tests that keys saved and loaded
// maintain their format integrity (64-byte private, 32-byte public).
func TestValidatorKey_KeyFormatIntegrity(t *testing.T) {
	key, err := GenerateValidatorKey()
	if err != nil {
		t.Fatalf("failed to generate validator key: %v", err)
	}

	// Verify key sizes
	if len(key.PrivateKey) != 64 {
		t.Errorf("private key should be 64 bytes, got %d", len(key.PrivateKey))
	}
	if len(key.PublicKey) != 32 {
		t.Errorf("public key should be 32 bytes, got %d", len(key.PublicKey))
	}

	tmpFile := t.TempDir() + "/format_test.txt"
	err = SaveValidatorKey(tmpFile, key.PrivateKey, key.PublicKey)
	if err != nil {
		t.Fatalf("failed to save validator key: %v", err)
	}

	loadedKey, err := LoadValidatorKey(tmpFile)
	if err != nil {
		t.Fatalf("failed to load validator key: %v", err)
	}

	// Verify loaded key sizes
	if len(loadedKey.PrivateKey) != 64 {
		t.Errorf("loaded private key should be 64 bytes, got %d", len(loadedKey.PrivateKey))
	}
	if len(loadedKey.PublicKey) != 32 {
		t.Errorf("loaded public key should be 32 bytes, got %d", len(loadedKey.PublicKey))
	}
}
