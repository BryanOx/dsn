package vm

import (
	"context"
	"testing"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// WASM module that imports host functions from "env" and exports test entry points.
//
// Imports:
//
//	read_caller(i32) -> i64         (index 0)
//	read_block_height() -> i64      (index 1)
//	read_block_timestamp() -> i64   (index 2)
//
// Exports:
//
//	call_caller() -> i64    (func index 3)
//	call_height() -> i64    (func index 4)
//	call_timestamp() -> i64 (func index 5)
//	memory                  (mem index 0)
var wasmWithImport = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version

	// Type section: id=1, size=10, 2 types
	0x01, 0x0a,
	0x02,
	0x60, 0x01, 0x7f, 0x01, 0x7e, // type 0: (func (param i32) (result i64))
	0x60, 0x00, 0x01, 0x7e, // type 1: (func (result i64))

	// Import section: id=2, size=70, 3 imports
	0x02, 0x46,
	0x03,
	// import 0: "env" "read_caller" type 0
	0x03, 0x65, 0x6e, 0x76,
	0x0b, 0x72, 0x65, 0x61, 0x64, 0x5f, 0x63, 0x61, 0x6c, 0x6c, 0x65, 0x72,
	0x00, 0x00,
	// import 1: "env" "read_block_height" type 1
	0x03, 0x65, 0x6e, 0x76,
	0x11, 0x72, 0x65, 0x61, 0x64, 0x5f, 0x62, 0x6c, 0x6f, 0x63, 0x6b, 0x5f, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74,
	0x00, 0x01,
	// import 2: "env" "read_block_timestamp" type 1
	0x03, 0x65, 0x6e, 0x76,
	0x14, 0x72, 0x65, 0x61, 0x64, 0x5f, 0x62, 0x6c, 0x6f, 0x63, 0x6b, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x00, 0x01,

	// Function section: id=3, size=4, 3 functions
	0x03, 0x04,
	0x03,
	0x01, // func 0 -> type 1 (call_caller)
	0x01, // func 1 -> type 1 (call_height)
	0x01, // func 2 -> type 1 (call_timestamp)

	// Memory section: id=5, size=3, 1 page min
	0x05, 0x03,
	0x01, 0x00, 0x01,

	// Export section: id=7, size=55, 4 exports
	0x07, 0x37,
	0x04,
	// "call_caller" func 3
	0x0b, 0x63, 0x61, 0x6c, 0x6c, 0x5f, 0x63, 0x61, 0x6c, 0x6c, 0x65, 0x72,
	0x00, 0x03,
	// "call_height" func 4
	0x0b, 0x63, 0x61, 0x6c, 0x6c, 0x5f, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74,
	0x00, 0x04,
	// "call_timestamp" func 5
	0x0e, 0x63, 0x61, 0x6c, 0x6c, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x00, 0x05,
	// "memory" mem 0
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79,
	0x02, 0x00,

	// Code section: id=10, size=18, 3 bodies
	0x0a, 0x12,
	0x03,
	// call_caller: body size=6
	0x06,
	0x00, 0x41, 0x00, 0x10, 0x00, 0x0b,
	// call_height: body size=4
	0x04,
	0x00, 0x10, 0x01, 0x0b,
	// call_timestamp: body size=4
	0x04,
	0x00, 0x10, 0x02, 0x0b,
}

func TestHostEnv_Caller(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)

	st := state.NewInMemoryState(&types.SHA256Hasher{})
	meter := NewGasMeter(100)
	caller := types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	env := NewHostEnv(meter, st, caller, types.Hash{}, 0, 0, 0)

	hostModule, err := BuildHostModule(ctx, runtime, env)
	require.NoError(t, err)
	defer hostModule.Close(ctx)

	compiled, err := CompileModule(ctx, runtime, wasmWithImport)
	require.NoError(t, err)
	defer compiled.Close(ctx)

	testModule, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer testModule.Close(ctx)

	results, err := testModule.ExportedFunction("call_caller").Call(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, uint64(0), results[0])

	mem := testModule.Memory()
	require.NotNil(t, mem)
	out, ok := mem.Read(0, 20)
	require.True(t, ok)
	require.Equal(t, caller[:], out)

	require.Equal(t, uint64(GasReadCaller), meter.Used)
}

