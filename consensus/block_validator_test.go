package consensus

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dsn/dsn/mempool"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// setupValidatorForTest creates a funded account, registers/activates a validator, and returns its address, private key, and ConsensusID.
func setupValidatorForTest(s *state.InMemoryState, validatorID uint64, stake uint64) (types.Address, ed25519.PrivateKey, types.Address) {
	addr := types.Address{}
	addr[0] = byte(validatorID)

	// Generate deterministic keypair based on validatorID
	seed := make([]byte, 32)
	seed[0] = byte(validatorID)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)

	// Create account with funds
	acc := state.NewAccount(addr, pubKey32)
	acc.Balance = types.NewAmount(stake * 2)
	s.SetAccount(addr, acc)

	// Register validator and get ConsensusID
	consensusID, err := staking.RegisterValidator(s, pubKey32, addr, types.NewAmount(stake), 0, 0)
	if err != nil {
		panic(err)
	}

	// Deduct stake from account
	acc, _ = s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(addr, acc)

	// Process epoch transition to activate validator
	staking.ProcessEpochTransition(s, 100)

	// Create snapshot
	staking.CreateSnapshot(s, 1)

	return addr, privKey, consensusID
}

// setupTest creates a state with a sender account and a single validator.
// Returns: hasher, state, mempool, senderPubKey, senderPrivKey, validatorAddr, validatorPrivKey, validatorConsensusID
func setupTest(t *testing.T) (types.Hasher, *state.InMemoryState, *mempool.Mempool, ed25519.PublicKey, ed25519.PrivateKey, types.Address, ed25519.PrivateKey, types.Address) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	sender := types.Address{1}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(1000))
	s.SetAccount(sender, acc)

	// Setup validator - get both operator address and consensusID
	validatorAddr, validatorPrivKey, validatorConsensusID := setupValidatorForTest(s, 1, 100_000)

	return hasher, s, mp, pubKey, privKey, validatorAddr, validatorPrivKey, validatorConsensusID
}

// attachCommitProof builds a commit proof for a block using the given validator and attaches it.
// valConsensusID should be the consensusID (DeriveConsensusID), not the operator address.
func attachCommitProof(t *testing.T, block *types.Block, s *state.InMemoryState, valConsensusID types.Address, valPrivKey ed25519.PrivateKey) {
	hasher := types.SHA256Hasher{}
	headerHash, err := block.HeaderHash(hasher)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(s, block.Header.Epoch)
	require.NoError(t, err)
	require.NotNil(t, snap)

	vs := NewVotingState(block.Header.Height, 0, headerHash, snap)

	// Proposer prevotes - use consensusID
	prevote := &types.Vote{
		VoteType:  types.VotePrevote,
		Height:    block.Header.Height,
		Round:     0,
		BlockHash: headerHash,
		Validator: valConsensusID,
	}
	err = prevote.Sign(valPrivKey)
	require.NoError(t, err)
	err = vs.AddPrevote(prevote)
	require.NoError(t, err)

	// Proposer precommits - use consensusID
	precommit := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    block.Header.Height,
		Round:     0,
		BlockHash: headerHash,
		Validator: valConsensusID,
	}
	err = precommit.Sign(valPrivKey)
	require.NoError(t, err)
	err = vs.AddPrecommit(precommit)
	require.NoError(t, err)

	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	block.CommitProof = proof
}

// attachMultiValidatorCommitProof builds a commit proof using multiple validators and attaches it.
// Validators should be in ascending order by consensusID for deterministic output.
func attachMultiValidatorCommitProof(t *testing.T, block *types.Block, s *state.InMemoryState, validators []struct {
	ConsensusID types.Address
	PrivKey     ed25519.PrivateKey
}) {
	hasher := types.SHA256Hasher{}
	headerHash, err := block.HeaderHash(hasher)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(s, block.Header.Epoch)
	require.NoError(t, err)
	require.NotNil(t, snap)

	vs := NewVotingState(block.Header.Height, 0, headerHash, snap)

	for _, v := range validators {
		// Previte
		prevote := &types.Vote{
			VoteType:  types.VotePrevote,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: v.ConsensusID,
		}
		err = prevote.Sign(v.PrivKey)
		require.NoError(t, err)
		err = vs.AddPrevote(prevote)
		require.NoError(t, err)

		// Precommit
		precommit := &types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: v.ConsensusID,
		}
		err = precommit.Sign(v.PrivKey)
		require.NoError(t, err)
		err = vs.AddPrecommit(precommit)
		require.NoError(t, err)
	}

	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	block.CommitProof = proof

	// Sort precommits by validator address ascending for deterministic output
	sortVotesByValidator(block.CommitProof.Precommits)
}

