package consensus

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/dsn/dsn/mempool"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// emitEventWasm returns a minimal WASM module that imports the env.emit_event
// host function and emits a single "transfer" event when its "test" entrypoint
// is called, then returns 0 (success). The topic bytes live at offset 0 and a
// 4-byte payload at offset 100 (initialized via the data section).
func emitEventWasm() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version

		// type section: type0 (i32,i32,i32,i32)->(), type1 ()->i32
		0x01, 0x0c, 0x02,
		0x60, 0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x00,
		0x60, 0x00, 0x01, 0x7f,

		// import section: env.emit_event (func kind, type 0)
		0x02, 0x12, 0x01,
		0x03, 0x65, 0x6e, 0x76,
		0x0a, 0x65, 0x6d, 0x69, 0x74, 0x5f, 0x65, 0x76, 0x65, 0x6e, 0x74,
		0x00, 0x00,

		// function section: 1 func, type 1
		0x03, 0x02, 0x01, 0x01,

		// memory section: 1 page min
		0x05, 0x03, 0x01, 0x00, 0x01,

		// export section: "test" func idx 1 (0 = imported emit_event)
		0x07, 0x08, 0x01, 0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x01,

		// data section: "transfer" @0, 4-byte payload @100
		// NOTE: i32.const offsets are signed LEB128, so 100 must be two bytes
		// (0xe4, 0x00), not the single byte 0x64 (= -28).
		0x0b, 0x18, 0x02,
		0x00, 0x41, 0x00, 0x0b, 0x08, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72,
		0x00, 0x41, 0xe4, 0x00, 0x0b, 0x04, 0x01, 0x02, 0x03, 0x04,

		// code section: emit_event(0, 8, 100, 4); return 0
		0x0a, 0x11, 0x01,
		0x0f, 0x00,
		0x41, 0x00, 0x41, 0x08, 0x41, 0xe4, 0x00, 0x41, 0x04,
		0x10, 0x00,
		0x41, 0x00,
		0x0b,
	}
}

// signContractTx signs the embedded contract transaction's own intent ID so the
// inner Validate() signature check passes (the outer Transaction is signed by
// wallet.KeyPair.Sign separately).
func signContractTx(kp *wallet.KeyPair, tx interface{}, hasher types.Hasher) error {
	sign := func(id types.Hash) error {
		sig, err := kp.SignHash(id)
		if err != nil {
			return err
		}
		switch t := tx.(type) {
		case *types.DeployContractTx:
			t.Signature = sig
		case *types.CallContractTx:
			t.Signature = sig
		default:
			return fmt.Errorf("unsupported contract tx type: %T", tx)
		}
		return nil
	}
	switch t := tx.(type) {
	case *types.DeployContractTx:
		id, err := t.ComputeIntentID(hasher)
		if err != nil {
			return err
		}
		return sign(id)
	case *types.CallContractTx:
		id, err := t.ComputeIntentID(hasher)
		if err != nil {
			return err
		}
		return sign(id)
	default:
		return fmt.Errorf("unsupported contract tx type: %T", tx)
	}
}

// TestBuildBlock_EventsRoot_Populated proves the block builder populates
// Header.EventsRoot from the block's collected events. It FAILS before the F2
// fix (header root was zero) and PASSES after, and a fully re-executed
// ValidateBlock must agree with the header.
func TestBuildBlock_EventsRoot_Populated(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := mempool.New(10000, 300*time.Second, s)

	// Sender account (funded beyond MaxFee for both txs)
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	sender := kp.Address()
	var pubKey32 [32]byte
	copy(pubKey32[:], kp.PublicKey[:])
	acc := state.NewAccount(sender, pubKey32)
	acc.AddBalance(types.NewAmount(20_000_000))
	s.SetAccount(sender, acc)

	// Validator (single, so it is the proposer at every height)
	_, validatorPrivKey, validatorConsensusID := setupValidatorForTest(s, 1, 100_000)
	staking.WriteUint64(s, staking.KeyTotalSupply, 10_000_000)

	// Deploy the event-emitting contract (nonce 1)
	wasmCode := emitEventWasm()
	codeHash := types.Hash(vm.SHA256Sum(wasmCode))
	deployTx := &types.Transaction{
		Version: 1, Nonce: 1, Sender: sender,
		TxType:    types.TxTypeDeployContract,
		MaxFee:    1_000_000,
		GasLimit:  5_000_000,
		Timestamp: uint64(time.Now().Unix()),
	}
	var deployBuf bytes.Buffer
	deployContractTx := &types.DeployContractTx{
		Sender: sender, Nonce: 1, WasmCode: wasmCode, CodeHash: codeHash,
		MaxFee: deployTx.MaxFee, GasLimit: deployTx.GasLimit,
	}
	require.NoError(t, signContractTx(kp, deployContractTx, hasher))
	require.NoError(t, deployContractTx.Encode(&deployBuf))
	deployTx.Payload = deployBuf.Bytes()
	require.NoError(t, kp.Sign(deployTx, hasher))
	require.NoError(t, mp.Submit(deployTx))

	contractID := vm.DeriveContractID(sender, 1, codeHash)

	// Call the contract entrypoint (nonce 2) — emits one event
	callTx := &types.Transaction{
		Version: 1, Nonce: 2, Sender: sender,
		TxType:    types.TxTypeCallContract,
		MaxFee:    1_000_000,
		GasLimit:  5_000_000,
		Timestamp: uint64(time.Now().Unix()),
	}
	var callBuf bytes.Buffer
	callContractTx := &types.CallContractTx{
		Sender: sender, Nonce: 2, ContractID: contractID, Entrypoint: "test",
		MaxFee: callTx.MaxFee, GasLimit: callTx.GasLimit,
	}
	require.NoError(t, signContractTx(kp, callContractTx, hasher))
	require.NoError(t, callContractTx.Encode(&callBuf))
	callTx.Payload = callBuf.Bytes()
	require.NoError(t, kp.Sign(callTx, hasher))
	require.NoError(t, mp.Submit(callTx))

	// Build the block with the VM so the contract txs produce events
	v, err := vm.NewVM(hasher)
	require.NoError(t, err)
	defer v.Close()

	block, err := BuildBlock(s, v, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)

	require.Equal(t, validatorConsensusID, block.Header.Proposer)
	require.Equal(t, 2, len(block.Transactions))
	require.NotEmpty(t, block.Events, "contract call should emit at least one event")
	require.Equal(t, "transfer", block.Events[0].Topic)

	// F2: header must carry the events root computed from block.Events
	expectedRoot := types.ComputeEventsRoot(block.Events)
	require.Equal(t, expectedRoot, block.Header.EventsRoot,
		"header EventsRoot must match ComputeEventsRoot(block.Events)")
	require.NotEqual(t, types.Hash{}, block.Header.EventsRoot)

	// Full validation path: re-execution must produce the same events root
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)

	freshS := state.NewInMemoryState(hasher)
	freshAcc := state.NewAccount(sender, pubKey32)
	freshAcc.AddBalance(types.NewAmount(20_000_000))
	freshS.SetAccount(sender, freshAcc)
	setupValidatorForTest(freshS, 1, 100_000)
	staking.WriteUint64(freshS, staking.KeyTotalSupply, 10_000_000)

	parent := &types.BlockHeader{Height: 0}
	err = ValidateBlock(block, parent, types.ZeroHash, freshS, hasher, v, 1)
	require.NoError(t, err)
}
