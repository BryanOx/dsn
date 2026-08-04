package vm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/tetratelabs/wazero"
)

// VM is the deterministic WASM execution engine.
type VM struct {
	runtime          wazero.Runtime
	hasher           types.Hasher
	maxDepth         uint32
	executionTimeout time.Duration
}

// WithTimeout sets the execution timeout for the VM.
func (vm *VM) WithTimeout(d time.Duration) *VM {
	vm.executionTimeout = d
	return vm
}

// NewVM creates a new VM with a deterministic wazero runtime.
func NewVM(hasher types.Hasher) (*VM, error) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create WASM runtime: %w", err)
	}
	return &VM{
		runtime:  runtime,
		hasher:   hasher,
		maxDepth: 64,
	}, nil
}

// ExecutionResult holds the output of a single contract execution.
type ExecutionResult struct {
	GasUsed         uint64
	ReturnData      []byte
	Events          []types.Event
	Reverted        bool
	ContractAddress *types.Address // Set for deploy operations
	GasLimit        uint64         // Gas limit for the execution
}

// Execute runs a contract call or deployment.
// For DeployContractTx: stores the bytecode and metadata.
// For CallContractTx: instantiates the module and calls the entrypoint.
// The blockHeight and blockTimestamp parameters provide the current block context
// for contract execution (e.g., for block-dependent logic).
// The txIndex parameter indicates the transaction's position in the block.
func (vm *VM) Execute(tx interface{}, st state.StateDB, blockHeight uint64, blockTimestamp uint64, txIndex uint32) (*ExecutionResult, error) {
	// Take state snapshot for rollback
	snapID := st.Snapshot()

	// Apply timeout to execution
	timeout := vm.executionTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create gas meter for the transaction
	var meter *GasMeter
	switch t := tx.(type) {
	case *types.DeployContractTx:
		meter = NewGasMeter(t.GasLimit)
	case *types.CallContractTx:
		meter = NewGasMeter(t.GasLimit)
	}

	result, err := func() (*ExecutionResult, error) {
		switch t := tx.(type) {
		case *types.DeployContractTx:
			return vm.executeDeploy(ctx, t, st, blockHeight, blockTimestamp, meter, txIndex)
		case *types.CallContractTx:
			return vm.executeCall(ctx, t, st, blockHeight, blockTimestamp, meter, txIndex)
		default:
			return nil, fmt.Errorf("unsupported transaction type: %T", tx)
		}
	}()

	// Rollback on error or revert
	if err != nil || (result != nil && result.Reverted) {
		if rbErr := st.RevertToSnapshot(snapID); rbErr != nil {
			return nil, fmt.Errorf("rollback failed: %w (original error: %v)", rbErr, err)
		}
		// T8-4: Discard events when execution reverts
		if result != nil {
			result.Events = nil
		}
	}

	return result, err
}

