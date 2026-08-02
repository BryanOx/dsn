package staking

import (
	"testing"

	"github.com/dsn/dsn/types"
)

// TestSlashAmount_Calculation tests the slash amount calculation with various percentages and stakes.
func TestSlashAmount_Calculation(t *testing.T) {
	tests := []struct {
		name     string
		slashPct uint64
		stake    uint64
		want     uint64
	}{
		{"5% of 100000", 500, 100000, 5000},
		{"10% of 100000", 1000, 100000, 10000},
		{"20% of 100000", 2000, 100000, 20000},
		{"0% of 100000", 0, 100000, 0},
		{"100% of 100000", 10000, 100000, 100000},
		{"5% of 200000", 500, 200000, 10000},
		{"10% of 250000", 1000, 250000, 25000},
		{"20% of 500000", 2000, 500000, 100000},
		// Edge case: fractional results truncate
		{"5% of 100001 (truncates)", 500, 100001, 5000}, // 100001 * 500 / 10000 = 5000.05 -> 5000
		{"1% of 100", 100, 100, 1},                      // 100 * 100 / 10000 = 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stake := types.NewAmount(tt.stake)
			got, err := SlashAmount(tt.slashPct, stake)
			if err != nil {
				t.Fatalf("SlashAmount error: %v", err)
			}
			gotUint64 := amountToUint64(got)
			if gotUint64 != tt.want {
				t.Errorf("SlashAmount(%d, %d) = %d, want %d", tt.slashPct, tt.stake, gotUint64, tt.want)
			}
		})
	}
}

// TestSlashAmount_ExceedsMax tests that slash percent > 10000 returns an error.
func TestSlashAmount_ExceedsMax(t *testing.T) {
	_, err := SlashAmount(10001, types.NewAmount(100000))
	if err == nil {
		t.Fatal("expected error for slash percent > 10000")
	}
}

// TestApplySlash_ActiveValidator tests slashing an active validator.
// Expected: stake reduced, status becomes Jailed, voting power = 0.
func TestApplySlash_ActiveValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register and activate validator
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}

	// Set current epoch to 5
	setCurrentEpoch(s, 5)

	// Slash 5%
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Verify validator state
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}

	if v.Status != ValidatorJailed {
		t.Errorf("status = %s, want jailed", v.Status)
	}
	if v.VotingPower != 0 {
		t.Errorf("voting power = %d, want 0", v.VotingPower)
	}
	if v.JailedUntil != 15 { // 5 + 10
		t.Errorf("JailedUntil = %d, want 15", v.JailedUntil)
	}

	// Verify stake was reduced (200000 - 5% = 190000)
	expectedStake := types.NewAmount(190000)
	if v.BondedStake.Cmp(expectedStake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, expectedStake)
	}
}

// TestApplySlash_JailedValidator tests slashing a jailed validator.
// Expected: stake reduced, status remains Jailed.
func TestApplySlash_JailedValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register, activate, then jail
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}
	if err := JailValidator(s, id, 10); err != nil {
		t.Fatal(err)
	}

	// Slash 5%
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Verify validator remains jailed
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}

	if v.Status != ValidatorJailed {
		t.Errorf("status = %s, want jailed", v.Status)
	}

	// Verify stake was reduced
	expectedStake := types.NewAmount(190000)
	if v.BondedStake.Cmp(expectedStake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, expectedStake)
	}
}

// TestApplySlash_UnstakingValidator tests slashing an unstaking validator.
// Expected: stake reduced, status remains Unstaking.
func TestApplySlash_UnstakingValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register, activate, then start unstaking
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}
	if err := StartUnstake(s, id, 14); err != nil {
		t.Fatal(err)
	}

	// Slash 5%
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Verify validator remains unstaking
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}

	if v.Status != ValidatorUnstaking {
		t.Errorf("status = %s, want unstaking", v.Status)
	}

	// Verify stake was reduced
	expectedStake := types.NewAmount(190000)
	if v.BondedStake.Cmp(expectedStake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, expectedStake)
	}
}

