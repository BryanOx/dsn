package types

import (
	"testing"
)

func TestNewGenesisBlock(t *testing.T) {
	proposer := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	block := NewGenesisBlock(proposer)

	if block.Header.Height != 0 {
		t.Errorf("Height = %d, want 0", block.Header.Height)
	}
	if block.Header.Version != 1 {
		t.Errorf("Version = %d, want 1", block.Header.Version)
	}
	if block.Header.PreviousHash.IsZero() != true {
		t.Error("PreviousHash should be zero for genesis")
	}
	if block.Header.StateRoot.IsZero() != true {
		t.Error("StateRoot should be zero for genesis")
	}
	if len(block.Transactions) != 0 {
		t.Errorf("Transactions length = %d, want 0", len(block.Transactions))
	}
}

func TestGenesisBlockHeight(t *testing.T) {
	proposer := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	block := NewGenesisBlock(proposer)

	if block.Header.Height != 0 {
		t.Errorf("Genesis block height = %d, want 0", block.Header.Height)
	}
}

func TestGenesisBlockPreviousHash(t *testing.T) {
	proposer := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	block := NewGenesisBlock(proposer)

	if !block.Header.PreviousHash.IsZero() {
		t.Error("Genesis block previous hash should be zero hash")
	}
}

func TestBlockHeaderHash_Deterministic(t *testing.T) {
	header := &BlockHeader{
		Version:       1,
		Height:        42,
		PreviousHash:  Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		StateRoot:     Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		TxRoot:        Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		ReceiptRoot:   Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		ValidatorRoot: Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		Timestamp:     1700000000,
		Proposer:      Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}),
	}

	hasher := SHA256Hasher{}

	// Compute twice - should be deterministic
	hash1, err := header.HeaderHash(hasher)
	if err != nil {
		t.Fatalf("HeaderHash failed: %v", err)
	}

	hash2, err := header.HeaderHash(hasher)
	if err != nil {
		t.Fatalf("HeaderHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("HeaderHash should be deterministic")
	}
}

func TestBlockHeaderHash_DifferentHeights(t *testing.T) {
	proposer := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	hasher := SHA256Hasher{}

	header1 := &BlockHeader{
		Version:       1,
		Height:        1,
		PreviousHash:  Hash{},
		StateRoot:     Hash{},
		TxRoot:        Hash{},
		ReceiptRoot:   Hash{},
		ValidatorRoot: Hash{},
		Timestamp:     1700000000,
		Proposer:      proposer,
	}

	header2 := &BlockHeader{
		Version:       1,
		Height:        2,
		PreviousHash:  Hash{},
		StateRoot:     Hash{},
		TxRoot:        Hash{},
		ReceiptRoot:   Hash{},
		ValidatorRoot: Hash{},
		Timestamp:     1700000000,
		Proposer:      proposer,
	}

	hash1, _ := header1.HeaderHash(hasher)
	hash2, _ := header2.HeaderHash(hasher)

	if hash1 == hash2 {
		t.Error("Different heights should produce different hashes")
	}
}

func TestFeeSummary_Split70_20_10(t *testing.T) {
	summary := NewFeeSummary(1000)

	if summary.ValidatorShare != 700 {
		t.Errorf("ValidatorShare = %d, want 700", summary.ValidatorShare)
	}
	if summary.BurnShare != 200 {
		t.Errorf("BurnShare = %d, want 200", summary.BurnShare)
	}
	if summary.TreasuryShare != 100 {
		t.Errorf("TreasuryShare = %d, want 100", summary.TreasuryShare)
	}
	if summary.TotalFees != 1000 {
		t.Errorf("TotalFees = %d, want 1000", summary.TotalFees)
	}
}

func TestFeeSummary_SplitRounding(t *testing.T) {
	// 3 * 70 / 100 = 210 / 100 = 2 (integer division)
	// 3 * 20 / 100 = 60 / 100 = 0
	// 3 * 10 / 100 = 30 / 100 = 0
	summary := NewFeeSummary(3)
	if summary.ValidatorShare != 2 {
		t.Errorf("ValidatorShare = %d, want 2", summary.ValidatorShare)
	}
	if summary.BurnShare != 0 {
		t.Errorf("BurnShare = %d, want 0", summary.BurnShare)
	}
	if summary.TreasuryShare != 0 {
		t.Errorf("TreasuryShare = %d, want 0", summary.TreasuryShare)
	}
}
