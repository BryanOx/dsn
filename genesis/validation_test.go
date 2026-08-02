package genesis

import (
	"strings"
	"testing"
	"time"
)

// validGenesis returns a genesis document that passes all validation checks.
func validGenesis() *GenesisDoc {
	return &GenesisDoc{
		GenesisVersion: 1,
		GenesisTime:    time.Now().Truncate(time.Second),
		ChainID:        "test-valid-chain",
		InitialHeight:  1,
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
			Enabled:    false,
			AnnualRate: "0",
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:      "0123456789abcdef0123456789abcdef01234567",
				PubKey:       "abcdef0123456789abcdef0123456789abcdef01",
				ConsensusKey: "abcdef0123456789abcdef0123456789abcdef01",
				Stake:        10000000,
				Commission:   "1000",
			},
			{
				Address:      "fedcba9876543210fedcba9876543210fedcba98",
				PubKey:       "0123456789abcdef0123456789abcdef01234567",
				ConsensusKey: "0123456789abcdef0123456789abcdef01234567",
				Stake:        20000000,
				Commission:   "500",
			},
		},
		InitialBalances: []BalanceEntry{
			{
				Address: "0123456789abcdef0123456789abcdef01234567",
				Amount:  100000000,
			},
		},
		Treasury: TreasuryEntry{
			Address:        "9999999999999999999999999999999999999999",
			InitialBalance: 1000000000,
		},
	}
}

// TestValidateGenesis_DuplicateAddress tests that ValidateGenesis rejects
// genesis with duplicate validator addresses.
func TestValidateGenesis_DuplicateAddress(t *testing.T) {
	doc := validGenesis()

	// Add a duplicate validator with the same address
	doc.InitialValidators = append(doc.InitialValidators, ValidatorEntry{
		Address:      doc.InitialValidators[0].Address, // Same as first validator
		PubKey:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ConsensusKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Stake:        5000000,
		Commission:   "1000",
	})

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for duplicate validator address")
	}

	// Check error message contains relevant text
	if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "address") {
		t.Fatalf("expected duplicate address error, got: %v", err)
	}
}

// TestValidateGenesis_DuplicatePubkey tests that ValidateGenesis rejects
// genesis with duplicate validator pubkeys.
func TestValidateGenesis_DuplicatePubkey(t *testing.T) {
	doc := validGenesis()

	// Add a validator with the same pubkey but different address
	doc.InitialValidators = append(doc.InitialValidators, ValidatorEntry{
		Address:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", // Different address
		PubKey:       doc.InitialValidators[0].PubKey,            // Same as first validator
		ConsensusKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Stake:        5000000,
		Commission:   "1000",
	})

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for duplicate validator pubkey")
	}

	// Check error message contains relevant text
	if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "pubkey") {
		t.Fatalf("expected duplicate pubkey error, got: %v", err)
	}
}

// TestValidateGenesis_LowStake tests that ValidateGenesis rejects
// genesis with validator stake below MinimumStake.
func TestValidateGenesis_LowStake(t *testing.T) {
	doc := validGenesis()

	// Set minimum stake higher than one validator's stake
	doc.EpochParams.MinimumStake = 15000000

	// First validator has 10000000 stake, which is below minimum
	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for validator stake below minimum")
	}

	// Check error message mentions stake or minimum
	if !strings.Contains(err.Error(), "stake") && !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("expected stake below minimum error, got: %v", err)
	}
}

// TestValidateGenesis_BadAddress tests that ValidateGenesis rejects
// genesis with invalid addresses (longer than 40 hex chars).
func TestValidateGenesis_BadAddress(t *testing.T) {
	doc := validGenesis()

	// Set an invalid address (more than 40 hex chars)
	doc.InitialValidators[0].Address = "0123456789abcdef0123456789abcdef0123456789abcdef" // 48 chars

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for invalid address")
	}

	// Check error message mentions address
	if !strings.Contains(err.Error(), "address") && !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid address error, got: %v", err)
	}
}