// TestApplySlash_TotalBondedUpdated verifies that TotalBonded decreases by the exact slash amount.
func TestApplySlash_TotalBondedUpdated(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register and activate validator
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}

	// Check initial total bonded
	initialTotal, err := TotalBonded(s)
	if err != nil {
		t.Fatal(err)
	}
	if initialTotal.Cmp(stake) != 0 {
		t.Errorf("initial total = %s, want %s", initialTotal, stake)
	}

	// Slash 5% = 10000
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Check total bonded decreased
	total, err := TotalBonded(s)
	if err != nil {
		t.Fatal(err)
	}

	expectedTotal := types.NewAmount(190000)
	if total.Cmp(expectedTotal) != 0 {
		t.Errorf("total bonded = %s, want %s", total, expectedTotal)
	}
}

// TestApplySlash_MultipleSlashes tests slashing the same validator twice.
// Expected: cumulative reduction (first 5%, then another 5% of original = 190000).
func TestApplySlash_MultipleSlashes(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register and activate validator
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}

	// First slash: 5% of 200000 = 10000 -> 190000
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("first ApplySlash error: %v", err)
	}

	// Verify after first slash
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorJailed {
		t.Errorf("status after first slash = %s, want jailed", v.Status)
	}

	// Second slash on the jailed validator: 5% of 190000 = 9500 -> 180500
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("second ApplySlash error: %v", err)
	}

	// Verify after second slash
	v, err = GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}

	// Status should still be Jailed
	if v.Status != ValidatorJailed {
		t.Errorf("status after second slash = %s, want jailed", v.Status)
	}

	// Verify stake: 200000 - 10000 - 9500 = 180500
	expectedStake := types.NewAmount(180500)
	if v.BondedStake.Cmp(expectedStake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, expectedStake)
	}

	// Verify total bonded
	total, err := TotalBonded(s)
	if err != nil {
		t.Fatal(err)
	}
	if total.Cmp(expectedStake) != 0 {
		t.Errorf("total bonded = %s, want %s", total, expectedStake)
	}
}

// TestOffenseFromEvidence_DoublePrevote tests that DoubleSignEvidence with Prevote/Prevote returns SlashDoublePrevote.
func TestOffenseFromEvidence_DoublePrevote(t *testing.T) {
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))

	ev := &types.DoubleSignEvidence{
		VoteA: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    100,
			Round:     1,
			BlockHash: types.Hash{1, 2, 3},
			Validator: addr,
			Signature: []byte("sig1"),
		},
		VoteB: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    100,
			Round:     1,
			BlockHash: types.Hash{4, 5, 6}, // Different hash
			Validator: addr,
			Signature: []byte("sig2"),
		},
	}

	offense, err := OffenseFromEvidence(ev)
	if err != nil {
		t.Fatalf("OffenseFromEvidence error: %v", err)
	}
	if offense != SlashDoublePrevote {
		t.Errorf("offense = %d, want %d (SlashDoublePrevote)", offense, SlashDoublePrevote)
	}
}

// TestOffenseFromEvidence_DoublePrecommit tests that DoubleSignEvidence with Precommit/Precommit returns SlashDoublePrecommit.
func TestOffenseFromEvidence_DoublePrecommit(t *testing.T) {
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))

	ev := &types.DoubleSignEvidence{
		VoteA: types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    100,
			Round:     1,
			BlockHash: types.Hash{1, 2, 3},
			Validator: addr,
			Signature: []byte("sig1"),
		},
		VoteB: types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    100,
			Round:     1,
			BlockHash: types.Hash{4, 5, 6}, // Different hash
			Validator: addr,
			Signature: []byte("sig2"),
		},
	}

	offense, err := OffenseFromEvidence(ev)
	if err != nil {
		t.Fatalf("OffenseFromEvidence error: %v", err)
	}
	if offense != SlashDoublePrecommit {
		t.Errorf("offense = %d, want %d (SlashDoublePrecommit)", offense, SlashDoublePrecommit)
	}
}

