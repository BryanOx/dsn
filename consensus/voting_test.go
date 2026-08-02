package consensus

import (
	"crypto/ed25519"
	"testing"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// setupVotingTest creates a voting state, private key, validator address, consensusID, and snapshot for testing.
// Uses the mockStakingState from system_test.go to create proper snapshots.
// The validator is registered with the generated keypair so signing in tests works correctly.
// Returns: (VotingState, privateKey, consensusID, operatorAddr, snapshot)
// - consensusID: for votes and snapshot lookups (DeriveConsensusID)
// - operatorAddr: for account operations (pub[:20])
func setupVotingTest(t *testing.T) (*VotingState, ed25519.PrivateKey, types.Address, types.Address, *staking.ValidatorSnapshot) {
	// Generate key pair for validator
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)

	// operatorAddr is used for account operations (pub[:20])
	operatorAddr, err := types.AddressFromBytes(pubKey[:20])
	require.NoError(t, err)

	// consensusID is used for consensus operations (SHA256(pub)[:20])
	consensusID := types.DeriveConsensusID(pubKey32)

	// Create mock state and register validator with the generated key
	s := newMockStakingState()

	acc := state.NewAccount(operatorAddr, pubKey32)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(operatorAddr, acc)

	_, err = staking.RegisterValidator(s, pubKey32, operatorAddr, types.NewAmount(100_000), 0, 0)
	require.NoError(t, err)

	acc, _ = s.GetAccount(operatorAddr)
	acc.SubBalance(types.NewAmount(100_000))
	s.SetAccount(operatorAddr, acc)

	// Process epoch transition to activate validator
	err = staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err)

	// Create snapshot for epoch 1
	snap, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.Equal(t, uint64(100_000), snap.TotalPower)

	// Create block hash for voting
	hasher := types.SHA256Hasher{}
	blockHash, _ := hasher.Hash([]byte("block-data"))

	// Create voting state
	vs := NewVotingState(1, 0, blockHash, snap)

	return vs, privKey, consensusID, operatorAddr, snap
}

// TestVotingState_New creates from snapshot with correct height/round
func TestVotingState_New(t *testing.T) {
	vs, _, _, _, snap := setupVotingTest(t)

	require.Equal(t, uint64(1), vs.height)
	require.Equal(t, uint32(0), vs.round)
	require.Equal(t, uint64(0), vs.PrevotePower())
	require.Equal(t, uint64(0), vs.PrecommitPower())
	require.NotNil(t, vs.Snapshot())
	require.Equal(t, snap.SetHash, vs.Snapshot().SetHash)
}

// TestVotingState_AddPrevote_Valid adds a valid prevote successfully
func TestVotingState_AddPrevote_Valid(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	// Create prevote
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: types.Hash{}, // placeholder, will be corrected
		Validator: consensusID,
	}

	// Need actual block hash - get from voting state directly
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))
	prevote.BlockHash = blockHash

	// Sign the vote
	err := prevote.Sign(privKey)
	require.NoError(t, err)

	// Add prevote
	err = vs.AddPrevote(prevote)
	require.NoError(t, err)

	require.Equal(t, uint64(100_000), vs.PrevotePower())
	require.Equal(t, uint64(0), vs.PrecommitPower())
}

// TestVotingState_AddPrecommit_Valid adds a valid precommit successfully
func TestVotingState_AddPrecommit_Valid(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create precommit
	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}

	err := precommit.Sign(privKey)
	require.NoError(t, err)

	err = vs.AddPrecommit(precommit)
	require.NoError(t, err)

	require.Equal(t, uint64(0), vs.PrevotePower())
	require.Equal(t, uint64(100_000), vs.PrecommitPower())
}

// TestVotingState_PrevoteMajority checks 100% power reaches threshold
func TestVotingState_PrevoteMajority(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)

	err := vs.AddPrevote(prevote)
	require.NoError(t, err)

	require.True(t, vs.HasPrevoteMajority(), "100%% power should have majority")
}

// TestVotingState_PrecommitMajority checks 100% power reaches threshold
func TestVotingState_PrecommitMajority(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	precommit.Sign(privKey)

	err := vs.AddPrecommit(precommit)
	require.NoError(t, err)

	require.True(t, vs.HasPrecommitMajority(), "100%% power should have majority")
}

// TestVotingState_DuplicatePrevote rejects double voting
func TestVotingState_DuplicatePrevote(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)

	err := vs.AddPrevote(prevote)
	require.NoError(t, err)

	// Try to add same vote again
	err = vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate vote")
}

