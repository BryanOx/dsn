// Package staking implements the on-chain validator registry, staking ledger,
// epoch transitions, and active validator set derivation for DSN.
//
// All operations are deterministic and part of the state root via the kvstore
// in the state package. No external dependencies, no floating point, no clocks.
package staking

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dsn/dsn/types"
)

// ValidatorStatus represents the current state of a validator in its lifecycle.
type ValidatorStatus uint8

const (
	ValidatorPending   ValidatorStatus = 0
	ValidatorActive    ValidatorStatus = 1
	ValidatorJailed    ValidatorStatus = 2
	ValidatorUnstaking ValidatorStatus = 3
	ValidatorRemoved   ValidatorStatus = 4
)

var statusNames = map[ValidatorStatus]string{
	ValidatorPending:   "pending",
	ValidatorActive:    "active",
	ValidatorJailed:    "jailed",
	ValidatorUnstaking: "unstaking",
	ValidatorRemoved:   "removed",
}

func (s ValidatorStatus) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", s)
}

// Allowed state transitions for the validator lifecycle.
//
//	pending   -> active
//	active    -> jailed, unstaking
//	jailed    -> active
//	unstaking -> removed
var allowedTransitions = map[ValidatorStatus][]ValidatorStatus{
	ValidatorPending:   {ValidatorActive},
	ValidatorActive:    {ValidatorJailed, ValidatorUnstaking},
	ValidatorJailed:    {ValidatorActive},
	ValidatorUnstaking: {ValidatorRemoved},
	ValidatorRemoved:   {},
}

// IsValidTransition returns true if moving from `from` to `to` is allowed.
func IsValidTransition(from, to ValidatorStatus) bool {
	for _, s := range allowedTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Sentinel errors for validator lifecycle violations.
var (
	ErrInvalidTransition     = errors.New("invalid validator status transition")
	ErrValidatorNotFound     = errors.New("validator not found")
	ErrValidatorExists       = errors.New("validator already registered")
	ErrInsufficientStake     = errors.New("insufficient stake for validator")
	ErrValidatorJailed       = errors.New("validator is jailed")
	ErrValidatorNotActive    = errors.New("validator is not active")
	ErrValidatorNotPending   = errors.New("validator is not pending")
	ErrValidatorNotUnstaking = errors.New("validator is not unstaking")
)

// MinimumValidatorStake is the minimum DSN tokens required to register as a validator.
const MinimumValidatorStake uint64 = 100_000

// CommissionRateMaxBasisPoints is the maximum commission rate (100%).
const CommissionRateMaxBasisPoints uint16 = 10000

// Validator represents an on-chain validator with its full lifecycle state.
type Validator struct {
	ConsensusID     types.Address // CANONICAL identity — SHA256(pubkey)[:20]
	PublicKey       [32]byte      // Ed25519 consensus public key
	OperatorAddress types.Address // Staking/admin account address
	RewardAddress   types.Address // Future reward destination (default to OperatorAddress)
	BondedStake     types.Amount
	Status          ValidatorStatus
	CommissionRate  uint16 // basis points: 0-10000 (0% to 100%)
	VotingPower     uint64
	JailedUntil     uint64 // epoch number, 0 = not jailed
	ActivationEpoch uint64 // epoch when validator became/will become active
	UnstakeEpoch    uint64 // epoch when unstaking completes
}

// transitionTo moves the validator to a new status, validating the transition.
func (v *Validator) transitionTo(newStatus ValidatorStatus) error {
	if !IsValidTransition(v.Status, newStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, v.Status, newStatus)
	}
	v.Status = newStatus
	return nil
}

