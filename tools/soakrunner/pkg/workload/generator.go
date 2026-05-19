package workload

import (
	"context"
)

// Transaction represents a generic transaction that can be submitted
type Transaction struct {
	Type      string
	Sender    []byte
	Recipient []byte
	Amount    uint64
	Nonce     uint64
	Payload   []byte
	MaxFee    uint64
	GasLimit  uint64
}

// Generator is the interface for workload generators
type Generator interface {
	// Generate creates a batch of transactions for the given block height
	Generate(ctx context.Context, blockHeight uint64) ([]Transaction, error)
	// Name returns the name of the generator
	Name() string
}

// GeneratorFactory creates generators based on profile name
type GeneratorFactory func(cfg interface{}) Generator

var generators = make(map[string]GeneratorFactory)

// Register registers a generator factory for a profile name
func Register(name string, factory GeneratorFactory) {
	generators[name] = factory
}

// Get returns a generator for the given profile name
func Get(name string, cfg interface{}) (Generator, error) {
	factory, ok := generators[name]
	if !ok {
		return nil, nil // Return nil for unknown profiles (handled by caller)
	}
	return factory(cfg), nil
}