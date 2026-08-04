package staking

import (
	"bytes"
	"testing"

	"github.com/BryanOx/dsn/types"
)

func TestValidatorStatusNames(t *testing.T) {
	tests := []struct {
		status ValidatorStatus
		name   string
	}{
		{ValidatorPending, "pending"},
		{ValidatorActive, "active"},
		{ValidatorJailed, "jailed"},
		{ValidatorUnstaking, "unstaking"},
		{ValidatorRemoved, "removed"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.name {
			t.Errorf("ValidatorStatus(%d).String() = %q, want %q", tt.status, got, tt.name)
		}
	}
}

func TestValidatorStatusUnknown(t *testing.T) {
	unknown := ValidatorStatus(99).String()
	if unknown == "" {
		t.Fatal("unknown status string should not be empty")
	}
}

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		from ValidatorStatus
		to   ValidatorStatus
		want bool
	}{
		// Valid transitions
		{ValidatorPending, ValidatorActive, true},
		{ValidatorActive, ValidatorJailed, true},
		{ValidatorActive, ValidatorUnstaking, true},
		{ValidatorJailed, ValidatorActive, true},
		{ValidatorUnstaking, ValidatorRemoved, true},

		// Invalid transitions
		{ValidatorPending, ValidatorJailed, false},
		{ValidatorPending, ValidatorUnstaking, false},
		{ValidatorPending, ValidatorRemoved, false},
		{ValidatorActive, ValidatorPending, false},
		{ValidatorActive, ValidatorRemoved, false},
		{ValidatorJailed, ValidatorPending, false},
		{ValidatorJailed, ValidatorUnstaking, false},
		{ValidatorJailed, ValidatorRemoved, false},
		{ValidatorUnstaking, ValidatorPending, false},
		{ValidatorUnstaking, ValidatorActive, false},
		{ValidatorUnstaking, ValidatorJailed, false},
		{ValidatorRemoved, ValidatorPending, false},
		{ValidatorRemoved, ValidatorActive, false},
		{ValidatorRemoved, ValidatorJailed, false},
		{ValidatorRemoved, ValidatorUnstaking, false},
	}
	for _, tt := range tests {
		got := IsValidTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("IsValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestNewValidator_Valid(t *testing.T) {
	pubKey := [32]byte{1, 2, 3}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)
	commission := uint16(1000) // 10%

	v, err := NewValidator(types.Address{1}, pubKey, addr, stake, commission, 0)
	if err != nil {
		t.Fatalf("NewValidator failed: %v", err)
	}
	expectedCID := types.Address{1}
	if v.ConsensusID != expectedCID {
		t.Errorf("ConsensusID = %v, want %v", v.ConsensusID, expectedCID)
	}
	if v.Status != ValidatorPending {
		t.Errorf("Status = %s, want pending", v.Status)
	}
	if v.BondedStake.Cmp(stake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, stake)
	}
	if v.CommissionRate != commission {
		t.Errorf("CommissionRate = %d, want %d", v.CommissionRate, commission)
	}
	if v.ActivationEpoch != 1 {
		t.Errorf("ActivationEpoch = %d, want 1 (next epoch)", v.ActivationEpoch)
	}
}

func TestNewValidator_BelowMinimumStake(t *testing.T) {
	pubKey := [32]byte{}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake - 1)

	_, err := NewValidator(types.Address{1}, pubKey, addr, stake, 1000, 0)
	if err == nil {
		t.Fatal("expected error for stake below minimum")
	}
}

func TestNewValidator_InvalidCommission(t *testing.T) {
	pubKey := [32]byte{}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	_, err := NewValidator(types.Address{1}, pubKey, addr, stake, 10001, 0) // > 10000
	if err == nil {
		t.Fatal("expected error for commission > 10000")
	}
}

func TestValidatorTransitionTo(t *testing.T) {
	pubKey := [32]byte{}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)
	v, _ := NewValidator(types.Address{1}, pubKey, addr, stake, 0, 0)

	// pending -> active
	if err := v.transitionTo(ValidatorActive); err != nil {
		t.Fatalf("pending->active should work: %v", err)
	}

	// active -> jailed
	if err := v.transitionTo(ValidatorJailed); err != nil {
		t.Fatalf("active->jailed should work: %v", err)
	}

	// jailed -> active
	if err := v.transitionTo(ValidatorActive); err != nil {
		t.Fatalf("jailed->active should work: %v", err)
	}

	// active -> unstaking
	if err := v.transitionTo(ValidatorUnstaking); err != nil {
		t.Fatalf("active->unstaking should work: %v", err)
	}

	// unstaking -> removed
	if err := v.transitionTo(ValidatorRemoved); err != nil {
		t.Fatalf("unstaking->removed should work: %v", err)
	}

	// removed -> anything should fail
	if err := v.transitionTo(ValidatorActive); err == nil {
		t.Fatal("removed->active should fail")
	}
}

