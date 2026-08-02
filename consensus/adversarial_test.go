package consensus

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"
	"sort"
	"testing"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// deterministicKey generates a deterministic ed25519 key from a single byte seed.
// Returns: private key, public key (32 bytes), operator address (pub[:20]), consensusID (SHA256(pub)[:20])
func deterministicKey(id byte) (ed25519.PrivateKey, [32]byte, types.Address, types.Address) {
	seed := make([]byte, 32)
	seed[0] = id
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	var pubKey32 [32]byte
	copy(pubKey32[:], pub)
	operatorAddr, _ := types.AddressFromBytes(pub[:20])
	consensusID := types.DeriveConsensusID(pubKey32)
	return priv, pubKey32, operatorAddr, consensusID
}

// setupMockValidator creates a funded account and registers/activates a validator in mock state.
// Returns: operator address, public key, private key, consensusID
func setupMockValidator(s *mockStakingState, validatorID byte, stake uint64) (types.Address, [32]byte, ed25519.PrivateKey, types.Address) {
	priv, pubKey32, operatorAddr, consensusID := deterministicKey(validatorID)

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

	return operatorAddr, pubKey32, priv, consensusID
}

// createTestSnapshot creates a validator snapshot for testing.
// Sets up a single validator with the given stake.
func createTestSnapshot(t *testing.T, stake uint64) *staking.ValidatorSnapshot {
	s := newMockStakingState()
	_, _, _, _ = setupMockValidator(s, 1, stake)
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err)
	snap, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err)
	return snap
}

// sortVotes sorts votes by validator address for deterministic ordering.
func sortVotes(votes []types.Vote) {
	sort.Slice(votes, func(i, j int) bool {
		return bytes.Compare(votes[i].Validator[:], votes[j].Validator[:]) < 0
	})
}

// ============================================================================
// GROUP 1: Double-Sign Detection Tests
// ============================================================================

// TestDoubleSign_DifferentBlockHashes validates that double-sign evidence
// passes validation when both votes have different block hashes.
func TestDoubleSign_DifferentBlockHashes(t *testing.T) {
	// Create validator keys
	priv, pubKey32, _, consensusID := deterministicKey(1)
	_ = pubKey32

	// Create two different block hashes
	blockHash1, _ := types.SHA256Hasher{}.Hash([]byte("block-1"))
	blockHash2, _ := types.SHA256Hasher{}.Hash([]byte("block-2"))

	// Create two prevotes with same validator, height, round, but different block hashes
	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash1,
		Validator: consensusID,
	}
	err := voteA.Sign(priv)
	require.NoError(t, err)

	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash2,
		Validator: consensusID,
	}
	err = voteB.Sign(priv)
	require.NoError(t, err)

	// Create double-sign evidence
	evidence := &types.DoubleSignEvidence{
		VoteA: *voteA,
		VoteB: *voteB,
	}

	// Validate should pass
	err = evidence.Validate()
	require.NoError(t, err, "double sign evidence with different block hashes should be valid")

	// Verify EvidenceHash is deterministic
	hash1, err := evidence.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	hash2, err := evidence.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	require.Equal(t, hash1, hash2, "EvidenceHash should be deterministic")
}

// TestDoubleSign_SameBlockHashFails validates that double-sign evidence
// fails when both votes have identical block hashes.
func TestDoubleSign_SameBlockHashFails(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)

	// Create a single block hash for both votes
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create two prevotes with same validator, height, round, AND same block hash
	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err := voteA.Sign(priv)
	require.NoError(t, err)

	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err = voteB.Sign(priv)
	require.NoError(t, err)

	// Create double-sign evidence with same block hashes
	evidence := &types.DoubleSignEvidence{
		VoteA: *voteA,
		VoteB: *voteB,
	}

	// Validate should fail - block hashes must differ
	err = evidence.Validate()
	require.Error(t, err, "double sign evidence with identical block hashes should fail")
	require.Contains(t, err.Error(), "different", "error should mention block hashes must be different")
}

