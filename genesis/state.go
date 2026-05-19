package genesis

import (
	"encoding/hex"
	"strings"

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
func InitGenesisState(doc *GenesisDoc, db *bbolt.DB, hasher types.Hasher) (types.Hash, error) {
	// Create in-memory state for initial account setup
	s := state.NewInMemoryState(hasher)

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

	// T2-12: Insert validator entries into validators bucket
	// First ensure the validators bucket exists
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(ValidatorBucketName))
		return err
	}); err != nil {
		return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
	}

	// Insert validators
	for _, v := range doc.InitialValidators {
		addr, err := parseAddress(v.Address)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		// Encode validator data
		pubKey, err := hex.DecodeString(stripHexPrefix(v.PubKey))
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		commission, err := parseCommissionToUint64(v.Commission)
		if err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
		}

		validatorData := encodeValidatorEntry(addr, pubKey, v.Stake, commission)

		if err := db.Update(func(tx *bbolt.Tx) error {
			return tx.Bucket([]byte(ValidatorBucketName)).Put(addr.Bytes(), validatorData)
		}); err != nil {
			return types.Hash{}, ErrGenesisValidationFailed.Wrap(err)
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

// encodeValidatorEntry encodes validator data for storage
func encodeValidatorEntry(addr types.Address, pubKey []byte, stake uint64, commission uint64) []byte {
	// Format: address(20) + pubkey_len(1) + pubkey + stake(8) + commission(8)
	data := make([]byte, 0, 20+1+len(pubKey)+8+8)

	data = append(data, addr[:]...)
	data = append(data, byte(len(pubKey)))
	data = append(data, pubKey...)

	// Add stake as big-endian uint64
	stakeBytes := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		stakeBytes[i] = byte(stake & 0xff)
		stake >>= 8
	}
	data = append(data, stakeBytes...)

	// Add commission as big-endian uint64
	commBytes := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		commBytes[i] = byte(commission & 0xff)
		commission >>= 8
	}
	data = append(data, commBytes...)

	return data
}