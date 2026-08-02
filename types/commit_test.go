package types

import (
	"bytes"
	"testing"
)

func TestCommitProofEncodeDecode_RoundTrip(t *testing.T) {
	c := CommitProof{
		Height:    100,
		BlockHash: Hash{1, 2, 3, 4},
		Precommits: []Vote{
			{
				VoteType:  VotePrecommit,
				Height:    100,
				Round:     5,
				BlockHash: Hash{1, 2, 3, 4},
				Validator: Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
				Signature: []byte{1, 2, 3},
			},
			{
				VoteType:  VotePrecommit,
				Height:    100,
				Round:     5,
				BlockHash: Hash{1, 2, 3, 4},
				Validator: Address{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
				Signature: []byte{4, 5, 6},
			},
		},
		TotalPower:  100,
		SignedPower: 67,
		SetHash:     Hash{9, 9, 9},
	}

	var buf bytes.Buffer
	if err := c.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	c2 := CommitProof{}
	if err := c2.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if c.Height != c2.Height {
		t.Errorf("Height mismatch: got %d, want %d", c2.Height, c.Height)
	}
	if c.BlockHash != c2.BlockHash {
		t.Errorf("BlockHash mismatch")
	}
	if len(c.Precommits) != len(c2.Precommits) {
		t.Errorf("Precommits length mismatch: got %d, want %d", len(c2.Precommits), len(c.Precommits))
	}
	if c.TotalPower != c2.TotalPower {
		t.Errorf("TotalPower mismatch: got %d, want %d", c2.TotalPower, c.TotalPower)
	}
	if c.SignedPower != c2.SignedPower {
		t.Errorf("SignedPower mismatch: got %d, want %d", c2.SignedPower, c.SignedPower)
	}
	if c.SetHash != c2.SetHash {
		t.Errorf("SetHash mismatch")
	}
}

func TestCommitProof_HasTwoThirdsMajority(t *testing.T) {
	tests := []struct {
		name       string
		total      uint64
		signed     uint64
		wantResult bool
	}{
		{"Total=100, Signed=67 -> true", 100, 67, true},
		{"Total=100, Signed=66 -> false (66*3=198 < 200)", 100, 66, false},
		{"Total=100, Signed=100 -> true", 100, 100, true},
		{"Total=3, Signed=2 -> true (2*3=6 >= 3*2=6)", 3, 2, true},
		{"Total=10, Signed=6 -> false (6*3=18 < 10*2=20)", 10, 6, false},
		{"Total=0, Signed=0 -> false", 0, 0, false},
		{"Total=10, Signed=7 -> true (7*3=21 >= 10*2=20)", 10, 7, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CommitProof{
				TotalPower:  tt.total,
				SignedPower: tt.signed,
			}
			got := c.HasTwoThirdsMajority()
			if got != tt.wantResult {
				t.Errorf("HasTwoThirdsMajority() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestCommitProofValidate_Valid(t *testing.T) {
	c := CommitProof{
		Height:      100,
		BlockHash:   Hash{1},
		Precommits:  []Vote{{Signature: []byte{1}}},
		TotalPower:  100,
		SignedPower: 67,
		SetHash:     Hash{1},
	}

	if err := c.Validate(); err != nil {
		t.Errorf("Valid commit proof should pass validation: %v", err)
	}
}

func TestCommitProofValidate_EmptyPrecommits(t *testing.T) {
	c := CommitProof{
		Height:      100,
		BlockHash:   Hash{1},
		Precommits:  []Vote{},
		TotalPower:  100,
		SignedPower: 67,
		SetHash:     Hash{1},
	}

	if err := c.Validate(); err == nil {
		t.Error("Empty precommits should fail validation")
	}
}

func TestCommitProofValidate_InsufficientPower(t *testing.T) {
	c := CommitProof{
		Height:      100,
		BlockHash:   Hash{1},
		Precommits:  []Vote{{Signature: []byte{1}}},
		TotalPower:  100,
		SignedPower: 50, // only 50%, need 66%
		SetHash:     Hash{1},
	}

	if err := c.Validate(); err == nil {
		t.Error("Insufficient power should fail validation")
	}
}

func TestCommitProofValidate_SignedPowerExceedsTotal(t *testing.T) {
	c := CommitProof{
		Height:      100,
		BlockHash:   Hash{1},
		Precommits:  []Vote{{Signature: []byte{1}}},
		TotalPower:  100,
		SignedPower: 150, // exceeds total
		SetHash:     Hash{1},
	}

	if err := c.Validate(); err == nil {
		t.Error("Signed power exceeding total should fail validation")
	}
}