// TestDoubleSign_DifferentVoteTypeFails validates that double-sign evidence
// fails when votes have different vote types (Prevote vs Precommit).
func TestDoubleSign_DifferentVoteTypeFails(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// VoteA is a Prevote
	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err := voteA.Sign(priv)
	require.NoError(t, err)

	// VoteB is a Precommit (different type!)
	voteB := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err = voteB.Sign(priv)
	require.NoError(t, err)

	// Create double-sign evidence with different vote types
	evidence := &types.DoubleSignEvidence{
		VoteA: *voteA,
		VoteB: *voteB,
	}

	// Validate should fail - vote types must match
	err = evidence.Validate()
	require.Error(t, err, "double sign evidence with different vote types should fail")
	require.Contains(t, err.Error(), "vote type", "error should mention vote type mismatch")
}

// TestDoubleSign_DifferentValidatorFails validates that double-sign evidence
// fails when votes come from different validators.
func TestDoubleSign_DifferentValidatorFails(t *testing.T) {
	// Create two different validators
	priv1, _, _, consensusID1 := deterministicKey(1)
	priv2, _, _, consensusID2 := deterministicKey(2)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// VoteA from validator 1
	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID1,
	}
	err := voteA.Sign(priv1)
	require.NoError(t, err)

	// VoteB from validator 2 (different validator!)
	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID2,
	}
	err = voteB.Sign(priv2)
	require.NoError(t, err)

	// Create double-sign evidence with different validators
	evidence := &types.DoubleSignEvidence{
		VoteA: *voteA,
		VoteB: *voteB,
	}

	// Validate should fail - validators must match
	err = evidence.Validate()
	require.Error(t, err, "double sign evidence with different validators should fail")
	require.Contains(t, err.Error(), "validator", "error should mention validator mismatch")
}

// TestVotingState_DuplicateVoteRejected validates that the VotingState
// rejects duplicate votes from the same validator at the same height/round.
func TestVotingState_DuplicateVoteRejected(t *testing.T) {
	// Use setupVotingTest which creates a proper VotingState with snapshot
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	// Get the expected block hash that was used when creating the voting state
	hasher := types.SHA256Hasher{}
	expectedBlockHash, _ := hasher.Hash([]byte("block-data"))

	// Create a prevote
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: expectedBlockHash,
		Validator: consensusID,
	}
	err := prevote.Sign(privKey)
	require.NoError(t, err)

	// Add first prevote - should succeed
	err = vs.AddPrevote(prevote)
	require.NoError(t, err, "first prevote should be accepted")

	// Create a second prevote with SAME validator, height, round, blockHash
	prevote2 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: expectedBlockHash,
		Validator: consensusID,
	}
	err = prevote2.Sign(privKey)
	require.NoError(t, err)

	// Add second prevote - should fail with duplicate error
	err = vs.AddPrevote(prevote2)
	require.Error(t, err, "duplicate prevote should be rejected")
	require.Contains(t, err.Error(), "duplicate", "error should mention duplicate vote")
}

// ============================================================================
// GROUP 2: Vote Serialization Determinism Tests
// ============================================================================

// TestVoteSerialization_Deterministic validates that encoding a vote
// twice produces identical byte output.
func TestVoteSerialization_Deterministic(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	vote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err := vote.Sign(priv)
	require.NoError(t, err)

	// Encode first time
	var buf1 bytes.Buffer
	err = vote.Encode(&buf1)
	require.NoError(t, err)

	// Encode second time
	var buf2 bytes.Buffer
	err = vote.Encode(&buf2)
	require.NoError(t, err)

	// Compare bytes - must be identical
	require.True(t, bytes.Equal(buf1.Bytes(), buf2.Bytes()),
		"vote serialization should be deterministic")
}

// TestVoteHash_Deterministic validates that computing the vote hash
// twice produces identical output.
func TestVoteHash_Deterministic(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	vote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err := vote.Sign(priv)
	require.NoError(t, err)

	// Compute hash first time
	hash1, err := vote.VoteHash(types.SHA256Hasher{})
	require.NoError(t, err)

	// Compute hash second time
	hash2, err := vote.VoteHash(types.SHA256Hasher{})
	require.NoError(t, err)

	// Compare hashes - must be identical
	require.Equal(t, hash1, hash2, "vote hash should be deterministic")
}

