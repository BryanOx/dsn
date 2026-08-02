package genesis

import (
	"testing"
	"time"

	"github.com/dsn/dsn/types"
)

// TestGenesisHash_Deterministic tests that HashGenesis() always returns
// the same hash for the same genesis document.
func TestGenesisHash_Deterministic(t *testing.T) {
	doc := &GenesisDoc{
		GenesisTime:   time.Now().Truncate(time.Second),
		ChainID:       "test-chain-1",
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock:    100,
			MaxBytesPerBlock: 1048576,
			MaxGasPerBlock:   10000000,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 21,
			MaxValidators:         100,
			MinimumStake:          1000000,
		},
		InflationParams: InflationParams{
			Enabled:      false,
			AnnualRate:   "0",
			MintPerBlock: 0,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:      "0123456789abcdef0123456789abcdef01234567",
				PubKey:       "0123456789abcdef0123456789abcdef01234567",
				ConsensusKey: "0123456789abcdef0123456789abcdef01234567",
				Stake:        10000000,
				Commission:   "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	// Hash the same document twice
	hash1, err := HashGenesis(doc)
	if err != nil {
		t.Fatalf("first hash failed: %v", err)
	}

	hash2, err := HashGenesis(doc)
	if err != nil {
		t.Fatalf("second hash failed: %v", err)
	}

	// Verify hashes are identical
	if hash1 != hash2 {
		t.Fatalf("non-deterministic hash: %v != %v", hash1, hash2)
	}
}

// TestGenesisHash_SameContentDifferentObjects tests that two genesis docs
// with identical content but created separately produce the same hash.
func TestGenesisHash_SameContentDifferentObjects(t *testing.T) {
	// Create first genesis document
	doc1 := &GenesisDoc{
		GenesisTime:   time.Unix(1234567890, 0),
		ChainID:       "test-chain-identical",
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock:    100,
			MaxBytesPerBlock: 1048576,
			MaxGasPerBlock:   10000000,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 21,
			MaxValidators:         100,
			MinimumStake:          1000000,
		},
		InflationParams: InflationParams{
			Enabled:      false,
			AnnualRate:   "0",
			MintPerBlock: 0,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:      "0123456789abcdef0123456789abcdef01234567",
				PubKey:       "abcdef0123456789abcdef0123456789abcdef01",
				ConsensusKey: "abcdef0123456789abcdef0123456789abcdef01",
				Stake:        10000000,
				Commission:   "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	// Create second genesis document with identical content
	doc2 := &GenesisDoc{
		GenesisTime:   time.Unix(1234567890, 0),
		ChainID:       "test-chain-identical",
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock:    100,
			MaxBytesPerBlock: 1048576,
			MaxGasPerBlock:   10000000,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 21,
			MaxValidators:         100,
			MinimumStake:          1000000,
		},
		InflationParams: InflationParams{
			Enabled:      false,
			AnnualRate:   "0",
			MintPerBlock: 0,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:      "0123456789abcdef0123456789abcdef01234567",
				PubKey:       "abcdef0123456789abcdef0123456789abcdef01",
				ConsensusKey: "abcdef0123456789abcdef0123456789abcdef01",
				Stake:        10000000,
				Commission:   "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	hash1, err := HashGenesis(doc1)
	if err != nil {
		t.Fatalf("hash1 failed: %v", err)
	}

	hash2, err := HashGenesis(doc2)
	if err != nil {
		t.Fatalf("hash2 failed: %v", err)
	}

	if hash1 != hash2 {
		t.Fatalf("identical content produced different hashes: %v != %v", hash1, hash2)
	}
}

// TestGenesisHash_DifferentContentProducesDifferentHash tests that different
// genesis content produces different hashes.
func TestGenesisHash_DifferentContentProducesDifferentHash(t *testing.T) {
	doc1 := &GenesisDoc{
		GenesisTime:   time.Unix(1234567890, 0),
		ChainID:       "chain-a",
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock: 100,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   1000000,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:    "0123456789abcdef0123456789abcdef01234567",
				PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
				Stake:      10000000,
				Commission: "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	doc2 := &GenesisDoc{
		GenesisTime:   time.Unix(1234567890, 0),
		ChainID:       "chain-b", // Different ChainID
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock: 100,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   1000000,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:    "0123456789abcdef0123456789abcdef01234567",
				PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
				Stake:      10000000,
				Commission: "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	hash1, err := HashGenesis(doc1)
	if err != nil {
		t.Fatalf("hash1 failed: %v", err)
	}

	hash2, err := HashGenesis(doc2)
	if err != nil {
		t.Fatalf("hash2 failed: %v", err)
	}

	// Different content should produce different hashes
	if hash1 == hash2 {
		t.Fatal("different content produced same hash")
	}
}

// TestGenesisHash_EmptyValidatorsProducesHash tests that genesis with empty
// validators still produces a valid hash.
func TestGenesisHash_EmptyValidatorsProducesHash(t *testing.T) {
	doc := &GenesisDoc{
		GenesisTime:   time.Unix(1234567890, 0),
		ChainID:       "test-chain-empty",
		InitialHeight: 1,
		ConsensusParams: ConsensusParams{
			MaxTxPerBlock: 100,
		},
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   1000000,
		},
		InitialValidators: []ValidatorEntry{},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "fedcba9876543210fedcba9876543210fedcba98",
			InitialBalance: 1000000000,
		},
	}

	hash, err := HashGenesis(doc)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}

	// Hash should not be zero
	if hash == (types.Hash{}) {
		t.Fatal("hash should not be zero")
	}
}
