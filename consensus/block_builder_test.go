package consensus

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/BryanOx/dsn/mempool"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

type mockSigner struct{}

func (m *mockSigner) Sign(hash types.Hash) ([]byte, error) {
	sig := make([]byte, 64)
	copy(sig, hash[:])
	return sig, nil
}

// setupTestValidator creates a funded account, registers and activates a validator
func setupTestValidator(s *state.InMemoryState, validatorID uint64, stake uint64) types.Address {
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

	// Process epoch transition to activate validator
	err = staking.ProcessEpochTransition(s, 100)
	if err != nil {
		panic(err)
	}

	// Create snapshot for epoch 1
	_, err = staking.CreateSnapshot(s, 1)
	if err != nil {
		panic(err)
	}

	return addr
}

func TestBuildBlock_Basic(t *testing.T) {
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

	tx := &types.Transaction{
		Version: 1, Nonce: 1, Sender: sender, MaxFee: 10,
		Payload:  types.EncodeTransferPayload(sender, 0),
		IntentID: types.Hash{1}, Timestamp: uint64(time.Now().Unix()),
	}
	tx.Signature = ed25519.Sign(privKey, tx.IntentID[:])
	mp.Submit(tx)

	// Setup validator through staking (use different address from sender)
	validatorAddr := setupTestValidator(s, 100, 100_000)
	staking.WriteUint64(s, staking.KeyTotalSupply, 10_000_000)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorAddr, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.Header.Height)
	require.Equal(t, validatorAddr, block.Header.Proposer)
	require.Equal(t, 1, len(block.Transactions))
	require.NotEqual(t, types.Hash{}, block.Header.StateRoot)
	require.NotEqual(t, 0, len(block.Signature))
	require.Equal(t, uint64(10), block.FeeSummary.TotalFees)
	require.Equal(t, uint64(1), block.Header.Epoch)
}

func TestBuildBlock_EmptyMempool(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	// Setup validator (distinct address from sender)
	validatorAddr := setupTestValidator(s, 100, 100_000)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorAddr, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)
	require.Equal(t, 0, len(block.Transactions))
}

func TestBuildBlock_HeaderTimestampSingleRead(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	validatorAddr := setupTestValidator(s, 100, 100_000)

	// The header timestamp must be a single wall-clock read performed while
	// building the block (the same value contracts observe during execution).
	before := uint64(time.Now().Unix())
	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorAddr, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)
	after := uint64(time.Now().Unix())

	require.GreaterOrEqual(t, block.Header.Timestamp, before,
		"header timestamp must not predate block building")
	require.LessOrEqual(t, block.Header.Timestamp, after,
		"header timestamp must not postdate block building")
}

func TestBuildBlock_TxLimit(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	sender := types.Address{1}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(10000))
	s.SetAccount(sender, acc)

	for i := 0; i < 5; i++ {
		tx := &types.Transaction{
			Version: 1, Nonce: uint64(i + 1), Sender: sender, MaxFee: 10,
			Payload:  types.EncodeTransferPayload(sender, 0),
			IntentID: types.Hash{byte(i + 1)}, Timestamp: uint64(time.Now().Unix()),
		}
		tx.Signature = ed25519.Sign(privKey, tx.IntentID[:])
		mp.Submit(tx)
	}

	// Setup validator (distinct address from sender)
	validatorAddr := setupTestValidator(s, 100, 100_000)
	staking.WriteUint64(s, staking.KeyTotalSupply, 10_000_000)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorAddr, &mockSigner{}, hasher, 3, nil, 1)
	require.NoError(t, err)
	require.Equal(t, 3, len(block.Transactions))
}