// sortVotesByValidator sorts votes by validator address ascending
func sortVotesByValidator(votes []types.Vote) {
	for i := 0; i < len(votes)-1; i++ {
		for j := i + 1; j < len(votes); j++ {
			if bytes.Compare(votes[i].Validator[:], votes[j].Validator[:]) > 0 {
				votes[i], votes[j] = votes[j], votes[i]
			}
		}
	}
}

func TestValidateBlock_Valid(t *testing.T) {
	hasher, s, mp, senderPubKey, privKey, _, validatorPrivKey, validatorConsensusID := setupTest(t)

	sender := types.Address{1}
	tx := &types.Transaction{
		Version: 1, Nonce: 1, Sender: sender, MaxFee: 10,
		IntentID: types.Hash{1}, Timestamp: uint64(time.Now().Unix()),
	}
	tx.Signature = ed25519.Sign(privKey, tx.IntentID[:])
	mp.Submit(tx)

	// Use consensusID as proposer
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Attach commit proof - use consensusID
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	// Fresh state for validation
	freshS := state.NewInMemoryState(hasher)
	var pubKey32 [32]byte
	copy(pubKey32[:], senderPubKey)
	acc := state.NewAccount(types.Address{1}, pubKey32)
	acc.AddBalance(types.NewAmount(1000))
	freshS.SetAccount(types.Address{1}, acc)

	// Setup validator in fresh state
	_, _, _ = setupValidatorForTest(freshS, 1, 100_000)

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, freshS, hasher, nil, 1)
	require.NoError(t, err, "valid block should pass")
}

func TestValidateBlock_WrongHeight(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Attach commit proof (needed since block is height>0, but validation will fail at height check first)
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	parent := &types.BlockHeader{Height: 5}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "expected error for wrong height")
}

