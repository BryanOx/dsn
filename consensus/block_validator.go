package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/vm"
)

var (
	ErrEventsRootMismatch = errors.New("events root mismatch")
)

var (
	ErrWrongHeight           = errors.New("block height does not follow parent")
	ErrWrongPreviousHash     = errors.New("previous hash mismatch")
	ErrWrongProposer         = errors.New("wrong proposer for this height")
	ErrInvalidSignature      = errors.New("block signature is invalid")
	ErrWrongValidatorRoot    = errors.New("validator root mismatch")
	ErrStateRootMismatch     = errors.New("state root mismatch")
	ErrTxRootMismatch        = errors.New("transaction root mismatch")
	ErrFeeSummaryMismatch    = errors.New("fee summary mismatch")
	ErrWrongValidatorSetHash = errors.New("validator set hash mismatch")
	ErrWrongEpoch            = errors.New("epoch mismatch")
	ErrInvalidCommitProof    = errors.New("invalid commit proof")
)

// ValidateBlock validates a block using the given state and hasher.
// If vm is provided, contract transactions will be re-executed using the VM
// to verify state root correctness. The block must carry a valid commit proof
// (all non-genesis blocks do).
func ValidateBlock(block *types.Block, parentHeader *types.BlockHeader,
	expectedPrevHash types.Hash, s *state.InMemoryState, hasher types.Hasher,
	vm *vm.VM, blockTimeSec uint64) error {

	return validateBlock(block, parentHeader, expectedPrevHash, s, hasher, vm, blockTimeSec, false, false)
}

// ValidateBlockForSync validates a block being replayed during block sync
// (snapshot restore tail or fresh-node catch-up). It is identical to
// ValidateBlock except that the wall-clock timestamp drift check is skipped:
// replayed blocks were produced by the peer seconds ago and the ±5s live-gossip
// freshness window does not apply to historical replay. Chain-relative checks
// (epoch, proposer, validator set, hashes, commit proof, re-executed state
// root) are unchanged and remain the integrity guarantee.
func ValidateBlockForSync(block *types.Block, parentHeader *types.BlockHeader,
	expectedPrevHash types.Hash, s *state.InMemoryState, hasher types.Hasher,
	vm *vm.VM, blockTimeSec uint64) error {

	return validateBlock(block, parentHeader, expectedPrevHash, s, hasher, vm, blockTimeSec, false, true)
}

// ValidateBlockProposal validates a block proposal that does not yet carry a
// commit proof. Used by non-proposer validators before voting: a proposal is
// still signed and checked for correctness, but the 2/3 precommit proof is
// only produced after the voting round completes.
func ValidateBlockProposal(block *types.Block, parentHeader *types.BlockHeader,
	expectedPrevHash types.Hash, s *state.InMemoryState, hasher types.Hasher,
	vm *vm.VM, blockTimeSec uint64) error {

	return validateBlock(block, parentHeader, expectedPrevHash, s, hasher, vm, blockTimeSec, true, false)
}

