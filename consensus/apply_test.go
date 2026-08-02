package consensus

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
	"github.com/stretchr/testify/require"
)

// TestApplyTransaction_ChargesGasCappedAtMaxFee verifies a contract
// transaction is never charged more than MaxFee, even when execution consumes
// more gas than the sender declared (F4).
func TestApplyTransaction_ChargesGasCappedAtMaxFee(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	sender := types.Address{1}
	pubKey, _, _ := ed25519.GenerateKey(rand.Reader)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(1_000_000))
	require.NoError(t, s.SetAccount(sender, acc))

	vmInstance, err := vm.NewVM(hasher)
	require.NoError(t, err)

	// Deploy tx: gas limit is far above the max fee so the fee cap binds.
	wasmCode := emitEventWasm()
	codeHash := types.Hash(vm.SHA256Sum(wasmCode))
	deployTx := &types.Transaction{
		Version:   1,
		Nonce:     1,
		Sender:    sender,
		TxType:    types.TxTypeDeployContract,
		MaxFee:    500,
		GasLimit:  1_000_000,
		Timestamp: uint64(time.Now().Unix()),
	}
	var buf bytes.Buffer
	inner := &types.DeployContractTx{
		Sender: sender, Nonce: 1, WasmCode: wasmCode, CodeHash: codeHash,
		MaxFee: deployTx.MaxFee, GasLimit: deployTx.GasLimit,
	}
	require.NoError(t, inner.Encode(&buf))
	deployTx.Payload = buf.Bytes()

	header := &types.BlockHeader{Height: 1, Timestamp: uint64(time.Now().Unix())}

	exec, err := ApplyTransaction(s, deployTx, header, vmInstance, hasher, 0)
	require.NoError(t, err)

	// The cap must bind: charged exactly MaxFee, not the full gas used.
	require.Equal(t, uint64(500), exec.Fee)
	require.Equal(t, deployTx.IntentID, exec.Receipt)

	// Sender balance drops by exactly the capped amount.
	accAfter, err := s.GetAccount(sender)
	require.NoError(t, err)
	require.Equal(t, 0, accAfter.Balance.Cmp(types.NewAmount(999_500)))
}

// TestApplyTransaction_ChargesActualGas verifies a contract transaction is
// charged the gas actually used when that is below MaxFee (T8-3 refund).
func TestApplyTransaction_ChargesActualGas(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	sender := types.Address{1}
	pubKey, _, _ := ed25519.GenerateKey(rand.Reader)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(1_000_000))
	require.NoError(t, s.SetAccount(sender, acc))

	vmInstance, err := vm.NewVM(hasher)
	require.NoError(t, err)

	// A deploy of a tiny module uses far less than the declared max fee.
	wasmCode := emitEventWasm()
	codeHash := types.Hash(vm.SHA256Sum(wasmCode))
	deployTx := &types.Transaction{
		Version:   1,
		Nonce:     1,
		Sender:    sender,
		TxType:    types.TxTypeDeployContract,
		MaxFee:    1_000_000,
		GasLimit:  1_000_000,
		Timestamp: uint64(time.Now().Unix()),
	}
	var buf bytes.Buffer
	inner := &types.DeployContractTx{
		Sender: sender, Nonce: 1, WasmCode: wasmCode, CodeHash: codeHash,
		MaxFee: deployTx.MaxFee, GasLimit: deployTx.GasLimit,
	}
	require.NoError(t, inner.Encode(&buf))
	deployTx.Payload = buf.Bytes()

	header := &types.BlockHeader{Height: 1, Timestamp: uint64(time.Now().Unix())}

	exec, err := ApplyTransaction(s, deployTx, header, vmInstance, hasher, 0)
	require.NoError(t, err)

	// The actual gas used (deploy base + code bytes) is well below MaxFee.
	require.Less(t, exec.Fee, uint64(1_000_000))
	require.Greater(t, exec.Fee, uint64(0))

	accAfter, err := s.GetAccount(sender)
	require.NoError(t, err)
	require.Equal(t, 0, accAfter.Balance.Cmp(types.NewAmount(1_000_000-exec.Fee)))
}

// TestApplyTransaction_StandardTxChargesFullMaxFee verifies standard
// transactions still charge the full MaxFee.
func TestApplyTransaction_StandardTxChargesFullMaxFee(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	sender := types.Address{1}
	recipient := types.Address{2}
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	var pubKey32 [32]byte
	copy(pubKey32[:], pubKey)
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(1_000_000))
	require.NoError(t, s.SetAccount(sender, acc))

	tx := &types.Transaction{
		Version:   1,
		ChainID:   0,
		Sender:    sender,
		Nonce:     1,
		Payload:   types.EncodeTransferPayload(recipient, 1000),
		MaxFee:    100,
		GasLimit:  50000,
		Timestamp: uint64(time.Now().Unix()),
	}
	intentID, err := tx.ComputeIntentID(hasher)
	require.NoError(t, err)
	tx.IntentID = intentID
	tx.Signature = ed25519.Sign(privKey, intentID[:])

	header := &types.BlockHeader{Height: 1, Timestamp: uint64(time.Now().Unix())}

	exec, err := ApplyTransaction(s, tx, header, nil, hasher, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), exec.Fee)

	accAfter, err := s.GetAccount(sender)
	require.NoError(t, err)
	// 1_000_000 - 100 fee - 1000 transfer
	require.Equal(t, 0, accAfter.Balance.Cmp(types.NewAmount(998_900)))
}