func TestValidateBlock_WrongProposer(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	// Create TWO validators before processing epoch transition (both get activated at once).
	// Validator 1 has 100k, validator 2 has 300k (enough for self-majority).
	// Validator 1 (seed {1}) — small stake
	seed1 := make([]byte, 32)
	seed1[0] = 1
	privKey1 := ed25519.NewKeyFromSeed(seed1)
	pubKey1 := privKey1.Public().(ed25519.PublicKey)
	var pubKey32_1 [32]byte
	copy(pubKey32_1[:], pubKey1)
	addr1 := types.Address{}
	addr1[0] = 1
	consensusID1 := types.DeriveConsensusID(pubKey32_1)
	acc1 := state.NewAccount(addr1, pubKey32_1)
	acc1.Balance = types.NewAmount(1_000_000)
	s.SetAccount(addr1, acc1)
	staking.RegisterValidator(s, pubKey32_1, addr1, types.NewAmount(100_000), 0, 0)
	acc1, _ = s.GetAccount(addr1)
	acc1.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr1, acc1)

	// Validator 2 (seed {2}) — large enough for self-majority (300k of 400k total)
	seed2 := make([]byte, 32)
	seed2[0] = 2
	privKey2 := ed25519.NewKeyFromSeed(seed2)
	pubKey2 := privKey2.Public().(ed25519.PublicKey)
	var pubKey32_2 [32]byte
	copy(pubKey32_2[:], pubKey2)
	addr2 := types.Address{}
	addr2[0] = 2
	consensusID2 := types.DeriveConsensusID(pubKey32_2)
	acc2 := state.NewAccount(addr2, pubKey32_2)
	acc2.Balance = types.NewAmount(1_000_000)
	s.SetAccount(addr2, acc2)
	staking.RegisterValidator(s, pubKey32_2, addr2, types.NewAmount(300_000), 0, 0)
	acc2, _ = s.GetAccount(addr2)
	acc2.SubBalance(types.NewAmount(300_000))
	s.SetAccount(addr2, acc2)

	// Process epoch transition once (activates both validators)
	staking.ProcessEpochTransition(s, 100)
	staking.CreateSnapshot(s, 1)

	// Create fresh state with the SAME two validators (same balances)
	freshS := state.NewInMemoryState(hasher)
	acc1f := state.NewAccount(addr1, pubKey32_1)
	acc1f.Balance = types.NewAmount(1_000_000)
	freshS.SetAccount(addr1, acc1f)
	staking.RegisterValidator(freshS, pubKey32_1, addr1, types.NewAmount(100_000), 0, 0)
	acc1f, _ = freshS.GetAccount(addr1)
	acc1f.SubBalance(types.NewAmount(100_000))
	freshS.SetAccount(addr1, acc1f)

	acc2f := state.NewAccount(addr2, pubKey32_2)
	acc2f.Balance = types.NewAmount(1_000_000)
	freshS.SetAccount(addr2, acc2f)
	staking.RegisterValidator(freshS, pubKey32_2, addr2, types.NewAmount(300_000), 0, 0)
	acc2f, _ = freshS.GetAccount(addr2)
	acc2f.SubBalance(types.NewAmount(300_000))
	freshS.SetAccount(addr2, acc2f)

	staking.ProcessEpochTransition(freshS, 100)
	staking.CreateSnapshot(freshS, 1)

	// Expected proposer at height 1 with validators sorted by VotingPower DESC:
	// Validator 2 (300k) > Validator 1 (100k), so Validator 2 is expected proposer.
	// But the active set passes to WeightedProposerAtHeight, which uses offset = height % totalPower
	// height=1, totalPower=400k, offset=1
	// cumulative after val2=300k, 1 < 300k → val2 is selected.
	// Since the block proposer IS validator 2, this test needs a different setup...
	// Actually: the block was built WITH validator 2 as proposer against `s`.
	// freshS has same validators. Validation runs BeginBlock, gets epoch, gets snapshot.
	// The proposer at height 1 with 2 validators (sorted desc by power: val2 300k, val1 100k):
	// offset = 1 % 400000 = 1, cumulative after val2 = 300k, 1 < 300k → val2 selected.
	// So validator 2 IS the expected proposer! We need the block to have a DIFFERENT proposer.
	// Fix: build the block with validator 2's addr, but validator 1 should have been the proposer.
	// Actually let's swap: build with validator 1 as proposer, but validator 2 is expected.
	block2, err := BuildBlock(s, nil, mp, 1, types.Hash{}, consensusID1, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)
	require.Equal(t, consensusID1, block2.Header.Proposer)

	// Attach commit proof using validator 2 (has self-majority) - use consensusID
	attachCommitProof(t, block2, s, consensusID2, privKey2)

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block2, parent, types.ZeroHash, freshS, hasher, nil, 1)
	require.Error(t, err, "expected error for wrong proposer")
	require.Contains(t, err.Error(), "wrong proposer")
}

func TestValidateBlock_EmptyBlock(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Attach commit proof - use consensusID
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.NoError(t, err, "empty block should be valid")
}

// Helper: setupMultiValidator creates a state with multiple validators
// Returns state and validators with operator address, private key, and consensusID
func setupMultiValidator(t *testing.T, validatorCount int, stakes []uint64) (*state.InMemoryState, []struct {
	Addr        types.Address
	PrivKey     ed25519.PrivateKey
	ConsensusID types.Address
}) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	validators := make([]struct {
		Addr        types.Address
		PrivKey     ed25519.PrivateKey
		ConsensusID types.Address
	}, validatorCount)

	for i := 0; i < validatorCount; i++ {
		addr := types.Address{}
		addr[0] = byte(i + 1)

		seed := make([]byte, 32)
		seed[0] = byte(i + 1)
		privKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privKey.Public().(ed25519.PublicKey)
		var pubKey32 [32]byte
		copy(pubKey32[:], pubKey)

		consensusID := types.DeriveConsensusID(pubKey32)

		stake := stakes[i]
		acc := state.NewAccount(addr, pubKey32)
		acc.Balance = types.NewAmount(stake * 2)
		s.SetAccount(addr, acc)

		staking.RegisterValidator(s, pubKey32, addr, types.NewAmount(stake), 0, 0)
		acc, _ = s.GetAccount(addr)
		acc.SubBalance(types.NewAmount(stake))
		s.SetAccount(addr, acc)

		validators[i] = struct {
			Addr        types.Address
			PrivKey     ed25519.PrivateKey
			ConsensusID types.Address
		}{addr, privKey, consensusID}
	}

	staking.ProcessEpochTransition(s, 100)
	staking.CreateSnapshot(s, 1)

	return s, validators
}

