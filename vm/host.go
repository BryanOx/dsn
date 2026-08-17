package vm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"golang.org/x/crypto/sha3"
)

// HostEnv holds the execution context for host functions.
type HostEnv struct {
	meter       *GasMeter
	state       state.StateDB
	caller      types.Address
	contractID  types.Hash
	blockHeight uint64
	timestamp   uint64
	txIndex     uint32
	eventLog    *EventLog
	callDepth   int
}

// MaxCallDepth is the maximum allowed contract call depth.
const MaxCallDepth = 64

// EnterCall increments the call depth. Returns error if max depth exceeded.
func (env *HostEnv) EnterCall() error {
	if env.callDepth >= MaxCallDepth {
		return ErrCallDepthExceeded
	}
	env.callDepth++
	return nil
}

// ExitCall decrements the call depth.
func (env *HostEnv) ExitCall() {
	if env.callDepth > 0 {
		env.callDepth--
	}
}

// NewHostEnv creates a new host environment for contract execution.
func NewHostEnv(meter *GasMeter, st state.StateDB, caller types.Address, contractID types.Hash, blockHeight, timestamp uint64, txIndex uint32) *HostEnv {
	return &HostEnv{
		meter:       meter,
		state:       st,
		caller:      caller,
		contractID:  contractID,
		blockHeight: blockHeight,
		timestamp:   timestamp,
		txIndex:     txIndex,
		eventLog:    NewEventLog(),
		callDepth:   0,
	}
}

