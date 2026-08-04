package staking

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/BryanOx/dsn/types"
)

// KVStore is the subset of state functionality needed for validator registry operations.
// Implemented by both state.InMemoryState and state.PersistentState.
type KVStore interface {
	GetBytes(key string) ([]byte, bool)
	SetBytes(key string, value []byte) error
	DeleteBytes(key string) error
}

// KV store key prefixes for deterministic key naming.
const (
	keyValidatorCount    = "validator/count"
	keyValidatorPrefix   = "validator/"
	keyValBySeqPrefix    = "validator/by-seq/"
	keyValPubkeyPrefix   = "validator/pubkey/"
	keyValOperatorPrefix = "validator/operator/"
	keyTotalBonded       = "staking/total_bonded"
	keyCurrentEpoch      = "epoch/current"
	keyBlocksPerEpoch    = "epoch/blocks_per_epoch"
	keyUnstakeCooldown   = "staking/unstake_cooldown"
)

// Default constants (can be overridden via SetEpochConfig).
const (
	DefaultBlocksPerEpoch  uint64 = 100
	DefaultUnstakeCooldown uint64 = 14 // epochs
)

// consensusIDToHex converts a consensus ID to a hex string for key storage.
func consensusIDToHex(id types.Address) string {
	return fmt.Sprintf("%x", id[:])
}

// hexToConsensusID converts a hex string back to an Address.
func hexToConsensusID(hexStr string) (types.Address, error) {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return types.Address{}, err
	}
	if len(b) != 20 {
		return types.Address{}, fmt.Errorf("invalid consensus ID length: %d", len(b))
	}
	var id types.Address
	copy(id[:], b)
	return id, nil
}

func validatorKey(consensusID types.Address) string {
	return fmt.Sprintf("%s%s", keyValidatorPrefix, consensusIDToHex(consensusID))
}

// validatorBySeqKey returns the key for mapping sequence number to consensus ID.
// This maintains deterministic ordering of validators.
func validatorBySeqKey(seq uint64) string {
	return fmt.Sprintf("%s%d", keyValBySeqPrefix, seq)
}

// validatorPubkeyKey returns the index key for looking up a validator by public key.
func validatorPubkeyKey(pubKey [32]byte) string {
	return fmt.Sprintf("%s%x", keyValPubkeyPrefix, pubKey[:])
}

// validatorOperatorKey returns the index key for looking up a validator by operator address.
func validatorOperatorKey(addr types.Address) string {
	return fmt.Sprintf("%s%s", keyValOperatorPrefix, addr.String())
}

// --- Count ---

// ValidatorCount returns the number of registered validators.
func ValidatorCount(s KVStore) (uint64, error) {
	val, ok := s.GetBytes(keyValidatorCount)
	if !ok {
		return 0, nil
	}
	return binary.BigEndian.Uint64(val), nil
}

func setValidatorCount(s KVStore, count uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)
	return s.SetBytes(keyValidatorCount, buf[:])
}

// --- Total Bonded ---

// TotalBonded returns the sum of all bonded stake across all validators.
func TotalBonded(s KVStore) (types.Amount, error) {
	val, ok := s.GetBytes(keyTotalBonded)
	if !ok {
		return types.NewAmount(0), nil
	}
	amt := types.NewAmount(0)
	if err := amt.UnmarshalBinary(val); err != nil {
		return types.NewAmount(0), err
	}
	return amt, nil
}

func setTotalBonded(s KVStore, amt types.Amount) error {
	data, err := amt.MarshalBinary()
	if err != nil {
		return err
	}
	return s.SetBytes(keyTotalBonded, data)
}

// --- CRUD ---