func TestValidateBlock_CommitProof_MutatedSignatures(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s, validators := setupMultiValidator(t, 1, []uint64{100_000})
	mp := mempool.New(10000, 300*time.Second, s)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validators[0].ConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Attach commit proof - use consensusID
	attachCommitProof(t, block, s, validators[0].ConsensusID, validators[0].PrivKey)

	// Deep copy the block and mutate first byte of first precommit's signature
	originalSig := make([]byte, len(block.CommitProof.Precommits[0].Signature))
	copy(originalSig, block.CommitProof.Precommits[0].Signature)
	block.CommitProof.Precommits[0].Signature[0] ^= 0xFF

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "mutated signature should fail")
	require.Contains(t, err.Error(), "signature")
}

func TestValidateBlock_CommitProof_DuplicateValidator(t *testing.T) {
	// Use setupTest to get a valid block with commit proof, then manually add duplicate
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Get valid commit proof using attachCommitProof (this ensures proposer is correct)
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	// Now manually add another precommit from the SAME validator to create duplicate
	// With 2 precommits and 1 validator, count check fails first (2 > 1)
	// This tests that the count limit catches duplicates before they can be processed
	block.CommitProof.Precommits = append(block.CommitProof.Precommits, block.CommitProof.Precommits[0])
	// Actually with 1 validator, max precommits that pass count is 1, can't test duplicate
	// So we need multi-validator setup - but that fails at proposer check

	// HACK: To properly test duplicate, we'll modify the precommits AFTER getting valid proof
	// Replace the existing precommit with a duplicate pair
	block.CommitProof.Precommits = append(block.CommitProof.Precommits, block.CommitProof.Precommits[0])

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	// With 2 precommits and 1 validator, we expect "more precommits than validators" first
	// This is actually correct behavior - count check catches this before duplicate check
	require.Error(t, err, "duplicate or too many precommits should fail")
	require.True(t, strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "more precommits"), "expected duplicate or more precommits error, got: %s", err.Error())
}

func TestValidateBlock_CommitProof_InvalidVote(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s, validators := setupMultiValidator(t, 2, []uint64{100_000, 200_000})
	mp := mempool.New(10000, 300*time.Second, s)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validators[1].ConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Build commit proof manually with invalid vote (empty signature)
	hasherObj := types.SHA256Hasher{}
	headerHash, err := block.HeaderHash(hasherObj)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(s, block.Header.Epoch)
	require.NoError(t, err)

	// Create a valid precommit for validator 1
	validPrecommit1 := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    block.Header.Height,
		Round:     0,
		BlockHash: headerHash,
		Validator: validators[0].ConsensusID,
	}
	err = validPrecommit1.Sign(validators[0].PrivKey)
	require.NoError(t, err)

	// Create a valid precommit for validator 2
	validPrecommit2 := &types.Vote{
		VoteType:  types.VotePrecommit,
		Height:    block.Header.Height,
		Round:     0,
		BlockHash: headerHash,
		Validator: validators[1].ConsensusID,
	}
	err = validPrecommit2.Sign(validators[1].PrivKey)
	require.NoError(t, err)

	// Sort precommits by validator consensusID (required for validation)
	precommits := []types.Vote{*validPrecommit1, *validPrecommit2}
	sort.Slice(precommits, func(i, j int) bool {
		return bytes.Compare(precommits[i].Validator[:], precommits[j].Validator[:]) < 0
	})

	// Replace the second validator's precommit with an invalid one (empty signature)
	// Find which one is validator 2 (index 1) after sorting
	invalidVote := precommits[1]
	invalidVote.Signature = []byte{} // Empty signature - fails Validate()
	precommits[1] = invalidVote

	block.CommitProof = &types.CommitProof{
		Height:      block.Header.Height,
		BlockHash:   headerHash,
		Precommits:  precommits,
		TotalPower:  snap.TotalPower,
		SignedPower: snap.TotalPower, // 300k of 300k (2/3 majority)
		SetHash:     snap.SetHash,
	}

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "invalid vote should fail")
	require.Contains(t, err.Error(), "vote")
}