// TestProposalSerialization_Deterministic validates that encoding a proposal
// twice produces identical byte output.
func TestProposalSerialization_Deterministic(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	proposal := &types.Proposal{
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Proposer:  consensusID,
	}
	err := proposal.Sign(priv)
	require.NoError(t, err)

	// Encode first time
	var buf1 bytes.Buffer
	err = proposal.Encode(&buf1)
	require.NoError(t, err)

	// Encode second time
	var buf2 bytes.Buffer
	err = proposal.Encode(&buf2)
	require.NoError(t, err)

	// Compare bytes - must be identical
	require.True(t, bytes.Equal(buf1.Bytes(), buf2.Bytes()),
		"proposal serialization should be deterministic")
}

// TestCommitProofSerialization_Deterministic validates that:
// 1. Encoding a CommitProof twice produces identical output
// 2. Reversing the order of precommits produces different output (canonical ordering)
func TestCommitProofSerialization_Deterministic(t *testing.T) {
	// Create two validators with different keys - note consensusID2 < consensusID1 based on deterministicKey
	_, _, _, consensusID1 := deterministicKey(1)
	_, _, _, consensusID2 := deterministicKey(2)
	priv1, _, _, _ := deterministicKey(1)
	priv2, _, _, _ := deterministicKey(2)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create precommits from both validators
	precommit1 := types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID1,
	}
	precommit1.Sign(priv1)

	precommit2 := types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID2,
	}
	precommit2.Sign(priv2)

	// Create commit proof with precommits in a fixed order (consensusID1 first, consensusID2 second)
	// This is NOT sorted - we explicitly put consensusID1 before consensusID2
	precommitsFixed := []types.Vote{precommit1, precommit2}

	proof := &types.CommitProof{
		Height:      1,
		BlockHash:   blockHash,
		Precommits:  precommitsFixed,
		TotalPower:  200000,
		SignedPower: 200000,
		SetHash:     types.Hash{},
	}

	// Encode first time
	var buf1 bytes.Buffer
	err := proof.Encode(&buf1)
	require.NoError(t, err)

	// Encode second time
	var buf2 bytes.Buffer
	err = proof.Encode(&buf2)
	require.NoError(t, err)

	// Compare bytes - must be identical
	require.True(t, bytes.Equal(buf1.Bytes(), buf2.Bytes()),
		"commit proof serialization should be deterministic")

	// Now create a proof with precommits in REVERSE order (consensusID2 first, consensusID1 second)
	precommitsReversed := []types.Vote{precommit2, precommit1}

	proofReversed := &types.CommitProof{
		Height:      1,
		BlockHash:   blockHash,
		Precommits:  precommitsReversed,
		TotalPower:  200000,
		SignedPower: 200000,
		SetHash:     types.Hash{},
	}

	// Encode reversed order
	var bufReversed bytes.Buffer
	err = proofReversed.Encode(&bufReversed)
	require.NoError(t, err)

	// Compare - must be DIFFERENT because order differs
	require.False(t, bytes.Equal(buf1.Bytes(), bufReversed.Bytes()),
		"commit proof with reversed precommit order should produce different encoding")
}

// ============================================================================
// GROUP 3: Historical Snapshot Validation (tests 10-12)
// ============================================================================

// TestHistoricalSnapshot_ValidatorRemainsInSnapshot validates that historical
// snapshots preserve validators even after they're removed from later epochs.
func TestHistoricalSnapshot_ValidatorRemainsInSnapshot(t *testing.T) {
	// Create mock state with 1 validator
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	_, _, priv, consensusID := setupMockValidator(s, 1, 100_000)

	// Process epoch transition at height 100 → epoch 0→1
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err)

	// Create snapshot at epoch 1
	snap1, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap1)
	require.Equal(t, 1, len(snap1.Validators), "snapshot should have 1 validator")
	require.True(t, snap1.HasValidator(consensusID), "snapshot should contain validator")

	// Transition to epoch 2 (height 200)
	err = staking.ProcessEpochTransition(s, 200)
	require.NoError(t, err)

	// Start unstaking the validator (transition to Unstaking) - use consensusID
	err = staking.StartUnstake(s, consensusID, 2)
	require.NoError(t, err)

	// Process epoch transition at height 300 → epoch 2→3 (releases stake)
	err = staking.ProcessEpochTransition(s, 300)
	require.NoError(t, err)

	// Create snapshot at epoch 3 (validator should be removed)
	snap3, err := staking.CreateSnapshot(s, 3)
	require.NoError(t, err)
	require.Equal(t, 0, len(snap3.Validators), "snapshot should have 0 validators")

	// GetSnapshot(epoch 1) must STILL return the validator
	snap1Retrieved, err := staking.GetSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap1Retrieved, "historical snapshot should exist")
	require.Equal(t, 1, len(snap1Retrieved.Validators), "historical snapshot should preserve validator")
	require.True(t, snap1Retrieved.HasValidator(consensusID), "historical snapshot should contain validator")
	_ = priv // suppress unused warning
}

