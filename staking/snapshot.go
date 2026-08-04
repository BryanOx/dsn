package staking

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/BryanOx/dsn/types"
)

// Snapshot key prefixes
const (
	keySnapshotPrefix = "snapshot/"
	keyLatestEpoch    = "snapshot/latest"
)

// ValidatorSnapshot represents a frozen snapshot of the validator set at an epoch.
// This is used for deterministic block validation and replay.
type ValidatorSnapshot struct {
	Epoch      uint64
	Validators []*Validator // sorted by VotingPower DESC, ConsensusID ASC
	TotalPower uint64
	SetHash    types.Hash

	// Runtime indexes (NOT serialized — rebuilt on UnmarshalBinary)
	byID map[types.Address]*Validator // ConsensusID → Validator
}

// buildIndexes rebuilds the runtime indexes for O(1) lookup.
func (s *ValidatorSnapshot) buildIndexes() {
	s.byID = make(map[types.Address]*Validator)
	for _, v := range s.Validators {
		s.byID[v.ConsensusID] = v
	}
}

// CreateSnapshot computes and persists a validator set snapshot for the given epoch.
// Gets active validators from the staking state, computes total power and set hash.
// Stores in kvstore with key "snapshot/{epoch}" and updates "snapshot/latest".
func CreateSnapshot(s StakingState, epoch uint64) (*ValidatorSnapshot, error) {
	// Get active validators
	active, err := GetActiveValidators(s)
	if err != nil {
		return nil, fmt.Errorf("get active validators: %w", err)
	}

	// Sort validators: VotingPower DESC, ID ASC (must match GetActiveValidators ordering)
	sortedVals := make([]*Validator, len(active))
	copy(sortedVals, active)

	// Calculate total voting power
	var totalPower uint64
	for _, v := range sortedVals {
		totalPower += v.VotingPower
	}

	// Compute set hash: hash(concatenated serialized validators via SHA256)
	setHash := computeValidatorSetHash(sortedVals)

	// Create snapshot
	snap := &ValidatorSnapshot{
		Epoch:      epoch,
		Validators: sortedVals,
		TotalPower: totalPower,
		SetHash:    setHash,
	}

	// Serialize and store
	data, err := snap.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	// Store with key "snapshot/{epoch}"
	key := fmt.Sprintf("%s%d", keySnapshotPrefix, epoch)
	if err := s.SetBytes(key, data); err != nil {
		return nil, fmt.Errorf("store snapshot: %w", err)
	}

	// Update latest epoch
	if err := setLatestSnapshotEpoch(s, epoch); err != nil {
		return nil, fmt.Errorf("update latest epoch: %w", err)
	}

	return snap, nil
}

// GetSnapshot retrieves a snapshot for the given epoch.
// Returns nil, nil if not found (no error).
func GetSnapshot(s KVStore, epoch uint64) (*ValidatorSnapshot, error) {
	key := fmt.Sprintf("%s%d", keySnapshotPrefix, epoch)
	data, ok := s.GetBytes(key)
	if !ok {
		return nil, nil
	}

	snap := &ValidatorSnapshot{}
	if err := snap.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot for epoch %d: %w", epoch, err)
	}

	return snap, nil
}

// LatestSnapshotEpoch returns the most recent epoch with a snapshot.
func LatestSnapshotEpoch(s KVStore) (uint64, error) {
	val, ok := s.GetBytes(keyLatestEpoch)
	if !ok {
		return 0, nil
	}
	return binary.BigEndian.Uint64(val), nil
}

func setLatestSnapshotEpoch(s KVStore, epoch uint64) error {
	// Only update if this epoch is newer than the current latest
	current, err := LatestSnapshotEpoch(s)
	if err != nil {
		return err
	}
	if epoch <= current {
		return nil // already have a more recent snapshot
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], epoch)
	return s.SetBytes(keyLatestEpoch, buf[:])
}

// computeValidatorSetHash computes SHA256 hash of concatenated serialized validators.
// Returns zero hash for empty validator set (sentinel value for no validators).
func computeValidatorSetHash(validators []*Validator) types.Hash {
	if len(validators) == 0 {
		return types.Hash{}
	}

	var buf bytes.Buffer

	for _, v := range validators {
		var vBuf bytes.Buffer
		// Serialize each validator
		v.Encode(&vBuf)
		buf.Write(vBuf.Bytes())
	}

	// Hash the concatenation
	hasher := types.SHA256Hasher{}
	h, _ := hasher.Hash(buf.Bytes())
	return h
}