// TestValidateGenesis_FutureTimestamp tests that ValidateGenesis rejects
// genesis with genesis_time too far in the future.
func TestValidateGenesis_FutureTimestamp(t *testing.T) {
	doc := validGenesis()

	// Set genesis time to 10 minutes in the future (beyond 5 min threshold)
	doc.GenesisTime = time.Now().Add(10 * time.Minute)

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for genesis time too far in the future")
	}

	// Check error message mentions future
	if !strings.Contains(err.Error(), "future") {
		t.Fatalf("expected future timestamp error, got: %v", err)
	}
}

// TestValidateGenesis_ZeroParams tests that ValidateGenesis handles
// zero values for numeric parameters.
func TestValidateGenesis_ZeroParams(t *testing.T) {
	tests := []struct {
		name    string
		setZero func(*GenesisDoc)
	}{
		{
			name: "zero MaxTxPerBlock",
			setZero: func(d *GenesisDoc) {
				d.ConsensusParams.MaxTxPerBlock = 0
			},
		},
		{
			name: "zero BlocksPerEpoch",
			setZero: func(d *GenesisDoc) {
				d.EpochParams.BlocksPerEpoch = 0
			},
		},
		{
			name: "zero MaxValidators",
			setZero: func(d *GenesisDoc) {
				d.EpochParams.MaxValidators = 0
			},
		},
		{
			name: "zero MinimumStake",
			setZero: func(d *GenesisDoc) {
				d.EpochParams.MinimumStake = 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validGenesis()
			tt.setZero(doc)

			// Just verify validation runs without panic
			// The actual behavior (accept/reject) depends on validation rules
			_ = ValidateGenesis(doc)
		})
	}
}

// TestValidateGenesis_ValidGenesis tests that a valid genesis passes validation.
func TestValidateGenesis_ValidGenesis(t *testing.T) {
	doc := validGenesis()

	err := ValidateGenesis(doc)
	if err != nil {
		t.Fatalf("valid genesis should pass validation: %v", err)
	}
}

// TestValidateGenesis_EmptyValidators tests that ValidateGenesis rejects
// genesis with no initial validators.
func TestValidateGenesis_EmptyValidators(t *testing.T) {
	doc := validGenesis()
	doc.InitialValidators = []ValidatorEntry{}

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for empty validators")
	}

	// Check error message mentions validators
	if !strings.Contains(err.Error(), "validator") {
		t.Fatalf("expected validators error, got: %v", err)
	}
}

// TestValidateGenesis_InvalidTreasuryAddress tests that ValidateGenesis
// rejects genesis with invalid treasury address.
func TestValidateGenesis_InvalidTreasuryAddress(t *testing.T) {
	doc := validGenesis()
	doc.Treasury.Address = "this_is_not_a_valid_hex_address_that_is_far_too_long"

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for invalid treasury address")
	}
}

// TestValidateGenesis_InvalidBalanceAddress tests that ValidateGenesis
// rejects genesis with invalid balance addresses.
func TestValidateGenesis_InvalidBalanceAddress(t *testing.T) {
	doc := validGenesis()
	doc.InitialBalances[0].Address = "invalid_address_with_non_hex_chars_!!!!"

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for invalid balance address")
	}
}

// TestValidateGenesis_CommissionRate tests that ValidateGenesis validates
// commission rates.
func TestValidateGenesis_CommissionRate(t *testing.T) {
	doc := validGenesis()

	// Set commission rate exceeding 100% (10000 basis points)
	doc.InitialValidators[0].Commission = "15000"

	err := ValidateGenesis(doc)
	if err == nil {
		t.Fatal("expected error for commission rate > 100%")
	}

	// Check error mentions commission
	if !strings.Contains(err.Error(), "commission") {
		t.Fatalf("expected commission error, got: %v", err)
	}
}