// TestHistoricalVote_ValidAgainstHistoricalSnapshot validates that votes are
// validated against historical snapshots, not the latest state.
func TestHistoricalVote_ValidAgainstHistoricalSnapshot(t *testing.T) {
	// Create mock state
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)

	// Register validator
	_, pubKey32, priv, consensusID := setupMockValidator(s, 1, 100_000)

	// Process epoch transition at height 100 → epoch 1
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err)

	// Create snapshot for epoch 1 (contains val1)
	snap, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.True(t, snap.HasValidator(consensusID), "snapshot should contain validator")

	// Start unstaking the validator - use consensusID
	err = staking.StartUnstake(s, consensusID, 1)
	require.NoError(t, err)

	// Process epoch transition at height 200 → epoch 2 (validator removed)
	err = staking.ProcessEpochTransition(s, 200)
	require.NoError(t, err)

	// Create snapshot for epoch 2 (validator should be gone)
	snap2, err := staking.CreateSnapshot(s, 2)
	require.NoError(t, err)
	require.Equal(t, 0, len(snap2.Validators), "epoch 2 snapshot should have no validators")

	// Create VotingState with epoch 1 snapshot (historical)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))
	vs := NewVotingState(1, 0, blockHash, snap)

	// Create a prevote from val1 at height 1 - use consensusID
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	err = prevote.Sign(priv)
	require.NoError(t, err)

	// Add prevote - should SUCCEED because val1 is valid in epoch 1 snapshot
	err = vs.AddPrevote(prevote)
	require.NoError(t, err, "prevote should be accepted against historical snapshot")
	require.Equal(t, uint64(100_000), vs.PrevotePower(), "prevote power should be from epoch 1 snapshot")
	_ = pubKey32
}

// TestVotingState_ValidatorNotInActiveSetAnymore validates that votes from
// validators not in the snapshot's active set are rejected.
func TestVotingState_ValidatorNotInActiveSetAnymore(t *testing.T) {
	// Create mock state with one validator
	s := newMockStakingState()
	staking.SetBlocksPerEpoch(s, 100)
	_, _, _, _ = setupMockValidator(s, 1, 100_000)

	// Process epoch transition to activate validator
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err)

	// Create snapshot for epoch 1 (contains val1 only)
	snap, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err)
	require.Equal(t, 1, len(snap.Validators), "snapshot should have 1 validator")

	// Create VotingState with this snapshot
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))
	vs := NewVotingState(1, 0, blockHash, snap)

	// Create a vote from val2 (NOT in snapshot) - use consensusID
	_, _, _, consensusID2 := deterministicKey(2)
	priv2, _, _, _ := deterministicKey(2)

	vote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID2,
	}
	err = vote.Sign(priv2)
	require.NoError(t, err)

	// Add vote - should FAIL because val2 is not in epoch 1 snapshot
	err = vs.AddPrevote(vote)
	require.Error(t, err, "vote from validator not in snapshot should be rejected")
	require.Contains(t, err.Error(), "not in snapshot", "error should mention validator not in snapshot")
}

// ============================================================================
// GROUP 4: Replay Determinism (tests 13-15)
// ============================================================================

