package consensus

import (
	"testing"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// mockStakingState implements staking.StakingState for testing
type mockStakingState struct {
	accounts map[types.Address]*state.Account
	kv       map[string][]byte
}

func newMockStakingState() *mockStakingState {
	return &mockStakingState{
		accounts: make(map[types.Address]*state.Account),
		kv:       make(map[string][]byte),
	}
}

func (m *mockStakingState) GetAccount(addr types.Address) (*state.Account, error) {
	acc, ok := m.accounts[addr]
	if !ok {
		return state.NewAccount(addr, [32]byte{}), nil
	}
	return acc, nil
}

func (m *mockStakingState) SetAccount(addr types.Address, acc *state.Account) error {
	m.accounts[addr] = acc
	return nil
}

func (m *mockStakingState) GetBytes(key string) ([]byte, bool) {
	v, ok := m.kv[key]
	return v, ok
}

func (m *mockStakingState) SetBytes(key string, value []byte) error {
	m.kv[key] = value
	return nil
}

func (m *mockStakingState) DeleteBytes(key string) error {
	delete(m.kv, key)
	return nil
}

// setupValidator creates a funded account and registers/activates a validator
func setupValidator(s *mockStakingState, validatorID uint64, stake uint64) types.Address {
	addr := types.Address{}
	addr[0] = byte(validatorID)

	var pubKey [32]byte
	pubKey[0] = byte(validatorID)

	// Create account with funds
	acc := state.NewAccount(addr, pubKey)
	acc.Balance = types.NewAmount(stake * 2)
	s.SetAccount(addr, acc)

	// Register validator
	_, err := staking.RegisterValidator(s, pubKey, addr, types.NewAmount(stake), 0, 0)
	if err != nil {
		panic(err)
	}

	// Deduct stake from account
	acc, _ = s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(addr, acc)

	return addr
}

// TestBeginBlock_NonBoundary tests no transition at non-boundary heights
func TestBeginBlock_NonBoundary(t *testing.T) {
	s := newMockStakingState()

	// Set blocks per epoch to 100
	staking.SetBlocksPerEpoch(s, 100)

	// Setup validator (activated at epoch 1)
	setupValidator(s, 1, 100_000)
	staking.ProcessEpochTransition(s, 100)

	// At height 50, no transition should occur
	epoch, valSetHash, err := BeginBlock(s, 50, uint64(1))
	require.NoError(t, err)
	require.Equal(t, uint64(1), epoch, "epoch should be 1 at height 50")
	require.NotEqual(t, types.Hash{}, valSetHash, "validator set hash should not be zero")
}

// TestBeginBlock_Boundary tests transition at boundary (height % 100 == 0)
func TestBeginBlock_Boundary(t *testing.T) {
	s := newMockStakingState()

	// Set blocks per epoch to 100
	staking.SetBlocksPerEpoch(s, 100)

	// Setup validator (pending, will activate at epoch 1 at height 100)
	setupValidator(s, 1, 100_000)
	// Don't call ProcessEpochTransition yet - it should be called by BeginBlock

	// At height 100, epoch transition should occur
	epoch, valSetHash, err := BeginBlock(s, 100, uint64(1))
	require.NoError(t, err)
	require.Equal(t, uint64(1), epoch, "epoch should transition to 1 at height 100")
	require.NotEqual(t, types.Hash{}, valSetHash, "validator set hash should exist")
}

// TestBeginBlock_Genesis tests no-op at height 0
func TestBeginBlock_Genesis(t *testing.T) {
	s := newMockStakingState()

	epoch, valSetHash, err := BeginBlock(s, 0, uint64(1))
	require.NoError(t, err)
	require.Equal(t, uint64(0), epoch, "genesis should have epoch 0")
	require.Equal(t, types.Hash{}, valSetHash, "genesis should have zero hash")
}

// TestBeginBlock_SnapshotCreated tests snapshot exists after boundary
func TestBeginBlock_SnapshotCreated(t *testing.T) {
	s := newMockStakingState()

	staking.SetBlocksPerEpoch(s, 100)
	setupValidator(s, 1, 100_000)

	// Process transition at boundary
	BeginBlock(s, 100, uint64(1))

	// Get snapshot for epoch 1
	snap, err := staking.GetSnapshot(s, 1)
	require.NoError(t, err)
	require.NotNil(t, snap, "snapshot should exist for epoch 1")
	require.Equal(t, uint64(1), snap.Epoch)
	require.Equal(t, 1, len(snap.Validators))
}

// TestBeginBlock_ValidatorActivation tests pending validator activates via boundary
func TestBeginBlock_ValidatorActivation(t *testing.T) {
	s := newMockStakingState()

	staking.SetBlocksPerEpoch(s, 100)
	setupValidator(s, 1, 100_000)

	// Process boundary - this activates pending validators
	BeginBlock(s, 100, uint64(1))

	// Check validator is now active
	activeVals, err := staking.GetActiveValidators(s)
	require.NoError(t, err)
	require.Equal(t, 1, len(activeVals), "validator should be active after transition")
	require.Equal(t, staking.ValidatorActive, activeVals[0].Status)
}

// TestBeginBlock_Deterministic tests same state produces same result
func TestBeginBlock_Deterministic(t *testing.T) {
	s1 := newMockStakingState()
	s2 := newMockStakingState()

	staking.SetBlocksPerEpoch(s1, 100)
	staking.SetBlocksPerEpoch(s2, 100)

	// Setup identical validator in both states
	setupValidator(s1, 1, 100_000)
	setupValidator(s2, 1, 100_000)

	// Process at same height
	epoch1, hash1, err := BeginBlock(s1, 100, uint64(1))
	require.NoError(t, err)

	epoch2, hash2, err := BeginBlock(s2, 100, uint64(1))
	require.NoError(t, err)

	// Results should be identical
	require.Equal(t, epoch1, epoch2)
	require.Equal(t, hash1, hash2)
}

// TestFinalizeBlock_Noop tests FinalizeBlock returns nil
func TestFinalizeBlock_Noop(t *testing.T) {
	s := newMockStakingState()

	block := &types.Block{}
	err := FinalizeBlock(s, block)
	require.NoError(t, err)
}