// TestOffenseFromEvidence_InvalidCommit tests that InvalidCommitEvidence returns SlashInvalidCommit.
func TestOffenseFromEvidence_InvalidCommit(t *testing.T) {
	ev := &types.InvalidCommitEvidence{
		Proof:  types.CommitProof{Height: 100},
		Reason: "insufficient signatures",
	}

	offense, err := OffenseFromEvidence(ev)
	if err != nil {
		t.Fatalf("OffenseFromEvidence error: %v", err)
	}
	if offense != SlashInvalidCommit {
		t.Errorf("offense = %d, want %d (SlashInvalidCommit)", offense, SlashInvalidCommit)
	}
}

// TestOffenseFromEvidence_MalformedVote tests that MalformedVoteEvidence returns an error (not slashable).
func TestOffenseFromEvidence_MalformedVote(t *testing.T) {
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))

	ev := &types.MalformedVoteEvidence{
		BadVote: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    100,
			Round:     1,
			BlockHash: types.Hash{1, 2, 3},
			Validator: addr,
			Signature: nil, // malformed
		},
		Reason: "empty signature",
	}

	_, err := OffenseFromEvidence(ev)
	if err == nil {
		t.Fatal("expected error for MalformedVoteEvidence")
	}
}

// TestSetGetSlashParams tests the slash parameters storage and retrieval.
func TestSetGetSlashParams(t *testing.T) {
	s := newTestKVStore()

	// Default params
	params := GetSlashParams(s)
	if params.PercentDoublePrevote != SlashPercentDoublePrevote {
		t.Errorf("default PercentDoublePrevote = %d, want %d", params.PercentDoublePrevote, SlashPercentDoublePrevote)
	}

	// Set custom params
	custom := SlashParams{
		PercentDoublePrevote:   750,  // 7.5%
		PercentDoublePrecommit: 1500, // 15%
		PercentInvalidCommit:   2500, // 25%
		JailEpochs:             20,
	}
	if err := SetSlashParams(s, custom); err != nil {
		t.Fatalf("SetSlashParams error: %v", err)
	}

	// Verify retrieval
	retrieved := GetSlashParams(s)
	if retrieved.PercentDoublePrevote != 750 {
		t.Errorf("PercentDoublePrevote = %d, want 750", retrieved.PercentDoublePrevote)
	}
	if retrieved.PercentDoublePrecommit != 1500 {
		t.Errorf("PercentDoublePrecommit = %d, want 1500", retrieved.PercentDoublePrecommit)
	}
	if retrieved.PercentInvalidCommit != 2500 {
		t.Errorf("PercentInvalidCommit = %d, want 2500", retrieved.PercentInvalidCommit)
	}
	if retrieved.JailEpochs != 20 {
		t.Errorf("JailEpochs = %d, want 20", retrieved.JailEpochs)
	}
}

// TestApplySlash_RemovedValidator tests that slashing a removed validator returns an error.
func TestApplySlash_RemovedValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	// Register, activate, start unstaking, then remove
	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}
	if err := StartUnstake(s, id, 14); err != nil {
		t.Fatal(err)
	}
	if err := RemoveValidator(s, id); err != nil {
		t.Fatal(err)
	}

	// Try to slash removed validator
	err = ApplySlash(s, id, 500, 10)
	if err == nil {
		t.Fatal("expected error for slashing removed validator")
	}
}

// TestApplySlash_ZeroSlashAmount tests that zero slash amount doesn't change anything.
func TestApplySlash_ZeroSlashAmount(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatal(err)
	}

	// Slash 0%
	if err := ApplySlash(s, id, 0, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Verify validator unchanged
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorActive {
		t.Errorf("status = %s, want active", v.Status)
	}
	if v.BondedStake.Cmp(stake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, stake)
	}
}

// TestApplySlash_PendingValidator tests that pending validators can be slashed.
func TestApplySlash_PendingValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(200000)

	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Slash pending validator (5% = 10000)
	if err := ApplySlash(s, id, 500, 10); err != nil {
		t.Fatalf("ApplySlash error: %v", err)
	}

	// Verify stake reduced but status still Pending
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != ValidatorPending {
		t.Errorf("status = %s, want pending", v.Status)
	}

	expectedStake := types.NewAmount(190000)
	if v.BondedStake.Cmp(expectedStake) != 0 {
		t.Errorf("BondedStake = %s, want %s", v.BondedStake, expectedStake)
	}
}
