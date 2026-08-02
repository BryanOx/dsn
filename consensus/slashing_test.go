package consensus

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/dsn/dsn/mempool"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Helper Functions
// ============================================================================

// createDoubleSignEvidence creates a DoubleSignEvidence with deterministic keys.
// voteType should be types.VotePrevote or types.VotePrecommit.
// differentHash=true creates two different block hashes (valid), false creates same hash (invalid).
func createDoubleSignEvidence(t *testing.T, height uint64, round uint32, valID byte, voteType types.VoteType, differentHash bool) *types.DoubleSignEvidence {
	priv, _, _, consensusID := deterministicKey(valID)
	hasher := types.SHA256Hasher{}
	hash1, _ := hasher.Hash([]byte("block-a"))

	voteA := &types.Vote{
		VoteType:  voteType,
		Height:    height,
		Round:     round,
		BlockHash: hash1,
		Validator: consensusID,
	}
	require.NoError(t, voteA.Sign(priv))

	var voteB *types.Vote
	if differentHash {
		hash2, _ := hasher.Hash([]byte("block-b"))
		voteB = &types.Vote{
			VoteType:  voteType,
			Height:    height,
			Round:     round,
			BlockHash: hash2,
			Validator: consensusID,
		}
	} else {
		voteB = &types.Vote{
			VoteType:  voteType,
			Height:    height,
			Round:     round,
			BlockHash: hash1,
			Validator: consensusID,
		}
	}
	require.NoError(t, voteB.Sign(priv))

	return &types.DoubleSignEvidence{VoteA: *voteA, VoteB: *voteB}
}

// setupStakingValidator creates a funded account and registers/activates a validator in mock state.
// If this is the first validator (epoch == 0), it processes the epoch transition.
// For subsequent validators, call setupStakingValidatorWithEpoch directly.
// Returns the validator's address, public key, and ConsensusID.
func setupStakingValidator(s *mockStakingState, validatorID byte, stake uint64) (types.Address, [32]byte, types.Address) {
	// Check if we're already past genesis
	currentEpoch, _ := staking.CurrentEpoch(s)
	if currentEpoch == 0 {
		return setupStakingValidatorWithEpoch(s, validatorID, stake, 100)
	}
	// Already past genesis, just add validator without epoch transition
	return setupStakingValidatorNoTransition(s, validatorID, stake)
}

// setupStakingValidatorWithEpoch creates a validator and processes epoch transition at the given height.
func setupStakingValidatorWithEpoch(s *mockStakingState, validatorID byte, stake uint64, epochHeight uint64) (types.Address, [32]byte, types.Address) {
	_, pubKey32, operatorAddr, consensusID := deterministicKey(validatorID)

	// Create account with funds
	acc := state.NewAccount(operatorAddr, pubKey32)
	acc.Balance = types.NewAmount(stake * 2)
	s.SetAccount(operatorAddr, acc)

	// Register validator
	_, err := staking.RegisterValidator(s, pubKey32, operatorAddr, types.NewAmount(stake), 0, 0)
	if err != nil {
		panic(err)
	}

	// Deduct stake from account
	acc, _ = s.GetAccount(operatorAddr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(operatorAddr, acc)

	// Process epoch transition to activate validator
	err = staking.ProcessEpochTransition(s, epochHeight)
	if err != nil {
		panic(err)
	}

	return operatorAddr, pubKey32, consensusID
}

// setupStakingValidatorNoTransition adds a validator without processing epoch transition.
// Use when validators are already activated.
func setupStakingValidatorNoTransition(s *mockStakingState, validatorID byte, stake uint64) (types.Address, [32]byte, types.Address) {
	_, pubKey32, operatorAddr, consensusID := deterministicKey(validatorID)

	// Create account with funds
	acc := state.NewAccount(operatorAddr, pubKey32)
	acc.Balance = types.NewAmount(stake * 2)
	s.SetAccount(operatorAddr, acc)

	// Use current epoch so that ActivationEpoch aligns with the next transition
	currentEpoch, _ := staking.CurrentEpoch(s)

	// Register validator
	_, err := staking.RegisterValidator(s, pubKey32, operatorAddr, types.NewAmount(stake), 0, currentEpoch)
	if err != nil {
		panic(err)
	}

	// Deduct stake from account
	acc, _ = s.GetAccount(operatorAddr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(operatorAddr, acc)

	return operatorAddr, pubKey32, consensusID
}

// ============================================================================
// GROUP 1: ProcessEvidence Unit Tests
// ============================================================================

// TestProcessEvidence_DoublePrevote_Success tests that a valid double prevote evidence
// is processed correctly, slashing the validator by 5%.
func TestProcessEvidence_DoublePrevote_Success(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Setup validator with 100000 stake
	_, _, valID := setupStakingValidator(s, 1, 100_000)

	// Create double prevote evidence
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)
	require.NoError(t, ev.Validate())

	// Process evidence
	id, err := ProcessEvidence(s, ev, 10)
	require.NoError(t, err)
	require.Equal(t, valID, id, "should return validator ID")

	// Verify validator is jailed
	v, err := staking.GetValidator(s, id)
	require.NoError(t, err)
	require.Equal(t, staking.ValidatorJailed, v.Status, "validator should be jailed")
	require.Greater(t, v.JailedUntil, uint64(0), "jailed until should be set")
	require.Equal(t, uint64(95_000), amountToUint64(v.BondedStake), "bonded stake should be reduced by 5%")
}

// TestProcessEvidence_DoublePrecommit_Success tests that a valid double precommit evidence
// is processed correctly, slashing the validator by 10%.
func TestProcessEvidence_DoublePrecommit_Success(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Setup validator with 100000 stake
	_, _, valID := setupStakingValidator(s, 1, 100_000)

	// Create double precommit evidence
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrecommit, true)
	require.NoError(t, ev.Validate())

	// Process evidence
	id, err := ProcessEvidence(s, ev, 10)
	require.NoError(t, err)
	require.Equal(t, valID, id)

	// Verify validator is jailed
	v, err := staking.GetValidator(s, id)
	require.NoError(t, err)
	require.Equal(t, staking.ValidatorJailed, v.Status)
	require.Equal(t, uint64(90_000), amountToUint64(v.BondedStake), "bonded stake should be reduced by 10%")
}

