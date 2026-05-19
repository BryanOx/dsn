package types

import (
	"bytes"
	"testing"
)

func TestAddressFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid 20 bytes", bytes.Repeat([]byte{0xAB}, 20), false},
		{"nil slice", nil, true},
		{"empty slice", []byte{}, true},
		{"too short", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}, true},
		{"too long", bytes.Repeat([]byte{0xAB}, 21), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := AddressFromBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressFromBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
			// If no error, verify length
			if err == nil && len(addr.Bytes()) != 20 {
				t.Errorf("Address length = %d, want 20", len(addr.Bytes()))
			}
		})
	}
}

func TestAddressStringAndParse(t *testing.T) {
	// Test known Bitcoin Base58Check vector (with mainnet version 0x00)
	// 00f30e0e00ba6a4a056962d3a9e2a1e9d5e1e9c0 -> 1MsHGLubC6Yf5Q
	testInput := []byte{0x00, 0xf3, 0x0e, 0x0e, 0x00, 0xba, 0x6a, 0x4a, 0x05, 0x69, 0x62, 0xd3, 0xa9, 0xe2, 0xa1, 0xe9, 0xd5, 0xe1, 0xe9, 0xc0}

	addr, err := AddressFromBytes(testInput)
	if err != nil {
		t.Fatalf("AddressFromBytes() failed: %v", err)
	}

	encoded := addr.String()
	if encoded == "" {
		t.Error("String() should not return empty")
	}

	// Test round-trip
	parsedAddr, err := ParseAddress(encoded)
	if err != nil {
		t.Errorf("ParseAddress() failed: %v", err)
	}

	if !bytes.Equal(addr.Bytes(), parsedAddr.Bytes()) {
		t.Errorf("round-trip: got %v, want %v", parsedAddr.Bytes(), addr.Bytes())
	}
}

func TestAddressParseInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"invalid base58 chars", "Invalid+Chars!"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAddress(tt.input)
			if err == nil {
				t.Error("ParseAddress() should fail for invalid input")
			}
		})
	}
}

func TestAddressEncodeDecode(t *testing.T) {
	// Test serialization round-trip
	original := bytes.Repeat([]byte{0xCD}, 20)
	addr, err := AddressFromBytes(original)
	if err != nil {
		t.Fatalf("AddressFromBytes() failed: %v", err)
	}

	var buf bytes.Buffer
	if err := addr.Encode(&buf); err != nil {
		t.Errorf("Encode() failed: %v", err)
	}

	var decoded Address
	if err := decoded.Decode(&buf); err != nil {
		t.Errorf("Decode() failed: %v", err)
	}

	if !bytes.Equal(decoded.Bytes(), original) {
		t.Errorf("Encode/Decode round-trip: got %v, want %v", decoded.Bytes(), original)
	}
}

func TestAddressEquality(t *testing.T) {
	addr1, _ := AddressFromBytes(bytes.Repeat([]byte{0xAB}, 20))
	addr2, _ := AddressFromBytes(bytes.Repeat([]byte{0xAB}, 20))
	addr3, _ := AddressFromBytes(bytes.Repeat([]byte{0xCD}, 20))

	if addr1 != addr2 {
		t.Error("Equal addresses should be ==")
	}
	if addr1 == addr3 {
		t.Error("Different addresses should not be ==")
	}
}