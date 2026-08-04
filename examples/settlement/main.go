//go:build wasip1

package main

import (
	"github.com/BryanOx/dsn/sdk/wasm"
)

// State keys
var (
	settledKey = []byte("settled")
	nonceKey   = []byte("nonce")
)

//export init
func init() {
	// Constructor takes no args for minimal settlement
	// Parties register via commit()
}

//export commit
func commit(ptr uint32, length uint32) uint64 {
	wasm.SetCalldataFromMemory(ptr, length)

	// Read 32-byte commitment hash from calldata
	commitData := wasm.ReadArg(32)
	if len(commitData) < 32 {
		return 1 // invalid commitment
	}

	caller := wasm.ReadCaller()

	// Check if already settled
	settled, err := wasm.ReadStorage(settledKey)
	if err == nil && len(settled) > 0 && settled[0] == 1 {
		return 4 // already settled
	}

	// Store commitment for this party
	// Use first 20 bytes of caller address as key suffix
	partyKey := append([]byte("commit_"), caller[:]...)
	wasm.WriteStorage(partyKey, commitData[:32])

	wasm.EmitEvent("Commit", caller[:])
	return 0
}

//export settle
func settle(ptr uint32, length uint32) uint64 {
	wasm.SetCalldataFromMemory(ptr, length)

	// Check if already settled
	settled, _ := wasm.ReadStorage(settledKey)
	if len(settled) > 0 && settled[0] == 1 {
		return 4 // already settled
	}

	// Parse calldata: party1Addr(20) + amount1(8) + party2Addr(20) + amount2(8) + nonce(8)
	p1Data := wasm.ReadArg(20)
	amt1Data := wasm.ReadArg(8)
	p2Data := wasm.ReadArg(20)
	amt2Data := wasm.ReadArg(8)
	nonceData := wasm.ReadArg(8)

	if len(p1Data) < 20 || len(amt1Data) < 8 || len(p2Data) < 20 || len(amt2Data) < 8 || len(nonceData) < 8 {
		return 1 // invalid settlement data
	}

	// Verify both parties committed
	p1CommitKey := append([]byte("commit_"), p1Data[:20]...)
	p2CommitKey := append([]byte("commit_"), p2Data[:20]...)

	p1Commit, err1 := wasm.ReadStorage(p1CommitKey)
	p2Commit, err2 := wasm.ReadStorage(p2CommitKey)

	if err1 != nil || err2 != nil {
		return 2 // not all parties committed
	}

	if len(p1Commit) < 32 || len(p2Commit) < 32 {
		return 2 // invalid commitments
	}

	// Store settlement nonce for replay protection
	wasm.WriteStorage(nonceKey, nonceData[:8])

	// Mark as settled
	wasm.WriteStorage(settledKey, []byte{1})

	// Build settlement event data
	eventData := make([]byte, 0, 64)
	eventData = append(eventData, p1Data[:20]...)
	eventData = append(eventData, amt1Data[:8]...)
	eventData = append(eventData, p2Data[:20]...)
	eventData = append(eventData, amt2Data[:8]...)
	eventData = append(eventData, nonceData[:8]...)

	wasm.EmitEvent("Settlement", eventData)
	return 0
}
