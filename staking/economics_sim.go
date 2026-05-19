package staking

import (
	"sort"
)

// EconomicParams defines all configurable parameters for the simulation
type EconomicParams struct {
	InitialSupply      uint64   // initial token supply
	InflationRateBP    uint64   // basis points, default 500 (5%)
	BlocksPerEpoch     uint64   // default 100
	BlockTimeSec       uint64   // default 1
	ActiveValidators   int      // number of validators
	AvgFeePerBlock     uint64   // average fee revenue per block
	SlashEventsPerYear float64  // expected slashing events per year
	SlashAvgPct        float64  // average slash % (weighted by offense mix)
	OpCostPerYear      uint64   // validator operational cost in tokens
	ValidatorStakes    []uint64 // individual validator stakes
}

// YearlyProjection holds the simulated state after one year
type YearlyProjection struct {
	Year              int
	StartSupply       uint64
	EndSupply         uint64
	AnnualIssuance    uint64
	TreasuryInflow    uint64
	FeeRevenue        uint64
	FeeBurn           uint64
	TotalBurned       uint64
	TreasuryBalance   uint64
	MinValidatorROI   float64
	MaxValidatorROI   float64
	AvgValidatorROI   float64
	Nakamoto1_3       int // validators needed for 33% power
	Nakamoto1_2       int // validators needed for 50% power
	Nakamoto2_3       int // validators needed for 66% power
	GiniCoefficient   float64
}

// Simulate runs the economic simulation for the given number of years
func Simulate(params EconomicParams, years int) []YearlyProjection {
	results := make([]YearlyProjection, 0, years)

	// Constants
	const secondsPerYear = 31536000

	// Calculate epochs per year
	epochsPerYear := secondsPerYear / (params.BlockTimeSec * params.BlocksPerEpoch)

	// Initialize state
	currentSupply := params.InitialSupply
	treasuryBalance := uint64(0)
	totalBurned := uint64(0)

	// Pre-calculate total stake
	totalStake := calculateTotalStake(params.ValidatorStakes)

	for year := 1; year <= years; year++ {
		// 1. Per-epoch issuance: (totalSupply * inflationRateBP) / (epochsPerYear * 10000)
		perEpochIssuance := (currentSupply * params.InflationRateBP) / (epochsPerYear * 10000)

		// 2. Annual issuance: perEpochIssuance * epochsPerYear
		annualIssuance := perEpochIssuance * epochsPerYear

		// 3. Validator pool: annualIssuance * 90 / 100
		validatorPool := annualIssuance * 90 / 100

		// 4. Treasury inflow: annualIssuance * 10 / 100
		treasuryInflow := annualIssuance * 10 / 100

		// 5. Annual fees: avgFeePerBlock * blocksPerEpoch * epochsPerYear
		annualFees := params.AvgFeePerBlock * params.BlocksPerEpoch * epochsPerYear

		// 6. Fee to validators: annualFees * 70 / 100
		feeToValidators := annualFees * 70 / 100

		// 7. Fee burn: annualFees * 20 / 100
		feeBurn := annualFees * 20 / 100

		// 8. Fee to treasury: annualFees * 10 / 100
		feeToTreasury := annualFees * 10 / 100

		// 9. Slashing loss: totalSupply * slashEventsPerYear * slashAvgPct / 100
		// Need to handle float multiplication properly
		slashLossFloat := float64(currentSupply) * params.SlashEventsPerYear * params.SlashAvgPct / 100
		slashingLoss := uint64(slashLossFloat)

		// Update treasury (inflow from issuance + fees to treasury)
		treasuryBalance += treasuryInflow + feeToTreasury

		// Update burned tokens (fee burn)
		totalBurned += feeBurn

		// Update supply: start + issuance - fee burn - slashing loss
		// Use checked arithmetic to prevent overflow
		var endSupply uint64
		supplyAfterIssuance := currentSupply + annualIssuance
		if supplyAfterIssuance < currentSupply {
			supplyAfterIssuance = ^uint64(0) // overflow - max value
		}

		// Subtract fee burn
		if supplyAfterIssuance > feeBurn {
			supplyAfterBurn := supplyAfterIssuance - feeBurn
			// Subtract slashing loss
			if supplyAfterBurn > slashingLoss {
				endSupply = supplyAfterBurn - slashingLoss
			} else {
				endSupply = 0
			}
		} else {
			endSupply = 0
		}

		// Calculate validator ROI metrics
		minROI, maxROI, avgROI := calculateValidatorROI(
			params.ValidatorStakes,
			totalStake,
			validatorPool,
			feeToValidators,
			params.SlashEventsPerYear,
			params.SlashAvgPct,
			params.OpCostPerYear,
			params.ActiveValidators,
		)

		// Calculate Nakamoto coefficients
		nakamoto1_3 := calculateNakamoto(params.ValidatorStakes, 0.33)
		nakamoto1_2 := calculateNakamoto(params.ValidatorStakes, 0.50)
		nakamoto2_3 := calculateNakamoto(params.ValidatorStakes, 0.66)

		// Calculate Gini coefficient
		gini := calculateGini(params.ValidatorStakes)

		projection := YearlyProjection{
			Year:            year,
			StartSupply:     currentSupply,
			EndSupply:       endSupply,
			AnnualIssuance:  annualIssuance,
			TreasuryInflow:  treasuryInflow,
			FeeRevenue:      annualFees,
			FeeBurn:         feeBurn,
			TotalBurned:     totalBurned,
			TreasuryBalance: treasuryBalance,
			MinValidatorROI: minROI,
			MaxValidatorROI: maxROI,
			AvgValidatorROI: avgROI,
			Nakamoto1_3:     nakamoto1_3,
			Nakamoto1_2:     nakamoto1_2,
			Nakamoto2_3:     nakamoto2_3,
			GiniCoefficient: gini,
		}

		results = append(results, projection)

		// Update current supply for next year
		currentSupply = endSupply

		// Recalculate total stake for next year (if needed)
		totalStake = calculateTotalStake(params.ValidatorStakes)
	}

	return results
}