// validateBlock contains the full validation pipeline shared by ValidateBlock,
// ValidateBlockForSync and ValidateBlockProposal. When skipCommitProof is true
// the commit proof requirement is omitted (used for proposals that have not
// been voted on yet). When skipTimeDrift is true the ±5s wall-clock timestamp
// check is omitted (used when replaying historical blocks during sync).
func validateBlock(block *types.Block, parentHeader *types.BlockHeader,
	expectedPrevHash types.Hash, s *state.InMemoryState, hasher types.Hasher,
	vm *vm.VM, blockTimeSec uint64, skipCommitProof, skipTimeDrift bool) error {

	// Height continuity is only checkable when the parent header is present:
	// a fresh node replaying from the first block (or a node restoring from a
	// snapshot) has no parent stored, and the re-executed state-root
	// transition below is the integrity guarantee there.
	if parentHeader != nil && block.Header.Height != parentHeader.Height+1 {
		return fmt.Errorf("%w: expected %d, got %d", ErrWrongHeight,
			parentHeader.Height+1, block.Header.Height)
	}

	// Skip PreviousHash check for genesis block (height 0) and when the parent
	// header is unavailable (fresh node or snapshot-restore boundary, where
	// loadParentHeader falls back to the zero hash): the parent chain is
	// legitimately absent there, so the re-executed state-root transition below
	// is the integrity guarantee instead of the link hash.
	if block.Header.Height > 0 && expectedPrevHash != (types.Hash{}) {
		if block.Header.PreviousHash != expectedPrevHash {
			return fmt.Errorf("%w: expected %x, got %x", ErrWrongPreviousHash,
				expectedPrevHash, block.Header.PreviousHash)
		}
	}

	// Header freshness: reject timestamps that drift beyond the allowed skew.
	// Contracts execute with this timestamp, so it must be sane before execution.
	// Skipped when replaying historical blocks during sync (skipTimeDrift):
	// those blocks were produced seconds ago by a peer and the ±5s live
	// freshness window does not apply; chain-relative integrity holds instead.
	if !skipTimeDrift {
		if err := types.ValidateTimestamp(block.Header.Timestamp); err != nil {
			return fmt.Errorf("%w: %v", types.ErrInvalidTimestamp, err)
		}
	}

	// Monotonicity: a child block must not carry an older timestamp than its parent.
	if parentHeader != nil && parentHeader.Timestamp > block.Header.Timestamp {
		return fmt.Errorf("%w: parent timestamp %d is after block timestamp %d",
			types.ErrInvalidTimestamp, parentHeader.Timestamp, block.Header.Timestamp)
	}

	// 0. Process evidence BEFORE BeginBlock (same as BuildBlock — ensures deterministic replay)
	for _, ev := range block.Evidence {
		if _, err := ProcessEvidence(s, ev, block.Header.Height); err != nil {
			return fmt.Errorf("process evidence: %w", err)
		}
	}

	// 1. Run BeginBlock on fresh state (same as BuildBlock — ensures deterministic replay)
	_, _, err := BeginBlock(s, block.Header.Height, blockTimeSec)
	if err != nil {
		return fmt.Errorf("begin block: %w", err)
	}

	// 2. Derive expected proposer from state's active validators
	activeVals, err := staking.GetActiveValidators(s)
	if err != nil {
		return fmt.Errorf("get active validators: %w", err)
	}

	// 3. Verify epoch field
	expectedEpoch, _ := staking.CurrentEpoch(s)
	if block.Header.Epoch != expectedEpoch {
		return fmt.Errorf("%w: expected %d, got %d", ErrWrongEpoch,
			expectedEpoch, block.Header.Epoch)
	}

	// 4. Verify validator set hash
	snap, _ := staking.GetSnapshot(s, expectedEpoch)
	if snap == nil {
		return fmt.Errorf("no snapshot for epoch %d", expectedEpoch)
	}
	if block.Header.ValidatorSetHash != snap.SetHash {
		return fmt.Errorf("%w: expected %x, got %x", ErrWrongValidatorSetHash,
			snap.SetHash, block.Header.ValidatorSetHash)
	}

	// 5. Verify proposer
	if len(activeVals) > 0 {
		expectedProposer := WeightedProposerAtHeight(block.Header.Height, activeVals)
		if block.Header.Proposer != expectedProposer {
			return fmt.Errorf("%w: expected %x, got %x", ErrWrongProposer,
				expectedProposer, block.Header.Proposer)
		}
	} else {
		// If no active validators, we can't validate proposer - this is an edge case
		// In production there should always be at least 1 active validator
		return fmt.Errorf("%w: no active validators in state", ErrWrongProposer)
	}

	// 6. Verify validator root
	// GetActiveSetAddresses returns operator addresses, not ConsensusIDs.
	// Since we already have activeVals (line 55), extract ConsensusIDs directly.
	consensusIDs := make([]types.Address, len(activeVals))
	for i, v := range activeVals {
		consensusIDs[i] = v.ConsensusID
	}
	expectedValRoot := ValidatorRoot(consensusIDs)
	if block.Header.ValidatorRoot != expectedValRoot {
		return fmt.Errorf("%w: expected %x, got %x", ErrWrongValidatorRoot,
			expectedValRoot, block.Header.ValidatorRoot)
	}

	if len(block.Signature) == 0 {
		return ErrInvalidSignature
	}

	// 7. Execute user txs (uses fresh state from BeginBlock)
	var totalFees uint64
	var allEvents []types.Event
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		exec, err := ApplyTransaction(s, tx, &block.Header, vm, hasher, uint32(i))
		if err != nil {
			return fmt.Errorf("tx %x: %w", tx.IntentID, err)
		}
		totalFees += exec.Fee
		allEvents = append(allEvents, exec.Events...)
	}

	// 7b. FinalizeBlock - distribute fees (same as BuildBlock)
	partialBlock := &types.Block{
		FeeSummary: types.NewFeeSummary(totalFees),
	}
	if err := FinalizeBlock(s, partialBlock); err != nil {
		return fmt.Errorf("finalize block: %w", err)
	}

	// T4-8: Verify EventsRoot - compute from executed events and compare with header
	computedEventsRoot := types.ComputeEventsRoot(allEvents)
	if computedEventsRoot != block.Header.EventsRoot {
		return fmt.Errorf("%w: computed %x, block has %x",
			ErrEventsRootMismatch, computedEventsRoot[:], block.Header.EventsRoot[:])
	}

	stateRoot, err := s.Commit()
	if err != nil {
		return err
	}
	if stateRoot != block.Header.StateRoot {
		return fmt.Errorf("%w: computed %x, block has %x",
			ErrStateRootMismatch, stateRoot, block.Header.StateRoot)
	}

	txRoot := computeTxRoot(block.Transactions, hasher)
	if txRoot != block.Header.TxRoot {
		return fmt.Errorf("%w: computed %x, block has %x",
			ErrTxRootMismatch, txRoot, block.Header.TxRoot)
	}

	expectedFees := types.NewFeeSummary(totalFees)
	if expectedFees != block.FeeSummary {
		return fmt.Errorf("%w: computed TotalFees=%d, block has %d",
			ErrFeeSummaryMismatch, expectedFees.TotalFees, block.FeeSummary.TotalFees)
	}

	// 8. Verify commit proof (if present - genesis block has nil proof)
	if !skipCommitProof {
		if err := validateCommitProof(block, s, expectedEpoch); err != nil {
			return fmt.Errorf("commit proof: %w", err)
		}
	}

	return nil
}

