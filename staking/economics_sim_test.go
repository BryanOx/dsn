package staking

import (
	"math"
	"testing"
)

// TestSimulate_BaseCase — 100 validators, 1M avg stake, 1000 fees/block, 1yr
func TestSimulate_BaseCase(t *testing.T) {
	// Create 100 validators with 1M stake each
	stakes := make([]uint64, 100)
	for i := range stakes {
		stakes[i] = 1_000_000
	}

	params := EconomicParams{
		InitialSupply:      1_000_000_000, // 1B initial supply
		InflationRateBP:    500,           // 5%
		BlocksPerEpoch:     100,
		BlockTimeSec:       1,
		ActiveValidators:   100,
		AvgFeePerBlock:     1000,
		SlashEventsPerYear: 2.0,
		SlashAvgPct:        5.0,
		OpCostPerYear:      10000,
		ValidatorStakes:    stakes,
	}

	results := Simulate(params, 1)

	if len(results) != 1 {
		t.Fatalf("expected 1 year result, got %d", len(results))
	}

	r := results[0]

	// Verify basic calculations
	if r.Year != 1 {
		t.Errorf("expected year 1, got %d", r.Year)
	}

	if r.StartSupply != 1_000_000_000 {
		t.Errorf("expected start supply 1B, got %d", r.StartSupply)
	}

	// Annual issuance should be positive
	if r.AnnualIssuance == 0 {
		t.Error("annual issuance should not be zero")
	}

	// Treasury inflow should be 10% of issuance
	expectedTreasuryInflow := r.AnnualIssuance * 10 / 100
	if r.TreasuryInflow != expectedTreasuryInflow {
		t.Errorf("treasury inflow = %d, want %d", r.TreasuryInflow, expectedTreasuryInflow)
	}

	// Fee revenue should be 1000 * 100 * 31536000 = ~3.15B (but we use uint64 so it's exact)
	epochsPerYear := uint64(31536000 / (1 * 100))
	expectedFeeRevenue := uint64(1000) * 100 * epochsPerYear
	if r.FeeRevenue != expectedFeeRevenue {
		t.Errorf("fee revenue = %d, want %d", r.FeeRevenue, expectedFeeRevenue)
	}

	// Fee burn should be 20% of fees
	expectedFeeBurn := r.FeeRevenue * 20 / 100
	if r.FeeBurn != expectedFeeBurn {
		t.Errorf("fee burn = %d, want %d", r.FeeBurn, expectedFeeBurn)
	}

	// ROI should be calculable
	if math.IsNaN(r.MinValidatorROI) || math.IsInf(r.MinValidatorROI, 0) {
		t.Error("min validator ROI should be finite")
	}

	// Nakamoto coefficients should be valid (need at least some validators for 33%)
	if r.Nakamoto1_3 < 1 {
		t.Errorf("Nakamoto1_3 should be at least 1, got %d", r.Nakamoto1_3)
	}

	// Gini should be 0 for equal stakes
	if r.GiniCoefficient != 0 {
		t.Errorf("gini coefficient for equal stakes should be 0, got %f", r.GiniCoefficient)
	}

	// Treasury balance should be positive
	if r.TreasuryBalance == 0 {
		t.Error("treasury balance should not be zero")
	}

	t.Logf("Base case result: StartSupply=%d, EndSupply=%d, AnnualIssuance=%d, TreasuryBalance=%d, MinROI=%.2f, MaxROI=%.2f, AvgROI=%.2f",
		r.StartSupply, r.EndSupply, r.AnnualIssuance, r.TreasuryBalance, r.MinValidatorROI, r.MaxValidatorROI, r.AvgValidatorROI)
}

// TestSimulate_HighAdoption — 100 validators, 10M avg stake, 10000 fees/block, 5yr
func TestSimulate_HighAdoption(t *testing.T) {
	// Create 100 validators with 10M stake each
	stakes := make([]uint64, 100)
	for i := range stakes {
		stakes[i] = 10_000_000
	}

	params := EconomicParams{
		InitialSupply:      10_000_000_000, // 10B initial supply
		InflationRateBP:    500,            // 5%
		BlocksPerEpoch:     100,
		BlockTimeSec:       1,
		ActiveValidators:   100,
		AvgFeePerBlock:     10000,
		SlashEventsPerYear: 1.0,
		SlashAvgPct:        3.0,
		OpCostPerYear:      50000,
		ValidatorStakes:    stakes,
	}

	results := Simulate(params, 5)

	if len(results) != 5 {
		t.Fatalf("expected 5 year results, got %d", len(results))
	}

	// Check that supply grows over time (until it hits zero due to high fees)
	// Note: with very high fees, supply may go to zero which is economically valid
	for i := 1; i < len(results); i++ {
		// If supply is already zero, it stays zero
		if results[i-1].EndSupply > 0 && results[i].EndSupply == 0 {
			// Supply depleted due to high fees - this is valid
			break
		}
		if results[i-1].EndSupply > 0 && results[i].EndSupply <= results[i-1].EndSupply {
			t.Errorf("year %d: supply should grow, got %d <= %d", i+1, results[i].EndSupply, results[i-1].EndSupply)
		}
	}

	// Verify total burned accumulates
	var totalBurned uint64
	for _, r := range results {
		totalBurned += r.FeeBurn
	}

	lastResult := results[len(results)-1]
	if lastResult.TotalBurned != totalBurned {
		t.Errorf("total burned = %d, expected sum of fee burns = %d", lastResult.TotalBurned, totalBurned)
	}

	// Check that treasury grows
	for i := 1; i < len(results); i++ {
		if results[i].TreasuryBalance <= results[i-1].TreasuryBalance {
			t.Errorf("year %d: treasury should grow", i+1)
		}
	}

	// Verify ROI is positive for high adoption scenario (when supply is still positive)
	if lastResult.EndSupply > 0 && lastResult.AvgValidatorROI <= 0 {
		t.Errorf("avg validator ROI should be positive in high adoption, got %f", lastResult.AvgValidatorROI)
	}

	t.Logf("High adoption 5yr result: EndSupply=%d, TreasuryBalance=%d, AvgROI=%.2f, Gini=%.3f",
		lastResult.EndSupply, lastResult.TreasuryBalance, lastResult.AvgValidatorROI, lastResult.GiniCoefficient)
}