// Encode serializes the validator to binary (BigEndian, deterministic).
// Layout: ConsensusID(20) + PublicKey(32) + OperatorAddress(20) + RewardAddress(20) +
//
//	BondedStake(16) + Status(1) + CommissionRate(2) + VotingPower(8) +
//	JailedUntil(8) + ActivationEpoch(8) + UnstakeEpoch(8) = 143 bytes
func (v *Validator) Encode(w io.Writer) error {
	// ConsensusID (20 bytes)
	if _, err := w.Write(v.ConsensusID[:]); err != nil {
		return err
	}

	// PublicKey (32 bytes)
	if _, err := w.Write(v.PublicKey[:]); err != nil {
		return err
	}

	// OperatorAddress (20 bytes)
	if _, err := w.Write(v.OperatorAddress[:]); err != nil {
		return err
	}

	// RewardAddress (20 bytes)
	if _, err := w.Write(v.RewardAddress[:]); err != nil {
		return err
	}

	// BondedStake as 16 bytes big-endian
	stakeBytes, err := v.BondedStake.MarshalBinary()
	if err != nil {
		return err
	}
	var stakeBuf [16]byte
	copy(stakeBuf[16-len(stakeBytes):], stakeBytes)
	if _, err := w.Write(stakeBuf[:]); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, uint8(v.Status)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.CommissionRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.VotingPower); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.JailedUntil); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.ActivationEpoch); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, v.UnstakeEpoch)
}

// Decode deserializes a validator from binary.
func (v *Validator) Decode(r io.Reader) error {
	// ConsensusID (20 bytes)
	if _, err := io.ReadFull(r, v.ConsensusID[:]); err != nil {
		return err
	}

	// PublicKey (32 bytes)
	if _, err := io.ReadFull(r, v.PublicKey[:]); err != nil {
		return err
	}

	// OperatorAddress (20 bytes)
	operatorBytes := make([]byte, 20)
	if _, err := io.ReadFull(r, operatorBytes); err != nil {
		return err
	}
	operatorAddr, err := types.AddressFromBytes(operatorBytes)
	if err != nil {
		return err
	}
	v.OperatorAddress = operatorAddr

	// RewardAddress (20 bytes)
	rewardBytes := make([]byte, 20)
	if _, err := io.ReadFull(r, rewardBytes); err != nil {
		return err
	}
	rewardAddr, err := types.AddressFromBytes(rewardBytes)
	if err != nil {
		return err
	}
	v.RewardAddress = rewardAddr

	var stakeBuf [16]byte
	if _, err := io.ReadFull(r, stakeBuf[:]); err != nil {
		return err
	}
	v.BondedStake = types.NewAmount(0)
	if err := v.BondedStake.UnmarshalBinary(stakeBuf[:]); err != nil {
		return err
	}

	var status uint8
	if err := binary.Read(r, binary.BigEndian, &status); err != nil {
		return err
	}
	v.Status = ValidatorStatus(status)

	if err := binary.Read(r, binary.BigEndian, &v.CommissionRate); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &v.VotingPower); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &v.JailedUntil); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &v.ActivationEpoch); err != nil {
		return err
	}
	return binary.Read(r, binary.BigEndian, &v.UnstakeEpoch)
}

// ValidateCommissionRate checks that commission is within valid range.
func ValidateCommissionRate(rate uint16) error {
	if rate > CommissionRateMaxBasisPoints {
		return fmt.Errorf("commission rate %d exceeds max %d", rate, CommissionRateMaxBasisPoints)
	}
	return nil
}

// NewValidator creates a new pending validator with the given parameters.
func NewValidator(consensusID types.Address, pubKey [32]byte, operatorAddr types.Address, stake types.Amount, commission uint16, currentEpoch uint64) (*Validator, error) {
	if stake.Cmp(types.NewAmount(MinimumValidatorStake)) < 0 {
		return nil, fmt.Errorf("%w: got %s, need at least %d", ErrInsufficientStake, stake, MinimumValidatorStake)
	}
	if err := ValidateCommissionRate(commission); err != nil {
		return nil, err
	}

	return &Validator{
		ConsensusID:     consensusID,
		PublicKey:       pubKey,
		OperatorAddress: operatorAddr,
		RewardAddress:   operatorAddr, // default to operator address
		BondedStake:     stake,
		Status:          ValidatorPending,
		CommissionRate:  commission,
		VotingPower:     0, // computed at activation
		JailedUntil:     0,
		ActivationEpoch: currentEpoch + 1, // activates next epoch
		UnstakeEpoch:    0,
	}, nil
}
