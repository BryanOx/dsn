package vm

import (
	"math"
	"testing"

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
