package types

import (
	"bytes"
	"testing"
)

func TestWriteVarBytes(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"nil slice", nil, false},
		{"empty slice", []byte{}, false},
		{"small data", []byte{1, 2, 3}, false},
		{"medium data", bytes.Repeat([]byte{0xAB}, 256), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := writeVarBytes(&buf, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeVarBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Read back and verify
			result, err := readVarBytes(&buf)
			if err != nil {
				t.Errorf("readVarBytes() error = %v", err)
				return
			}

			if !bytes.Equal(result, tt.data) {
				t.Errorf("round-trip: got %v, want %v", result, tt.data)
			}
		})
	}
}

func TestReadVarBytesMaxLength(t *testing.T) {
	var buf bytes.Buffer

	// Test data that exceeds max length (can't allocate 4GB but can test boundary)
	// We'll test that the function validates the length limit
	tooLongData := make([]byte, MaxVarBytesLength+1)
	err := writeVarBytes(&buf, tooLongData)
	if err == nil {
		t.Error("should reject data exceeding max length")
	}
}

func TestReadVarBytesInvalidLength(t *testing.T) {
	// Create a buffer with a length that's too big for the available data
	badBuf := bytes.NewBuffer([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x01}) // says 4GB but has only 1 byte
	_, err := readVarBytes(badBuf)
	if err == nil {
		t.Error("should fail when length exceeds available data")
	}
}