// TestVotingState_WrongHeight rejects vote with wrong height
func TestVotingState_WrongHeight(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create vote with wrong height (2 instead of 1)
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    2,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)

	err := vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "height")
}

// TestVotingState_WrongBlockHash rejects vote with wrong block hash
func TestVotingState_WrongBlockHash(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	// Use different block hash than what voting state expects
	wrongHash, _ := types.SHA256Hasher{}.Hash([]byte("wrong-data"))

	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: wrongHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)

	err := vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "block hash")
}

// TestVotingState_ValidatorNotInSnapshot rejects unknown validator
func TestVotingState_ValidatorNotInSnapshot(t *testing.T) {
	_, _, _, _, snap := setupVotingTest(t)

	// Create a voting state
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))
	vs := NewVotingState(1, 0, blockHash, snap)

	// Create vote from unknown validator
	unknownAddr, _ := types.AddressFromBytes([]byte{99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99})

	// Generate a key for the unknown validator
	_, unknownPriv, _ := ed25519.GenerateKey(nil)

	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: unknownAddr,
	}
	prevote.Sign(unknownPriv)

	err := vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in snapshot")
}

// TestVotingState_InvalidSignature rejects bad signature
func TestVotingState_InvalidSignature(t *testing.T) {
	vs, _, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create vote signed by WRONG key (not the validator's key)
	wrongPub, wrongPriv, _ := ed25519.GenerateKey(nil)
	var wrongPubKey [32]byte
	copy(wrongPubKey[:], wrongPub)

	// But set the validator address to the correct one
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID, // This is the correct validator address
	}
	// Sign with the wrong key
	voteHash, _ := prevote.VoteHash(types.SHA256Hasher{})
	prevote.Signature = ed25519.Sign(wrongPriv, voteHash[:])

	// Try to add - should fail because signature doesn't match correct pubkey
	err := vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature")
}

// TestVotingState_BuildCommitProof_Valid builds proof with majority
func TestVotingState_BuildCommitProof_Valid(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Add prevote
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)
	vs.AddPrevote(prevote)

	// Add precommit
	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	precommit.Sign(privKey)
	vs.AddPrecommit(precommit)

	// Build commit proof
	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.Equal(t, uint64(1), proof.Height)
	require.Equal(t, blockHash, proof.BlockHash)
	require.Equal(t, 1, len(proof.Precommits))
	require.Equal(t, uint64(100_000), proof.TotalPower)
	require.Equal(t, uint64(100_000), proof.SignedPower)
	require.True(t, proof.HasTwoThirdsMajority())
}

// TestVotingState_BuildCommitProof_NoMajority fails without majority
func TestVotingState_BuildCommitProof_NoMajority(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Only add prevote, no precommit
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)
	vs.AddPrevote(prevote)

	// Try to build commit proof without precommit majority
	_, err := vs.BuildCommitProof()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no precommit majority")
}

// TestVotingState_DeterministicPrevotes same votes produce same result
func TestVotingState_DeterministicPrevotes(t *testing.T) {
	// Create a voting state with a deterministic key
	vs1, privKey, consensusID, operatorAddr, _ := setupVotingTest(t)
	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Add a prevote
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)
	err := vs1.AddPrevote(prevote)
	require.NoError(t, err)

	// Create a second voting state from an identical state using the SAME keypair
	s2 := newMockStakingState()
	pubKey := privKey.Public().(ed25519.PublicKey)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)

	acc := state.NewAccount(operatorAddr, pubKey32)
	acc.Balance = types.NewAmount(200_000)
	s2.SetAccount(operatorAddr, acc)

	_, err = staking.RegisterValidator(s2, pubKey32, operatorAddr, types.NewAmount(100_000), 0, 0)
	require.NoError(t, err)

	acc, _ = s2.GetAccount(operatorAddr)
	acc.SubBalance(types.NewAmount(100_000))
	s2.SetAccount(operatorAddr, acc)

	err = staking.ProcessEpochTransition(s2, 100)
	require.NoError(t, err)

	snap2, err := staking.CreateSnapshot(s2, 1)
	require.NoError(t, err)

	vs2 := NewVotingState(1, 0, blockHash, snap2)

	// Add the same prevote to vs2 (using consensusID)
	prevote2 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote2.Sign(privKey)
	err = vs2.AddPrevote(prevote2)
	require.NoError(t, err)

	// Both should have same power (both validators have 100k power in their respective snapshots)
	require.Equal(t, vs1.PrevotePower(), vs2.PrevotePower())
	require.Equal(t, vs1.HasPrevoteMajority(), vs2.HasPrevoteMajority())
}

