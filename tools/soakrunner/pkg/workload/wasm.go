package workload

import (
	"context"
	"crypto/rand"
	"encoding/binary"

	"github.com/dsn/dsn/types"
)

// WASMConfig holds configuration for WASM workload
type WASMConfig struct {
	NumAccounts    int
	TPS            int
	ChainID        uint32
	MaxFee         uint64
	GasLimit       uint64
}

// wasmGenerator generates WASM contract call transactions
type wasmGenerator struct {
	cfg         WASMConfig
	accounts    []types.Address
	nonceMap    map[[20]byte]uint64
	contractAddr types.Address
}

// NewWASMGenerator creates a new WASM workload generator
func NewWASMGenerator(cfg interface{}) Generator {
	c, ok := cfg.(WASMConfig)
	if !ok {
		c = WASMConfig{
			NumAccounts: 100,
			TPS:         100,
			ChainID:     1,
			MaxFee:      50000,
			GasLimit:    100000,
		}
	}
	g := &wasmGenerator{
		cfg:      c,
		accounts: make([]types.Address, c.NumAccounts),
		nonceMap: make(map[[20]byte]uint64),
	}
	// Initialize accounts with random addresses
	for i := 0; i < c.NumAccounts; i++ {
		var addr types.Address
		rand.Read(addr[:])
		g.accounts[i] = addr
	}
	// Use a default contract address (counter contract)
	contractBytes := []byte("countercontract00000")
	var contractAddr types.Address
	copy(contractAddr[:], contractBytes)
	g.contractAddr = contractAddr

	return g
}

func init() {
	Register("wasm", NewWASMGenerator)
}

// Name returns the name of the generator
func (g *wasmGenerator) Name() string {
	return "wasm"
}

// Generate creates WASM contract call transactions
func (g *wasmGenerator) Generate(ctx context.Context, blockHeight uint64) ([]Transaction, error) {
	txs := make([]Transaction, 0, g.cfg.TPS)
	accounts := g.accounts

	for i := 0; i < g.cfg.TPS; i++ {
		// Select random sender
		senderIdx := randInt(len(accounts))
		sender := accounts[senderIdx]

		// Get and increment nonce for sender
		var senderKey [20]byte
		copy(senderKey[:], sender[:])
		nonce := g.nonceMap[senderKey]
		g.nonceMap[senderKey] = nonce + 1

		// Generate contract call payload (simple counter operation)
		payload := generateWasmPayload(blockHeight, i)

		txs = append(txs, Transaction{
			Type:      "call_contract",
			Sender:    sender[:],
			Recipient: g.contractAddr[:],
			Amount:    0,
			Nonce:     nonce,
			Payload:   payload,
			MaxFee:    g.cfg.MaxFee,
			GasLimit:  g.cfg.GasLimit,
		})
	}

	return txs, nil
}

// generateWasmPayload generates a simple WASM contract call payload
// Format: method_id(4) + arg(4) = 8 bytes
func generateWasmPayload(blockHeight uint64, index int) []byte {
	buf := make([]byte, 8)
	// Method ID: 0 = increment, 1 = get, 2 = reset
	methodID := uint32(randInt(3))
	binary.BigEndian.PutUint32(buf[0:4], methodID)
	// Argument: block height + index as unique value
	arg := uint32(blockHeight<<16) ^ uint32(index)
	binary.BigEndian.PutUint32(buf[4:8], arg)
	return buf
}