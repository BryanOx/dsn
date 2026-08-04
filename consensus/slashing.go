package consensus

import (
	"encoding/binary"
	"fmt"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
)

// evidenceProcessedKey returns the kvstore key for tracking processed evidence.
// Key format: "evidence/processed/{hex_hash}"
func evidenceProcessedKey(evidenceHash types.Hash) string {
	return fmt.Sprintf("evidence/processed/%x", evidenceHash[:])
}

// IsEvidenceProcessed checks if evidence with the given hash has already been processed.
// Returns true if already processed (double-slash prevention).
func IsEvidenceProcessed(s staking.KVStore, evidenceHash types.Hash) (bool, error) {
	_, ok := s.GetBytes(evidenceProcessedKey(evidenceHash))
	return ok, nil
}

// MarkEvidenceProcessed records that evidence with the given hash has been processed.
// Persists the height at which it was processed as the value.
func MarkEvidenceProcessed(s staking.KVStore, evidenceHash types.Hash, height uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], height)
	return s.SetBytes(evidenceProcessedKey(evidenceHash), buf[:])
}

// ProcessEvidence validates and applies slashing for a single evidence object.
// Returns the validator ConsensusID that was slashed, or error if evidence is invalid/already processed.
//
// Execution flow:
// 1. Validate the evidence (ev.Validate())
// 2. Check evidence hash is not already processed (double-slash prevention)
// 3. Resolve the slash offense via staking.OffenseFromEvidence
// 4. Extract the validator ConsensusID from the evidence
// 5. Call staking.SlashValidator(s, id, ev) to apply punishment
// 6. Mark evidence as processed
func ProcessEvidence(s staking.StakingState, ev types.Evidence, height uint64) (validatorID types.Address, err error) {
	// 1. Validate the evidence
	if err := ev.Validate(); err != nil {
		return types.Address{}, fmt.Errorf("evidence validation: %w", err)
	}

	// 2. Compute evidence hash and check for duplicates
	hasher := types.SHA256Hasher{}
	hash, err := ev.EvidenceHash(hasher)
	if err != nil {
		return types.Address{}, fmt.Errorf("evidence hash: %w", err)
	}

	processed, err := IsEvidenceProcessed(s, hash)
	if err != nil {
		return types.Address{}, err
	}
	if processed {
		return types.Address{}, fmt.Errorf("%w: evidence %x", types.ErrDuplicateEvidence, hash[:8])
	}

	// 3. For v1, only process DoubleSignEvidence
	doubleSign, ok := ev.(*types.DoubleSignEvidence)
	if !ok {
		return types.Address{}, fmt.Errorf("unsupported evidence type: only DoubleSignEvidence supported in v1")
	}

	// 4. Look up validator by ConsensusID from VoteA
	val, err := staking.GetValidator(s, doubleSign.VoteA.Validator)
	if err != nil {
		return types.Address{}, fmt.Errorf("lookup validator: %w", err)
	}

	// 5. Apply slashing
	validatorID = val.ConsensusID
	if err := staking.SlashValidator(s, validatorID, ev); err != nil {
		return types.Address{}, fmt.Errorf("slash validator %s: %w", validatorID, err)
	}

	// 6. Mark evidence as processed to prevent double-slashing
	if err := MarkEvidenceProcessed(s, hash, height); err != nil {
		return types.Address{}, fmt.Errorf("mark evidence processed: %w", err)
	}

	return validatorID, nil
}

// ProcessBlockEvidence processes evidence embedded in a block during BeginBlock.
// This is called BEFORE BeginBlock so slashing is reflected in epoch transitions and snapshots.
func ProcessBlockEvidence(s staking.StakingState, block *types.Block) error {
	for _, ev := range block.Evidence {
		if _, err := ProcessEvidence(s, ev, block.Header.Height); err != nil {
			return fmt.Errorf("process evidence: %w", err)
		}
	}
	return nil
}
