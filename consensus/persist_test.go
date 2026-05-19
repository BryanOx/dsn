package consensus

import (
	"fmt"
	"os"
	"testing"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestChainStore_StoreLoadHeader(t *testing.T) {
	dbPath := "test_chain.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	header := &types.BlockHeader{
		Version:       1,
		Height:        42,
		PreviousHash:  types.Hash{1, 2, 3},
		StateRoot:     types.Hash{4, 5, 6},
		TxRoot:        types.Hash{7, 8, 9},
		ReceiptRoot:   types.Hash{10, 11, 12},
		ValidatorRoot: types.Hash{13, 14, 15},
		Timestamp:     1234567890,
		Proposer:      types.Address{7},
	}

	err = StoreBlockHeader(ps, header)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadBlockHeader(ps, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Height != 42 {
		t.Errorf("expected height 42, got %d", loaded.Height)
	}
	if loaded.Version != 1 {
		t.Errorf("expected version 1, got %d", loaded.Version)
	}
	if loaded.Proposer != header.Proposer {
		t.Errorf("expected proposer %v, got %v", header.Proposer, loaded.Proposer)
	}
}

func TestChainStore_StoreLoadTip(t *testing.T) {
	dbPath := "test_tip.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	header := &types.BlockHeader{Version: 1, Height: 100}
	err = StoreTip(ps, header)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadTip(ps)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Height != 100 {
		t.Errorf("expected height 100, got %d", loaded.Height)
	}
}

func TestChainStore_TipNotFound(t *testing.T) {
	dbPath := "test_notip.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	_, err = LoadTip(ps)
	if err == nil {
		t.Error("expected error for missing tip")
	}
}

func TestChainStore_BlockNotFound(t *testing.T) {
	dbPath := "test_noblock.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	_, err = LoadBlockHeader(ps, 1)
	if err == nil {
		t.Error("expected error for missing block header")
	}
}

func TestChainStore_MultipleHeaders(t *testing.T) {
	dbPath := "test_multi.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	// Store multiple headers at different heights
	for height := uint64(1); height <= 5; height++ {
		header := &types.BlockHeader{
			Version:      1,
			Height:       height,
			PreviousHash: types.Hash{byte(height - 1)},
			Proposer:     types.Address{byte(height)},
		}
		err = StoreBlockHeader(ps, header)
		if err != nil {
			t.Fatalf("failed to store header at height %d: %v", height, err)
		}
	}

	// Load and verify each
	for height := uint64(1); height <= 5; height++ {
		loaded, err := LoadBlockHeader(ps, height)
		if err != nil {
			t.Fatalf("failed to load header at height %d: %v", height, err)
		}
		if loaded.Height != height {
			t.Errorf("expected height %d, got %d", height, loaded.Height)
		}
	}
}

func TestChainStore_UpdateTip(t *testing.T) {
	dbPath := "test_update_tip.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	// Store initial tip
	header1 := &types.BlockHeader{Version: 1, Height: 50}
	err = StoreTip(ps, header1)
	if err != nil {
		t.Fatal(err)
	}

	loaded1, err := LoadTip(ps)
	if err != nil {
		t.Fatal(err)
	}
	if loaded1.Height != 50 {
		t.Errorf("expected height 50, got %d", loaded1.Height)
	}

	// Update tip
	header2 := &types.BlockHeader{Version: 1, Height: 100}
	err = StoreTip(ps, header2)
	if err != nil {
		t.Fatal(err)
	}

	loaded2, err := LoadTip(ps)
	if err != nil {
		t.Fatal(err)
	}
	if loaded2.Height != 100 {
		t.Errorf("expected height 100, got %d", loaded2.Height)
	}
}

func TestStoreLoadProposal(t *testing.T) {
	dbPath := "test_proposal.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create and store proposal
	proposal := &types.Proposal{
		Height:    5,
		Round:     0,
		BlockHash: types.Hash{1, 2, 3},
		Proposer:  types.Address{10},
		Signature: []byte("test-signature"),
	}

	err = StoreProposal(ps, proposal)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadProposal(ps, 5)
	require.NoError(t, err)
	require.Equal(t, proposal.Height, loaded.Height)
	require.Equal(t, proposal.Round, loaded.Round)
	require.Equal(t, proposal.BlockHash, loaded.BlockHash)
	require.Equal(t, proposal.Proposer, loaded.Proposer)
}

func TestStoreLoadVote(t *testing.T) {
	dbPath := "test_vote.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create and store a prevote
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    10,
		Round:     1,
		BlockHash: types.Hash{5, 6, 7},
		Validator: types.Address{20},
		Signature: []byte("prevote-sig"),
	}

	err = StoreVote(ps, prevote)
	require.NoError(t, err)

	// Load votes at height 10
	votes, err := LoadVotes(ps, 10, 0) // 0 means all types
	require.NoError(t, err)
	require.Equal(t, 1, len(votes))
	require.Equal(t, types.VotePrevote, votes[0].VoteType)

	// Load only prevotes
	prevotes, err := LoadVotes(ps, 10, types.VotePrevote)
	require.NoError(t, err)
	require.Equal(t, 1, len(prevotes))

	// Load only precommits (should be empty)
	precommits, err := LoadVotes(ps, 10, types.VotePrecommit)
	require.NoError(t, err)
	require.Equal(t, 0, len(precommits))
}