// MarshalBinary serializes the snapshot to binary.
// Layout: Epoch(8) + ValidatorCount(8) + [Validator entries: each EncodedValidator(103)] + TotalPower(8) + SetHash(32)
func (s *ValidatorSnapshot) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Epoch
	if err := binary.Write(buf, binary.BigEndian, s.Epoch); err != nil {
		return nil, err
	}

	// Validator count
	if err := binary.Write(buf, binary.BigEndian, uint64(len(s.Validators))); err != nil {
		return nil, err
	}

	// Each validator (103 bytes each)
	for _, v := range s.Validators {
		if err := v.Encode(buf); err != nil {
			return nil, err
		}
	}

	// Total power
	if err := binary.Write(buf, binary.BigEndian, s.TotalPower); err != nil {
		return nil, err
	}

	// Set hash
	buf.Write(s.SetHash[:])

	return buf.Bytes(), nil
}

// UnmarshalBinary deserializes the snapshot from binary.
func (s *ValidatorSnapshot) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)

	// Epoch
	if err := binary.Read(r, binary.BigEndian, &s.Epoch); err != nil {
		return err
	}

	// Validator count
	var count uint64
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return err
	}

	// Read validators
	s.Validators = make([]*Validator, count)
	for i := uint64(0); i < count; i++ {
		v := &Validator{}
		if err := v.Decode(r); err != nil {
			return fmt.Errorf("validator %d: %w", i, err)
		}
		s.Validators[i] = v
	}

	// Total power
	if err := binary.Read(r, binary.BigEndian, &s.TotalPower); err != nil {
		return err
	}

	// Set hash
	if _, err := r.Read(s.SetHash[:]); err != nil {
		return err
	}

	// Build runtime indexes for O(1) lookup
	s.buildIndexes()

	return nil
}

// Ensure ValidatorSnapshot implements sort.Interface for deterministic ordering
var _ sort.Interface = (*ValidatorSnapshot)(nil)

func (s *ValidatorSnapshot) Len() int {
	return len(s.Validators)
}

func (s *ValidatorSnapshot) Less(i, j int) bool {
	if s.Validators[i].VotingPower != s.Validators[j].VotingPower {
		return s.Validators[i].VotingPower > s.Validators[j].VotingPower
	}
	return bytes.Compare(s.Validators[i].ConsensusID[:], s.Validators[j].ConsensusID[:]) < 0
}

func (s *ValidatorSnapshot) Swap(i, j int) {
	s.Validators[i], s.Validators[j] = s.Validators[j], s.Validators[i]
}

// ValidatorByID returns the validator by consensus ID.
// Returns error if consensus ID not found.
func (s *ValidatorSnapshot) ValidatorByID(consensusID types.Address) (*Validator, error) {
	if s.byID == nil {
		s.buildIndexes() // lazy init
	}
	v, ok := s.byID[consensusID]
	if !ok {
		return nil, fmt.Errorf("validator %s not found in snapshot epoch %d", consensusID, s.Epoch)
	}
	return v, nil
}

// ValidatorByPubKey returns the validator by public key.
// Returns error if public key not found.
func (s *ValidatorSnapshot) ValidatorByPubKey(pubKey [32]byte) (*Validator, error) {
	if s.byID == nil {
		s.buildIndexes() // lazy init
	}
	for _, v := range s.Validators {
		if v.PublicKey == pubKey {
			return v, nil
		}
	}
	return nil, fmt.Errorf("validator with pubkey %x not found in snapshot epoch %d", pubKey[:], s.Epoch)
}

// GetValidatorPower returns the voting power of a validator by consensus ID.
func (s *ValidatorSnapshot) GetValidatorPower(consensusID types.Address) (uint64, error) {
	v, err := s.ValidatorByID(consensusID)
	if err != nil {
		return 0, err
	}
	return v.VotingPower, nil
}

// HasValidator returns true if the consensus ID is in this snapshot's validator set.
func (s *ValidatorSnapshot) HasValidator(consensusID types.Address) bool {
	if s.byID == nil {
		s.buildIndexes() // lazy init
	}
	_, ok := s.byID[consensusID]
	return ok
}
