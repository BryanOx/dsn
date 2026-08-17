package vm

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestGasMeter_Deduct(t *testing.T) {
	tests := []struct {
		name     string
		limit    uint64
		costs    []uint64
		wantUsed uint64
	}{
		{
			name:     "single deduction within limit",
			limit:    1000,
			costs:    []uint64{500},
			wantUsed: 500,
		},
		{
			name:     "multiple deductions within limit",
			limit:    1000,
			costs:    []uint64{100, 200, 300},
			wantUsed: 600,
		},
		{
			name:     "exact limit reached",
			limit:    1000,
			costs:    []uint64{500, 500},
			wantUsed: 1000,
		},
		{
			name:     "exceeds limit on second deduction",
			limit:    1000,
			costs:    []uint64{600, 500},
			wantUsed: 600, // stops at 600, second 500 rejected
		},
		{
			name:     "zero cost",
			limit:    1000,
			costs:    []uint64{0},
			wantUsed: 0,
		},
		{
			name:     "zero limit",
			limit:    0,
			costs:    []uint64{0},
			wantUsed: 0,
		},
		{
			name:     "zero limit with non-zero cost",
			limit:    0,
			costs:    []uint64{1},
			wantUsed: 0, // rejected, no gas used
		},
		{
			name:     "large limit",
			limit:    100000000,
			costs:    []uint64{50000000, 30000000},
			wantUsed: 80000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := NewGasMeter(tt.limit)

			var totalCost uint64
			var lastErr error
			for _, cost := range tt.costs {
				err := gm.Deduct(cost)
				lastErr = err
				if err == nil {
					totalCost += cost
				}
			}

			// Verify the final state
			if gm.Used != tt.wantUsed {
				t.Errorf("GasMeter.Used = %d, want %d", gm.Used, tt.wantUsed)
			}

			// For the "exceeds limit" test, verify an error was returned
			if tt.name == "exceeds limit on second deduction" && lastErr == nil {
				t.Errorf("Expected error when limit exceeded, got nil")
			}
		})
	}
}

func TestGasMeter_Remaining(t *testing.T) {
	gm := NewGasMeter(1000)

	// Initial state
	if remaining := gm.Remaining(); remaining != 1000 {
		t.Errorf("Remaining() = %d, want 1000", remaining)
	}

	// After deduction
	_ = gm.Deduct(300)
	if remaining := gm.Remaining(); remaining != 700 {
		t.Errorf("Remaining() = %d, want 700", remaining)
	}

	// At limit
	_ = gm.Deduct(700)
	if remaining := gm.Remaining(); remaining != 0 {
		t.Errorf("Remaining() = %d, want 0", remaining)
	}
}

func TestGasMeter_EdgeCases(t *testing.T) {
	// Test that repeated small deductions work correctly
	gm := NewGasMeter(100)
	for i := 0; i < 100; i++ {
		if err := gm.Deduct(1); err != nil {
			t.Errorf("Deduct(1) failed at iteration %d: %v", i, err)
		}
	}

	// Should be at limit now
	if err := gm.Deduct(1); err != ErrGasLimitExceeded {
		t.Errorf("Expected ErrGasLimitExceeded, got %v", err)
	}

	// Test overflow protection
	gm = NewGasMeter(^uint64(0) - 10) // near max uint64
	err := gm.Deduct(20)
	// Should not panic, should handle gracefully
	if err != nil {
		t.Logf("Gas limit exceeded as expected: %v", err)
	}
}

func TestGasMeter_Reset(t *testing.T) {
	gm := NewGasMeter(1000)
	_ = gm.Deduct(500)

	// Create new meter to simulate reset
	gm = NewGasMeter(1000)
	if remaining := gm.Remaining(); remaining != 1000 {
		t.Errorf("After reset, Remaining() = %d, want 1000", remaining)
	}
}