func TestStoreLoadCommitProof(t *testing.T) {
	dbPath := "test_proof.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create commit proof
	proof := &types.CommitProof{
		Height:      15,
		BlockHash:   types.Hash{8, 9, 10},
		Precommits:  []types.Vote{},
		TotalPower:  100_000,
		SignedPower: 100_000,
		SetHash:     types.Hash{11, 12, 13},
	}

	err = StoreCommitProof(ps, proof)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadCommitProof(ps, 15)
	require.NoError(t, err)
	require.Equal(t, proof.Height, loaded.Height)
	require.Equal(t, proof.BlockHash, loaded.BlockHash)
	require.Equal(t, proof.TotalPower, loaded.TotalPower)
	require.Equal(t, proof.SignedPower, loaded.SignedPower)
	require.Equal(t, proof.SetHash, loaded.SetHash)
}

func TestLoadNonExistent(t *testing.T) {
	dbPath := "test_nonexistent.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Load non-existent proposal
	_, err = LoadProposal(ps, 999)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Load non-existent commit proof
	_, err = LoadCommitProof(ps, 999)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Load non-existent votes (should return empty, not error)
	votes, err := LoadVotes(ps, 999, 0)
	require.NoError(t, err)
	require.Equal(t, 0, len(votes))
}

func TestStoreMultipleVotes(t *testing.T) {
	dbPath := "test_multivotes.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	height := uint64(20)

	// Store multiple votes from different validators
	for i := 0; i < 3; i++ {
		vote := &types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    height,
			Round:     0,
			BlockHash: types.Hash{byte(i)},
			Validator: types.Address{byte(i + 1)},
			Signature: []byte{byte(i)},
		}
		err = StoreVote(ps, vote)
		require.NoError(t, err)
	}

	// Load all votes at this height
	votes, err := LoadVotes(ps, height, 0)
	require.NoError(t, err)
	require.Equal(t, 3, len(votes))
}

func TestStoreLoadEvidence(t *testing.T) {
	dbPath := "test_evidence.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create DoubleSignEvidence
	evidence := &types.DoubleSignEvidence{
		VoteA: types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    10,
			Round:     1,
			BlockHash: types.Hash{1, 2, 3},
			Validator: types.Address{5},
			Signature: []byte("sig-a"),
		},
		VoteB: types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    10,
			Round:     1,
			BlockHash: types.Hash{4, 5, 6}, // different hash
			Validator: types.Address{5},     // same validator
			Signature: []byte("sig-b"),
		},
	}

	// Store evidence
	err = StoreEvidence(ps, evidence)
	require.NoError(t, err)

	// Get hash to verify loading
	hash, err := evidence.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hashPrefix := fmt.Sprintf("%x", hash[:8])

	// Load evidence by height and hash prefix
	loaded, err := LoadEvidence(ps, 10, hashPrefix)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify it's the correct type and content
	doubleSign, ok := loaded.(*types.DoubleSignEvidence)
	require.True(t, ok, "expected DoubleSignEvidence")
	require.Equal(t, evidence.VoteA.Height, doubleSign.VoteA.Height)
	require.Equal(t, evidence.VoteA.Validator, doubleSign.VoteA.Validator)
	require.Equal(t, evidence.VoteB.BlockHash, doubleSign.VoteB.BlockHash)
}

func TestStoreEvidenceDuplicate(t *testing.T) {
	dbPath := "test_dup_evidence.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create evidence
	evidence := &types.DoubleSignEvidence{
		VoteA: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    5,
			Round:     0,
			BlockHash: types.Hash{1},
			Validator: types.Address{1},
			Signature: []byte("sig1"),
		},
		VoteB: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    5,
			Round:     0,
			BlockHash: types.Hash{2},
			Validator: types.Address{1},
			Signature: []byte("sig2"),
		},
	}

	// Store first time - should succeed
	err = StoreEvidence(ps, evidence)
	require.NoError(t, err)

	// Store second time - should fail with ErrDuplicateEvidence
	err = StoreEvidence(ps, evidence)
	require.ErrorIs(t, err, types.ErrDuplicateEvidence)
}