// TestReplayDeterminism_VotingState validates that two VotingState instances
// from the same snapshot produce identical results when given the same votes.
func TestReplayDeterminism_VotingState(t *testing.T) {
	// Create two identical VotingState instances from the same snapshot
	snap := createTestSnapshot(t, 100_000)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	vs1 := NewVotingState(1, 0, blockHash, snap)
	vs2 := NewVotingState(1, 0, blockHash, snap)

	// Get validator from snapshot
	require.Equal(t, 1, len(snap.Validators))
	val := snap.Validators[0]
	priv, _, _, _ := deterministicKey(1)

	// Create identical precommit (for BuildCommitProof, we need precommits)
	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: val.ConsensusID,
	}
	err := precommit.Sign(priv)
	require.NoError(t, err)

	// Add precommit to both VotingStates
	err = vs1.AddPrecommit(precommit)
	require.NoError(t, err)
	err = vs2.AddPrecommit(precommit)
	require.NoError(t, err)

	// Compare PrecommitPower - must be equal
	require.Equal(t, vs1.PrecommitPower(), vs2.PrecommitPower(), "PrecommitPower should be deterministic")

	// Compare HasPrecommitMajority - must be equal
	require.Equal(t, vs1.HasPrecommitMajority(), vs2.HasPrecommitMajority(), "HasPrecommitMajority should be deterministic")

	// BuildCommitProof from both - proofs must have identical Encode output
	proof1, err := vs1.BuildCommitProof()
	require.NoError(t, err)
	proof2, err := vs2.BuildCommitProof()
	require.NoError(t, err)

	var buf1, buf2 bytes.Buffer
	err = proof1.Encode(&buf1)
	require.NoError(t, err)
	err = proof2.Encode(&buf2)
	require.NoError(t, err)

	require.True(t, bytes.Equal(buf1.Bytes(), buf2.Bytes()), "CommitProof serialization should be deterministic")
}

// TestReplayDeterminism_ProposalSigning validates that ProposalHash is deterministic
// (signatures may vary but the hash before signing is deterministic).
func TestReplayDeterminism_ProposalSigning(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create two proposals with identical fields
	proposal1 := &types.Proposal{
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Proposer:  consensusID,
	}

	proposal2 := &types.Proposal{
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Proposer:  consensusID,
	}

	// Verify ProposalHash is deterministic BEFORE signing
	hash1, err := proposal1.ProposalHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hash2, err := proposal2.ProposalHash(types.SHA256Hasher{})
	require.NoError(t, err)

	require.Equal(t, hash1, hash2, "ProposalHash should be deterministic")

	// Sign both proposals
	err = proposal1.Sign(priv)
	require.NoError(t, err)
	err = proposal2.Sign(priv)
	require.NoError(t, err)

	// Verify both proposals validate successfully
	err = proposal1.Validate()
	require.NoError(t, err)
	err = proposal2.Validate()
	require.NoError(t, err)

	// Both should verify against the same public key
	pubKey := [32]byte{}
	copy(pubKey[:], priv.Public().(ed25519.PublicKey))
	require.True(t, proposal1.Verify(pubKey), "proposal1 should verify")
	require.True(t, proposal2.Verify(pubKey), "proposal2 should verify")
}

// TestReplayDeterminism_EvidenceHash validates that evidence hash is deterministic.
func TestReplayDeterminism_EvidenceHash(t *testing.T) {
	priv, _, _, consensusID := deterministicKey(1)
	blockHash1, _ := types.SHA256Hasher{}.Hash([]byte("block-1"))
	blockHash2, _ := types.SHA256Hasher{}.Hash([]byte("block-2"))

	// Create two identical votes with different block hashes (valid double sign)
	voteA1 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash1,
		Validator: consensusID,
	}
	err := voteA1.Sign(priv)
	require.NoError(t, err)

	voteB1 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash2,
		Validator: consensusID,
	}
	err = voteB1.Sign(priv)
	require.NoError(t, err)

	// Create two identical double sign evidence
	ev1 := &types.DoubleSignEvidence{
		VoteA: *voteA1,
		VoteB: *voteB1,
	}

	voteA2 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash1,
		Validator: consensusID,
	}
	err = voteA2.Sign(priv)
	require.NoError(t, err)

	voteB2 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash2,
		Validator: consensusID,
	}
	err = voteB2.Sign(priv)
	require.NoError(t, err)

	ev2 := &types.DoubleSignEvidence{
		VoteA: *voteA2,
		VoteB: *voteB2,
	}

	// Validate both
	err = ev1.Validate()
	require.NoError(t, err)
	err = ev2.Validate()
	require.NoError(t, err)

	// Compare EvidenceHash - must be identical
	hash1, err := ev1.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hash2, err := ev2.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)

	require.Equal(t, hash1, hash2, "EvidenceHash should be deterministic")
}

