package staking

import (
	"testing"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// mockState implements StakingState for testing
type mockState struct {
	accounts map[types.Address]*state.Account
	kv      map[string][]byte
}

func newMockState() *mockState {
	return &mockState{
		accounts: make(map[types.Address]*state.Account),
		kv:       make(map[string][]byte),
	}
}

func (m *mockState) GetAccount(addr types.Address) (*state.Account, error) {
	acc, ok := m.accounts[addr]
	if !ok {
		return state.NewAccount(addr, [32]byte{}), nil
	}
	return acc, nil
}

func (m *mockState) SetAccount(addr types.Address, acc *state.Account) error {
	m.accounts[addr] = acc
	return nil
}

func (m *mockState) GetBytes(key string) ([]byte, bool) {
	v, ok := m.kv[key]
	return v, ok
}

func (m *mockState) SetBytes(key string, value []byte) error {
	m.kv[key] = value
	return nil
}

func (m *mockState) DeleteBytes(key string) error {
	delete(m.kv, key)
	return nil
}

// TestCreateAndGetSnapshot tests creating and retrieving a snapshot
func TestCreateAndGetSnapshot(t *testing.T) {
	s := newMockState()

	// Create a validator
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	// Fund account and register validator
	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(addr, acc)

	// Register validator
	cid, err := RegisterValidator(s, pubKey, addr, types.NewAmount(100_000), 0, 0)
	require.NoError(t, err)
	require.Equal(t, types.DeriveConsensusID(pubKey), cid)

	// Process epoch transition to activate
	err = ProcessEpochTransition(s, 100)
	require.NoError(t, err)

	// Create snapshot for epoch 1
	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.Equal(t, uint64(1), snap.Epoch)
	require.Equal(t, 1, len(snap.Validators))
	require.Equal(t, uint64(100_000), snap.TotalPower)

	// Get snapshot
	retrieved, err := GetSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, snap.Epoch, retrieved.Epoch)
	require.Equal(t, snap.TotalPower, retrieved.TotalPower)
	require.Equal(t, snap.SetHash, retrieved.SetHash)
	require.Equal(t, 1, len(retrieved.Validators))
}

// TestSnapshotRoundTrip tests binary encode/decode round-trip
func TestSnapshotRoundTrip(t *testing.T) {
	v1 := &Validator{
		BondedStake:  types.NewAmount(100),
		VotingPower:  100,
		ConsensusID:  types.Address{1},
	}
	v2 := &Validator{
		BondedStake:  types.NewAmount(200),
		VotingPower:  200,
		ConsensusID:  types.Address{2},
	}

	original := &ValidatorSnapshot{
		Epoch:      5,
		Validators: []*Validator{v2, v1}, // Will be sorted
		TotalPower: 300,
		SetHash:    types.Hash{1, 2, 3},
	}

	// Encode
	data, err := original.MarshalBinary()
	require.NoError(t, err)

	// Decode
	decoded := &ValidatorSnapshot{}
	err = decoded.UnmarshalBinary(data)
	require.NoError(t, err)

	require.Equal(t, original.Epoch, decoded.Epoch)
	require.Equal(t, original.TotalPower, decoded.TotalPower)
	require.Equal(t, original.SetHash, decoded.SetHash)
	// Validators should be sorted
	require.Equal(t, 2, len(decoded.Validators))
	require.Equal(t, uint64(200), decoded.Validators[0].VotingPower)
	require.Equal(t, uint64(100), decoded.Validators[1].VotingPower)
}

// TestSnapshotSetHash tests that same validators produce same hash
func TestSnapshotSetHash(t *testing.T) {
	s1 := newMockState()
	s2 := newMockState()

	// Use IDENTICAL validator configuration in both states
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	// Setup state 1
	acc1 := state.NewAccount(addr, pubKey)
	acc1.Balance = types.NewAmount(200_000)
	s1.SetAccount(addr, acc1)
	RegisterValidator(s1, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s1, 100)

	// Setup state 2 (same address and pubkey as state 1)
	acc2 := state.NewAccount(addr, pubKey)
	acc2.Balance = types.NewAmount(200_000)
	s2.SetAccount(addr, acc2)
	RegisterValidator(s2, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s2, 100)

	// Create snapshots
	snap1, err := CreateSnapshot(s1, 1)
	require.NoError(t, err)

	snap2, err := CreateSnapshot(s2, 1)
	require.NoError(t, err)

	// Hashes should match for same validator configuration
	require.Equal(t, snap1.SetHash, snap2.SetHash)
}