func TestLoadAllEvidence(t *testing.T) {
	dbPath := "test_all_evidence.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Store multiple evidence at different heights
	evidences := []*types.DoubleSignEvidence{
		{
			VoteA: types.Vote{VoteType: types.VotePrecommit, Height: 10, Round: 0, BlockHash: types.Hash{1}, Validator: types.Address{1}, Signature: []byte("sig1")},
			VoteB: types.Vote{VoteType: types.VotePrecommit, Height: 10, Round: 0, BlockHash: types.Hash{2}, Validator: types.Address{1}, Signature: []byte("sig2")},
		},
		{
			VoteA: types.Vote{VoteType: types.VotePrecommit, Height: 5, Round: 0, BlockHash: types.Hash{1}, Validator: types.Address{2}, Signature: []byte("sig3")},
			VoteB: types.Vote{VoteType: types.VotePrecommit, Height: 5, Round: 0, BlockHash: types.Hash{2}, Validator: types.Address{2}, Signature: []byte("sig4")},
		},
		{
			VoteA: types.Vote{VoteType: types.VotePrecommit, Height: 20, Round: 0, BlockHash: types.Hash{1}, Validator: types.Address{3}, Signature: []byte("sig5")},
			VoteB: types.Vote{VoteType: types.VotePrecommit, Height: 20, Round: 0, BlockHash: types.Hash{2}, Validator: types.Address{3}, Signature: []byte("sig6")},
		},
	}

	for _, ev := range evidences {
		err = StoreEvidence(ps, ev)
		require.NoError(t, err)
	}

	// Load all evidence
	all, err := LoadAllEvidence(ps)
	require.NoError(t, err)
	require.Equal(t, 3, len(all))

	// Verify sorted by height descending (newest first)
	require.Equal(t, uint64(20), all[0].Height())
	require.Equal(t, uint64(10), all[1].Height())
	require.Equal(t, uint64(5), all[2].Height())
}

func TestLoadNonExistentEvidence(t *testing.T) {
	dbPath := "test_nonevidence.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Try to load non-existent evidence - should return nil, nil (not error)
	loaded, err := LoadEvidence(ps, 999, "deadbeef")
	require.NoError(t, err)
	require.Nil(t, loaded)
}

func TestEvidenceCount(t *testing.T) {
	dbPath := "test_count.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Initially count should be 0
	count := EvidenceCount(ps)
	require.Equal(t, 0, count)

	// Store 5 evidence objects at different heights
	for i := uint64(1); i <= 5; i++ {
		evidence := &types.DoubleSignEvidence{
			VoteA: types.Vote{VoteType: types.VotePrecommit, Height: i, Round: 0, BlockHash: types.Hash{byte(i)}, Validator: types.Address{byte(i)}, Signature: []byte("sig")},
			VoteB: types.Vote{VoteType: types.VotePrecommit, Height: i, Round: 0, BlockHash: types.Hash{byte(i + 10)}, Validator: types.Address{byte(i)}, Signature: []byte("sig")},
		}
		err = StoreEvidence(ps, evidence)
		require.NoError(t, err)
	}

	// Count should be 5
	count = EvidenceCount(ps)
	require.Equal(t, 5, count)
}

func TestStoreInvalidCommitEvidence(t *testing.T) {
	dbPath := "test_invalid_commit.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create InvalidCommitEvidence
	evidence := &types.InvalidCommitEvidence{
		Proof: types.CommitProof{
			Height:      15,
			BlockHash:   types.Hash{1, 2, 3},
			Precommits:  []types.Vote{},
			TotalPower:  100_000,
			SignedPower: 50_000,
			SetHash:     types.Hash{4, 5, 6},
		},
		Reason: "insufficient voting power",
	}

	// Store evidence
	err = StoreEvidence(ps, evidence)
	require.NoError(t, err)

	// Get hash to verify loading
	hash, err := evidence.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hashPrefix := fmt.Sprintf("%x", hash[:8])

	// Load evidence by height and hash prefix
	loaded, err := LoadEvidence(ps, 15, hashPrefix)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify it's the correct type and content
	invalidCommit, ok := loaded.(*types.InvalidCommitEvidence)
	require.True(t, ok, "expected InvalidCommitEvidence")
	require.Equal(t, evidence.Proof.Height, invalidCommit.Proof.Height)
	require.Equal(t, evidence.Reason, invalidCommit.Reason)
}

func TestStoreMalformedVoteEvidence(t *testing.T) {
	dbPath := "test_malformed_vote.db"
	defer os.Remove(dbPath)

	hasher := types.SHA256Hasher{}
	ps, err := state.NewPersistentState(dbPath, hasher, false)
	require.NoError(t, err)
	defer ps.Close()

	// Create MalformedVoteEvidence
	evidence := &types.MalformedVoteEvidence{
		BadVote: types.Vote{
			VoteType:  types.VotePrevote,
			Height:    25,
			Round:     2,
			BlockHash: types.Hash{7, 8, 9},
			Validator: types.Address{10},
			Signature: []byte("bad-sig"),
		},
		Reason: "invalid signature format",
	}

	// Store evidence
	err = StoreEvidence(ps, evidence)
	require.NoError(t, err)

	// Get hash to verify loading
	hash, err := evidence.EvidenceHash(types.SHA256Hasher{})
	require.NoError(t, err)
	hashPrefix := fmt.Sprintf("%x", hash[:8])

	// Load evidence by height and hash prefix
	loaded, err := LoadEvidence(ps, 25, hashPrefix)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify it's the correct type and content
	malformed, ok := loaded.(*types.MalformedVoteEvidence)
	require.True(t, ok, "expected MalformedVoteEvidence")
	require.Equal(t, evidence.BadVote.Height, malformed.BadVote.Height)
	require.Equal(t, evidence.BadVote.Validator, malformed.BadVote.Validator)
	require.Equal(t, evidence.Reason, malformed.Reason)
}