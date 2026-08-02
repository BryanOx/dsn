//go:build wasip1

package main

import "github.com/dsn/dsn/sdk/wasm"

// State keys
var (
	arbiterKey     = []byte("arbiter")
	depositorKey   = []byte("depositor")
	escrowedKey    = []byte("escrowed")
	beneficiaryKey = []byte("beneficiary")
	releasedKey    = []byte("released")
)

//export init
func init() {
	data := wasm.ReadCalldata()
	if len(data) >= 20 {
		// Store arbiter address (first 20 bytes of calldata)
		wasm.WriteStorage(arbiterKey, data[:20])
	}
}

//export deposit
func deposit(ptr uint32, length uint32) uint64 {
	wasm.SetCalldataFromMemory(ptr, length)

	// Read depositor is the caller
	depositor := wasm.ReadCaller()

	// Check if already escrowed
	existing, err := wasm.ReadStorage(escrowedKey)
	if err == nil && len(existing) > 0 {
		return 1 // already escrowed
	}

	// Record depositor
	wasm.WriteStorage(depositorKey, depositor[:])

	// Record escrowed amount (next 8 bytes of calldata)
	amtData := wasm.ReadArg(8)
	if len(amtData) >= 8 {
		wasm.WriteStorage(escrowedKey, amtData)
	}

	// Emit Deposit event
	wasm.EmitEvent("Deposit", depositor[:])
	return 0
}

//export release
func release(ptr uint32, length uint32) uint64 {
	wasm.SetCalldataFromMemory(ptr, length)

	// Only arbiter can release
	caller := wasm.ReadCaller()
	arbiterVal, err := wasm.ReadStorage(arbiterKey)
	if err != nil || len(arbiterVal) < 20 {
		return 1 // no arbiter set
	}
	if !bytesEqual(caller[:], arbiterVal[:20]) {
		return 2 // not authorized
	}

	// Read beneficiary from calldata
	beneficiary := wasm.ReadArg(20)
	if len(beneficiary) < 20 {
		return 3 // invalid beneficiary
	}

	// Mark as released
	wasm.WriteStorage(releasedKey, []byte{1})
	wasm.WriteStorage(beneficiaryKey, beneficiary[:20])

	// Read escrowed amount
	escrowed, _ := wasm.ReadStorage(escrowedKey)
	amount := wasm.BEToUint64(escrowed)

	// Emit Release event
	eventData := make([]byte, 0, 28)
	eventData = append(eventData, beneficiary[:20]...)
	eventData = append(eventData, wasm.Uint64ToBE(amount)...)
	wasm.EmitEvent("Release", eventData)

	return 0
}

//export refund
func refund(ptr uint32, length uint32) uint64 {
	wasm.SetCalldataFromMemory(ptr, length)

	// Only arbiter can refund
	caller := wasm.ReadCaller()
	arbiterVal, err := wasm.ReadStorage(arbiterKey)
	if err != nil || len(arbiterVal) < 20 {
		return 1
	}
	if !bytesEqual(caller[:], arbiterVal[:20]) {
		return 2 // not authorized
	}

	// Check if already released
	released, _ := wasm.ReadStorage(releasedKey)
	if len(released) > 0 && released[0] == 1 {
		return 4 // already released
	}

	// Read depositor
	depositor, err := wasm.ReadStorage(depositorKey)
	if err != nil || len(depositor) < 20 {
		return 3 // no depositor
	}

	// Read escrowed amount
	escrowed, _ := wasm.ReadStorage(escrowedKey)
	amount := wasm.BEToUint64(escrowed)

	// Emit Refund event
	eventData := make([]byte, 0, 28)
	eventData = append(eventData, depositor[:20]...)
	eventData = append(eventData, wasm.Uint64ToBE(amount)...)
	wasm.EmitEvent("Refund", eventData)

	// Clear escrow state
	wasm.WriteStorage(escrowedKey, []byte{})
	wasm.WriteStorage(releasedKey, []byte{1})

	return 0
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