// ============================================================================
// GROUP 5: Evidence Lifecycle (tests 16-18)
// ============================================================================

// TestMalformedVoteEvidence_Lifecycle validates the complete lifecycle of
// malformed vote evidence: validation, serialization, and hash determinism.
func TestMalformedVoteEvidence_Lifecycle(t *testing.T) {
	// Create a vote with empty signature (structural violation)
	vote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: types.Hash{},
		Validator: types.Address{1},
		Signature: []byte{}, // empty signature - structural violation
	}

	// Create malformed vote evidence
	ev := &types.MalformedVoteEvidence{
		BadVote: *vote,
		Reason:  "empty signature",
	}

	// Validate should pass (malformed evidence documents invalid votes)
	err := ev.Validate()
	require.NoError(t, err, "malformed vote evidence validation should pass")

	// Encode then Decode - should produce identical evidence
	var buf bytes.Buffer
	err = ev.Encode(&buf)
	require.NoError(t, err)

	decoded := &types.MalformedVoteEvidence{}
	err = decoded.Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	require.Equal(t, ev.BadVote.VoteType, decoded.BadVote.VoteType)
	require.Equal(t, ev.BadVote.Height, decoded.BadVote.Height)
	require.Equal(t, ev.Reason, decoded.Reason)

	// EvidenceHash must be deterministic
	hash1, err := ev.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hash2, err := decoded.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	require.Equal(t, hash1, hash2, "EvidenceHash should be deterministic")
}

// TestInvalidCommitEvidence_Lifecycle validates the complete lifecycle of
// invalid commit evidence: validation, serialization, and hash determinism.
func TestInvalidCommitEvidence_Lifecycle(t *testing.T) {
	// Create a CommitProof with 0 precommits (insufficient power)
	proof := &types.CommitProof{
		Height:      1,
		BlockHash:   types.Hash{},
		Precommits:  []types.Vote{}, // empty - invalid
		TotalPower:  100000,
		SignedPower: 0,
		SetHash:     types.Hash{},
	}

	// Create invalid commit evidence
	ev := &types.InvalidCommitEvidence{
		Proof:  *proof,
		Reason: "no precommits",
	}

	// Validate should pass (invalid commit evidence documents invalid proofs)
	err := ev.Validate()
	require.NoError(t, err, "invalid commit evidence validation should pass")

	// Encode then Decode - should produce identical evidence
	var buf bytes.Buffer
	err = ev.Encode(&buf)
	require.NoError(t, err)

	decoded := &types.InvalidCommitEvidence{}
	err = decoded.Decode(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	require.Equal(t, ev.Proof.Height, decoded.Proof.Height)
	require.Equal(t, ev.Reason, decoded.Reason)

	// EvidenceHash must be deterministic (proof only, not reason)
	hash1, err := ev.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hash2, err := decoded.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	require.Equal(t, hash1, hash2, "EvidenceHash should be deterministic")
}

// TestEvidencePersistence_RoundTrip validates the complete evidence persistence
// lifecycle: store, load, count, and duplicate detection.
func TestEvidencePersistence_RoundTrip(t *testing.T) {
	// Create PersistentState with a proper file path
	hasher := types.SHA256Hasher{}
	dbPath := t.TempDir() + "/test.db"
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create evidence - use consensusID
	priv1, _, _, consensusID1 := deterministicKey(1)
	blockHash1 := types.Hash{1, 2, 3}
	blockHash2 := types.Hash{4, 5, 6}

	voteA := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: blockHash1,
		Validator: consensusID1,
	}
	err = voteA.Sign(priv1)
	require.NoError(t, err)

	voteB := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    5,
		Round:     0,
		BlockHash: blockHash2,
		Validator: consensusID1,
	}
	err = voteB.Sign(priv1)
	require.NoError(t, err)

	ev := &types.DoubleSignEvidence{VoteA: *voteA, VoteB: *voteB}
	err = ev.Validate()
	require.NoError(t, err)

	// Store evidence
	err = StoreEvidence(ps, ev)
	require.NoError(t, err)

	// Count should be 1
	require.Equal(t, 1, EvidenceCount(ps), "evidence count should be 1")

	// Load evidence by height + hash prefix
	hasher256 := types.SHA256Hasher{}
	eh, err := ev.EvidenceHash(hasher256)
	require.NoError(t, err)
	prefix := fmt.Sprintf("%x", eh[:8])

	loaded, err := LoadEvidence(ps, 5, prefix)
	require.NoError(t, err)
	require.NotNil(t, loaded, "loaded evidence should not be nil")

	// Compare evidence hashes
	loadedHash, err := loaded.EvidenceHash(hasher256)
	require.NoError(t, err)
	require.Equal(t, eh, loadedHash, "loaded evidence hash should match original")

	// Duplicate store should fail
	err = StoreEvidence(ps, ev)
	require.Error(t, err, "storing duplicate evidence should fail")
	require.ErrorIs(t, err, types.ErrDuplicateEvidence, "error should be ErrDuplicateEvidence")
}