func TestValidatorTransitionTo_Invalid(t *testing.T) {
	pubKey := [32]byte{}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)
	v, _ := NewValidator(types.Address{1}, pubKey, addr, stake, 0, 0)

	// pending -> jailed (invalid)
	if err := v.transitionTo(ValidatorJailed); err == nil {
		t.Fatal("pending->jailed should fail")
	}
}

func TestValidatorEncodeDecode(t *testing.T) {
	pubKey := [32]byte{1, 2, 3, 4, 5}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200_000)
	v1, _ := NewValidator(types.Address{42}, pubKey, addr, stake, 500, 10)
	v1.Status = ValidatorActive
	v1.VotingPower = 200_000
	v1.JailedUntil = 0
	v1.ActivationEpoch = 11

	var buf bytes.Buffer
	if err := v1.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	v2 := &Validator{}
	if err := v2.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if v2.PublicKey != v1.PublicKey {
		t.Errorf("PublicKey mismatch")
	}
	if v2.ConsensusID != v1.ConsensusID {
		t.Errorf("ConsensusID mismatch")
	}
	if v2.BondedStake.Cmp(v1.BondedStake) != 0 {
		t.Errorf("BondedStake mismatch: %s vs %s", v2.BondedStake, v1.BondedStake)
	}
	if v2.Status != v1.Status {
		t.Errorf("Status mismatch: %s vs %s", v2.Status, v1.Status)
	}
	if v2.CommissionRate != v1.CommissionRate {
		t.Errorf("CommissionRate mismatch: %d vs %d", v2.CommissionRate, v1.CommissionRate)
	}
	if v2.VotingPower != v1.VotingPower {
		t.Errorf("VotingPower mismatch: %d vs %d", v2.VotingPower, v1.VotingPower)
	}
	if v2.ActivationEpoch != v1.ActivationEpoch {
		t.Errorf("ActivationEpoch mismatch: %d vs %d", v2.ActivationEpoch, v1.ActivationEpoch)
	}
}

func TestValidatorRoundTripAllFields(t *testing.T) {
	// Test that all fields survive encode->decode
	pubKey := [32]byte{}
	for i := range pubKey {
		pubKey[i] = byte(i)
	}
	addr, _ := types.AddressFromBytes([]byte("aaaaaaaaaaaaaaaaaaaa"))

	v := &Validator{
		PublicKey:       pubKey,
		ConsensusID:     addr,
		BondedStake:     types.NewAmount(1_000_000),
		Status:          ValidatorJailed,
		CommissionRate:  2000,
		VotingPower:     500_000,
		JailedUntil:     5,
		ActivationEpoch: 1,
		UnstakeEpoch:    0,
	}

	var buf bytes.Buffer
	if err := v.Encode(&buf); err != nil {
		t.Fatal(err)
	}

	v2 := &Validator{}
	if err := v2.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}

	// Compare all fields
	if v2.PublicKey != v.PublicKey {
		t.Error("PublicKey mismatch")
	}
	if v2.ConsensusID != v.ConsensusID {
		t.Error("ConsensusID mismatch")
	}
	if v2.BondedStake.Cmp(v.BondedStake) != 0 {
		t.Error("BondedStake mismatch")
	}
	if v2.Status != v.Status {
		t.Error("Status mismatch")
	}
	if v2.CommissionRate != v.CommissionRate {
		t.Error("CommissionRate mismatch")
	}
	if v2.VotingPower != v.VotingPower {
		t.Error("VotingPower mismatch")
	}
	if v2.JailedUntil != v.JailedUntil {
		t.Error("JailedUntil mismatch")
	}
	if v2.ActivationEpoch != v.ActivationEpoch {
		t.Error("ActivationEpoch mismatch")
	}
	if v2.UnstakeEpoch != v.UnstakeEpoch {
		t.Error("UnstakeEpoch mismatch")
	}
}

func TestValidateCommissionRate(t *testing.T) {
	if err := ValidateCommissionRate(0); err != nil {
		t.Errorf("0 should be valid: %v", err)
	}
	if err := ValidateCommissionRate(10000); err != nil {
		t.Errorf("10000 should be valid: %v", err)
	}
	if err := ValidateCommissionRate(10001); err == nil {
		t.Error("10001 should be invalid")
	}
}