// RegisterValidator creates a new pending validator.
// Returns the consensus ID (types.Address) on success.
// The bonded stake is provided; the caller must deduct the amount from the
// sender's balance before calling this function.
func RegisterValidator(s KVStore, pubKey [32]byte, operatorAddr types.Address, stake types.Amount, commission uint16, currentEpoch uint64) (types.Address, error) {
	// Derive consensus ID from public key
	consensusID := types.DeriveConsensusID(pubKey)

	// Check that this validator doesn't already exist (by consensus ID)
	if _, ok := s.GetBytes(validatorKey(consensusID)); ok {
		return types.Address{}, fmt.Errorf("%w: consensus ID %s", ErrValidatorExists, consensusID)
	}

	// Check that this pubkey is not already registered
	if _, ok := s.GetBytes(validatorPubkeyKey(pubKey)); ok {
		return types.Address{}, fmt.Errorf("%w: public key already registered", ErrValidatorExists)
	}

	// Check that this operator address is not already registered
	if _, ok := s.GetBytes(validatorOperatorKey(operatorAddr)); ok {
		return types.Address{}, fmt.Errorf("%w: operator address %s", ErrValidatorExists, operatorAddr)
	}

	// Get current count for ordering
	count, err := ValidatorCount(s)
	if err != nil {
		return types.Address{}, err
	}

	// Create validator with derived consensus ID
	v, err := NewValidator(consensusID, pubKey, operatorAddr, stake, commission, currentEpoch)
	if err != nil {
		return types.Address{}, err
	}

	// Serialize and store validator at consensus ID key
	var buf bytes.Buffer
	if err := v.Encode(&buf); err != nil {
		return types.Address{}, err
	}
	if err := s.SetBytes(validatorKey(consensusID), buf.Bytes()); err != nil {
		return types.Address{}, err
	}

	// Add to by-pubkey index
	if err := s.SetBytes(validatorPubkeyKey(pubKey), consensusID[:]); err != nil {
		return types.Address{}, err
	}

	// Add to by-operator index
	if err := s.SetBytes(validatorOperatorKey(operatorAddr), consensusID[:]); err != nil {
		return types.Address{}, err
	}

	// Add to ordered list (by-seq)
	nextSeq := count + 1
	if err := s.SetBytes(validatorBySeqKey(nextSeq), consensusID[:]); err != nil {
		return types.Address{}, err
	}

	if err := setValidatorCount(s, nextSeq); err != nil {
		return types.Address{}, err
	}

	// Update total bonded
	total, err := TotalBonded(s)
	if err != nil {
		return types.Address{}, err
	}
	newTotal, err := total.Add(stake)
	if err != nil {
		return types.Address{}, err
	}
	if err := setTotalBonded(s, newTotal); err != nil {
		return types.Address{}, err
	}

	return consensusID, nil
}

// GetValidator retrieves a validator by ConsensusID.
func GetValidator(s KVStore, consensusID types.Address) (*Validator, error) {
	val, ok := s.GetBytes(validatorKey(consensusID))
	if !ok {
		return nil, fmt.Errorf("%w: consensus ID %s", ErrValidatorNotFound, consensusID)
	}
	v := &Validator{}
	if err := v.Decode(bytes.NewReader(val)); err != nil {
		return nil, err
	}
	return v, nil
}

// GetValidatorByPubKey retrieves a validator by their public key.
func GetValidatorByPubKey(s KVStore, pubKey [32]byte) (*Validator, error) {
	idBytes, ok := s.GetBytes(validatorPubkeyKey(pubKey))
	if !ok {
		return nil, fmt.Errorf("%w: public key %x", ErrValidatorNotFound, pubKey[:])
	}
	var consensusID types.Address
	copy(consensusID[:], idBytes)
	return GetValidator(s, consensusID)
}

// GetValidatorByOperator retrieves a validator by their operator address.
func GetValidatorByOperator(s KVStore, addr types.Address) (*Validator, error) {
	idBytes, ok := s.GetBytes(validatorOperatorKey(addr))
	if !ok {
		return nil, fmt.Errorf("%w: operator address %s", ErrValidatorNotFound, addr)
	}
	var consensusID types.Address
	copy(consensusID[:], idBytes)
	return GetValidator(s, consensusID)
}

// UpdateValidator stores a modified validator back to the kvstore.
// Voting power is derived from bonded stake only for active validators;
// jailed and unstaking validators have zero voting power.
func UpdateValidator(s KVStore, v *Validator) error {
	// Recalculate voting power based on status
	if v.Status == ValidatorActive {
		v.VotingPower = stakeToVotingPower(v.BondedStake)
	} else {
		v.VotingPower = 0
	}

	var buf bytes.Buffer
	if err := v.Encode(&buf); err != nil {
		return err
	}
	return s.SetBytes(validatorKey(v.ConsensusID), buf.Bytes())
}

// stakeToVotingPower converts bonded stake to voting power.
// For now, 1 DSN = 1 voting power (simple linear mapping).
func stakeToVotingPower(stake types.Amount) uint64 {
	// Amount stores as *big.Int; convert to uint64.
	// The minimum stake is 100k DSN so this is safe for v1.
	return stakeToUint64(stake)
}

func stakeToUint64(stake types.Amount) uint64 {
	data, _ := stake.MarshalBinary()
	if len(data) == 0 {
		return 0
	}
	// Pad to 8 bytes max (safe for v1 since stake fits in uint64)
	if len(data) > 8 {
		// Truncate to 8 bytes (loses high bits — should not happen in v1)
		data = data[len(data)-8:]
	}
	var val uint64
	for _, b := range data {
		val = (val << 8) | uint64(b)
	}
	return val
}

// --- Active Validator Set ---

// getValidatorIDBySeq retrieves the consensus ID for a validator at the given sequence position.
func getValidatorIDBySeq(s KVStore, seq uint64) (types.Address, error) {
	idBytes, ok := s.GetBytes(validatorBySeqKey(seq))
	if !ok {
		return types.Address{}, fmt.Errorf("%w: sequence %d", ErrValidatorNotFound, seq)
	}
	var consensusID types.Address
	copy(consensusID[:], idBytes)
	return consensusID, nil
}

