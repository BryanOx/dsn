package genesis

import (
	"encoding/hex"
	"strings"
	"time"
)

// ValidateGenesis performs comprehensive validation of a genesis document.
// It collects all validation errors and returns them together.
func ValidateGenesis(doc *GenesisDoc) error {
	var errs []string

	// Check genesis_version is set and >= 1
	if doc.GenesisVersion < 1 {
		errs = append(errs, "genesis_version must be >= 1")
	}

	// T2-7: Check chain_id is non-empty
	if doc.ChainID == "" {
		errs = append(errs, "missing required field: chain_id")
	}

	// T2-8: Check genesis_time is not too far in the future
	now := time.Now()
	maxFuture := now.Add(5 * time.Minute)
	if doc.GenesisTime.After(maxFuture) {
		errs = append(errs, "genesis_time is too far in the future")
	}

	// T2-9: Check all numeric params are non-negative
	if doc.ConsensusParams.MaxTxPerBlock < 0 {
		errs = append(errs, "negative value for max_tx_per_block")
	}
	if doc.ConsensusParams.MaxBytesPerBlock < 0 {
		errs = append(errs, "negative value for max_bytes_per_block")
	}
	if doc.ConsensusParams.MaxGasPerBlock < 0 {
		errs = append(errs, "negative value for max_gas_per_block")
	}
	if doc.EpochParams.BlocksPerEpoch < 0 {
		errs = append(errs, "negative value for blocks_per_epoch")
	}
	if doc.EpochParams.UnstakeCooldownEpochs < 0 {
		errs = append(errs, "negative value for unstake_cooldown_epochs")
	}
	if doc.EpochParams.MaxValidators < 0 {
		errs = append(errs, "negative value for max_validators")
	}
	if doc.EpochParams.MinimumStake < 0 {
		errs = append(errs, "negative value for minimum_stake")
	}

	// Check initial validators exist (network needs validators)
	if len(doc.InitialValidators) == 0 {
		errs = append(errs, "no initial validators defined")
	}

	// T2-4: Check for duplicate validator addresses and pubkeys
	addrSeen := make(map[string]bool)
	pubkeySeen := make(map[string]bool)

	for _, v := range doc.InitialValidators {
		// Normalize address to lowercase for comparison
		addrLower := strings.ToLower(v.Address)
		if addrSeen[addrLower] {
			errs = append(errs, "duplicate validator address: "+v.Address)
		}
		addrSeen[addrLower] = true

		// Normalize pubkey to lowercase for comparison
		pubkeyLower := strings.ToLower(v.PubKey)
		if pubkeySeen[pubkeyLower] {
			errs = append(errs, "duplicate validator pubkey: "+v.PubKey)
		}
		pubkeySeen[pubkeyLower] = true
	}

	// T2-5: Check validator stake >= MinimumStake
	minStake := doc.EpochParams.MinimumStake
	for _, v := range doc.InitialValidators {
		if v.Stake < minStake {
			errs = append(errs, "validator stake below minimum: "+formatStake(v.Stake))
		}
	}

	// T2-6: Validate all addresses (validator addresses, account addresses, treasury)
	// Validator addresses
	for _, v := range doc.InitialValidators {
		if err := validateHexAddress(v.Address); err != nil {
			errs = append(errs, "invalid address: "+v.Address)
		}
	}

	// Account addresses
	for _, b := range doc.InitialBalances {
		if err := validateHexAddress(b.Address); err != nil {
			errs = append(errs, "invalid address: "+b.Address)
		}
	}

	// Treasury address
	if err := validateHexAddress(doc.Treasury.Address); err != nil {
		errs = append(errs, "invalid address: "+doc.Treasury.Address)
	}

	// Validate commission rate is valid (0-10000 representing 0-100%)
	for _, v := range doc.InitialValidators {
		commission, err := parseCommission(v.Commission)
		if err != nil {
			errs = append(errs, "invalid validator commission: "+v.Commission)
		} else if commission > 10000 {
			errs = append(errs, "commission rate exceeds 100%: "+v.Commission)
		}
	}

	if len(errs) > 0 {
		return &ValidationErrors{Errors: errs}
	}

	return nil
}

// validateHexAddress checks if a hex-encoded address is valid (max 20 bytes when decoded)
func validateHexAddress(addr string) error {
	if addr == "" {
		return nil // Empty addresses are allowed for accounts that don't exist
	}

	// Remove 0x prefix if present
	addr = strings.TrimPrefix(addr, "0x")

	// Check length (40 hex chars = 20 bytes)
	if len(addr) > 40 {
		return ErrInvalidAddress
	}

	// Check that it's valid hex
	if len(addr) > 0 {
		_, err := hex.DecodeString(addr)
		if err != nil {
			return ErrInvalidAddress
		}
	}

	return nil
}

// formatStake formats a stake value for error messages
func formatStake(stake uint64) string {
	return strings.TrimPrefix(uint64ToHex(stake), "0x")
}

// uint64ToHex converts a uint64 to hex string
func uint64ToHex(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = "0123456789abcdef"[n%16]
		n /= 16
	}
	return string(buf[i:])
}

// parseCommission parses a commission rate string to a uint64 (basis points)
func parseCommission(c string) (uint64, error) {
	// Commission can be in various formats: "1000", "10.00%", "0.1"
	// For simplicity, we handle the basic case of a number
	var result uint64
	_, err := parseSimpleNumber(c, &result)
	return result, err
}

// parseSimpleNumber parses a simple number string to uint64
func parseSimpleNumber(s string, result *uint64) (bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, ErrInvalidCommission
	}

	// Remove any % sign
	s = strings.TrimSuffix(s, "%")

	// Parse as integer (assuming basis points)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, ErrInvalidCommission
		}
		n = n*10 + int(c-'0')
	}

	*result = uint64(n)
	return true, nil
}

// ValidationErrors holds multiple validation error messages
type ValidationErrors struct {
	Errors []string
}

func (e *ValidationErrors) Error() string {
	return strings.Join(e.Errors, "; ")
}

// Predefined validation errors
var (
	ErrInvalidAddress    = &GenesisError{Code: "INVALID_ADDRESS", Message: "address is invalid"}
	ErrInvalidCommission = &GenesisError{Code: "INVALID_COMMISSION", Message: "commission rate is invalid"}
)