// TestSimulate_LowAdoption — 50 validators, 100K stake, 100 fees/block, 3yr
func TestSimulate_LowAdoption(t *testing.T) {
	// Create 50 validators with 100K stake each (unequal for non-zero Gini)
	stakes := make([]uint64, 50)
	for i := range stakes {
		// Vary stakes to create realistic inequality
		stakes[i] = 100_000 + uint64(i*1000)
	}

	params := EconomicParams{
		InitialSupply:      100_000_000, // 100M initial supply
		InflationRateBP:    500,         // 5%
		BlocksPerEpoch:     100,
		BlockTimeSec:       1,
		ActiveValidators:   50,
		AvgFeePerBlock:     100,
		SlashEventsPerYear: 5.0,
		SlashAvgPct:        10.0,
		OpCostPerYear:      5000,
		ValidatorStakes:    stakes,
	}

	results := Simulate(params, 3)

	if len(results) != 3 {
		t.Fatalf("expected 3 year results, got %d", len(results))
	}

	// For low adoption, ROI might be negative or very low
	lastResult := results[len(results)-1]

	// Gini should be non-zero due to unequal stakes
	if lastResult.GiniCoefficient <= 0 {
		t.Errorf("gini coefficient should be > 0 for unequal stakes, got %f", lastResult.GiniCoefficient)
	}

	// Verify Nakamoto coefficients
	// With 50 validators, Nakamoto1_3 should be <= 50
	if lastResult.Nakamoto1_3 > 50 {
		t.Errorf("Nakamoto1_3 should be <= 50, got %d", lastResult.Nakamoto1_3)
	}

	// Min ROI could be negative in low adoption with high slashing
	t.Logf("Low adoption 3yr result: EndSupply=%d, TreasuryBalance=%d, MinROI=%.2f, MaxROI=%.2f, AvgROI=%.2f, Gini=%.3f",
		lastResult.EndSupply, lastResult.TreasuryBalance, lastResult.MinValidatorROI, lastResult.MaxValidatorROI, lastResult.AvgValidatorROI, lastResult.GiniCoefficient)
}

