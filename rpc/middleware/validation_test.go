package middleware

import (
	"strings"
	"testing"
)

// TestValidateHexString tests hex string validation.
func TestValidateHexString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		maxLen  int
		wantErr bool
	}{
		{"empty string passes", "", 100, false},
		{"valid hex passes", "0xabc123", 100, false},
		{"valid hex without prefix", "abc123", 100, false},
		{"invalid hex characters", "0xabcxyz", 100, true},
		{"exceeds max length", "0x" + strings.Repeat("a", 1001), 100, true},
		{"valid 64 char hex", "0x" + strings.Repeat("a", 64), 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHexString(tt.input, tt.maxLen)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ValidateHexString(%q, %d) error = %v, wantErr = %v", tt.input, tt.maxLen, err, tt.wantErr)
			}
		})
	}
}

// TestValidateAddress tests address validation.
func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"empty address fails", "", true},
		{"valid address passes", "0x1234567890123456789012345678901234567890", false},
		{"too short fails", "0x12345678901234567890123456789012345678", true},
		{"too long fails", "0x123456789012345678901234567890123456789012", true},
		{"missing 0x prefix fails", "1234567890123456789012345678901234567890", true},
		{"invalid hex in address fails", "0x123456789012345678901234567890123456789g", true},
		{"uppercase hex passes", "0x123456789012345678901234567890123456789A", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddress(tt.address)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ValidateAddress(%q) error = %v, wantErr = %v", tt.address, err, tt.wantErr)
			}
		})
	}
}

// TestValidateBlockHash tests block hash validation.
func TestValidateBlockHash(t *testing.T) {
	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"valid hash passes", "abc123def4567890123456789012345678901234567890123456789012345678", false},
		{"too short fails", "abc123", true},
		{"too long fails", "abc123def45678901234567890123456789012345678901234567890123456789012", true},
		{"invalid hex fails", "abc123def456789012345678901234567890123456789012345678901234567g", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlockHash(tt.hash)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ValidateBlockHash(%q) error = %v, wantErr = %v", tt.hash, err, tt.wantErr)
			}
		})
	}
}

// TestValidateTransactionHash tests transaction hash validation.
func TestValidateTransactionHash(t *testing.T) {
	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"valid 64 char hash passes", "abc123def4567890123456789012345678901234567890123456789012345678", false},
		{"valid 66 char hash with 0x passes", "0xabc123def4567890123456789012345678901234567890123456789012345678", false},
		{"wrong length fails", "abc123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransactionHash(tt.hash)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ValidateTransactionHash(%q) error = %v, wantErr = %v", tt.hash, err, tt.wantErr)
			}
		})
	}
}

// TestValidateGasLimit tests gas limit validation.
func TestValidateGasLimit(t *testing.T) {
	tests := []struct {
		name     string
		gasLimit uint64
		wantErr  bool
	}{
		{"zero is allowed", 0, false},
		{"valid limit passes", 100000, false},
		{"max allowed passes", 1000000, false},
		{"exceeds max fails", 1000001, true},
		{"much larger fails", 5000000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGasLimit(tt.gasLimit)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ValidateGasLimit(%d) error = %v, wantErr = %v", tt.gasLimit, err, tt.wantErr)
			}
		})
	}
}

// TestParseAndValidateHex tests hex parsing and validation.
func TestParseAndValidateHex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLen    int
		wantBytes string
		wantErr   bool
	}{
		{"empty returns nil", "", 100, "", false},
		{"valid hex parses", "0xabcdef", 100, "\xab\xcd\xef", false},
		{"valid hex without prefix parses", "abcdef", 100, "\xab\xcd\xef", false},
		{"invalid hex fails", "0xxyz", 100, "", true},
		{"exceeds max length fails", "0x" + string(make([]byte, 101)), 100, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, err := ParseAndValidateHex(tt.input, tt.maxLen)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ParseAndValidateHex(%q, %d) error = %v, wantErr = %v", tt.input, tt.maxLen, err, tt.wantErr)
			}
			if !tt.wantErr && tt.wantBytes != string(bytes) {
				t.Errorf("ParseAndValidateHex(%q, %d) = %v, want %v", tt.input, tt.maxLen, string(bytes), tt.wantBytes)
			}
		})
	}
}

// TestNormalizeHex tests hex string normalization.
func TestNormalizeHex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"0xabc", "0xabc"},
		{"abc", "0xabc"},
		{"0XABC", "0xABC"}, // Note: converts to lowercase
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeHex(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeHex(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsValidHex tests the hex character validation helper.
func TestIsValidHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"0123456789abcdef", true},
		{"0123456789ABCDEF", true},
		{"0xabc123", false}, // contains 0x prefix
		{"abcxyz", false},
		{"deadbeef", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHex(tt.input)
			if got != tt.want {
				t.Errorf("isValidHex(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
