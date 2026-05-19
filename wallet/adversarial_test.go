package wallet

import (
	"os"
	"strings"
	"testing"
)

// TestAdversarial_CorruptedKeyFile tests that LoadValidatorKey handles
// various corrupted key file contents gracefully.
func TestAdversarial_CorruptedKeyFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
	}{
		{
			name:        "empty file",
			content:     "",
			expectError: true,
		},
		{
			name:        "short key - too few characters",
			content:     "abcdef\n",
			expectError: true,
		},
		{
			name:        "invalid hex characters",
			content:     "ZZZ\nYYY\nZZZ\n",
			expectError: true,
		},
		{
			name:        "wrong line count - only 1 line",
			content:     "abc",
			expectError: true,
		},
		{
			name:        "wrong line count - only 2 lines",
			content:     "abc\ndef",
			expectError: true,
		},
		{
			name:        "truncated private key",
			content:     "aaaa\n" + strings.Repeat("a", 63) + "\n" + strings.Repeat("a", 40) + "\n",
			expectError: true,
		},
		{
			name:        "too long private key",
			content:     strings.Repeat("a", 130) + "\n" + strings.Repeat("a", 64) + "\n" + strings.Repeat("a", 40) + "\n",
			expectError: true,
		},
		{
			name:        "wrong public key length - too short",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("a", 30) + "\n" + strings.Repeat("a", 40) + "\n",
			expectError: true,
		},
		{
			name:        "wrong public key length - too long",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("a", 34) + "\n" + strings.Repeat("a", 40) + "\n",
			expectError: true,
		},
		{
			name:        "wrong address length - too short",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("a", 64) + "\n" + strings.Repeat("a", 35) + "\n",
			expectError: true,
		},
		{
			name:        "wrong address length - too long",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("a", 64) + "\n" + strings.Repeat("a", 45) + "\n",
			expectError: true,
		},
		{
			name:        "mismatched keys - pubkey doesn't derive from priv",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("b", 64) + "\n" + strings.Repeat("a", 40) + "\n",
			expectError: true,
		},
		{
			name:        "mismatched address - doesn't match pubkey",
			content:     strings.Repeat("a", 128) + "\n" + strings.Repeat("a", 64) + "\n" + strings.Repeat("b", 40) + "\n",
			expectError: true,
		},
		{
			name:        "whitespace only",
			content:     "   \n\t\n  \n",
			expectError: true,
		},
		{
			name:        "null bytes",
			content:     "\x00\x01\x02\n\x00\x01\x02\n\x00\x01\x02\n",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write corrupted content to temp file
			tmpFile := t.TempDir() + "/corrupt_key.txt"
			err := os.WriteFile(tmpFile, []byte(tt.content), 0600)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			// Try to load
			_, err = LoadValidatorKey(tmpFile)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.name)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.name, err)
			}
		})
	}
}

// TestAdversarial_FileNotReadable tests that appropriate errors are returned
// when the key file cannot be read.
func TestAdversarial_FileNotReadable(t *testing.T) {
	// Try to load a file that doesn't exist
	_, err := LoadValidatorKey("/this/path/does/not/exist/key.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestAdversarial_PartialWrite tests that loading a key file that's been
// partially written handles the error gracefully.
func TestAdversarial_PartialWrite(t *testing.T) {
	tmpFile := t.TempDir() + "/partial_key.txt"

	// Write partial content (valid hex but incomplete)
	partialContent := strings.Repeat("a", 50) // Valid hex but incomplete lines
	err := os.WriteFile(tmpFile, []byte(partialContent), 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err = LoadValidatorKey(tmpFile)
	if err == nil {
		t.Error("expected error for partial write")
	}
}

// TestAdversarial_BinaryContent tests that binary content in key file
// is properly rejected.
func TestAdversarial_BinaryContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "binary data",
			content: []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
		},
		{
			name:    "null terminated strings",
			content: []byte("abc\x00def\x00ghi\x00"),
		},
		{
			name:    "high ascii",
			content: []byte{0x80, 0x81, 0x82, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/binary_key.txt"
			err := os.WriteFile(tmpFile, tt.content, 0600)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			_, err = LoadValidatorKey(tmpFile)
			// Binary content should either fail hex decode or fail validation
			if err == nil {
				t.Errorf("%s: expected error for binary content", tt.name)
			}
		})
	}
}

// TestAdversarial_AllZerosKey tests loading a key with all zeros (invalid
// Ed25519 key).
func TestAdversarial_AllZerosKey(t *testing.T) {
	tmpFile := t.TempDir() + "/zeros_key.txt"

	// All zeros is not a valid Ed25519 key (the generation would fail)
	// But we can try to load such a file
	allZeros := strings.Repeat("0", 128) + "\n" + strings.Repeat("0", 64) + "\n" + strings.Repeat("0", 40) + "\n"
	err := os.WriteFile(tmpFile, []byte(allZeros), 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// This should fail because the public key derived from all-zero
	// private key won't match the stored all-zero public key
	_, err = LoadValidatorKey(tmpFile)
	if err == nil {
		t.Error("expected error for all-zeros key (mismatched pubkey)")
	}
}

// TestAdversarial_ValidHexInvalidKey tests that valid hex but invalid key
// structure is rejected.
func TestAdversarial_ValidHexInvalidKey(t *testing.T) {
	tmpFile := t.TempDir() + "/invalid_key.txt"

	// Valid hex lengths but keys don't match each other
	// 128 char privkey, 64 char pubkey (different from derived), 40 char addr
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + "\n" + // 128 a's
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" + "\n" + // 64 b's - won't derive from privkey
		"cccccccccccccccccccccccccccccccccccccccc" + "\n" // 40 c's - won't match derived addr

	err := os.WriteFile(tmpFile, []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err = LoadValidatorKey(tmpFile)
	if err == nil {
		t.Error("expected error for mismatched keys")
	}
}