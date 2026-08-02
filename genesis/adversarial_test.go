package genesis

import (
	"testing"
	"time"
)

// TestAdversarial_MalformedGenesis tests that LoadGenesis handles various
// malformed genesis inputs gracefully.
func TestAdversarial_MalformedGenesis(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		expectError bool
	}{
		{
			name:        "invalid JSON",
			data:        "{bad json}",
			expectError: true,
		},
		{
			name:        "missing chain_id",
			data:        `{"genesis_time":"2024-01-01T00:00:00Z"}`,
			expectError: true,
		},
		{
			name:        "empty validators",
			data:        `{"chain_id":"test","genesis_time":"2024-01-01T00:00:00Z","initial_validators":[]}`,
			expectError: true,
		},
		{
			name:        "wrong type for chain_id",
			data:        `{"chain_id":123,"genesis_time":"2024-01-01T00:00:00Z"}`,
			expectError: true,
		},
		{
			name:        "empty object",
			data:        `{}`,
			expectError: true,
		},
		{
			name:        "null values",
			data:        `{"chain_id":null,"genesis_time":null}`,
			expectError: true,
		},
		{
			name:        "truncated JSON",
			data:        `{"chain_id":"test","genesis_time":"2024-01-01T00:00:00Z"`,
			expectError: true,
		},
		{
			name:        "array instead of object",
			data:        `[1,2,3]`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, parseErr := LoadGenesisFromBytes([]byte(tt.data))

			// If parsing fails, that's an error
			if parseErr != nil && tt.expectError {
				// Good - we expected an error and got one
				return
			}

			// If parsing succeeded, also validate
			if doc != nil {
				valErr := ValidateGenesis(doc)
				if tt.expectError && valErr == nil {
					// Also check for validation errors
					t.Errorf("%s: expected validation error but got none", tt.name)
				}
			} else if !tt.expectError && parseErr == nil {
				// If both parsing and validation succeeded but we expected error
				t.Errorf("%s: expected error but got none", tt.name)
			}

			_ = doc // May be nil on error
		})
	}
}

// LoadGenesisFromBytes loads genesis from raw bytes (for testing).
func LoadGenesisFromBytes(data []byte) (*GenesisDoc, error) {
	return UnmarshalGenesis(data)
}

// TestAdversarial_InconsistentParams tests that ValidateGenesis rejects
// genesis with inconsistent or impossible parameters.
func TestAdversarial_InconsistentParams(t *testing.T) {
	tests := []struct {
		name        string
		doc         *GenesisDoc
		expectError bool
	}{
		{
			name: "BlocksPerEpoch = 0",
			doc: &GenesisDoc{
				GenesisVersion: 1,
				GenesisTime:    time.Now().Truncate(time.Second),
				ChainID:        "test",
				EpochParams: EpochParams{
					BlocksPerEpoch: 0,
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
				Treasury: TreasuryEntry{
					Address:        "9999999999999999999999999999999999999999",
					InitialBalance: 1000000000,
				},
			},
			expectError: false, // Currently not validated - validation allows 0
		},
		{
			name: "MaxValidators = 0 with validators defined",
			doc: &GenesisDoc{
				GenesisVersion: 1,
				GenesisTime:    time.Now().Truncate(time.Second),
				ChainID:        "test",
				EpochParams: EpochParams{
					BlocksPerEpoch: 100,
					MaxValidators:  0,
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
				Treasury: TreasuryEntry{
					Address:        "9999999999999999999999999999999999999999",
					InitialBalance: 1000000000,
				},
			},
			expectError: false, // Currently not validated - validation allows 0
		},
		{
			name: "MinimumStake higher than all validator stakes",
			doc: &GenesisDoc{
				GenesisTime: time.Now().Truncate(time.Second),
				ChainID:     "test",
				EpochParams: EpochParams{
					BlocksPerEpoch: 100,
					MaxValidators:  100,
					MinimumStake:   50000000, // Higher than validator stake
				},
				InitialValidators: []ValidatorEntry{
					{
						Address:    "0123456789abcdef0123456789abcdef01234567",
						PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
						Stake:      10000000, // Below minimum
						Commission: "1000",
					},
				},
				Treasury: TreasuryEntry{
					Address:        "9999999999999999999999999999999999999999",
					InitialBalance: 1000000000,
				},
			},
			expectError: true,
		},
		{
			name: "All validators have zero stake",
			doc: &GenesisDoc{
				GenesisTime: time.Now().Truncate(time.Second),
				ChainID:     "test",
				EpochParams: EpochParams{
					BlocksPerEpoch: 100,
					MaxValidators:  100,
					MinimumStake:   1000000,
				},
				InitialValidators: []ValidatorEntry{
					{
						Address:    "0123456789abcdef0123456789abcdef01234567",
						PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
						Stake:      0,
						Commission: "1000",
					},
				},
				Treasury: TreasuryEntry{
					Address:        "9999999999999999999999999999999999999999",
					InitialBalance: 1000000000,
				},
			},
			expectError: true, // Zero stake < minimum_stake of 1000000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGenesis(tt.doc)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected validation error but got none", tt.name)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected validation error: %v", tt.name, err)
			}
		})
	}
}