// TestProcessEvidence_DoubleSlashPrevention tests that processing the same evidence
// twice returns an ErrDuplicateEvidence error.
func TestProcessEvidence_DoubleSlashPrevention(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create evidence
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// First processing succeeds
	_, err := ProcessEvidence(s, ev, 10)
	require.NoError(t, err)

	// Second processing fails with duplicate error
	_, err = ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrDuplicateEvidence)
}

// TestProcessEvidence_InvalidEvidence tests that evidence with same block hash fails validation.
func TestProcessEvidence_InvalidEvidence(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create double sign evidence with SAME block hash (invalid)
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, false)
	err := ev.Validate()
	require.Error(t, err, "evidence with same block hash should fail validation")
	require.Contains(t, err.Error(), "different")

	// ProcessEvidence should also fail because evidence.Validate() fails
	_, err = ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation")
}

// TestProcessEvidence_UnsupportedType tests that unsupported evidence types return
// an appropriate error message.
func TestProcessEvidence_UnsupportedType(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create an InvalidCommitEvidence (unsupported in v1)
	proof := &types.CommitProof{
		Height:      5,
		BlockHash:   types.Hash{1, 2, 3},
		Precommits:  []types.Vote{},
		TotalPower:  100000,
		SignedPower: 0,
		SetHash:     types.Hash{},
	}
	ev := &types.InvalidCommitEvidence{
		Proof:  *proof,
		Reason: "test",
	}

	// ProcessEvidence should return unsupported error
	_, err := ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported evidence type")
}

// TestProcessEvidence_ValidatorNotFound tests that evidence for a validator not in
// state returns an error.
func TestProcessEvidence_ValidatorNotFound(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Don't register any validator
	// Create evidence with validator key 1
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// ProcessEvidence should fail with lookup error
	_, err := ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup validator")
}

// ============================================================================
// GROUP 2: ProcessBlockEvidence Integration
// ============================================================================

// TestProcessBlockEvidence_EmptyList tests that processing an empty evidence list
// returns nil (no-op).
func TestProcessBlockEvidence_EmptyList(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create empty block
	block := &types.Block{
		Header:   types.BlockHeader{Height: 10},
		Evidence: nil,
	}

	err := ProcessBlockEvidence(s, block)
	require.NoError(t, err, "empty evidence list should be a no-op")
}

