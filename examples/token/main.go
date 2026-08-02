//go:build wasip1

package main

import "github.com/dsn/dsn/sdk/wasm"

// State keys
var (
	balancePrefix  = []byte("balance_")
	totalSupplyKey = []byte("total_supply")
)

//export init
func init() {
	data := wasm.ReadCalldata()
	if len(data) >= 8 {
		totalSupply := wasm.BEToUint64(data)
		wasm.WriteStorage(totalSupplyKey, wasm.Uint64ToBE(totalSupply))
		// Assign all tokens to deployer
		deployer := wasm.ReadCaller()
		deployerKey := append(balancePrefix, deployer[:]...)
		wasm.WriteStorage(deployerKey, wasm.Uint64ToBE(totalSupply))
	}
}

//export balanceOf
func balanceOf(ptr uint32, length uint32) uint64 {
	// Read owner address from calldata at ptr
	// owner is 20 bytes
	wasm.SetCalldataFromMemory(ptr, length)
	ownerData := wasm.ReadArg(20)
	if len(ownerData) < 20 {
		return 0
	}
	var owner [20]byte
	copy(owner[:], ownerData)

	key := append(balancePrefix, owner[:]...)
	val, err := wasm.ReadStorage(key)
	if err != nil {
		return 0
	}
	return wasm.BEToUint64(val)
}

//export transfer
func transfer(ptr uint32, length uint32) uint64 {
	// Parse: receiver (20 bytes) + amount (8 bytes)
	wasm.SetCalldataFromMemory(ptr, length)

	toData := wasm.ReadArg(20)
	amtData := wasm.ReadArg(8)
	if len(toData) < 20 || len(amtData) < 8 {
		return 1 // error
	}

	var to [20]byte
	copy(to[:], toData)
	amount := wasm.BEToUint64(amtData)
	sender := wasm.ReadCaller()

	// Read sender balance
	senderKey := append(balancePrefix, sender[:]...)
	senderBal, err := wasm.ReadStorage(senderKey)
	if err != nil {
		return 1
	}

	senderBalance := wasm.BEToUint64(senderBal)
	if senderBalance < amount {
		return 1 // insufficient balance
	}

	// Deduct from sender
	newSenderBal := senderBalance - amount
	wasm.WriteStorage(senderKey, wasm.Uint64ToBE(newSenderBal))

	// Credit to receiver
	toKey := append(balancePrefix, to[:]...)
	toBal, _ := wasm.ReadStorage(toKey)
	newToBal := wasm.BEToUint64(toBal) + amount
	wasm.WriteStorage(toKey, wasm.Uint64ToBE(newToBal))

	// Emit Transfer event: receiver (20 bytes) + amount (8 bytes)
	eventData := make([]byte, 0, 28)
	eventData = append(eventData, to[:]...)
	eventData = append(eventData, amtData...)
	wasm.EmitEvent("Transfer", eventData)

	return 0 // success
}

//export totalSupply
func totalSupply() uint64 {
	val, err := wasm.ReadStorage(totalSupplyKey)
	if err != nil {
		return 0
	}
	return wasm.BEToUint64(val)
}