func (vm *VM) executeDeploy(ctx context.Context, tx *types.DeployContractTx, st state.StateDB, blockHeight uint64, blockTimestamp uint64, meter *GasMeter, txIndex uint32) (*ExecutionResult, error) {
	// Validate WASM module
	compiled, err := CompileModule(ctx, vm.runtime, tx.WasmCode)
	if err != nil {
		return nil, err
	}
	defer compiled.Close(ctx)

	// Compute contract ID
	contractID := DeriveContractID(tx.Sender, tx.Nonce, tx.CodeHash)

	// Check if contract already exists
	_, err = st.GetContractMeta(contractID)
	if err == nil {
		return nil, types.ErrContractAlreadyExists
	}

	// T2-2: Enforce deploy gas limit - deduct deployment gas immediately
	deploymentGas := GasDeployBase + GasPerCodeByte*uint64(len(tx.WasmCode))
	if err := meter.Deduct(deploymentGas); err != nil {
		return &ExecutionResult{
			GasUsed:  meter.Used,
			Reverted: true,
			GasLimit: tx.GasLimit,
		}, ErrGasLimitExceeded
	}

	// For deployment, we don't need the host environment as there's no runtime execution
	// But we set block context for consistency and potential future use

	// Store bytecode
	if err := vm.storeCode(st, contractID, tx.WasmCode); err != nil {
		return nil, fmt.Errorf("store code: %w", err)
	}

	// Store metadata — use the actual code hash, not the tx.CodeHash (which callers may leave zero)
	actualCodeHash := types.Hash(SHA256Sum(tx.WasmCode))
	meta := &types.ContractMetadata{
		ContractID:  contractID,
		CodeHash:    actualCodeHash,
		Deployer:    tx.Sender,
		BlockHeight: blockHeight,
	}
	if err := vm.storeContractMeta(st, contractID, meta); err != nil {
		return nil, fmt.Errorf("store meta: %w", err)
	}

	// Execute constructor if the module exports "init" or "constructor"
	exportedFuncs := compiled.ExportedFunctions()
	constructorExports := []string{"init", "constructor"}
	for _, exportName := range constructorExports {
		if _, exists := exportedFuncs[exportName]; exists {
			// Create a fresh GasMeter for constructor that starts from remaining gas
			// (per spec: "Constructor execution gets a fresh GasMeter that starts from remaining gas")
			constructorMeter := NewGasMeter(meter.Remaining())
			hostEnv := NewHostEnv(constructorMeter, st, tx.Sender, contractID, blockHeight, blockTimestamp, txIndex)

			hostModule, err := BuildHostModule(ctx, vm.runtime, hostEnv)
			if err != nil {
				return nil, fmt.Errorf("constructor host module: %w", err)
			}
			defer hostModule.Close(ctx)

			moduleConfig := wazero.NewModuleConfig().
				WithName(fmt.Sprintf("constructor_%x", contractID[:8]))

			module, err := vm.runtime.InstantiateModule(ctx, compiled, moduleConfig)
			if err != nil {
				return nil, fmt.Errorf("constructor instantiation: %w", err)
			}
			defer module.Close(ctx)

			// Write contractID to fixed memory location for SDK access
			mem := module.Memory()
			if !mem.Write(ContractIDMemOffset, contractID[:]) {
				return nil, fmt.Errorf("failed to write contractID for constructor")
			}

			constructor := module.ExportedFunction(exportName)
			if constructor == nil {
				return nil, fmt.Errorf("constructor export '%s' declared but not found", exportName)
			}

			if err := hostEnv.EnterCall(); err != nil {
				return nil, err
			}

			_, constrErr := constructor.Call(ctx)
			hostEnv.ExitCall()

			if constrErr != nil {
				return nil, fmt.Errorf("constructor execution failed: %w", constrErr)
			}

			// Add constructor gas to total
			meter.Used += constructorMeter.Used
			break // Only run one constructor
		}
	}

	var contractAddr types.Address
	copy(contractAddr[:], contractID[:20])

	return &ExecutionResult{
		GasUsed:         meter.Used,
		ReturnData:      contractID[:],
		Reverted:        false,
		ContractAddress: &contractAddr,
		GasLimit:        tx.GasLimit,
	}, nil
}

func (vm *VM) executeCall(ctx context.Context, tx *types.CallContractTx, st state.StateDB, blockHeight uint64, blockTimestamp uint64, meter *GasMeter, txIndex uint32) (*ExecutionResult, error) {
	// Load contract code via loadCode (looks up codeHash from metadata first)
	code, err := vm.loadCode(st, tx.ContractID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContractNotFound, err)
	}

	// Compile module
	compiled, err := CompileModule(ctx, vm.runtime, code)
	if err != nil {
		return nil, err
	}
	defer compiled.Close(ctx)

	// Validate imports - only "env" module is allowed
	if err := validateImports(compiled); err != nil {
		return nil, err
	}

	// Use the pre-created gas meter from Execute()

	// Create host environment with real block context
	hostEnv := NewHostEnv(meter, st, tx.Sender, tx.ContractID, blockHeight, blockTimestamp, txIndex)

	// Build host module with all host functions
	hostModule, err := BuildHostModule(ctx, vm.runtime, hostEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to build host module: %w", err)
	}
	defer hostModule.Close(ctx)

	// Create module config - no WASI, deterministic config
	// Memory limits enforced via RuntimeConfig.WithMemoryLimitPages in NewRuntime()
	moduleConfig := wazero.NewModuleConfig().
		WithName(fmt.Sprintf("contract_%x", tx.ContractID[:8]))

	// Instantiate the contract module with the host module linked
	module, err := vm.runtime.InstantiateModule(ctx, compiled, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate contract module: %w", err)
	}
	defer module.Close(ctx)

	// Get the entrypoint function
	entrypoint := module.ExportedFunction(tx.Entrypoint)
	if entrypoint == nil {
		return nil, fmt.Errorf("%w: function '%s' not found", ErrHostFunction, tx.Entrypoint)
	}

	// Prepare call parameters - pass calldata as memory
	mem := module.Memory()
	var callParams []uint64
	if len(tx.Calldata) > 0 {
		// Write calldata to WASM memory at offset 0
		if uint32(len(tx.Calldata)) > mem.Size() {
			return nil, fmt.Errorf("calldata too large")
		}
		if !mem.Write(0, tx.Calldata) {
			return nil, fmt.Errorf("failed to write calldata")
		}
		// Pass pointer (0) and length as params
		callParams = []uint64{0, uint64(len(tx.Calldata))}
	}

	// Write contractID to a fixed memory location for SDK access
	if !mem.Write(ContractIDMemOffset, tx.ContractID[:]) {
		return nil, fmt.Errorf("failed to write contractID")
	}

	// Enforce call depth limit
	if err := hostEnv.EnterCall(); err != nil {
		return nil, err
	}
	defer hostEnv.ExitCall()

	// Call the entrypoint function
	var results []uint64
	if len(callParams) > 0 {
		results, err = entrypoint.Call(ctx, callParams...)
	} else {
		results, err = entrypoint.Call(ctx)
	}

	// Check for execution error
	if err != nil {
		// Check if it was a gas limit error
		if meter.Used >= tx.GasLimit {
			return &ExecutionResult{
				GasUsed:  meter.Used,
				Reverted: true,
			}, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrHostFunction, err)
	}

	// Collect return data if any
	var returnData []byte
	if len(results) > 0 && results[0] != 0 {
		// Return code non-zero means error
		return &ExecutionResult{
			GasUsed:  meter.Used,
			Reverted: true,
			GasLimit: tx.GasLimit,
		}, nil
	}
	// If return data was written to memory, read it
	// For now, we return empty - contracts can write to known offsets

	// Collect events from the event log
	events := hostEnv.eventLog.Events()

	return &ExecutionResult{
		GasUsed:    meter.Used,
		ReturnData: returnData,
		Events:     events,
		Reverted:   false,
		GasLimit:   tx.GasLimit,
	}, nil
}