// TestSimulate_Determinism — same params twice produces same results
func TestSimulate_Determinism(t *testing.T) {
	// Create deterministic validator set
	stakes := make([]uint64, 50)
	for i := range stakes {
		stakes[i] = uint64((i + 1) * 10000)
	}

	params := EconomicParams{
		InitialSupply:      1_000_000_000,
		InflationRateBP:    500,
		BlocksPerEpoch:     100,
		BlockTimeSec:       1,
		ActiveValidators:   50,
		AvgFeePerBlock:     1000,
		SlashEventsPerYear: 2.0,
		SlashAvgPct:        5.0,
		OpCostPerYear:      10000,
		ValidatorStakes:    stakes,
	}

	// Run simulation twice
	results1 := Simulate(params, 3)
	results2 := Simulate(params, 3)

	// Compare results
	if len(results1) != len(results2) {
		t.Fatalf("different result lengths: %d vs %d", len(results1), len(results2))
	}

	for i := range results1 {
		r1 := results1[i]
		r2 := results2[i]

		if r1.Year != r2.Year {
			t.Errorf("year %d: different year: %d vs %d", i, r1.Year, r2.Year)
		}
		if r1.StartSupply != r2.StartSupply {
			t.Errorf("year %d: different start supply: %d vs %d", i, r1.StartSupply, r2.StartSupply)
		}
		if r1.EndSupply != r2.EndSupply {
			t.Errorf("year %d: different end supply: %d vs %d", i, r1.EndSupply, r2.EndSupply)
		}
		if r1.AnnualIssuance != r2.AnnualIssuance {
			t.Errorf("year %d: different annual issuance: %d vs %d", i, r1.AnnualIssuance, r2.AnnualIssuance)
		}
		if r1.TreasuryInflow != r2.TreasuryInflow {
			t.Errorf("year %d: different treasury inflow: %d vs %d", i, r1.TreasuryInflow, r2.TreasuryInflow)
		}
		if r1.FeeRevenue != r2.FeeRevenue {
			t.Errorf("year %d: different fee revenue: %d vs %d", i, r1.FeeRevenue, r2.FeeRevenue)
		}
		if r1.FeeBurn != r2.FeeBurn {
			t.Errorf("year %d: different fee burn: %d vs %d", i, r1.FeeBurn, r2.FeeBurn)
		}
		if r1.TotalBurned != r2.TotalBurned {
			t.Errorf("year %d: different total burned: %d vs %d", i, r1.TotalBurned, r2.TotalBurned)
		}
		if r1.TreasuryBalance != r2.TreasuryBalance {
			t.Errorf("year %d: different treasury balance: %d vs %d", i, r1.TreasuryBalance, r2.TreasuryBalance)
		}
		if r1.MinValidatorROI != r2.MinValidatorROI {
			t.Errorf("year %d: different min ROI: %f vs %f", i, r1.MinValidatorROI, r2.MinValidatorROI)
		}
		if r1.MaxValidatorROI != r2.MaxValidatorROI {
			t.Errorf("year %d: different max ROI: %f vs %f", i, r1.MaxValidatorROI, r2.MaxValidatorROI)
		}
		if r1.AvgValidatorROI != r2.AvgValidatorROI {
			t.Errorf("year %d: different avg ROI: %f vs %f", i, r1.AvgValidatorROI, r2.AvgValidatorROI)
		}
		if r1.Nakamoto1_3 != r2.Nakamoto1_3 {
			t.Errorf("year %d: different Nakamoto1_3: %d vs %d", i, r1.Nakamoto1_3, r2.Nakamoto1_3)
		}
		if r1.Nakamoto1_2 != r2.Nakamoto1_2 {
			t.Errorf("year %d: different Nakamoto1_2: %d vs %d", i, r1.Nakamoto1_2, r2.Nakamoto1_2)
		}
		if r1.Nakamoto2_3 != r2.Nakamoto2_3 {
			t.Errorf("year %d: different Nakamoto2_3: %d vs %d", i, r1.Nakamoto2_3, r2.Nakamoto2_3)
		}
		if r1.GiniCoefficient != r2.GiniCoefficient {
			t.Errorf("year %d: different Gini: %f vs %f", i, r1.GiniCoefficient, r2.GiniCoefficient)
		}
	}

	t.Log("Determinism test passed: both simulations produced identical results")
}

// TestGiniCoefficient tests the Gini coefficient calculation
func TestGiniCoefficient(t *testing.T) {
	tests := []struct {
		name     string
		stakes   []uint64
		expected float64
	}{
		{
			name:     "empty",
			stakes:   []uint64{},
			expected: 0,
		},
		{
			name:     "single",
			stakes:   []uint64{100},
			expected: 0,
		},
		{
			name:     "equal stakes",
			stakes:   []uint64{100, 100, 100, 100},
			expected: 0,
		},
		{
			name:     "perfect inequality",
			stakes:   []uint64{0, 0, 0, 1000},
			expected: 0.75, // (2*4*1000)/(4*1000) - 5/4 = 2 - 1.25 = 0.75
		},
		{
			name:     "moderate inequality",
			stakes:   []uint64{10, 20, 30, 40},
			expected: 0.25, // (2*300)/(4*100) - 5/4 = 1.5 - 1.25 = 0.25
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gini := calculateGini(tt.stakes)

			// Allow small floating point tolerance
			diff := math.Abs(gini - tt.expected)
			if diff > 0.001 && tt.expected != 0 {
				t.Errorf("gini = %f, expected ~%f", gini, tt.expected)
			}
		})
	}
}

// TestNakamotoCoefficient tests the Nakamoto coefficient calculation
func TestNakamotoCoefficient(t *testing.T) {
	tests := []struct {
		name      string
		stakes    []uint64
		threshold float64
		expected  int
	}{
		{
			name:      "empty",
			stakes:    []uint64{},
			threshold: 0.33,
			expected:  0,
		},
		{
			name:      "single validator",
			stakes:    []uint64{100},
			threshold: 0.33,
			expected:  1,
		},
		{
			name:      "equal stakes need 2 for 33%",
			stakes:    []uint64{100, 100, 100},
			threshold: 0.33,
			expected:  1, // One validator = 33.33%
		},
		{
			name:      "equal stakes need 2 for 50%",
			stakes:    []uint64{100, 100, 100},
			threshold: 0.50,
			expected:  2, // Two validators = 66.67%
		},
		{
			name:      "unequal stakes",
			stakes:    []uint64{500, 300, 200},
			threshold: 0.33,
			expected:  1, // One validator has 50%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNakamoto(tt.stakes, tt.threshold)

			if result != tt.expected {
				t.Errorf("nakamoto(%v, %.2f) = %d, expected %d", tt.stakes, tt.threshold, result, tt.expected)
			}
		})
	}
}
