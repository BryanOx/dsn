package vm

import (
	"math"
	"testing"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestGasDeduct_Basic(t *testing.T) {
	meter := NewGasMeter(100)
	err := meter.Deduct(10)
	require.NoError(t, err)
	require.Equal(t, uint64(10), meter.Used)
}

func TestGasDeduct_ExactLimit(t *testing.T) {
	meter := NewGasMeter(100)
	err := meter.Deduct(100)
	require.NoError(t, err)
	require.Equal(t, uint64(100), meter.Used)
}

func TestGasDeduct_Exceeded(t *testing.T) {
	meter := NewGasMeter(100)
	err := meter.Deduct(101)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGasLimitExceeded)
	require.Equal(t, uint64(0), meter.Used)
}

func TestGasRemaining(t *testing.T) {
	meter := NewGasMeter(100)
	err := meter.Deduct(30)
	require.NoError(t, err)
	require.Equal(t, uint64(70), meter.Remaining())
}

func TestGasZeroLimit(t *testing.T) {
	meter := NewGasMeter(0)
	err := meter.Deduct(1)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGasLimitExceeded)
}

func TestGasMultipleDeducts(t *testing.T) {
	meter := NewGasMeter(100)
	require.NoError(t, meter.Deduct(20))
	require.NoError(t, meter.Deduct(30))
	require.NoError(t, meter.Deduct(40))
	require.Equal(t, uint64(90), meter.Used)
	require.Equal(t, uint64(10), meter.Remaining())
}

func TestGasOverflow(t *testing.T) {
	meter := NewGasMeter(100)
	err := meter.Deduct(math.MaxUint64)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGasLimitExceeded)
	require.Equal(t, uint64(0), meter.Used)
}

// T2-3: Verify gas schedule constants match expected values from spec
func TestGasScheduleConstants(t *testing.T) {
	// Instruction gas costs
	require.Equal(t, uint64(1), GasBaseOp, "GasBaseOp should be 1")
	require.Equal(t, uint64(3), GasMemoryOp, "GasMemoryOp should be 3")
	require.Equal(t, uint64(5), GasControlFlow, "GasControlFlow should be 5")
	require.Equal(t, uint64(100), GasNondeterministic, "GasNondeterministic should be 100")

	// Host function gas costs
	require.Equal(t, uint64(20), GasReadStorage, "GasReadStorage should be 20")
	require.Equal(t, uint64(50), GasWriteStorage, "GasWriteStorage should be 50")
	require.Equal(t, uint64(2), GasPerStorageByte, "GasPerStorageByte should be 2")
	require.Equal(t, uint64(30), GasEmitEvent, "GasEmitEvent should be 30")
	require.Equal(t, uint64(10), GasReadCaller, "GasReadCaller should be 10")
	require.Equal(t, uint64(10), GasReadBlockHeight, "GasReadBlockHeight should be 10")
	require.Equal(t, uint64(10), GasReadBlockTimestamp, "GasReadBlockTimestamp should be 10")
	require.Equal(t, uint64(100), GasTransferToken, "GasTransferToken should be 100")

	// Deployment gas costs
	require.Equal(t, uint64(1000), GasDeployBase, "GasDeployBase should be 1000")
	require.Equal(t, uint64(1), GasPerCodeByte, "GasPerCodeByte should be 1")
}

// T2-8: Verify deterministic gas across replay
func TestDeterministicGas(t *testing.T) {
	// First GasMeter
	meter1 := NewGasMeter(1000000)
	_ = meter1.Deduct(100)
	_ = meter1.Deduct(200)
	_ = meter1.Deduct(50)
	used1 := meter1.Used

	// Second GasMeter - same operations
	meter2 := NewGasMeter(1000000)
	_ = meter2.Deduct(100)
	_ = meter2.Deduct(200)
	_ = meter2.Deduct(50)
	used2 := meter2.Used

	// Both should have identical used gas
	require.Equal(t, used1, used2, "Gas meters should produce identical Used values across replay")
	require.Equal(t, uint64(350), used1)
}

// PR1-1.1 GREEN: TestGasMeterPerInstruction proves instruction-level gas metering is wired.
// Deploys a WASM contract with 100 nop instructions + i32.const 0,
// asserts callResult.GasUsed >= 100 (each op costs GasBaseOp=1).
func TestGasMeterPerInstruction(t *testing.T) {
	hasher := &types.SHA256Hasher{}
	st := state.NewInMemoryState(hasher)
	vmInst, err := NewVM(hasher)
	require.NoError(t, err)
	defer vmInst.Close()

	// Build valid WASM module: 100 nop instructions + i32.const 0
	wb := newWasmBuilder()
	typeIdx := wb.addFuncType(nil, []byte{0x7f}) // () -> i32
	wb.addMemory(1)
	body := make([]byte, 0, 103)
	for i := 0; i < 100; i++ {
		body = append(body, 0x01) // nop
	}
	body = append(body, 0x41, 0x00) // i32.const 0
	moduleFuncIdx := wb.addFunc(typeIdx, body)
	wb.addExport("run", 0x00, moduleFuncIdx)
	wb.addExport("memory", 0x02, 0)
	wasmModule := wb.build()

	// Deploy the contract
	deployTx := &types.DeployContractTx{
		Sender:   types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Nonce:    0,
		CodeHash: types.Hash{},
		WasmCode: wasmModule,
		GasLimit: 1_000_000,
	}
	deployResult, err := vmInst.Execute(deployTx, st, 1, 1000, 0)
	require.NoError(t, err)
	require.False(t, deployResult.Reverted)

	contractID := DeriveContractID(deployTx.Sender, deployTx.Nonce, deployTx.CodeHash)

	// Call the entrypoint
	callTx := &types.CallContractTx{
		Sender:     deployTx.Sender,
		Nonce:      1,
		ContractID: contractID,
		Entrypoint: "run",
		Calldata:   []byte{},
		GasLimit:   1_000_000,
	}
	callResult, err := vmInst.Execute(callTx, st, 2, 1000, 0)
	require.NoError(t, err)
	require.False(t, callResult.Reverted)

	// Proves instruction-level metering is wired: 101 instructions × GasBaseOp(1) ≥ 100
	require.GreaterOrEqual(t, callResult.GasUsed, uint64(100),
		"GasUsed should reflect instruction-level metering (≥ 100 for 100 nop instructions)")
	t.Logf("GasUsed=%d for 100 nops (GasBaseOp=%d)", callResult.GasUsed, GasBaseOp)
}

