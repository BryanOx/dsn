package types

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"testing"
)

func TestVoteEncodeDecode_RoundTrip(t *testing.T) {
	v := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Validator: Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
		Signature: []byte{1, 2, 3, 4, 5},
	}

	var buf bytes.Buffer
	if err := v.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	v2 := Vote{}
	if err := v2.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if v.VoteType != v2.VoteType {
		t.Errorf("VoteType mismatch: got %v, want %v", v2.VoteType, v.VoteType)
	}
	if v.Height != v2.Height {
		t.Errorf("Height mismatch: got %d, want %d", v2.Height, v.Height)
	}
	if v.Round != v2.Round {
		t.Errorf("Round mismatch: got %d, want %d", v2.Round, v.Round)
	}
	if v.BlockHash != v2.BlockHash {
		t.Errorf("BlockHash mismatch")
	}
	if v.Validator != v2.Validator {
		t.Errorf("Validator mismatch")
	}
	if !bytes.Equal(v.Signature, v2.Signature) {
		t.Errorf("Signature mismatch")
	}
}

func TestVoteSignVerify_Valid(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	v := Vote{
		VoteType:  VotePrecommit,
		Height:    42,
		Round:     3,
		BlockHash: Hash{9, 8, 7, 6, 5, 4, 3, 2, 1},
		Validator: Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}

	if err := v.Sign(privKey); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	var pk [32]byte
	copy(pk[:], pubKey)
	if !v.Verify(pk) {
		t.Error("Verify failed for valid signature")
	}
}

func TestVoteSignVerify_WrongKey(t *testing.T) {
	_, privKey1, _ := ed25519.GenerateKey(nil)
	_, privKey2, _ := ed25519.GenerateKey(nil)

	v := Vote{
		VoteType:  VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: Hash{0},
		Validator: Address{1},
	}

	if err := v.Sign(privKey1); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify with wrong key
	var pk2 [32]byte
	copy(pk2[:], privKey2.Public().(ed25519.PublicKey))
	if v.Verify(pk2) {
		t.Error("Verify should fail with wrong key")
	}
}

func TestVoteHash_Deterministic(t *testing.T) {
	v := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Validator: Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
	}

	hash1, err := v.VoteHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("VoteHash failed: %v", err)
	}

	hash2, err := v.VoteHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("VoteHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("VoteHash should be deterministic")
	}
}

func TestVoteHash_Different(t *testing.T) {
	v1 := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Validator: Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
	}

	v2 := Vote{
		VoteType:  VotePrecommit, // different type
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Validator: Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
	}

	hash1, _ := v1.VoteHash(SHA256Hasher{})
	hash2, _ := v2.VoteHash(SHA256Hasher{})

	if hash1 == hash2 {
		t.Error("Different votes should produce different hashes")
	}
}

func TestVoteValidate_Valid(t *testing.T) {
	v := Vote{
		VoteType:  VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64), // valid Ed25519 signature length
	}

	if err := v.Validate(); err != nil {
		t.Errorf("Valid vote should pass validation: %v", err)
	}
}

func TestVoteValidate_InvalidType(t *testing.T) {
	v := Vote{
		VoteType:  VoteType(255), // invalid type
		Height:    1,
		Round:     0,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64),
	}

	if err := v.Validate(); err == nil {
		t.Error("Invalid vote type should fail validation")
	}
}

func TestVoteValidate_EmptySignature(t *testing.T) {
	v := Vote{
		VoteType:  VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: []byte{},
	}

	if err := v.Validate(); err == nil {
		t.Error("Empty signature should fail validation")
	}
}

func TestVoteDecode_Truncated(t *testing.T) {
	// Write incomplete vote data
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint8(VotePrevote))
	binary.Write(&buf, binary.BigEndian, uint64(1)) // height
	binary.Write(&buf, binary.BigEndian, uint32(0)) // round
	// missing rest of data

	v := Vote{}
	if err := v.Decode(bytes.NewReader(buf.Bytes())); err == nil {
		t.Error("Truncated data should return error")
	}
}
