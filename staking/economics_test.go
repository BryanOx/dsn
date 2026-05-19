package staking

import (
	"testing"
)

func TestEpochsPerYear(t *testing.T) {
	// Test case 1: blockTimeSec=1, blocksPerEpoch=100
	result := EpochsPerYear(1, 100)
	expected := uint64(315360) // 365*24*60*60 / (1*100)
	if result != expected {
		t.Errorf("EpochsPerYear(1, 100) = %d, want %d", result, expected)
	}

	// Test case 2: blockTimeSec=2, blocksPerEpoch=100
	result = EpochsPerYear(2, 100)
	expected = uint64(157680) // 365*24*60*60 / (2*100)
	if result != expected {
		t.Errorf("EpochsPerYear(2, 100) = %d, want %d", result, expected)
	}
}

func TestPerEpochIssuance(t *testing.T) {
	// Test case 1: 1M total supply - should be below precision threshold
	result := PerEpochIssuance(1_000_000, 1, 100)
	// With 1M supply, 5% annual = 50k/year
	// epochsPerYear = 315360
	// perEpoch = 500000 / 3153600000 = 0 (truncates to 0)
	if result != 0 {
		t.Errorf("PerEpochIssuance(1M, 1, 100) = %d, want 0 (below precision)", result)
	}

	// Test case 2: 100B total supply - should be > 0
	result = PerEpochIssuance(100_000_000_000, 1, 100)
	if result == 0 {
		t.Fatal("PerEpochIssuance(100B, 1, 100) should be > 0")
	}

	// Verify annual rate is approximately 5%
	// annualRate = result * epochsPerYear
	// Should be approximately totalSupply * 5%
	epochsPerYear := EpochsPerYear(1, 100)
	annualIssuance := result * epochsPerYear
	actualRate := float64(annualIssuance) / float64(100_000_000_000)

	// Should be close to 5% (allow some deviation due to integer math)
	if actualRate < 0.04 || actualRate > 0.06 {
		t.Errorf("annual rate = %f, want ~0.05 (5%%), issuance = %d", actualRate, result)
	}
}

func TestIssueEpochTokens(t *testing.T) {
	s := newTestStakingState()

	// Set initial total supply: 1T
	if err := writeUint64(s, KeyTotalSupply, 1_000_000_000_000); err != nil {
		t.Fatalf("writeUint64 failed: %v", err)
	}

	// Issue epoch tokens
	issuance, err := IssueEpochTokens(s, 1, 1, 100)
	if err != nil {
		t.Fatalf("IssueEpochTokens failed: %v", err)
	}
	if issuance == 0 {
		t.Fatal("issuance should be > 0")
	}

	// Verify total supply increased
	totalSupply := readUint64(s, KeyTotalSupply)
	expectedTotal := 1_000_000_000_000 + issuance
	if totalSupply != expectedTotal {
		t.Errorf("total supply = %d, want %d", totalSupply, expectedTotal)
	}

	// Verify treasury supply (10% of issuance)
	treasurySupply := readUint64(s, KeyTreasurySupply)
	expectedTreasury := issuance * 10 / 100
	if treasurySupply != expectedTreasury {
		t.Errorf("treasury supply = %d, want %d", treasurySupply, expectedTreasury)
	}

	// Verify issued supply
	issuedSupply := readUint64(s, KeyIssuedSupply)
	if issuedSupply != issuance {
		t.Errorf("issued supply = %d, want %d", issuedSupply, issuance)
	}

	// Verify treasury account exists and has balance >= treasury supply
	treasuryAcc, err := s.GetAccount(TreasuryAddress)
	if err != nil {
		t.Fatalf("GetAccount(TreasuryAddress) failed: %v", err)
	}
	if treasuryAcc == nil {
		t.Fatal("treasury account should exist")
	}
	balance := stakeToUint64(treasuryAcc.Balance)
	if balance < treasurySupply {
		t.Errorf("treasury account balance = %d, want >= %d", balance, treasurySupply)
	}

	// Verify epoch validator pool (90% of issuance)
	validatorPool := readUint64(s, KeyEpochValidatorPool+"1")
	expectedValidatorPool := issuance * 90 / 100
	if validatorPool != expectedValidatorPool {
		t.Errorf("validator pool = %d, want %d", validatorPool, expectedValidatorPool)
	}
}

func TestIssueEpochTokens_ZeroSupply(t *testing.T) {
	s := newTestStakingState()

	// Ensure total supply is 0
	if err := writeUint64(s, KeyTotalSupply, 0); err != nil {
		t.Fatalf("writeUint64 failed: %v", err)
	}

	// Issue epoch tokens with zero supply
	issuance, err := IssueEpochTokens(s, 1, 1, 100)
	if err != nil {
		t.Fatalf("IssueEpochTokens failed: %v", err)
	}
	if issuance != 0 {
		t.Errorf("issuance = %d, want 0", issuance)
	}

	// Total supply should remain 0
	totalSupply := readUint64(s, KeyTotalSupply)
	if totalSupply != 0 {
		t.Errorf("total supply = %d, want 0", totalSupply)
	}
}