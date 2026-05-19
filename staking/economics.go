package staking

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
)

// Economics key prefixes for deterministic key naming.
const (
	KeyTotalSupply      = "economics/total_supply"
	KeyTreasurySupply   = "economics/treasury_supply"
	KeyIssuedSupply     = "economics/issued_supply"
	KeyEpochIssuance    = "economics/epoch_issuance/"
	KeyEpochValidatorPool = "economics/epoch_validator_pool/"

	YearlyInflationBasisPoints uint64 = 500       // 5% inflation
	BasisPointsDenominator     uint64 = 10_000
	SecondsPerYear             uint64 = 365 * 24 * 60 * 60 // 31536000
)

// readUint64 reads a uint64 value from the KV store.
// Returns 0 if the key doesn't exist or value is too short.
func readUint64(s KVStore, key string) uint64 {
	val, ok := s.GetBytes(key)
	if !ok || len(val) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(val)
}

// writeUint64 stores a uint64 value to the KV store as 8-byte big-endian.
func writeUint64(s KVStore, key string, val uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], val)
	return s.SetBytes(key, buf[:])
}

// EpochsPerYear calculates the deterministic number of epochs per year.
// Formula: SecondsPerYear / (blockTimeSec * blocksPerEpoch)
func EpochsPerYear(blockTimeSec, blocksPerEpoch uint64) uint64 {
	return SecondsPerYear / (blockTimeSec * blocksPerEpoch)
}

// PerEpochIssuance calculates the per-epoch DSN issuance using integer-only math.
// Formula: (totalSupply * 500) / (epochsPerYear * 10000)
// Uses math/big.Int for intermediate overflow protection.
// The final result fits in uint64 for v1.
func PerEpochIssuance(totalSupply uint64, blockTimeSec, blocksPerEpoch uint64) uint64 {
	epochsPerYear := EpochsPerYear(blockTimeSec, blocksPerEpoch)
	if epochsPerYear == 0 {
		return 0
	}

	// Use big.Int for intermediate calculations to prevent overflow
	totalSupplyBig := new(big.Int).SetUint64(totalSupply)
	inflationBP := new(big.Int).SetUint64(YearlyInflationBasisPoints)
	denom := new(big.Int).SetUint64(epochsPerYear * BasisPointsDenominator)

	// result = (totalSupply * 500) / (epochsPerYear * 10000)
	result := new(big.Int).Mul(totalSupplyBig, inflationBP)
	result.Div(result, denom)

	// Final result fits in uint64 for v1
	if !result.IsUint64() {
		return 0
	}
	return result.Uint64()
}

// IssueEpochTokens performs the full issuance flow for the given epoch.
// Returns total issuance amount.
func IssueEpochTokens(s StakingState, epoch uint64, blockTimeSec, blocksPerEpoch uint64) (uint64, error) {
	// 1. Read KeyTotalSupply from kvstore (0 if not set)
	totalSupply := readUint64(s, KeyTotalSupply)

	// 2. Calculate PerEpochIssuance
	issuance := PerEpochIssuance(totalSupply, blockTimeSec, blocksPerEpoch)

	// 3. If issuance == 0, return early
	if issuance == 0 {
		return 0, nil
	}

	// 4. Split: treasuryAmount = issuance * 10 / 100, validatorAmount = issuance * 90 / 100
	treasuryAmount := issuance * 10 / 100
	validatorAmount := issuance * 90 / 100

	// 5. Credit treasury (assumes CreditTreasury exists in treasury.go)
	if err := CreditTreasury(s, treasuryAmount); err != nil {
		return 0, fmt.Errorf("credit treasury: %w", err)
	}

	// 6. Update KeyTotalSupply: newTotal = totalSupply + issuance
	newTotal := totalSupply + issuance
	if err := writeUint64(s, KeyTotalSupply, newTotal); err != nil {
		return 0, fmt.Errorf("update total supply: %w", err)
	}

	// 7. Update KeyIssuedSupply: current + issuance
	currentIssued := readUint64(s, KeyIssuedSupply)
	newIssued := currentIssued + issuance
	if err := writeUint64(s, KeyIssuedSupply, newIssued); err != nil {
		return 0, fmt.Errorf("update issued supply: %w", err)
	}

	// 8. Store KeyEpochIssuance + epoch
	epochIssuanceKey := KeyEpochIssuance + strconv.FormatUint(epoch, 10)
	if err := writeUint64(s, epochIssuanceKey, issuance); err != nil {
		return 0, fmt.Errorf("store epoch issuance: %w", err)
	}

	// 9. Store KeyEpochValidatorPool + epoch
	epochValidatorPoolKey := KeyEpochValidatorPool + strconv.FormatUint(epoch, 10)
	if err := writeUint64(s, epochValidatorPoolKey, validatorAmount); err != nil {
		return 0, fmt.Errorf("store epoch validator pool: %w", err)
	}

	// 10. Return issuance, nil
	return issuance, nil
}