// TestProcessBlockEvidence_MultipleEvidence tests that multiple evidence items
// are processed correctly.
func TestProcessBlockEvidence_MultipleEvidence(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Setup first validator (this also processes epoch transition)
	_, _, val1ConsensusID := setupStakingValidator(s, 1, 100_000)
	// Setup second validator without epoch transition
	_, _, val2ConsensusID := setupStakingValidatorNoTransition(s, 2, 100_000)

	// Process epoch transition to activate validator 2
	err := staking.ProcessEpochTransition(s, 200)
	require.NoError(t, err)

	// Create evidence for validator 1
	ev1 := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)
	// Create evidence for validator 2
	ev2 := createDoubleSignEvidence(t, 6, 0, 2, types.VotePrecommit, true)

	// Create block with both evidence
	block := &types.Block{
		Header:   types.BlockHeader{Height: 10},
		Evidence: []types.Evidence{ev1, ev2},
	}

	err = ProcessBlockEvidence(s, block)
	require.NoError(t, err, "multiple evidence should be processed")

	// Verify both validators are slashed
	v1, _ := staking.GetValidator(s, val1ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v1.Status, "validator 1 should be jailed")
	require.Equal(t, uint64(95_000), amountToUint64(v1.BondedStake), "validator 1 should be slashed 5%")

	v2, _ := staking.GetValidator(s, val2ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v2.Status, "validator 2 should be jailed")
	require.Equal(t, uint64(90_000), amountToUint64(v2.BondedStake), "validator 2 should be slashed 10%")
}

// TestProcessBlockEvidence_DoubleSlashInBlock tests that duplicate evidence in the
// same block is caught and stops processing.
func TestProcessBlockEvidence_DoubleSlashInBlock(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create the same evidence twice
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// Create block with duplicate evidence
	block := &types.Block{
		Header:   types.BlockHeader{Height: 10},
		Evidence: []types.Evidence{ev, ev},
	}

	// Should fail on second evidence
	err := ProcessBlockEvidence(s, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

// ============================================================================
// GROUP 3: BuildBlock+Evidence Integration
// ============================================================================

// TestBuildBlock_WithEvidence tests that BuildBlock correctly processes evidence
// and slashes the validator.
func TestBuildBlock_WithEvidence(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	// Setup validator using InMemoryState
	addr, _, valConsensusID := setupValidatorForTest(s, 1, 100_000)

	// Create double sign evidence against the validator
	ev := createDoubleSignEvidence(t, 1, 0, 1, types.VotePrevote, true)

	// Build block with evidence
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, addr, &mockSigner{}, hasher, 100, []types.Evidence{ev})
	require.NoError(t, err)
	require.NotNil(t, block)

	// Verify validator was slashed
	v, err := staking.GetValidator(s, valConsensusID)
	require.NoError(t, err)
	require.Equal(t, staking.ValidatorJailed, v.Status, "validator should be jailed")
	require.Equal(t, uint64(95_000), amountToUint64(v.BondedStake), "bonded stake should be reduced by 5%")

	// Verify evidence is in block
	require.Equal(t, 1, len(block.Evidence), "block should contain the evidence")
}

// TestBuildBlock_WithEvidence_StateRootChange tests that adding evidence changes
// the state root compared to building without evidence.
func TestBuildBlock_WithEvidence_StateRootChange(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s1 := state.NewInMemoryState(hasher)
	s2 := state.NewInMemoryState(hasher)

	mp1 := mempool.New(10000, 300*time.Second, s1)
	mp2 := mempool.New(10000, 300*time.Second, s2)

	// Setup validators in both states
	_, _, val1ConsensusID := setupValidatorForTest(s1, 1, 100_000)
	_, _, val2ConsensusID := setupValidatorForTest(s2, 1, 100_000)

	// Create evidence
	ev := createDoubleSignEvidence(t, 1, 0, 1, types.VotePrevote, true)

	// Build block WITH evidence
	block1, err := BuildBlock(s1, nil, mp1, 1, types.Hash{}, val1ConsensusID, &mockSigner{}, hasher, 100, []types.Evidence{ev})
	require.NoError(t, err)

	// Build block WITHOUT evidence
	block2, err := BuildBlock(s2, nil, mp2, 1, types.Hash{}, val2ConsensusID, &mockSigner{}, hasher, 100, nil)
	require.NoError(t, err)

	// State roots MUST differ because evidence affects state
	require.NotEqual(t, block1.Header.StateRoot, block2.Header.StateRoot,
		"state root should differ when evidence is processed")
}

// TestBuildBlock_MultipleEvidence tests that BuildBlock processes multiple evidence items.
func TestBuildBlock_MultipleEvidence(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	// Setup first validator
	_, _, val1ConsensusID := setupValidatorForTest(s, 1, 100_000)

	// Manually setup second validator (don't call setupValidatorForTest again - it would re-process epoch transition)
	addr2 := types.Address{}
	addr2[0] = 2

	seed2 := make([]byte, 32)
	seed2[0] = 2
	privKey2 := ed25519.NewKeyFromSeed(seed2)
	pubKey2 := privKey2.Public().(ed25519.PublicKey)
	var pubKey32_2 [32]byte
	copy(pubKey32_2[:], pubKey2)

	acc2 := state.NewAccount(addr2, pubKey32_2)
	acc2.Balance = types.NewAmount(200_000)
	s.SetAccount(addr2, acc2)

	val2ConsensusID, err := staking.RegisterValidator(s, pubKey32_2, addr2, types.NewAmount(100_000), 0, 1)
	if err != nil {
		panic(err)
	}

	acc2, _ = s.GetAccount(addr2)
	acc2.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr2, acc2)

	// Process epoch transition to activate validator 2
	staking.ProcessEpochTransition(s, 200)
	staking.CreateSnapshot(s, 2)

	// Create evidence for both validators
	ev1 := createDoubleSignEvidence(t, 1, 0, 1, types.VotePrevote, true)
	ev2 := createDoubleSignEvidence(t, 1, 0, 2, types.VotePrecommit, true)

	// Build block with both evidence
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, val1ConsensusID, &mockSigner{}, hasher, 100, []types.Evidence{ev1, ev2})
	require.NoError(t, err)
	require.NotNil(t, block)

	// Verify both validators are slashed
	v1, _ := staking.GetValidator(s, val1ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v1.Status)

	v2, _ := staking.GetValidator(s, val2ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v2.Status)
}