// GetActiveValidators returns all active validators sorted by
// (voting_power DESC, ConsensusID ASC) for deterministic consensus ordering.
func GetActiveValidators(s KVStore) ([]*Validator, error) {
	count, err := ValidatorCount(s)
	if err != nil {
		return nil, err
	}

	var active []*Validator
	for seq := uint64(1); seq <= count; seq++ {
		consensusID, err := getValidatorIDBySeq(s, seq)
		if err != nil {
			continue // skip deleted/missing
		}
		v, err := GetValidator(s, consensusID)
		if err != nil {
			continue
		}
		if v.Status == ValidatorActive {
			active = append(active, v)
		}
	}

	// Sort: voting power DESC, ConsensusID ASC
	sort.Slice(active, func(i, j int) bool {
		if active[i].VotingPower != active[j].VotingPower {
			return active[i].VotingPower > active[j].VotingPower
		}
		return bytes.Compare(active[i].ConsensusID[:], active[j].ConsensusID[:]) < 0
	})

	return active, nil
}

// GetActiveValidatorAddresses returns the operator addresses of all active validators.
func GetActiveValidatorAddresses(s KVStore) ([]types.Address, error) {
	active, err := GetActiveValidators(s)
	if err != nil {
		return nil, err
	}
	addrs := make([]types.Address, len(active))
	for i, v := range active {
		addrs[i] = v.OperatorAddress
	}
	return addrs, nil
}

// GetActiveSetAddresses returns the operator addresses of all active validators,
// sorted by (voting_power DESC, ConsensusID ASC). This is an alias for GetActiveValidatorAddresses.
func GetActiveSetAddresses(s KVStore) ([]types.Address, error) {
	return GetActiveValidatorAddresses(s)
}

// --- Status Transitions ---

// ActivateValidator moves a validator from pending to active at the given epoch.
func ActivateValidator(s KVStore, consensusID types.Address, epoch uint64) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if v.Status != ValidatorPending {
		return fmt.Errorf("%w: validator %s is %s", ErrValidatorNotPending, consensusID, v.Status)
	}
	if err := v.transitionTo(ValidatorActive); err != nil {
		return err
	}
	v.ActivationEpoch = epoch
	v.VotingPower = stakeToVotingPower(v.BondedStake)
	return UpdateValidator(s, v)
}

// JailValidator sets a validator's status to jailed until the specified epoch.
func JailValidator(s KVStore, consensusID types.Address, untilEpoch uint64) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if v.Status != ValidatorActive {
		return fmt.Errorf("%w: validator %s is %s", ErrValidatorNotActive, consensusID, v.Status)
	}
	if err := v.transitionTo(ValidatorJailed); err != nil {
		return err
	}
	v.JailedUntil = untilEpoch
	v.VotingPower = 0
	return UpdateValidator(s, v)
}

// UnjailValidator reactivates a jailed validator.
func UnjailValidator(s KVStore, consensusID types.Address) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if v.Status != ValidatorJailed {
		return fmt.Errorf("%w: validator %s is not jailed", ErrValidatorJailed, consensusID)
	}
	if err := v.transitionTo(ValidatorActive); err != nil {
		return err
	}
	v.JailedUntil = 0
	v.VotingPower = stakeToVotingPower(v.BondedStake)
	return UpdateValidator(s, v)
}

// StartUnstake begins the unstaking process for a validator.
func StartUnstake(s KVStore, consensusID types.Address, unstakeEpoch uint64) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if err := v.transitionTo(ValidatorUnstaking); err != nil {
		return err
	}
	v.UnstakeEpoch = unstakeEpoch
	v.VotingPower = 0
	return UpdateValidator(s, v)
}

// RemoveValidator removes a validator after unstaking cooldown.
func RemoveValidator(s KVStore, consensusID types.Address) error {
	v, err := GetValidator(s, consensusID)
	if err != nil {
		return err
	}
	if v.Status != ValidatorUnstaking {
		return fmt.Errorf("%w: validator %s is %s", ErrValidatorNotUnstaking, consensusID, v.Status)
	}
	if err := v.transitionTo(ValidatorRemoved); err != nil {
		return err
	}
	v.VotingPower = 0

	// Subtract from total bonded
	total, err := TotalBonded(s)
	if err != nil {
		return err
	}
	newTotal, err := total.Sub(v.BondedStake)
	if err != nil {
		return err
	}
	if err := setTotalBonded(s, newTotal); err != nil {
		return err
	}

	// Delete from by-pubkey index
	s.DeleteBytes(validatorPubkeyKey(v.PublicKey))

	// Delete from by-operator index
	s.DeleteBytes(validatorOperatorKey(v.OperatorAddress))

	// Mark as removed but keep record (soft delete)
	return UpdateValidator(s, v)
}