// validateCommitProof verifies the block's commit proof against the validator snapshot.
func validateCommitProof(block *types.Block, s *state.InMemoryState, epoch uint64) error {
	if block.Header.Height == 0 {
		return nil // genesis has no commit proof
	}

	proof := block.CommitProof
	if proof == nil {
		return fmt.Errorf("%w: nil proof", ErrInvalidCommitProof)
	}

	// Get snapshot for this epoch
	snap, err := staking.GetSnapshot(s, epoch)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	if snap == nil {
		return fmt.Errorf("%w: no snapshot for epoch %d", ErrInvalidCommitProof, epoch)
	}

	// Verify set hash matches
	if proof.SetHash != snap.SetHash {
		return fmt.Errorf("%w: set hash mismatch", ErrInvalidCommitProof)
	}

	// Verify height and block hash
	if proof.Height != block.Header.Height {
		return fmt.Errorf("%w: height mismatch", ErrInvalidCommitProof)
	}

	// Compute expected block hash
	expectedHash, err := block.HeaderHash(types.SHA256Hasher{})
	if err != nil {
		return err
	}
	if proof.BlockHash != expectedHash {
		return fmt.Errorf("%w: block hash mismatch", ErrInvalidCommitProof)
	}

	// Verify total power matches snapshot
	if proof.TotalPower != snap.TotalPower {
		return fmt.Errorf("%w: total power mismatch", ErrInvalidCommitProof)
	}

	// Verify precommit count doesn't exceed validator count (sanity limit)
	if len(proof.Precommits) > len(snap.Validators) {
		return fmt.Errorf("%w: more precommits (%d) than validators (%d)", ErrInvalidCommitProof, len(proof.Precommits), len(snap.Validators))
	}

	// Verify deterministic ordering: precommits MUST be sorted by validator address ascending
	if len(proof.Precommits) > 1 {
		for i := 1; i < len(proof.Precommits); i++ {
			if bytes.Compare(proof.Precommits[i].Validator[:], proof.Precommits[i-1].Validator[:]) < 0 {
				return fmt.Errorf("%w: precommits not sorted by validator address", ErrInvalidCommitProof)
			}
		}
	}

	// Verify each precommit
	var accumulatedPower uint64
	seen := make(map[types.Address]bool)
	for i, vote := range proof.Precommits {
		// Verify vote structure is valid (invalid vote type, empty signature, etc.)
		if err := vote.Validate(); err != nil {
			return fmt.Errorf("vote %d: %w", i, err)
		}
		if vote.VoteType != types.VotePrecommit {
			return fmt.Errorf("%w: vote %d is not a precommit", ErrInvalidCommitProof, i)
		}
		if vote.Height != proof.Height {
			return fmt.Errorf("%w: vote %d height mismatch", ErrInvalidCommitProof, i)
		}
		if vote.BlockHash != proof.BlockHash {
			return fmt.Errorf("%w: vote %d block hash mismatch", ErrInvalidCommitProof, i)
		}

		// Check no duplicate validators
		if seen[vote.Validator] {
			return fmt.Errorf("%w: duplicate validator %s", ErrInvalidCommitProof, vote.Validator)
		}
		seen[vote.Validator] = true

		// Verify validator is in snapshot and signature is valid
		val, err := snap.ValidatorByID(vote.Validator)
		if err != nil {
			return fmt.Errorf("%w: validator %s in commit proof: %v", ErrInvalidCommitProof, vote.Validator, err)
		}
		pubKey := val.PublicKey
		if !vote.Verify(pubKey) {
			return fmt.Errorf("%w: invalid signature from validator %s", ErrInvalidCommitProof, vote.Validator)
		}
		power := val.VotingPower
		accumulatedPower += power
	}

	// Verify signed power
	if accumulatedPower != proof.SignedPower {
		return fmt.Errorf("%w: signed power %d != accumulated %d", ErrInvalidCommitProof, proof.SignedPower, accumulatedPower)
	}

	// Verify 2/3 majority
	if !proof.HasTwoThirdsMajority() {
		return fmt.Errorf("%w: only %d/%d power", ErrWrongValidatorSetHash, proof.SignedPower, proof.TotalPower)
	}

	return nil
}