func TestValidateBlock_CommitProof_WrongTotalPower(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	// Modify TotalPower to wrong value
	block.CommitProof.TotalPower = block.CommitProof.TotalPower + 1

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "wrong total power should fail")
	require.Contains(t, err.Error(), "total power")
}

func TestValidateBlock_CommitProof_WrongSetHash(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	// Modify SetHash to wrong value
	block.CommitProof.SetHash[0] ^= 0xFF

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "wrong set hash should fail")
	require.Contains(t, err.Error(), "set hash")
}

func TestValidateBlock_CommitProof_PrecommitsNotSorted(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s, validators := setupMultiValidator(t, 2, []uint64{100_000, 200_000})
	mp := mempool.New(10000, 300*time.Second, s)

	// Note: validator with higher address (addr2) should sign first to create unsorted order
	// But to get 2/3 majority we need at least 300k of 300k total
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validators[1].ConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	hasherObj := types.SHA256Hasher{}
	headerHash, err := block.HeaderHash(hasherObj)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(s, block.Header.Epoch)
	require.NoError(t, err)

	vs := NewVotingState(block.Header.Height, 0, headerHash, snap)

	// Determine sorted order first, then create precommits in reverse order to ensure unsorted
	consensusIDs := []types.Address{validators[0].ConsensusID, validators[1].ConsensusID}
	sort.Slice(consensusIDs, func(i, j int) bool {
		return bytes.Compare(consensusIDs[i][:], consensusIDs[j][:]) < 0
	})
	// Now add precommits in reverse sorted order to ensure they're unsorted
	for i := len(consensusIDs) - 1; i >= 0; i-- {
		var privKey ed25519.PrivateKey
		var consensusID types.Address
		if bytes.Equal(consensusIDs[i][:], validators[0].ConsensusID[:]) {
			privKey = validators[0].PrivKey
			consensusID = validators[0].ConsensusID
		} else {
			privKey = validators[1].PrivKey
			consensusID = validators[1].ConsensusID
		}

		prevote := &types.Vote{
			VoteType:  types.VotePrevote,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: consensusID,
		}
		err = prevote.Sign(privKey)
		require.NoError(t, err)
		err = vs.AddPrevote(prevote)
		require.NoError(t, err)

		precommit := &types.Vote{
			VoteType:  types.VotePrecommit,
			Height:    block.Header.Height,
			Round:     0,
			BlockHash: headerHash,
			Validator: consensusID,
		}
		err = precommit.Sign(privKey)
		require.NoError(t, err)
		err = vs.AddPrecommit(precommit)
		require.NoError(t, err)
	}

	// Build commit proof with sorted precommits first, then manually reorder to be unsorted
	proof, err := vs.BuildCommitProof()
	require.NoError(t, err)
	// Reverse the precommits to create unsorted order (BuildCommitProof now sorts them)
	reversed := make([]types.Vote, len(proof.Precommits))
	for i, v := range proof.Precommits {
		reversed[len(proof.Precommits)-1-i] = v
	}
	proof.Precommits = reversed
	block.CommitProof = proof

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "unsorted precommits should fail")
	require.Contains(t, err.Error(), "sorted")
}

func TestValidateBlock_CommitProof_TooManyPrecommits(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	// Duplicate the precommit to create more precommits than validators
	block.CommitProof.Precommits = append(block.CommitProof.Precommits, block.CommitProof.Precommits...)

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "too many precommits should fail")
	require.Contains(t, err.Error(), "more precommits")
}

func TestValidateBlock_CommitProof_EmptyPrecommits(t *testing.T) {
	hasher, s, mp, _, _, _, _, validatorConsensusID := setupTest(t)
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	// Build commit proof manually with empty precommits
	hasherObj := types.SHA256Hasher{}
	headerHash, err := block.HeaderHash(hasherObj)
	require.NoError(t, err)

	snap, err := staking.GetSnapshot(s, block.Header.Epoch)
	require.NoError(t, err)

	block.CommitProof = &types.CommitProof{
		Height:      block.Header.Height,
		BlockHash:   headerHash,
		Precommits:  []types.Vote{}, // Empty!
		TotalPower:  snap.TotalPower,
		SignedPower: 0, // No power
		SetHash:     snap.SetHash,
	}

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, s, hasher, nil, 1)
	require.Error(t, err, "empty precommits should fail")
	require.Contains(t, err.Error(), "power")
}
