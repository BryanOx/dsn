package genesis

import (
	"testing"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

func TestInitGenesisState_EconomicKeys(t *testing.T) {
	db, err := bbolt.Open(t.TempDir()+"/test.db", 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	doc := &GenesisDoc{
		GenesisVersion: 1,
		ChainID:        "test-chain",
		InitialHeight:  1,
		EpochParams: EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 7,
			MaxValidators:         100,
			MinimumStake:          100000,
		},
		InflationParams: InflationParams{
			Enabled:    true,
			AnnualRate: "0.02",
		},
		InitialBalances: []BalanceEntry{
			{Address: "0x0000000000000000000000000000000000000001", Amount: 1000000},
			{Address: "0x0000000000000000000000000000000000000002", Amount: 2000000},
		},
		Treasury: TreasuryEntry{
			Address:        "0x0000000000000000000000000000000000000000",
			InitialBalance: 5000000,
		},
	}

	root, err := InitGenesisState(doc, s, db, hasher)
	if err != nil {
		t.Fatalf("InitGenesisState failed: %v", err)
	}
	if root == (types.Hash{}) {
		t.Error("expected non-zero state root")
	}

	totalSupply := staking.ReadUint64(s, staking.KeyTotalSupply)
	if totalSupply != 8000000 {
		t.Errorf("KeyTotalSupply = %d, want 8000000", totalSupply)
	}

	issuedSupply := staking.ReadUint64(s, staking.KeyIssuedSupply)
	if issuedSupply != 0 {
		t.Errorf("KeyIssuedSupply = %d, want 0", issuedSupply)
	}

	treasurySupply := staking.ReadUint64(s, staking.KeyTreasurySupply)
	if treasurySupply != 5000000 {
		t.Errorf("KeyTreasurySupply = %d, want 5000000", treasurySupply)
	}

	if got := staking.ReadUint64(s, "epoch/blocks_per_epoch"); got != 100 {
		t.Errorf("blocks_per_epoch = %d, want 100", got)
	}
	if got := staking.ReadUint64(s, "staking/unstake_cooldown"); got != 7 {
		t.Errorf("unstake_cooldown = %d, want 7", got)
	}
	if got := staking.ReadUint64(s, "staking/max_validators"); got != 100 {
		t.Errorf("max_validators = %d, want 100", got)
	}
	if got := staking.ReadUint64(s, "staking/minimum_stake"); got != 100000 {
		t.Errorf("minimum_stake = %d, want 100000", got)
	}

	if !staking.InflationEnabled(s) {
		t.Error("InflationEnabled = false, want true")
	}
	if got := staking.AnnualInflationBP(s); got != 200 {
		t.Errorf("AnnualInflationBP = %d, want 200", got)
	}
}

func TestInitGenesisState_ValidatorsInRegistry(t *testing.T) {
	db, err := bbolt.Open(t.TempDir()+"/test.db", 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	doc := &GenesisDoc{
		GenesisVersion: 1,
		ChainID:        "test-chain",
		InitialHeight:  1,
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   100000,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:    "0x0000000000000000000000000000000000000001",
				PubKey:     "0x0000000000000000000000000000000000000000000000000000000000000001",
				Stake:      1000000,
				Commission: "1000",
			},
		},
	}

	_, err = InitGenesisState(doc, s, db, hasher)
	if err != nil {
		t.Fatalf("InitGenesisState failed: %v", err)
	}

	count, err := staking.ValidatorCount(s)
	if err != nil {
		t.Fatalf("ValidatorCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("ValidatorCount = %d, want 1", count)
	}

	active, err := staking.GetActiveValidators(s)
	if err != nil {
		t.Fatalf("GetActiveValidators failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("GetActiveValidators = %d, want 1", len(active))
	}
}

func TestGenesisStartup_ValidatorsDetected(t *testing.T) {
	db, err := bbolt.Open(t.TempDir()+"/test.db", 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	doc := &GenesisDoc{
		GenesisVersion: 1,
		ChainID:        "test-chain",
		InitialHeight:  1,
		EpochParams: EpochParams{
			BlocksPerEpoch: 100,
			MinimumStake:   100000,
		},
		InitialValidators: []ValidatorEntry{
			{
				Address:    "0x0000000000000000000000000000000000000001",
				PubKey:     "0x0000000000000000000000000000000000000000000000000000000000000001",
				Stake:      1000000,
				Commission: "1000",
			},
		},
		InitialBalances: []BalanceEntry{
			{Address: "0x0000000000000000000000000000000000000002", Amount: 1000000},
		},
		Treasury: TreasuryEntry{
			Address:        "0x0000000000000000000000000000000000000000",
			InitialBalance: 5000000,
		},
	}

	_, err = InitGenesisState(doc, s, db, hasher)
	if err != nil {
		t.Fatalf("InitGenesisState failed: %v", err)
	}

	active, err := staking.GetActiveValidatorAddresses(s)
	if err != nil {
		t.Fatalf("GetActiveValidatorAddresses failed: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active validator, got %d", len(active))
	}

	totalSupply := staking.ReadUint64(s, staking.KeyTotalSupply)
	if totalSupply == 0 {
		t.Error("expected non-zero total supply")
	}
}