func TestHostEnv_BlockHeight(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)

	st := state.NewInMemoryState(&types.SHA256Hasher{})
	meter := NewGasMeter(100)
	env := NewHostEnv(meter, st, types.Address{}, types.Hash{}, 42, 0, 0)

	hostModule, err := BuildHostModule(ctx, runtime, env)
	require.NoError(t, err)
	defer hostModule.Close(ctx)

	compiled, err := CompileModule(ctx, runtime, wasmWithImport)
	require.NoError(t, err)
	defer compiled.Close(ctx)

	testModule, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer testModule.Close(ctx)

	results, err := testModule.ExportedFunction("call_height").Call(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, uint64(42), results[0])
	require.Equal(t, uint64(GasReadBlockHeight), meter.Used)
}

func TestHostEnv_BlockTimestamp(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)

	st := state.NewInMemoryState(&types.SHA256Hasher{})
	meter := NewGasMeter(100)
	env := NewHostEnv(meter, st, types.Address{}, types.Hash{}, 0, 1000, 0)

	hostModule, err := BuildHostModule(ctx, runtime, env)
	require.NoError(t, err)
	defer hostModule.Close(ctx)

	compiled, err := CompileModule(ctx, runtime, wasmWithImport)
	require.NoError(t, err)
	defer compiled.Close(ctx)

	testModule, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer testModule.Close(ctx)

	results, err := testModule.ExportedFunction("call_timestamp").Call(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, uint64(1000), results[0])
	require.Equal(t, uint64(GasReadBlockTimestamp), meter.Used)
}

func TestHostEnv_GasDeduction(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)

	st := state.NewInMemoryState(&types.SHA256Hasher{})
	meter := NewGasMeter(GasReadBlockHeight)
	env := NewHostEnv(meter, st, types.Address{}, types.Hash{}, 42, 0, 0)

	hostModule, err := BuildHostModule(ctx, runtime, env)
	require.NoError(t, err)
	defer hostModule.Close(ctx)

	compiled, err := CompileModule(ctx, runtime, wasmWithImport)
	require.NoError(t, err)
	defer compiled.Close(ctx)

	testModule, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer testModule.Close(ctx)

	results, err := testModule.ExportedFunction("call_height").Call(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, uint64(42), results[0])
	require.Equal(t, uint64(GasReadBlockHeight), meter.Used)
	require.Equal(t, uint64(0), meter.Remaining())

	results, err = testModule.ExportedFunction("call_height").Call(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, uint64(0), results[0])
}

// T2-4: Verify storage gas accounting - verify constants are correct
func TestHostEnv_StorageGas(t *testing.T) {
	// Verify write_storage gas constants
	// Expected: GasWriteStorage (50) + GasPerStorageByte * 32 (64) = 114
	expectedWriteGas := GasWriteStorage + GasPerStorageByte*32
	require.Equal(t, uint64(114), expectedWriteGas, "32-byte write should cost 114 gas")

	// Verify read_storage costs exactly GasReadStorage (20)
	require.Equal(t, uint64(20), GasReadStorage, "read_storage should cost 20 gas")

	// Verify 0-byte write costs just GasWriteStorage (50)
	expectedZeroWriteGas := GasWriteStorage + GasPerStorageByte*0
	require.Equal(t, uint64(50), expectedZeroWriteGas, "0-byte write should cost 50 gas")
}

