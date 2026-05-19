//go:build wasip1
// +build wasip1

package wasm

// This package provides Go WASM helper functions for DSN smart contracts.
// It declares go:wasmimport bindings for all 7 host functions and wraps
// them in ergonomic Go helpers.
//
// Build with: GOOS=wasip1 GOARCH=wasm go build

// ----------------------------------------------------------------------------
// Memory Layout
// ----------------------------------------------------------------------------
// The SDK uses fixed memory offsets for host function calls.
// These offsets must be in the WASM linear memory space.
// Offset 0-31: ContractID (32 bytes)
// Offset 32-63: Caller address buffer (20 bytes + padding)
// Offset 64-95: Result buffer for reads (max 64KB)
// Offset 96+: Temporary storage for keys, values, topics, data

const (
	// Memory offsets
	// ContractID is written by VM at offset 1024 before calling entrypoints
	contractIDOffset  = 1024
	callerBufOffset   = 32
	resultBufOffset   = 64
	tempBufOffset     = 256

	// Sizes
	callerBufSize = 32
	resultBufSize = 65536 // max storage value size
	tempBufSize   = 65536
)

// Global buffers - these are placed in WASM linear memory
// The Go compiler places these at specific offsets
var (
	// contractIDBytes is the contract's ID (32 bytes)
	contractIDBytes [32]byte

	// callerBuf is where read_caller writes the caller address
	callerBuf [callerBufSize]byte

	// resultBuf is where read_storage writes the result
	resultBuf [resultBufSize]byte

	// tempBuf is for temporary storage (keys, values, topics, data)
	tempBuf [tempBufSize]byte

	// calldata holds the calldata passed to the contract
	calldata []byte

	// calldataOffset tracks current read position in calldata
	calldataOffset int
)

// SetContractID sets the contract ID for storage operations.
// This should be called in the contract's init() function.
func SetContractID(id [32]byte) {
	contractIDBytes = id
}

// SetCalldata sets the calldata for the current call.
// This is called by the VM before executing the contract.
func SetCalldata(data []byte) {
	calldata = data
	calldataOffset = 0
}

// SetCalldataFromMemory reads calldata from a given memory offset.
// This is used when the VM passes calldata via memory pointer+length args.
// The VM writes calldata to offset 0 in WASM memory, and this function
// ensures the calldata global is properly set for ReadArg() to work.
func SetCalldataFromMemory(ptr uint32, len uint32) {
	// In Go WASM, the global calldata variable is already in linear memory.
	// When VM passes ptr=0 (where calldata was written), we just need to
	// ensure our calldata global is pointing to the right data.
	// Since SetCalldata was already called by the VM, calldata should already
	// be set. This function is for compatibility with the calling convention.
	//
	// For now, we just reset the offset since calldata is already set.
	// The ptr parameter indicates where in WASM memory to read from,
	// but since calldata global already points to that data, we just
	// reset our read position.
	calldataOffset = 0
}

// ----------------------------------------------------------------------------
// Host Function Imports
// ----------------------------------------------------------------------------

//go:wasmimport env read_storage
func wasm_read_storage(contractIDPtr uint32, contractIDLen uint32, keyPtr uint32, keyLen uint32, outPtr uint32) uint64

//go:wasmimport env write_storage
func wasm_write_storage(contractIDPtr uint32, contractIDLen uint32, keyPtr uint32, keyLen uint32, valuePtr uint32, valueLen uint32) uint64

//go:wasmimport env emit_event
func wasm_emit_event(topicPtr uint32, topicLen uint32, dataPtr uint32, dataLen uint32)

//go:wasmimport env read_caller
func wasm_read_caller(outPtr uint32) uint64

//go:wasmimport env read_block_height
func wasm_read_block_height() uint64

//go:wasmimport env read_block_timestamp
func wasm_read_block_timestamp() uint64

//go:wasmimport env transfer_token
func wasm_transfer_token(recipientPtr uint32, recipientLen uint32, amountHi uint64, amountLo uint64) uint64

// ----------------------------------------------------------------------------
// Helper Functions
// ----------------------------------------------------------------------------

// ReadCaller returns the caller's address (20 bytes).
func ReadCaller() [20]byte {
	// Call host function to write caller to buffer
	wasm_read_caller(callerBufOffset)

	// Copy to result
	var result [20]byte
	copy(result[:], callerBuf[:20])
	return result
}

// ReadStorage reads a value from contract storage by key.
// Returns the value and nil error on success.
func ReadStorage(key []byte) ([]byte, error) {
	if len(key) > MaxKeySize {
		return nil, ErrKeyTooLarge
	}

	// Copy key to temp buffer
	keyLen := len(key)
	copy(tempBuf[:keyLen], key)

	// Call read_storage
	resultLen := wasm_read_storage(
		contractIDOffset, 32,
		tempBufOffset, uint32(keyLen),
		resultBufOffset,
	)

	if resultLen == 0 {
		return nil, ErrKeyNotFound
	}

	// Copy result
	result := make([]byte, resultLen)
	copy(result, resultBuf[:resultLen])
	return result, nil
}

