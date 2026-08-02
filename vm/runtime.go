package vm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// MaxContractSize is the maximum allowed WASM bytecode size (1 MiB).
const MaxContractSize = 1 * 1024 * 1024

// NewRuntime creates a deterministic wazero runtime with no access to
// host FS, network, randomness, or clocks. This guarantees replay-safe
// WASM execution across all nodes.
func NewRuntime(ctx context.Context) (wazero.Runtime, error) {
	config := wazero.NewRuntimeConfig().
		// Use WebAssembly 1.0 for deterministic execution
		WithCoreFeatures(api.CoreFeaturesV1).
		// Limit memory to 256 pages (16 MiB) per contract
		WithMemoryLimitPages(256).
		// Close on context done to enforce timeouts
		WithCloseOnContextDone(true)

	runtime := wazero.NewRuntimeWithConfig(ctx, config)

	// WASI is NOT imported — contracts cannot access FS, network, clocks
	// wasi_snapshot_preview1 is intentionally excluded

	return runtime, nil
}

// CompileModule deterministically compiles WASM bytecode.
// Returns error if the module is invalid.
func CompileModule(ctx context.Context, runtime wazero.Runtime, code []byte) (wazero.CompiledModule, error) {
	if len(code) > MaxContractSize {
		return nil, ErrCodeTooLarge
	}
	compiled, err := runtime.CompileModule(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWasm, err)
	}
	return compiled, nil
}