// calculateTotalStake sums all validator stakes
func calculateTotalStake(stakes []uint64) uint64 {
	var total uint64
	for _, s := range stakes {
		total += s
	}
	return total
}

// calculateValidatorROI calculates ROI metrics for validators
func calculateValidatorROI(
	stakes []uint64,
	totalStake uint64,
	validatorPool uint64,
	feeToValidators uint64,
	slashEventsPerYear float64,
	slashAvgPct float64,
	opCostPerYear uint64,
	numValidators int,
) (minROI, maxROI, avgROI float64) {
	if len(stakes) == 0 || totalStake == 0 {
		return 0, 0, 0
	}

	var totalROI float64
	minROI = 1e18  // Start with a very large number
	maxROI = -1e18 // Start with a very small number

	for _, stake := range stakes {
		stakeShare := float64(stake) / float64(totalStake)

		// Rewards from issuance pool
		rewards := float64(validatorPool) * stakeShare

		// Rewards from fee pool
		rewards += float64(feeToValidators) * stakeShare

		// Slash risk (distributed across validators)
		slashRisk := float64(stake) * slashEventsPerYear * slashAvgPct / float64(numValidators) / 100

		// Operating cost
		operatingCost := float64(opCostPerYear)

		// Net return
		netReturn := rewards - slashRisk - operatingCost

		// ROI = netReturn / stake * 100
		roi := netReturn / float64(stake) * 100

		totalROI += roi

		if roi < minROI {
			minROI = roi
		}
		if roi > maxROI {
			maxROI = roi
		}
	}

	avgROI = totalROI / float64(len(stakes))

	// Handle edge case where minROI wasn't updated
	if minROI == 1e18 {
		minROI = avgROI
	}
	// Handle edge case where maxROI wasn't updated
	if maxROI == -1e18 {
		maxROI = avgROI
	}

	return minROI, maxROI, avgROI
}

// calculateGini calculates the Gini coefficient for validator stakes
func calculateGini(stakes []uint64) float64 {
	n := len(stakes)
	if n == 0 {
		return 0
	}

	// Sort stakes ascending
	sortedStakes := make([]uint64, n)
	copy(sortedStakes, stakes)
	sort.Slice(sortedStakes, func(i, j int) bool {
		return sortedStakes[i] < sortedStakes[j]
	})

	// Calculate sum of stakes
	var sumStakes uint64
	for _, s := range sortedStakes {
		sumStakes += s
	}

	if sumStakes == 0 {
		return 0
	}

	// Calculate weighted sum: sum((i+1) * stake[i])
	var weightedSum uint64
	for i, stake := range sortedStakes {
		weightedSum += uint64(i+1) * stake
	}

	// Gini coefficient formula
	gini := (2 * float64(weightedSum)) / (float64(n) * float64(sumStakes)) - float64(n+1)/float64(n)

	return gini
}

// calculateNakamoto calculates the number of validators needed to reach a threshold of total power
func calculateNakamoto(stakes []uint64, threshold float64) int {
	if len(stakes) == 0 || threshold <= 0 {
		return 0
	}

	// Sort stakes descending
	sortedStakes := make([]uint64, len(stakes))
	copy(sortedStakes, stakes)
	sort.Slice(sortedStakes, func(i, j int) bool {
		return sortedStakes[i] > sortedStakes[j]
	})

	// Calculate total stake
	var totalStake uint64
	for _, s := range sortedStakes {
		totalStake += s
	}

	if totalStake == 0 {
		return 0
	}

	// Find how many validators needed to reach threshold
	var cumulative uint64
	target := uint64(float64(totalStake) * threshold)

	for i, stake := range sortedStakes {
		cumulative += stake
		if float64(cumulative) >= float64(target) {
			return i + 1
		}
	}

	// If we get here, need all validators
	return len(sortedStakes)
}