// allowedHostFunctions is the whitelist of allowed "env" function imports.
var allowedHostFunctions = map[string]bool{
	"read_storage":         true,
	"write_storage":        true,
	"emit_event":           true,
	"read_caller":          true,
	"read_block_height":    true,
	"read_block_timestamp": true,
	"transfer_token":       true,
}

// validateImports checks that the compiled module only imports from allowed modules.
// Only the "env" module with whitelisted host functions is permitted.
func validateImports(compiled wazero.CompiledModule) error {
	for _, imp := range compiled.ImportedFunctions() {
		modName, fnName, _ := imp.Import()
		if modName != "env" {
			return fmt.Errorf("%w: module '%s' is not allowed (only 'env' is permitted)", ErrInvalidImport, modName)
		}
		if !allowedHostFunctions[fnName] {
			return fmt.Errorf("%w: function '%s' from module 'env' is not allowed", ErrInvalidImport, fnName)
		}
	}
	return nil
}

// storeCode stores the contract bytecode using the state's SetCode method.
func (vm *VM) storeCode(st state.StateDB, contractID types.Hash, code []byte) error {
	codeHash := types.Hash(SHA256Sum(code))
	return st.SetCode(codeHash, code)
}

// loadCode loads the contract bytecode from the state's GetCode method.
func (vm *VM) loadCode(st state.StateDB, contractID types.Hash) ([]byte, error) {
	// First get metadata to get code hash
	meta, err := st.GetContractMeta(contractID)
	if err != nil {
		return nil, err
	}
	// Then get code by code hash
	return st.GetCode(meta.CodeHash)
}

// storeContractMeta stores the contract metadata using the state's SetContractMeta method.
func (vm *VM) storeContractMeta(st state.StateDB, contractID types.Hash, meta *types.ContractMetadata) error {
	meta.ContractID = contractID
	return st.SetContractMeta(contractID, meta)
}

// loadContractMeta loads the contract metadata from the state's GetContractMeta method.
func (vm *VM) loadContractMeta(st state.StateDB, contractID types.Hash) (*types.ContractMetadata, error) {
	return st.GetContractMeta(contractID)
}

// SHA256Sum is a helper to compute SHA256 hash
func SHA256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// ContractIDMemOffset is the fixed memory offset where contractID is written
// before calling contract entrypoints. This is used by the SDK to locate
// the contractID for storage operations.
const ContractIDMemOffset uint32 = 1024

// Close releases the VM's resources.
func (vm *VM) Close() error {
	if vm.runtime != nil {
		return vm.runtime.Close(context.Background())
	}
	return nil
}
