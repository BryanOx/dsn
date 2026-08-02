package genesis

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// ValidatorBucketName is the BoltDB bucket name for validators
const ValidatorBucketName = "validators"

// InitGenesisState initializes the state from a validated genesis document.
// It creates/ensures necessary BoltDB buckets and inserts:
// 1. Treasury balance
// 2. Initial account balances
// 3. Initial validator entries
// Returns the state root hash.
func InitGenesisState(doc *GenesisDoc, s *state.InMemoryState, db *bbolt.DB, hasher types.Hasher) (types.Hash, error) {
	// T2-13: Initialize treasury account
	if doc.Treasury.Address != "" && doc.Treasury.InitialBalance > 0 {
		treasuryAddr, err := parseAddress(doc.Treasury.Address)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		acc := state.NewAccount(treasuryAddr, [32]byte{})
		if err := acc.AddBalance(types.NewAmount(doc.Treasury.InitialBalance)); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
		if err := s.SetAccount(treasuryAddr, acc); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
	}

	// T2-11: Insert initial account balances
	for _, b := range doc.InitialBalances {
		addr, err := parseAddress(b.Address)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		acc := state.NewAccount(addr, [32]byte{})
		if err := acc.AddBalance(types.NewAmount(b.Amount)); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
		if err := s.SetAccount(addr, acc); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
	}

	// Calculate and persist economic state
	var totalSupply uint64
	for _, b := range doc.InitialBalances {
		totalSupply += b.Amount
	}
	if doc.Treasury.Address != "" && doc.Treasury.InitialBalance > 0 {
		totalSupply += doc.Treasury.InitialBalance
	}

	if err := staking.WriteUint64(s, staking.KeyTotalSupply, totalSupply); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}
	if err := staking.WriteUint64(s, staking.KeyIssuedSupply, 0); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}
	if doc.Treasury.InitialBalance > 0 {
		if err := staking.WriteUint64(s, staking.KeyTreasurySupply, doc.Treasury.InitialBalance); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
	}

	// Persist EpochParams
	if err := staking.WriteUint64(s, "epoch/blocks_per_epoch", doc.EpochParams.BlocksPerEpoch); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}
	if err := staking.WriteUint64(s, "staking/unstake_cooldown", doc.EpochParams.UnstakeCooldownEpochs); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}
	if err := staking.WriteUint64(s, "staking/max_validators", doc.EpochParams.MaxValidators); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}
	if err := staking.WriteUint64(s, "staking/minimum_stake", doc.EpochParams.MinimumStake); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}

	// Parse and persist InflationParams
	var annualBP uint64
	if doc.InflationParams.AnnualRate != "" {
		rate, err := strconv.ParseFloat(doc.InflationParams.AnnualRate, 64)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(fmt.Errorf("invalid annual_rate: %w", err))
		}
		annualBP = uint64(rate * 10000)
	}

	if err := staking.SetInflationParams(s, doc.InflationParams.Enabled, annualBP); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}

	// T2-12: Insert validator entries into validators bucket
	// First ensure the validators bucket exists
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(ValidatorBucketName))
		return err
	}); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}

	// Register validators in the staking registry
	for _, v := range doc.InitialValidators {
		addr, err := parseAddress(v.Address)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		pubKey, err := hex.DecodeString(stripHexPrefix(v.PubKey))
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
		var pubKeyArr [32]byte
		copy(pubKeyArr[:], pubKey)

		stake := types.NewAmount(v.Stake)
		commissionUint64, err := parseCommissionToUint64(v.Commission)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}
		commission := uint16(commissionUint64)

		cid, err := staking.RegisterValidator(s, pubKeyArr, addr, stake, commission, 0)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(fmt.Errorf("register genesis validator: %w", err))
		}
		// Activate genesis validators immediately (epoch 1)
		if err := staking.ActivateValidator(s, cid, 1); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(fmt.Errorf("activate genesis validator: %w", err))
		}
	}

	// T2-14: Commit state and return root hash
	root, err := s.Commit()
	if err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}

	return root, nil
}

// parseAddress parses a hex string to a types.Address
func parseAddress(addrStr string) (types.Address, error) {
	addrStr = stripHexPrefix(addrStr)
	if addrStr == "" {
		return types.Address{}, ErrInvalidAddress
	}

	addrBytes, err := hex.DecodeString(addrStr)
	if err != nil {
		return types.Address{}, ErrInvalidAddress
	}

	return types.AddressFromBytes(addrBytes)
}

// stripHexPrefix removes 0x prefix from hex string
func stripHexPrefix(s string) string {
	if len(s) >= 2 && s[0:2] == "0x" {
		return s[2:]
	}
	return s
}

// parseCommissionToUint64 parses commission string to uint64 (basis points)
func parseCommissionToUint64(c string) (uint64, error) {
	c = stripHexPrefix(c)
	c = strings.TrimSuffix(c, "%")

	// Simple integer parsing
	var result uint64
	for _, ch := range c {
		if ch < '0' || ch > '9' {
			return 0, ErrInvalidCommission
		}
		result = result*10 + uint64(ch-'0')
	}

	return result, nil
}