// ============================================================================
// GROUP 4: Replay Determinism
// ============================================================================

// TestSlashingDeterminism_IdenticalEvidence tests that processing identical evidence
// on identical initial states produces identical validator state.
func TestSlashingDeterminism_IdenticalEvidence(t *testing.T) {
	// Create first state
	s1 := newMockStakingState()
	staking.SetBlocksPerEpoch(s1, 100)
	_, _, val1ConsensusID := setupStakingValidator(s1, 1, 100_000)

	// Create second state with identical setup
	s2 := newMockStakingState()
	staking.SetBlocksPerEpoch(s2, 100)
	_, _, val2ConsensusID := setupStakingValidator(s2, 1, 100_000)

	// Create identical evidence
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// Process evidence on both states
	_, err := ProcessEvidence(s1, ev, 10)
	require.NoError(t, err)

	_, err = ProcessEvidence(s2, ev, 10)
	require.NoError(t, err)

	// Compare validator states
	v1, _ := staking.GetValidator(s1, val1ConsensusID)
	v2, _ := staking.GetValidator(s2, val2ConsensusID)

	require.Equal(t, v1.BondedStake, v2.BondedStake, "bonded stake should be identical")
	require.Equal(t, v1.JailedUntil, v2.JailedUntil, "jailed until should be identical")
	require.Equal(t, v1.Status, v2.Status, "status should be identical")
}

// TestSlashingDeterminism_EvidenceHash tests that evidence hash is deterministic
// and different evidence produces different hashes.
func TestSlashingDeterminism_EvidenceHash(t *testing.T) {
	// Create first evidence
	ev1 := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// Create second evidence (identical)
	ev2 := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)

	// Hash both
	hash1, err := ev1.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	hash2, err := ev2.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	// Same evidence should produce same hash
	require.Equal(t, hash1, hash2, "identical evidence should have identical hash")

	// Create different evidence (different height)
	ev3 := createDoubleSignEvidence(t, 6, 0, 1, types.VotePrevote, true)
	hash3, err := ev3.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	// Different evidence should produce different hash
	require.NotEqual(t, hash1, hash3, "different evidence should have different hash")
}

// ============================================================================
// GROUP 5: Edge Cases
// ============================================================================

// TestEvidenceProcessed_Persistence tests that IsEvidenceProcessed correctly
// identifies processed evidence after ProcessEvidence is called.
func TestEvidenceProcessed_Persistence(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create evidence
	ev := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)
	hasher := types.SHA256Hasher{}
	hash, _ := ev.EvidenceHash(hasher)

	// Initially should not be processed
	processed, err := IsEvidenceProcessed(s, hash)
	require.NoError(t, err)
	require.False(t, processed, "evidence should not be processed initially")

	// Process evidence
	_, err = ProcessEvidence(s, ev, 10)
	require.NoError(t, err)

	// Now should be processed
	processed, err = IsEvidenceProcessed(s, hash)
	require.NoError(t, err)
	require.True(t, processed, "evidence should be marked as processed")
}

