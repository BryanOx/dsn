package vm

// Gas costs for WASM instructions (categories)
const (
	GasBaseOp           uint64 = 1   // basic arithmetic, local get/set
	GasMemoryOp         uint64 = 3   // memory load/store
	GasControlFlow      uint64 = 5   // branches, calls
	GasNondeterministic uint64 = 100 // operations that need extra verification
)

// Gas costs for host functions
const (
	GasReadStorage        uint64 = 20
	GasWriteStorage       uint64 = 50
	GasEmitEvent          uint64 = 30
	GasReadCaller         uint64 = 10
	GasReadBlockHeight    uint64 = 10
	GasReadBlockTimestamp uint64 = 10
	GasTransferToken      uint64 = 100
)

// Gas costs for contract deployment
const (
	GasDeployBase     uint64 = 1000 // base gas for contract deployment
	GasPerCodeByte    uint64 = 1    // per byte of WASM code
	GasPerStorageByte uint64 = 2    // per byte of storage value
)

// GasMeter tracks gas consumption during contract execution.
type GasMeter struct {
	Limit uint64
	Used  uint64
}

// NewGasMeter creates a new gas meter with the specified limit.
func NewGasMeter(limit uint64) *GasMeter {
	return &GasMeter{Limit: limit, Used: 0}
}

// Deduct attempts to consume `cost` gas. Returns error if insufficient.
func (gm *GasMeter) Deduct(cost uint64) error {
	if gm.Used+cost > gm.Limit {
		return ErrGasLimitExceeded
	}
	gm.Used += cost
	return nil
}

// Remaining returns the gas left.
func (gm *GasMeter) Remaining() uint64 {
	return gm.Limit - gm.Used
}