// WriteStorage writes a value to contract storage by key.
// Returns nil on success, error otherwise.
func WriteStorage(key []byte, value []byte) error {
	if len(key) > MaxKeySize {
		return ErrKeyTooLarge
	}
	if len(value) > MaxValueSize {
		return ErrValueTooLarge
	}

	// Copy key to temp buffer
	keyLen := len(key)
	copy(tempBuf[:keyLen], key)

	// Copy value after key
	valueOffset := tempBufOffset + keyLen
	valueLen := len(value)
	copy(tempBuf[keyLen:valueLen+keyLen], value)

	// Call write_storage
	errCode := wasm_write_storage(
		contractIDOffset, 32,
		tempBufOffset, uint32(keyLen),
		uint32(valueOffset), uint32(valueLen),
	)

	if errCode != 0 {
		return ErrStorageWriteFailed
	}
	return nil
}

// EmitEvent emits an event with the given topic and data.
func EmitEvent(topic string, data []byte) {
	topicLen := len(topic)
	dataLen := len(data)

	// Copy topic to temp buffer
	copy(tempBuf[:topicLen], topic)

	// Copy data after topic
	dataOffset := tempBufOffset + topicLen
	copy(tempBuf[topicLen:topicLen+dataLen], data)

	// Call emit_event
	wasm_emit_event(
		tempBufOffset, uint32(topicLen),
		uint32(dataOffset), uint32(dataLen),
	)
}

// ReadCalldata returns the calldata passed to the entrypoint.
func ReadCalldata() []byte {
	return calldata
}

// ReadArg reads an argument from calldata.
// Advances the internal offset for sequential reads.
func ReadArg(size int) []byte {
	if calldataOffset+size > len(calldata) {
		return nil
	}
	result := calldata[calldataOffset : calldataOffset+size]
	calldataOffset += size
	return result
}

// ResetCalldata resets the calldata read offset to the beginning.
func ResetCalldata() {
	calldataOffset = 0
}

// BEToUint64 converts big-endian bytes to uint64.
func BEToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return uint64(b[0])<<56 |
		uint64(b[1])<<48 |
		uint64(b[2])<<40 |
		uint64(b[3])<<32 |
		uint64(b[4])<<24 |
		uint64(b[5])<<16 |
		uint64(b[6])<<8 |
		uint64(b[7])
}

// Uint64ToBE converts uint64 to big-endian bytes.
func Uint64ToBE(v uint64) []byte {
	b := make([]byte, 8)
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
	return b
}

// ReadBlockHeight returns the current block height.
func ReadBlockHeight() uint64 {
	return wasm_read_block_height()
}

// ReadBlockTimestamp returns the current block timestamp (unix epoch seconds).
func ReadBlockTimestamp() uint64 {
	return wasm_read_block_timestamp()
}

// Transfer transfers tokens to a recipient.
// Amount is specified as a 128-bit value (hi:lo).
func Transfer(recipient [20]byte, hi uint64, lo uint64) error {
	// Copy recipient to temp buffer
	copy(tempBuf[:20], recipient[:])

	errCode := wasm_transfer_token(
		tempBufOffset, 20,
		hi, lo,
	)

	if errCode != 0 {
		return ErrTransferFailed
	}
	return nil
}

// TransferUint64 transfers tokens using a simple uint64 amount.
// Convenience wrapper for amounts that fit in 64 bits.
func TransferUint64(recipient [20]byte, amount uint64) error {
	return Transfer(recipient, 0, amount)
}

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

// Error codes returned by host functions
const (
	ErrCodeKeyNotFound    = 3
	ErrCodeValueTooLarge = 4
	ErrCodeWriteFailed   = 5
)

var (
	ErrKeyNotFound        = wasmError{"key not found"}
	ErrKeyTooLarge       = wasmError{"key too large (max 256 bytes)"}
	ErrValueTooLarge     = wasmError{"value too large (max 65536 bytes)"}
	ErrStorageWriteFailed = wasmError{"storage write failed"}
	ErrTransferFailed    = wasmError{"transfer failed"}
)

type wasmError struct {
	msg string
}

func (e wasmError) Error() string {
	return e.msg
}

// ----------------------------------------------------------------------------
// Constants
// ----------------------------------------------------------------------------

// MaxKeySize is the maximum size for storage keys.
const MaxKeySize = 256

// MaxValueSize is the maximum size for storage values.
const MaxValueSize = 65536

// CallerAddressSize is the size of caller addresses (20 bytes).
const CallerAddressSize = 20

// BlockHeight is a convenience alias for ReadBlockHeight.
var BlockHeight = ReadBlockHeight

// BlockTimestamp is a convenience alias for ReadBlockTimestamp.
var BlockTimestamp = ReadBlockTimestamp