// PR1-2.1 GREEN: TestReturnDataLengthPrefixed verifies return data reading with a 2-byte
// big-endian length prefix.
func TestReturnDataLengthPrefixed(t *testing.T) {
	// Build WASM module that writes length prefix and 10 data bytes to memory offset 2048
	// using i32.store8 (opcode 0x3a), then returns 0.
	wb := newWasmBuilder()
	retTypeIdx := wb.addFuncType(nil, []byte{0x7f}) // () -> i32
	wb.addMemory(1)

	// Generate body: write 12 bytes (2 length prefix + 10 data) to memory offset 2048
	// Each byte: i32.const 0 (address); i32.const <value>; i32.store8 align=0 offset=<2048+i>
	// i32.store8 pops address first, then value from the stack.
	var body []byte
	data := []byte{0x00, 0x0A, 0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x5E}
	for i, b := range data {
		offset := uint32(2048 + i)
		body = append(body, 0x41, 0x00) // i32.const 0 (base address)
		body = append(body, 0x41)       // i32.const value (signed LEB128)
		body = append(body, encodeSignedLEB128Bytes(int32(b))...)
		body = append(body, 0x3a, 0x00) // i32.store8 align=0
		body = append(body, encodeULEB128Bytes(offset)...)
	}
	body = append(body, 0x41, 0x00) // i32.const 0 (return value)

	moduleFuncIdx := wb.addFunc(retTypeIdx, body)
	wb.addExport("run", 0x00, moduleFuncIdx)
	returnDataModule := wb.build()

	expectedReturnData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x5E}

	hasher := &types.SHA256Hasher{}
	st := state.NewInMemoryState(hasher)
	vmInst, err := NewVM(hasher)
	require.NoError(t, err)
	defer vmInst.Close()

	deployTx := &types.DeployContractTx{
		Sender:   types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Nonce:    0,
		CodeHash: types.Hash{},
		WasmCode: returnDataModule,
		GasLimit: 1_000_000,
	}
	deployResult, err := vmInst.Execute(deployTx, st, 1, 1000, 0)
	require.NoError(t, err)
	require.False(t, deployResult.Reverted)

	contractID := DeriveContractID(deployTx.Sender, deployTx.Nonce, deployTx.CodeHash)

	callTx := &types.CallContractTx{
		Sender:     deployTx.Sender,
		Nonce:      1,
		ContractID: contractID,
		Entrypoint: "run",
		Calldata:   []byte{},
		GasLimit:   1_000_000,
	}
	callResult, err := vmInst.Execute(callTx, st, 2, 1000, 0)
	require.NoError(t, err)
	require.False(t, callResult.Reverted)

	require.Equal(t, expectedReturnData, callResult.ReturnData,
		"ReturnData should be the 10 bytes after the 2-byte big-endian length prefix")
}

// PR1-2.2 RED: TestEmptyReturnData verifies that a contract which writes nothing to
// offset 2048 produces nil or empty ReturnData.
func TestEmptyReturnData(t *testing.T) {
	// Minimal WASM module: returns 0
	wb := newWasmBuilder()
	typeIdx := wb.addFuncType(nil, []byte{0x7f}) // () -> i32
	wb.addMemory(1)
	moduleFuncIdx := wb.addFunc(typeIdx, []byte{
		0x41, 0x00, // i32.const 0
	})
	wb.addExport("run", 0x00, moduleFuncIdx)
	emptyReturnModule := wb.build()

	hasher := &types.SHA256Hasher{}
	st := state.NewInMemoryState(hasher)
	vmInst, err := NewVM(hasher)
	require.NoError(t, err)
	defer vmInst.Close()

	deployTx := &types.DeployContractTx{
		Sender:   types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Nonce:    0,
		CodeHash: types.Hash{},
		WasmCode: emptyReturnModule,
		GasLimit: 1_000_000,
	}
	deployResult, err := vmInst.Execute(deployTx, st, 1, 1000, 0)
	require.NoError(t, err)
	require.False(t, deployResult.Reverted)

	contractID := DeriveContractID(deployTx.Sender, deployTx.Nonce, deployTx.CodeHash)

	callTx := &types.CallContractTx{
		Sender:     deployTx.Sender,
		Nonce:      1,
		ContractID: contractID,
		Entrypoint: "run",
		Calldata:   []byte{},
		GasLimit:   1_000_000,
	}
	callResult, err := vmInst.Execute(callTx, st, 2, 1000, 0)
	require.NoError(t, err)
	require.False(t, callResult.Reverted)

	require.True(t, len(callResult.ReturnData) == 0,
		"ReturnData should be nil or empty when no data is written to offset 2048")
}