// TestVotingState_Prevotes returns all collected prevotes
func TestVotingState_Prevotes(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)
	vs.AddPrevote(prevote)

	prevotes := vs.Prevotes()
	require.Equal(t, 1, len(prevotes))
	require.Equal(t, consensusID, prevotes[0].Validator)
}

// TestVotingState_Precommits returns all collected precommits
func TestVotingState_Precommits(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	precommit.Sign(privKey)
	vs.AddPrecommit(precommit)

	precommits := vs.Precommits()
	require.Equal(t, 1, len(precommits))
	require.Equal(t, consensusID, precommits[0].Validator)
}

// TestVotingState_WrongRound rejects vote with wrong round
func TestVotingState_WrongRound(t *testing.T) {
	vs, privKey, consensusID, _, _ := setupVotingTest(t)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))

	// Create vote with wrong round (1 instead of 0)
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     1,
		BlockHash: blockHash,
		Validator: consensusID,
	}
	prevote.Sign(privKey)

	err := vs.AddPrevote(prevote)
	require.Error(t, err)
	require.Contains(t, err.Error(), "round")
}

// TestVotingState_MultipleValidators tests with multiple validators
func TestVotingState_MultipleValidators(t *testing.T) {
	// Create mock state with 3 validators using deterministic ed25519 keys
	s := newMockStakingState()

	// Generate 3 deterministic keypairs from seeds
	// Note: we need both operatorAddr (pub[:20]) and consensusID (SHA256(pub)[:20])
	type valKey struct {
		operatorAddr types.Address
		consensusID  types.Address
		pubKey       [32]byte
		privKey      ed25519.PrivateKey
	}
	vals := make([]valKey, 3)
	for i := 0; i < 3; i++ {
		seed := make([]byte, 32)
		seed[0] = byte(i + 1)
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)

		var pubKey32 [32]byte
		copy(pubKey32[:], pub)
		operatorAddr, err := types.AddressFromBytes(pub[:20])
		require.NoError(t, err)
		consensusID := types.DeriveConsensusID(pubKey32)

		vals[i] = valKey{operatorAddr: operatorAddr, consensusID: consensusID, pubKey: pubKey32, privKey: priv}
	}

	stakes := []uint64{100_000, 200_000, 300_000}
	for i := 0; i < 3; i++ {
		acc := state.NewAccount(vals[i].operatorAddr, vals[i].pubKey)
		acc.Balance = types.NewAmount(stakes[i] * 2)
		s.SetAccount(vals[i].operatorAddr, acc)

		_, err := staking.RegisterValidator(s, vals[i].pubKey, vals[i].operatorAddr, types.NewAmount(stakes[i]), 0, 0)
		require.NoError(t, err)

		acc, _ = s.GetAccount(vals[i].operatorAddr)
		acc.SubBalance(types.NewAmount(stakes[i]))
		s.SetAccount(vals[i].operatorAddr, acc)
	}

	staking.ProcessEpochTransition(s, 100)
	snap, _ := staking.CreateSnapshot(s, 1)

	// Total power should be 100k + 200k + 300k = 600k
	require.Equal(t, uint64(600_000), snap.TotalPower)

	blockHash, _ := types.SHA256Hasher{}.Hash([]byte("block-data"))
	vs := NewVotingState(1, 0, blockHash, snap)

	// Validator 0 (100k) prevotes - no majority yet (100k < 400k)
	// Use consensusID for votes, not operatorAddr
	prevote0 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: vals[0].consensusID,
	}
	prevote0.Sign(vals[0].privKey)
	vs.AddPrevote(prevote0)

	require.False(t, vs.HasPrevoteMajority(), "100k/600k should not have majority")

	// Add validator 1 (200k) - now 300k, still no majority (300k < 400k)
	prevote1 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: vals[1].consensusID,
	}
	prevote1.Sign(vals[1].privKey)
	vs.AddPrevote(prevote1)

	require.False(t, vs.HasPrevoteMajority(), "300k/600k should not have majority")

	// Add validator 2 (300k) - now 600k, should have majority (600k >= 400k)
	prevote2 := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    1,
		Round:     0,
		BlockHash: blockHash,
		Validator: vals[2].consensusID,
	}
	prevote2.Sign(vals[2].privKey)
	vs.AddPrevote(prevote2)

	require.True(t, vs.HasPrevoteMajority(), "600k/600k should have majority")
}