// TestEvidenceProcessed_NotProcessed tests that random hashes return false.
func TestEvidenceProcessed_NotProcessed(t *testing.T) {
	s := newMockStakingState()

	// Create a random hash
	randomHash := types.Hash{1, 2, 3, 4, 5}

	processed, err := IsEvidenceProcessed(s, randomHash)
	require.NoError(t, err)
	require.False(t, processed, "random hash should not be marked as processed")
}

// TestSlashAmount_ZeroPercent tests that slashing with 0% returns no error and
// doesn't change stake.
func TestSlashAmount_ZeroPercent(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	_, _, valConsensusID := setupStakingValidator(s, 1, 100_000)

	// Apply slash with 0%
	err := staking.ApplySlash(s, valConsensusID, 0, 10)
	require.NoError(t, err, "0% slash should not error")

	// Verify stake unchanged
	v, _ := staking.GetValidator(s, valConsensusID)
	require.Equal(t, uint64(100_000), amountToUint64(v.BondedStake), "stake should be unchanged")
}

// TestSlashAmount_MaxPercent tests that slashing with 100% (10000 basis points)
// reduces stake to zero.
func TestSlashAmount_MaxPercent(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	_, _, valConsensusID := setupStakingValidator(s, 1, 100_000)

	// Apply slash with 100% (10000 basis points)
	err := staking.ApplySlash(s, valConsensusID, 10000, 10)
	require.NoError(t, err, "100% slash should not error")

	// Verify stake is zero (or near zero due to rounding)
	v, _ := staking.GetValidator(s, valConsensusID)
	require.Equal(t, uint64(0), amountToUint64(v.BondedStake), "stake should be zero")
}

// TestSlashMultipleValidators tests that evidence against multiple validators
// correctly slashes only the targeted validators.
func TestSlashMultipleValidators(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Setup first validator (this also processes epoch transition)
	_, _, val1ConsensusID := setupStakingValidator(s, 1, 100_000)

	// Setup remaining validators without epoch transition (they're pending)
	_, _, val2ConsensusID := setupStakingValidatorNoTransition(s, 2, 100_000)
	_, _, val3ConsensusID := setupStakingValidatorNoTransition(s, 3, 100_000)

	// Process another epoch transition to activate pending validators
	err := staking.ProcessEpochTransition(s, 200)
	require.NoError(t, err)

	// Verify all validators are active before slashing
	v3Check, _ := staking.GetValidator(s, val3ConsensusID)
	require.Equal(t, staking.ValidatorActive, v3Check.Status, "validator 3 should be active after epoch 2")

	// Create evidence for validators 1 and 2
	ev1 := createDoubleSignEvidence(t, 5, 0, 1, types.VotePrevote, true)
	ev2 := createDoubleSignEvidence(t, 6, 0, 2, types.VotePrecommit, true)

	// Process both evidence
	_, err = ProcessEvidence(s, ev1, 10)
	require.NoError(t, err)

	_, err = ProcessEvidence(s, ev2, 10)
	require.NoError(t, err)

	// Verify validators 1 and 2 are slashed
	v1, _ := staking.GetValidator(s, val1ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v1.Status)

	v2, _ := staking.GetValidator(s, val2ConsensusID)
	require.Equal(t, staking.ValidatorJailed, v2.Status)

	// Verify validator 3 is NOT slashed
	v3, _ := staking.GetValidator(s, val3ConsensusID)
	require.Equal(t, staking.ValidatorActive, v3.Status, "validator 3 should still be active")
	require.Equal(t, uint64(100_000), amountToUint64(v3.BondedStake), "validator 3 stake should be unchanged")
}

// ============================================================================
// GROUP 6: Adversarial Safety
// ============================================================================

// TestFakeEvidence_Rejected tests that fake evidence with random signatures
// that don't match any validator is rejected.
func TestFakeEvidence_Rejected(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Setup validator with known key
	setupStakingValidator(s, 1, 100_000)

	// Create a vote signed by a DIFFERENT key (fake)
	_, _, _, fakeConsensusID := deterministicKey(99) // Different from validator 1
	fakePriv, _, _, _ := deterministicKey(99)

	hasher := types.SHA256Hasher{}
	hash1, _ := hasher.Hash([]byte("block-a"))
	hash2, _ := hasher.Hash([]byte("block-b"))

	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: hash1,
		Validator: fakeConsensusID, // Using fake consensusID
	}
	require.NoError(t, voteA.Sign(fakePriv))

	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: hash2,
		Validator: fakeConsensusID, // Using fake consensusID
	}
	require.NoError(t, voteB.Sign(fakePriv))

	ev := &types.DoubleSignEvidence{VoteA: *voteA, VoteB: *voteB}
	err := ev.Validate()
	require.NoError(t, err, "evidence validates because votes are self-consistent")

	// But ProcessEvidence should fail because the validator doesn't exist in state
	_, err = ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lookup validator")
}