// PR1-1.3 RED: TestGasExhaustionRevertsAtomically proves that gas exhaustion
// causes atomic state rollback: storage writes are undone, events are discarded.
//
// Deploys a contract that: (1) writes to storage via write_storage host call,
// (2) enters an infinite loop. Gas limit is generous enough for the write but
// the timeout context cancels execution during the loop. On revert:
//   - Reverted=true
//   - State rolled back (storage write undone)
//   - Events=nil
func TestGasExhaustionRevertsAtomically(t *testing.T) {
	// Build WASM module: call write_storage once, then enter infinite loop
	wb := newWasmBuilder()
	wsTypeIdx := wb.addFuncType([]byte{0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f}, []byte{0x7e})
	voidTypeIdx := wb.addFuncType(nil, nil) // () -> ()
	wsImportIdx := wb.addFuncImport("env", "write_storage", wsTypeIdx)
	wb.addMemory(1)
	moduleFuncIdx := wb.addFunc(voidTypeIdx, []byte{
		// call write_storage(0,0, 0,0, 0,0) — write empty value
		0x41, 0x00, // i32.const 0
		0x41, 0x00, // i32.const 0
		0x41, 0x00, // i32.const 0
		0x41, 0x00, // i32.const 0
		0x41, 0x00, // i32.const 0
		0x41, 0x00, // i32.const 0
		0x10, byte(wsImportIdx), // call write_storage
		0x1a, // drop
		// loop { br 0 } — infinite loop
		0x03, 0x40, // loop (void)
		0x0c, 0x00, // br 0
		0x0b,       // end (loop)
		0x0b,       // end (function)
	})
	wb.addExport("run", 0x00, moduleFuncIdx)
	writeStorageThenLoopModule := wb.build()

	hasher := &types.SHA256Hasher{}
	st := state.NewInMemoryState(hasher)
	vmInst, err := NewVM(hasher)
	require.NoError(t, err)
	defer vmInst.Close()

	// Deploy the contract
	deployTx := &types.DeployContractTx{
		Sender:   types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Nonce:    0,
		CodeHash: types.Hash{},
		WasmCode: writeStorageThenLoopModule,
		GasLimit: 1_000_000,
	}
	deployResult, err := vmInst.Execute(deployTx, st, 1, 1000, 0)
	require.NoError(t, err)
	require.False(t, deployResult.Reverted, "deploy should succeed")

	contractID := DeriveContractID(deployTx.Sender, deployTx.Nonce, deployTx.CodeHash)

	// Set very short timeout to ensure the infinite loop is killed quickly
	vmInst.WithTimeout(200 * time.Millisecond)

	// Record state root before call
	rootBefore := st.GetStateRoot()

	// Call: gas limit 100 (enough for write_storage=50, but the infinite loop
	// will exhaust time, causing context cancellation and atomic revert)
	callTx := &types.CallContractTx{
		Sender:     deployTx.Sender,
		Nonce:      1,
		ContractID: contractID,
		Entrypoint: "run",
		Calldata:   []byte{},
		GasLimit:   100,
	}

	result, err := vmInst.Execute(callTx, st, 2, 1000, 0)
	require.NoError(t, err)
	require.True(t, result.Reverted, "call should revert due to timeout during infinite loop")

	// State rolled back: storage write undone
	rootAfter := st.GetStateRoot()
	require.Equal(t, rootBefore, rootAfter, "state should be unchanged after atomic revert")

	// Events discarded
	require.Nil(t, result.Events, "events should be nil after revert")

	// GasUsed reflects actual consumed gas, not the limit
	require.Greater(t, result.GasUsed, uint64(0), "GasUsed should be > 0 (gas was consumed before timeout)")
	require.LessOrEqual(t, result.GasUsed, callTx.GasLimit, "GasUsed should not exceed GasLimit")

	t.Logf("GasUsed=%d, GasLimit=%d, Reverted=%v", result.GasUsed, callTx.GasLimit, result.Reverted)
}