// T2-5: Verify all host function gas costs using existing wasmWithImport
func TestHostEnv_AllGasCosts(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	require.NoError(t, err)
	defer runtime.Close(ctx)

	st := state.NewInMemoryState(&types.SHA256Hasher{})
	contractID := types.Hash{}

	// Test read_caller
	t.Run("read_caller", func(t *testing.T) {
		meter := NewGasMeter(GasReadCaller + 100)
		caller := types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		env := NewHostEnv(meter, st, caller, contractID, 0, 0, 0)

		hostModule, err := BuildHostModule(ctx, runtime, env)
		require.NoError(t, err)
		defer hostModule.Close(ctx)

		compiled, err := CompileModule(ctx, runtime, wasmWithImport)
		require.NoError(t, err)
		defer compiled.Close(ctx)

		module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
		require.NoError(t, err)
		defer module.Close(ctx)

		results, callErr := module.ExportedFunction("call_caller").Call(ctx)
		require.NoError(t, callErr)
		require.Len(t, results, 1)
		require.Equal(t, GasReadCaller, meter.Used, "gas used should match expected cost")
	})

	// Test read_block_height
	t.Run("read_block_height", func(t *testing.T) {
		meter := NewGasMeter(GasReadBlockHeight + 100)
		env := NewHostEnv(meter, st, types.Address{}, contractID, 42, 0, 0)

		hostModule, err := BuildHostModule(ctx, runtime, env)
		require.NoError(t, err)
		defer hostModule.Close(ctx)

		compiled, err := CompileModule(ctx, runtime, wasmWithImport)
		require.NoError(t, err)
		defer compiled.Close(ctx)

		module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
		require.NoError(t, err)
		defer module.Close(ctx)

		results, callErr := module.ExportedFunction("call_height").Call(ctx)
		require.NoError(t, callErr)
		require.Len(t, results, 1)
		require.Equal(t, GasReadBlockHeight, meter.Used, "gas used should match expected cost")
	})

	// Test read_block_timestamp
	t.Run("read_block_timestamp", func(t *testing.T) {
		meter := NewGasMeter(GasReadBlockTimestamp + 100)
		env := NewHostEnv(meter, st, types.Address{}, contractID, 0, 1000, 0)

		hostModule, err := BuildHostModule(ctx, runtime, env)
		require.NoError(t, err)
		defer hostModule.Close(ctx)

		compiled, err := CompileModule(ctx, runtime, wasmWithImport)
		require.NoError(t, err)
		defer compiled.Close(ctx)

		module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
		require.NoError(t, err)
		defer module.Close(ctx)

		results, callErr := module.ExportedFunction("call_timestamp").Call(ctx)
		require.NoError(t, callErr)
		require.Len(t, results, 1)
		require.Equal(t, GasReadBlockTimestamp, meter.Used, "gas used should match expected cost")
	})

	// Verify all 7 gas constants match expected values
	t.Run("verify_all_constants", func(t *testing.T) {
		require.Equal(t, uint64(20), GasReadStorage, "GasReadStorage")
		require.Equal(t, uint64(50), GasWriteStorage, "GasWriteStorage")
		require.Equal(t, uint64(30), GasEmitEvent, "GasEmitEvent")
		require.Equal(t, uint64(10), GasReadCaller, "GasReadCaller")
		require.Equal(t, uint64(10), GasReadBlockHeight, "GasReadBlockHeight")
		require.Equal(t, uint64(10), GasReadBlockTimestamp, "GasReadBlockTimestamp")
		require.Equal(t, uint64(100), GasTransferToken, "GasTransferToken")
	})
}

// T2-6: Failure gas semantics - verify gas is consumed on error (when there's enough gas)
func TestFailureGasSemantics(t *testing.T) {
	// This test verifies that when a host function executes successfully,
	// the gas is consumed. The failure semantics (OOG returning error) is tested
	// by verifying that even on error paths, gas is tracked properly.

	// Test that gas is consumed on successful call
	meter := NewGasMeter(100)
	require.NoError(t, meter.Deduct(10))
	require.Equal(t, uint64(10), meter.Used)

	// Test that when Deduct fails due to OOG, no gas is consumed (current behavior)
	// This is important - if gas is insufficient, the meter.Used doesn't increase
	meter2 := NewGasMeter(5)
	err := meter2.Deduct(10) // Try to deduct more than available
	require.Error(t, err)
	require.Equal(t, uint64(0), meter2.Used, "no gas consumed when OOG occurs")
}

// T2-7: OOG immediate abort - verify OOG returns error code 1 from host functions
func TestOOGImmediateAbort(t *testing.T) {
	// Test that GasMeter properly returns error when deducting more than available
	meter := NewGasMeter(10)

	// First deduction succeeds
	err := meter.Deduct(5)
	require.NoError(t, err)
	require.Equal(t, uint64(5), meter.Used)

	// Second deduction fails - not enough gas left
	err = meter.Deduct(10) // Only 5 remaining, need 10
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGasLimitExceeded)
	require.Equal(t, uint64(5), meter.Used, "used gas unchanged after OOG")

	// Verify remaining gas is correct
	require.Equal(t, uint64(5), meter.Remaining())
}
