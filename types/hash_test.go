package types

import (
	"testing"
)

func TestHashDeterminism(t *testing.T) {
	input := []byte("test input for determinism")

	h1, err := SHA256Hasher{}.Hash(input)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	h2, err := SHA256Hasher{}.Hash(input)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	if h1 != h2 {
		t.Error("Same input should produce same hash")
	}
}

func TestHashDifferentInputs(t *testing.T) {
	h1, _ := SHA256Hasher{}.Hash([]byte("input1"))
	h2, _ := SHA256Hasher{}.Hash([]byte("input2"))

	if h1 == h2 {
		t.Error("Different inputs should produce different hashes")
	}
}

func TestHashEmptyInput(t *testing.T) {
	// SHA256 of empty input is known: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	h, err := SHA256Hasher{}.Hash([]byte{})
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	// Verify length is 32
	if len(h) != 32 {
		t.Errorf("Hash length = %d, want 32", len(h))
	}
}

func TestHashLength(t *testing.T) {
	input := []byte("any input")
	h, _ := SHA256Hasher{}.Hash(input)

	if len(h) != 32 {
		t.Errorf("Hash length = %d, want 32", len(h))
	}
}

func TestHashInterface(t *testing.T) {
	var hasher Hasher = SHA256Hasher{}
	if hasher == nil {
		t.Error("Hasher should not be nil")
	}
}