// BuildHostModule builds a wazero host module with deterministic imports.
// Contracts can import functions from the "env" namespace.
func BuildHostModule(ctx context.Context, runtime wazero.Runtime, env *HostEnv) (api.Module, error) {
	builder := runtime.NewHostModuleBuilder("env")

	// read_storage(contractID_ptr, contractID_len, key_ptr, key_len, out_ptr) -> error_code (0=ok)
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, contractIDPtr uint32, contractIDLen uint32, keyPtr uint32, keyLen uint32, outPtr uint32) uint64 {
			// Deduct gas before doing work
			if err := env.meter.Deduct(GasReadStorage); err != nil {
				return 1 // error: gas limit
			}

			mem := module.Memory()
			// Bounds check: verify all pointers and lengths are within bounds
			if contractIDPtr+contractIDLen > uint32(mem.Size()) ||
				keyPtr+keyLen > uint32(mem.Size()) ||
				outPtr > uint32(mem.Size()) {
				return 2 // error: out of bounds
			}

			// Read contractID from WASM memory
			contractIDBytes, ok := mem.Read(contractIDPtr, contractIDLen)
			if !ok {
				return 2 // error: read failure
			}
			var contractID types.Hash
			copy(contractID[:], contractIDBytes)

			// Read key from WASM memory
			key, ok := mem.Read(keyPtr, keyLen)
			if !ok {
				return 2 // error: read failure
			}

			// Read from state
			value, err := env.state.GetContractStorage(contractID, key)
			if err != nil {
				return 3 // error: key not found
			}

			// Write value to output pointer
			if len(value) > 0 {
				if outPtr+uint32(len(value)) > uint32(mem.Size()) {
					return 2 // error: out of bounds
				}
				if !mem.Write(outPtr, value) {
					return 2 // error: write failure
				}
			}

			return 0 // success
		}).
		Export("read_storage")

	// write_storage(contractID_ptr, contractID_len, key_ptr, key_len, value_ptr, value_len) -> error_code
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, contractIDPtr uint32, contractIDLen uint32, keyPtr uint32, keyLen uint32, valuePtr uint32, valueLen uint32) uint64 {
			// Deduct base gas
			if err := env.meter.Deduct(GasWriteStorage); err != nil {
				return 1 // error: gas limit
			}

			// Check value size limit
			if valueLen > MaxStorageValueSize {
				return 4 // error: value too large
			}

			// Deduct per-byte gas
			if err := env.meter.Deduct(GasPerStorageByte * uint64(valueLen)); err != nil {
				return 1 // error: gas limit
			}

			mem := module.Memory()
			// Bounds check
			if contractIDPtr+contractIDLen > uint32(mem.Size()) ||
				keyPtr+keyLen > uint32(mem.Size()) ||
				valuePtr+valueLen > uint32(mem.Size()) {
				return 2 // error: out of bounds
			}

			// Read contractID
			contractIDBytes, ok := mem.Read(contractIDPtr, contractIDLen)
			if !ok {
				return 2 // error: read failure
			}
			var contractID types.Hash
			copy(contractID[:], contractIDBytes)

			// Read key
			key, ok := mem.Read(keyPtr, keyLen)
			if !ok {
				return 2 // error: read failure
			}

			// Read value
			value, ok := mem.Read(valuePtr, valueLen)
			if !ok {
				return 2 // error: read failure
			}

			// Write to state
			if err := env.state.SetContractStorage(contractID, key, value); err != nil {
				return 5 // error: write failure
			}

			return 0 // success
		}).
		Export("write_storage")

	// emit_event(topic_ptr, topic_len, data_ptr, data_len) -> error_code
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, topicPtr uint32, topicLen uint32, dataPtr uint32, dataLen uint32) {
			// Deduct gas
			if err := env.meter.Deduct(GasEmitEvent); err != nil {
				return
			}

			mem := module.Memory()
			// Bounds check
			if topicPtr+topicLen > uint32(mem.Size()) ||
				dataPtr+dataLen > uint32(mem.Size()) {
				return
			}

			// Read topic
			topicBytes, ok := mem.Read(topicPtr, topicLen)
			if !ok {
				return
			}

			// Read data
			data, ok := mem.Read(dataPtr, dataLen)
			if !ok {
				return
			}

			// Emit event
			env.eventLog.Emit(env.contractID, string(topicBytes), data, env.txIndex, env.blockHeight)
		}).
		Export("emit_event")

	// read_caller(out_ptr) -> error_code (writes 20 bytes to out_ptr)
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, outPtr uint32) uint64 {
			if err := env.meter.Deduct(GasReadCaller); err != nil {
				return 1 // error: gas limit
			}

			mem := module.Memory()
			if outPtr+20 > uint32(mem.Size()) {
				return 2 // error: out of bounds
			}

			// Write caller address (20 bytes) to memory
			if !mem.Write(outPtr, env.caller[:]) {
				return 2 // error: write failure
			}

			return 0 // success
		}).
		Export("read_caller")

	// read_block_height() -> uint64
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module) uint64 {
			if err := env.meter.Deduct(GasReadBlockHeight); err != nil {
				return 0
			}
			return env.blockHeight
		}).
		Export("read_block_height")

	// read_block_timestamp() -> uint64
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module) uint64 {
			if err := env.meter.Deduct(GasReadBlockTimestamp); err != nil {
				return 0
			}
			return env.timestamp
		}).
		Export("read_block_timestamp")

	// transfer_token(recipient_ptr, recipient_len, amount_hi, amount_lo) -> error_code
	// amount is 128-bit split into hi (upper 64) and lo (lower 64)
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, recipientPtr uint32, recipientLen uint32, amountHi uint64, amountLo uint64) uint64 {
			if err := env.meter.Deduct(GasTransferToken); err != nil {
				return 1 // error: gas limit
			}

			mem := module.Memory()
			// Bounds check
			if recipientPtr+recipientLen > uint32(mem.Size()) {
				return 2 // error: out of bounds
			}

			// Read recipient address
			recipientBytes, ok := mem.Read(recipientPtr, recipientLen)
			if !ok || len(recipientBytes) != 20 {
				return 2 // error: invalid address
			}

			var recipient types.Address
			copy(recipient[:], recipientBytes)

			// Build 128-bit amount from hi:lo
			var amountBuf [16]byte
			binary.BigEndian.PutUint64(amountBuf[:8], amountHi)
			binary.BigEndian.PutUint64(amountBuf[8:], amountLo)
			var amt types.Amount
			if err := amt.UnmarshalBinary(amountBuf[:]); err != nil {
				return 3 // error: invalid amount
			}

			// Execute transfer: caller → recipient
			if err := state.Transfer(env.state, env.caller, recipient, amt, nil); err != nil {
				return 3 // error: transfer failed
			}

			// Emit transfer event
			eventData := make([]byte, 0, 56)
			eventData = append(eventData, env.caller[:]...)
			eventData = append(eventData, recipient[:]...)
			eventData = append(eventData, amountBuf[:]...)
			env.eventLog.Emit(env.contractID, "transfer", eventData, env.txIndex, env.blockHeight)

			return 0 // success
		}).
		Export("transfer_token")

	// sha256(ptr, len, out_ptr) -> void
	// Reads len bytes from WASM memory at ptr, computes SHA-256 hash,
	// writes 32-byte hash to out_ptr.
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, ptr uint32, length uint32, outPtr uint32) {
			if err := env.meter.Deduct(GasNondeterministic); err != nil {
				return
			}

			mem := module.Memory()
			if ptr+length > uint32(mem.Size()) || outPtr+32 > uint32(mem.Size()) {
				return
			}

			data, ok := mem.Read(ptr, length)
			if !ok {
				return
			}

			hash := sha256.Sum256(data)
			mem.Write(outPtr, hash[:])
		}).
		Export("sha256")

	// keccak256(ptr, len, out_ptr) -> void
	// Reads len bytes from WASM memory at ptr, computes Keccak-256 hash,
	// writes 32-byte hash to out_ptr.
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, ptr uint32, length uint32, outPtr uint32) {
			if err := env.meter.Deduct(GasNondeterministic); err != nil {
				return
			}

			mem := module.Memory()
			if ptr+length > uint32(mem.Size()) || outPtr+32 > uint32(mem.Size()) {
				return
			}

			data, ok := mem.Read(ptr, length)
			if !ok {
				return
			}

			h := sha3.NewLegacyKeccak256()
			h.Write(data)
			var hash [32]byte
			copy(hash[:], h.Sum(nil))
			mem.Write(outPtr, hash[:])
		}).
		Export("keccak256")

	// get_balance(addr_ptr, out_ptr) -> void
	// Reads 20-byte address from WASM memory at addr_ptr,
	// looks up account balance, writes 16 bytes (two big-endian uint64s: hi, lo) to out_ptr.
	builder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, addrPtr uint32, outPtr uint32) {
			if err := env.meter.Deduct(GasReadStorage); err != nil {
				return
			}

			mem := module.Memory()
			if addrPtr+20 > uint32(mem.Size()) || outPtr+16 > uint32(mem.Size()) {
				return
			}

			addrBytes, ok := mem.Read(addrPtr, 20)
			if !ok {
				return
			}

			var addr types.Address
			copy(addr[:], addrBytes)

			acc, err := env.state.GetAccount(addr)
			if err != nil {
				// Account not found: write zeros
				var zero [16]byte
				mem.Write(outPtr, zero[:])
				return
			}

			// Get balance as bytes (big-endian, up to 16 bytes)
			balBytes, err := acc.Balance.MarshalBinary()
			if err != nil {
				var zero [16]byte
				mem.Write(outPtr, zero[:])
				return
			}

			// Pad to 16 bytes (big-endian)
			var bal16 [16]byte
			copy(bal16[16-len(balBytes):], balBytes)
			mem.Write(outPtr, bal16[:])
		}).
		Export("get_balance")

	return builder.Instantiate(ctx)
}