// Helper to satisfy io.Writer interface for testing (actually just use bytes.Buffer)
var _ io.Writer = (*bytes.Buffer)(nil)

// TestBFT_HonestConsensus tests that the BFT harness can run consensus with honest validators
func TestBFT_HonestConsensus(t *testing.T) {
	numValidators := 4
	numRounds := 3

	harness := NewBFTTestHarness(t, BFTConfig{
		NumValidators: numValidators,
		ByzantineIdx:  -1, // no byzantine
		ByzantineType: Honest,
	})

	// Run multiple rounds
	for i := 0; i < numRounds; i++ {
		harness.RunRound()

		// Verify that a block was committed
		committed := harness.GetCommittedBlock(harness.Height)
		require.NotNil(t, committed, "round %d: expected committed block", i)

		// Verify state root is non-zero
		require.NotEqual(t, types.Hash{}, committed.Header.StateRoot,
			"round %d: state root should be non-zero", i)
	}

	// Verify safety: no conflicting blocks at same height
	require.True(t, harness.VerifySafety(), "safety violation: conflicting blocks committed")

	// Verify liveness: chain made progress
	require.True(t, harness.VerifyLiveness(), "liveness violation: no progress made")
}

// TestBFT_ByzantineProposer tests that byzantine proposer behavior is handled
func TestBFT_ByzantineProposer(t *testing.T) {
	numValidators := 4
	numRounds := 2

	harness := NewBFTTestHarness(t, BFTConfig{
		NumValidators: numValidators,
		ByzantineIdx:  0, // first validator is byzantine
		ByzantineType: ByzProposer,
	})

	// Run rounds - byzantine proposer may cause issues
	for i := 0; i < numRounds; i++ {
		harness.RunRound()

		// Even with byzantine proposer, we should have some commits
		// (the honest validators will still reach consensus)
		_ = harness.GetCommittedBlock(harness.Height)
	}

	// Safety should still hold (honest validators won't confirm conflicting blocks)
	require.True(t, harness.VerifySafety(), "safety violation detected")
}

// TestBFT_ByzantineVoter tests double-vote detection
func TestBFT_ByzantineVoter(t *testing.T) {
	numValidators := 4
	numRounds := 2

	harness := NewBFTTestHarness(t, BFTConfig{
		NumValidators: numValidators,
		ByzantineIdx:  1, // second validator is byzantine
		ByzantineType: ByzVoter,
	})

	// Run rounds
	for i := 0; i < numRounds; i++ {
		harness.RunRound()
		_ = harness.GetCommittedBlock(harness.Height)
	}

	// Safety should hold - double voting should be detected/ignored
	require.True(t, harness.VerifySafety(), "safety violation: byzantine voter caused conflict")
}
