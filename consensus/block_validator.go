package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
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
// to verify state root correctness.
func ValidateBlock(block *types.Block, parentHeader *types.BlockHeader,
	expectedPrevHash types.Hash, s *state.InMemoryState, hasher types.Hasher,
	vm *vm.VM) error {

	if block.Header.Height != parentHeader.Height+1 {
		return fmt.Errorf("%w: expected %d, got %d", ErrWrongHeight,
			parentHeader.Height+1, block.Header.Height)
	}

	// Skip PreviousHash check for genesis block (height 0)
	if block.Header.Height > 0 {
		if block.Header.PreviousHash != expectedPrevHash {
			return fmt.Errorf("%w: expected %x, got %x", ErrWrongPreviousHash,
				expectedPrevHash, block.Header.PreviousHash)
		}
	}

	// 0. Process evidence BEFORE BeginBlock (same as BuildBlock — ensures deterministic replay)
	for _, ev := range block.Evidence {
		if _, err := ProcessEvidence(s, ev, block.Header.Height); err != nil {
			return fmt.Errorf("process evidence: %w", err)
		}
	}

	// 1. Run BeginBlock on fresh state (same as BuildBlock — ensures deterministic replay)
	_, _, err := BeginBlock(s, block.Header.Height, uint64(1))
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
	blockHeight := block.Header.Height
	blockTimestamp := block.Header.Timestamp
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		txIndex := uint32(i)

		// Check if this is a contract transaction and VM is available
		if tx.TxType == types.TxTypeDeployContract && vm != nil {
			// Decode and execute via VM with real block context
			contractTx, err := tx.DeployContract()
			if err != nil {
				return fmt.Errorf("decode deploy contract: %w", err)
			}
			execResult, err := vm.Execute(contractTx, s, blockHeight, blockTimestamp, txIndex)
			if err != nil {
				return fmt.Errorf("vm execute deploy: %w", err)
			}
			// Only collect events if execution was not reverted
			if !execResult.Reverted {
				allEvents = append(allEvents, execResult.Events...)
			}
			// Charge gas
			if err := chargeGasForValidate(s, tx.Sender, execResult.GasUsed); err != nil {
				return fmt.Errorf("charge gas: %w", err)
			}
			totalFees += tx.MaxFee
		} else if tx.TxType == types.TxTypeCallContract && vm != nil {
			// Decode and execute via VM with real block context
			contractTx, err := tx.CallContract()
			if err != nil {
				return fmt.Errorf("decode call contract: %w", err)
			}
			execResult, err := vm.Execute(contractTx, s, blockHeight, blockTimestamp, txIndex)
			if err != nil {
				return fmt.Errorf("vm execute call: %w", err)
			}
			// Only collect events if execution was not reverted
			if !execResult.Reverted {
				allEvents = append(allEvents, execResult.Events...)
			}
			// Charge gas
			if err := chargeGasForValidate(s, tx.Sender, execResult.GasUsed); err != nil {
				return fmt.Errorf("charge gas: %w", err)
			}
			totalFees += tx.MaxFee
		} else {
			// Standard transaction
			_, err := state.ApplyTransaction(s, tx, hasher, block.Header.Height)
			if err != nil {
				return fmt.Errorf("tx %x: %w", tx.IntentID, err)
			}
			totalFees += tx.MaxFee
		}
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
	if err := validateCommitProof(block, s, expectedEpoch); err != nil {
		return fmt.Errorf("commit proof: %w", err)
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

// chargeGasForValidate deducts the gas cost from the sender's account balance during validation.
func chargeGasForValidate(s *state.InMemoryState, sender types.Address, gasUsed uint64) error {
	// Gas price is 1 token per gas unit (simple model)
	gasCost := types.NewAmount(gasUsed)

	acc, err := s.GetAccount(sender)
	if err != nil {
		return fmt.Errorf("get sender account: %w", err)
	}

	if acc.Balance.Cmp(gasCost) < 0 {
		return fmt.Errorf("insufficient balance for gas: have %s, need %s", acc.Balance, gasCost)
	}

	if err := acc.SubBalance(gasCost); err != nil {
		return err
	}

	return s.SetAccount(sender, acc)
}