// TestMalformedEvidence_Rejected tests that malformed evidence (invalid vote fields)
// fails validation and is rejected by ProcessEvidence.
func TestMalformedEvidence_Rejected(t *testing.T) {
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	setupStakingValidator(s, 1, 100_000)

	// Create a vote with empty signature (malformed)
	// The vote will validate but when wrapped in evidence, it may fail
	_, _, _, consensusID := deterministicKey(1)
	hasher := types.SHA256Hasher{}
	hash1, _ := hasher.Hash([]byte("block-a"))
	hash2, _ := hasher.Hash([]byte("block-b"))

	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: hash1,
		Validator: consensusID,
		Signature: []byte{}, // Empty - will fail validation
	}

	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: hash2,
		Validator: consensusID,
		Signature: []byte{}, // Empty - will fail validation
	}

	// Vote.Validate() should fail
	err := voteA.Validate()
	require.Error(t, err, "vote with empty signature should fail validation")

	// Create evidence - it will call Vote.Validate() in its own Validate()
	ev := &types.DoubleSignEvidence{VoteA: *voteA, VoteB: *voteB}
	err = ev.Validate()
	require.Error(t, err, "evidence with malformed votes should fail validation")

	// ProcessEvidence should fail
	_, err = ProcessEvidence(s, ev, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation")
}

// ============================================================================
// Additional Tests: OffenseFromEvidence and SlashAmount
// ============================================================================

// TestOffenseFromEvidence_Mapping tests that OffenseFromEvidence correctly
// maps evidence types to slash offenses.
func TestOffenseFromEvidence_Mapping(t *testing.T) {
	// Test double prevote
	ev1 := createDoubleSignEvidence(t, 1, 0, 1, types.VotePrevote, true)
	offense1, err := staking.OffenseFromEvidence(ev1)
	require.NoError(t, err)
	require.Equal(t, staking.SlashDoublePrevote, offense1)

	// Test double precommit
	ev2 := createDoubleSignEvidence(t, 1, 0, 1, types.VotePrecommit, true)
	offense2, err := staking.OffenseFromEvidence(ev2)
	require.NoError(t, err)
	require.Equal(t, staking.SlashDoublePrecommit, offense2)
}

// TestSlashAmount_Calculation tests that SlashAmount correctly calculates
// the slash amount for various percentages.
func TestSlashAmount_Calculation(t *testing.T) {
	stake := types.NewAmount(100_000)

	// 5% = 500 basis points
	amount, err := staking.SlashAmount(500, stake)
	require.NoError(t, err)
	require.Equal(t, uint64(5_000), amountToUint64(amount))

	// 10% = 1000 basis points
	amount, err = staking.SlashAmount(1000, stake)
	require.NoError(t, err)
	require.Equal(t, uint64(10_000), amountToUint64(amount))

	// 20% = 2000 basis points
	amount, err = staking.SlashAmount(2000, stake)
	require.NoError(t, err)
	require.Equal(t, uint64(20_000), amountToUint64(amount))

	// 100% = 10000 basis points
	amount, err = staking.SlashAmount(10000, stake)
	require.NoError(t, err)
	require.Equal(t, uint64(100_000), amountToUint64(amount))
}

// TestSlashAmount_ExceedsMax tests that SlashAmount returns error for
// percentages exceeding 10000.
func TestSlashAmount_ExceedsMax(t *testing.T) {
	stake := types.NewAmount(100_000)

	_, err := staking.SlashAmount(10001, stake)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds max")
}

// ============================================================================
// Verify consistency with amountToUint64 from staking package
// ============================================================================

// amountToUint64 is a local copy for testing purposes.
// In production code, use the staking package's amountToUint64.
func amountToUint64(amt types.Amount) uint64 {
	data, _ := amt.MarshalBinary()
	if len(data) == 0 {
		return 0
	}
	if len(data) > 8 {
		data = data[len(data)-8:]
	}
	var val uint64
	for _, b := range data {
		val = (val << 8) | uint64(b)
	}
	return val
}
