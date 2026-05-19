package types

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestProposalEncodeDecode_RoundTrip(t *testing.T) {
	p := Proposal{
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Proposer:  Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
		Signature: []byte{1, 2, 3, 4, 5},
	}

	var buf bytes.Buffer
	if err := p.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	p2 := Proposal{}
	if err := p2.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if p.Height != p2.Height {
		t.Errorf("Height mismatch: got %d, want %d", p2.Height, p.Height)
	}
	if p.Round != p2.Round {
		t.Errorf("Round mismatch: got %d, want %d", p2.Round, p.Round)
	}
	if p.BlockHash != p2.BlockHash {
		t.Errorf("BlockHash mismatch")
	}
	if p.Proposer != p2.Proposer {
		t.Errorf("Proposer mismatch")
	}
	if !bytes.Equal(p.Signature, p2.Signature) {
		t.Errorf("Signature mismatch")
	}
}

func TestProposalSignVerify_Valid(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	p := Proposal{
		Height:    42,
		Round:     3,
		BlockHash: Hash{9, 8, 7, 6, 5, 4, 3, 2, 1},
		Proposer:  Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}

	if err := p.Sign(privKey); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	var pk [32]byte
	copy(pk[:], pubKey)
	if !p.Verify(pk) {
		t.Error("Verify failed for valid signature")
	}
}

func TestProposalSignVerify_WrongKey(t *testing.T) {
	_, privKey1, _ := ed25519.GenerateKey(nil)
	_, privKey2, _ := ed25519.GenerateKey(nil)

	p := Proposal{
		Height:    1,
		Round:     0,
		BlockHash: Hash{0},
		Proposer:  Address{1},
	}

	if err := p.Sign(privKey1); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify with wrong key
	var pk2 [32]byte
	copy(pk2[:], privKey2.Public().(ed25519.PublicKey))
	if p.Verify(pk2) {
		t.Error("Verify should fail with wrong key")
	}
}

func TestProposalHash_Deterministic(t *testing.T) {
	p := Proposal{
		Height:    100,
		Round:     5,
		BlockHash: Hash{1, 2, 3, 4},
		Proposer:  Address{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200},
	}

	hash1, err := p.ProposalHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("ProposalHash failed: %v", err)
	}

	hash2, err := p.ProposalHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("ProposalHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("ProposalHash should be deterministic")
	}
}

func TestProposalValidate_Valid(t *testing.T) {
	p := Proposal{
		Height:    1,
		Round:     0,
		BlockHash: Hash{1},
		Proposer:  Address{1},
		Signature: make([]byte, 64),
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Valid proposal should pass validation: %v", err)
	}
}

func TestProposalValidate_EmptySignature(t *testing.T) {
	p := Proposal{
		Height:    1,
		Round:     0,
		BlockHash: Hash{1},
		Proposer:  Address{1},
		Signature: []byte{},
	}

	if err := p.Validate(); err == nil {
		t.Error("Empty signature should fail validation")
	}
}