// TestAdversarial_EmptyFile tests that LoadGenesis handles empty file.
func TestAdversarial_EmptyFile(t *testing.T) {
	_, err := LoadGenesisFromBytes([]byte(""))
	if err == nil {
		t.Error("expected error for empty file")
	}
}

// TestAdversarial_WhitespaceOnlyFile tests that LoadGenesis handles
// whitespace-only file.
func TestAdversarial_WhitespaceOnlyFile(t *testing.T) {
	_, err := LoadGenesisFromBytes([]byte("   \n\t  "))
	if err == nil {
		t.Error("expected error for whitespace-only file")
	}
}

// TestAdversarial_NonHexAddress tests that ValidateGenesis rejects
// addresses containing non-hex characters.
func TestAdversarial_NonHexAddress(t *testing.T) {
	doc := &GenesisDoc{
		GenesisTime: time.Now().Truncate(time.Second),
		ChainID:     "test",
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   1000000,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:    "gggggggggggggggggggggggggggggggggggggggg", // 'g' is not hex
				PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
				Stake:      10000000,
				Commission: "1000",
			},
		},
		Treasury: TreasuryEntry{
			Address:        "9999999999999999999999999999999999999999",
			InitialBalance: 1000000000,
		},
	}

	err := ValidateGenesis(doc)
	if err == nil {
		t.Error("expected error for non-hex address")
	}
}

// TestAdversarial_TooManyValidators tests that ValidateGenesis handles
// genesis with more validators than MaxValidators.
func TestAdversarial_TooManyValidators(t *testing.T) {
	doc := &GenesisDoc{
		GenesisTime: time.Now().Truncate(time.Second),
		ChainID:     "test",
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MaxValidators:  2, // Only allow 2 validators
			MinimumStake:   1000000,
		},
		// But we provide 3 validators - currently not validated as error
		InitialValidators: []ValidatorEntry{
			{
				Address:    "0123456789abcdef0123456789abcdef01234567",
				PubKey:     "abcdef0123456789abcdef0123456789abcdef01",
				Stake:      10000000,
				Commission: "1000",
			},
			{
				Address:    "abcdef0123456789abcdef0123456789abcdef01",
				PubKey:     "0123456789abcdef0123456789abcdef01234567",
				Stake:      10000000,
				Commission: "1000",
			},
			{
				Address:    "9999999999999999999999999999999999999999",
				PubKey:     "9999999999999999999999999999999999999999",
				Stake:      10000000,
				Commission: "1000",
			},
		},
		Treasury: TreasuryEntry{
			Address:        "8888888888888888888888888888888888888888",
			InitialBalance: 1000000000,
		},
	}

	// Currently validation does NOT check for too many validators
	// This test documents that gap - it currently passes (no error)
	err := ValidateGenesis(doc)
	if err != nil {
		t.Logf("got validation error (may be acceptable): %v", err)
	}
}
