package workload

import (
	"context"
	"crypto/rand"
	"math/big"
)

// MixedConfig holds configuration for mixed workload
type MixedConfig struct {
	NumAccounts   int
	TPS           int
	ChainID       uint32
	MaxFee        uint64
	GasLimit      uint64
	TransferRatio float64
	WASMRatio     float64
}

// mixedGenerator combines transfer and WASM workloads
type mixedGenerator struct {
	cfg         MixedConfig
	transferGen Generator
	wasmGen     Generator
}

// NewMixedGenerator creates a new mixed workload generator
func NewMixedGenerator(cfg interface{}) Generator {
	c, ok := cfg.(MixedConfig)
	if !ok {
		c = MixedConfig{
			NumAccounts:   100,
			TPS:           100,
			ChainID:       1,
			MaxFee:        10000,
			GasLimit:      50000,
			TransferRatio: 0.7,
			WASMRatio:     0.3,
		}
	}

	// Create underlying generators
	transferCfg := TransferConfig{
		NumAccounts: c.NumAccounts,
		TPS:         c.TPS,
		ChainID:     c.ChainID,
		MaxFee:      c.MaxFee,
		GasLimit:    c.GasLimit,
	}
	wasmCfg := WASMConfig{
		NumAccounts: c.NumAccounts,
		TPS:         c.TPS,
		ChainID:     c.ChainID,
		MaxFee:      c.MaxFee,
		GasLimit:    c.GasLimit,
	}

	return &mixedGenerator{
		cfg:         c,
		transferGen: NewTransferGenerator(transferCfg),
		wasmGen:     NewWASMGenerator(wasmCfg),
	}
}

func init() {
	Register("mixed", NewMixedGenerator)
}

// Name returns the name of the generator
func (g *mixedGenerator) Name() string {
	return "mixed"
}

// Generate creates a mix of transfer and WASM transactions
func (g *mixedGenerator) Generate(ctx context.Context, blockHeight uint64) ([]Transaction, error) {
	// Calculate how many of each type based on ratio
	numTransfers := int(float64(g.cfg.TPS) * g.cfg.TransferRatio)
	numWASM := g.cfg.TPS - numTransfers

	// Generate transfer transactions
	transferTxs, err := g.transferGen.Generate(ctx, blockHeight)
	if err != nil {
		return nil, err
	}

	// Generate WASM transactions
	wasmTxs, err := g.wasmGen.Generate(ctx, blockHeight)
	if err != nil {
		return nil, err
	}

	// Take the required number from each
	result := make([]Transaction, 0, g.cfg.TPS)
	result = append(result, transferTxs[:numTransfers]...)
	result = append(result, wasmTxs[:numWASM]...)

	// Shuffle the result for better distribution
	shuffleTransactions(result)

	return result, nil
}

// shuffleTransactions randomly shuffles transactions using Fisher-Yates
func shuffleTransactions(txs []Transaction) {
	for i := len(txs) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			continue
		}
		txs[i], txs[j.Int64()] = txs[j.Int64()], txs[i]
	}
}