// TestSnapshotOrdering tests that validators are sorted by VotingPower DESC, ID ASC
func TestSnapshotOrdering(t *testing.T) {
	s := newMockState()

	// Create validators with different voting powers
	addrs := make([]types.Address, 3)
	pubKeys := [][32]byte{{1}, {2}, {3}}
	stakes := []uint64{100_000, 300_000, 200_000} // Different order to test sorting

	for i, addr := range addrs {
		addr, _ = types.AddressFromBytes([]byte{byte(i + 1), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
		addrs[i] = addr

		acc := state.NewAccount(addr, pubKeys[i])
		acc.Balance = types.NewAmount(stakes[i] * 2)
		s.SetAccount(addr, acc)

		RegisterValidator(s, pubKeys[i], addr, types.NewAmount(stakes[i]), 0, 0)
	}

	// Process epoch transition
	ProcessEpochTransition(s, 100)

	// Create snapshot
	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)

	// Should be sorted: 300k (pubKey{2}), 200k (pubKey{3}), 100k (pubKey{1})
	require.Equal(t, 3, len(snap.Validators))
	require.Equal(t, uint64(300_000), snap.Validators[0].VotingPower)
	require.Equal(t, types.DeriveConsensusID(pubKeys[1]), snap.Validators[0].ConsensusID)
	require.Equal(t, uint64(200_000), snap.Validators[1].VotingPower)
	require.Equal(t, types.DeriveConsensusID(pubKeys[2]), snap.Validators[1].ConsensusID)
	require.Equal(t, uint64(100_000), snap.Validators[2].VotingPower)
	require.Equal(t, types.DeriveConsensusID(pubKeys[0]), snap.Validators[2].ConsensusID)
}

// TestEmptySnapshot tests empty validator set creates valid snapshot
func TestEmptySnapshot(t *testing.T) {
	s := newMockState()

	// Create snapshot with no validators
	snap, err := CreateSnapshot(s, 0)
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.Equal(t, uint64(0), snap.Epoch)
	require.Equal(t, 0, len(snap.Validators))
	require.Equal(t, uint64(0), snap.TotalPower)
	// SetHash should be zero for empty set
	require.Equal(t, types.Hash{}, snap.SetHash)

	// Retrieve
	retrieved, err := GetSnapshot(s, 0)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, 0, len(retrieved.Validators))
}

// TestLatestSnapshotEpoch tests latest tracking works
func TestLatestSnapshotEpoch(t *testing.T) {
	s := newMockState()

	// Initially no snapshots
	epoch, err := LatestSnapshotEpoch(s)
	require.NoError(t, err)
	require.Equal(t, uint64(0), epoch)

	// Create snapshots at different epochs
	CreateSnapshot(s, 2)
	CreateSnapshot(s, 5)
	CreateSnapshot(s, 1)

	// Latest should be 5
	epoch, err = LatestSnapshotEpoch(s)
	require.NoError(t, err)
	require.Equal(t, uint64(5), epoch)
}

// TestSnapshotGetValidatorPubKey tests lookup of existing validator
func TestSnapshotGetValidatorPubKey(t *testing.T) {
	s := newMockState()

	// Create and activate validator
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(addr, acc)

	RegisterValidator(s, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s, 100)

	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)

	// Get validator by ConsensusID and check pubkey
	consensusID := types.DeriveConsensusID(pubKey)
	v, err := snap.ValidatorByID(consensusID)
	require.NoError(t, err)
	require.Equal(t, pubKey, v.PublicKey)
}

// TestSnapshotGetValidatorPubKey_NotFound tests lookup of non-existent validator
func TestSnapshotGetValidatorPubKey_NotFound(t *testing.T) {
	s := newMockState()

	// Create and activate validator
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(addr, acc)

	RegisterValidator(s, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s, 100)

	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)

	// Try to get non-existent validator (using a ConsensusID that doesn't exist)
	nonExistentCID := types.Address{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	_, err = snap.ValidatorByID(nonExistentCID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestSnapshotHasValidator tests HasValidator returns correct values
func TestSnapshotHasValidator(t *testing.T) {
	s := newMockState()

	// Create and activate validator
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(addr, acc)

	RegisterValidator(s, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s, 100)

	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)

	// Existing validator should return true (now takes ConsensusID)
	consensusID := types.DeriveConsensusID(pubKey)
	require.True(t, snap.HasValidator(consensusID), "existing validator should return true")

	// Non-existing should return false
	nonExistentCID := types.Address{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	require.False(t, snap.HasValidator(nonExistentCID), "non-existing validator should return false")
}

// TestSnapshotGetValidatorPower tests power lookup
func TestSnapshotGetValidatorPower(t *testing.T) {
	s := newMockState()

	// Create and activate validator
	addr, _ := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [32]byte
	pubKey[0] = 1

	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(200_000)
	s.SetAccount(addr, acc)

	RegisterValidator(s, pubKey, addr, types.NewAmount(100_000), 0, 0)
	ProcessEpochTransition(s, 100)

	snap, err := CreateSnapshot(s, 1)
	require.NoError(t, err)

	// Get power (now takes ConsensusID, not operator address)
	consensusID := types.DeriveConsensusID(pubKey)
	power, err := snap.GetValidatorPower(consensusID)
	require.NoError(t, err)
	require.Equal(t, uint64(100_000), power)
}