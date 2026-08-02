package service

import (
	"context"
	"math/big"
)

// NodeService defines the interface for RPC service methods.
// All implementations must delegate to the node's underlying state and mempool.
type NodeService interface {
	// GetBlock returns block by number or hash.
	GetBlock(ctx context.Context, blockNumber *uint64, blockHash *string) (*BlockResult, error)

	// GetTransaction returns transaction by hash.
	GetTransaction(ctx context.Context, txHash string) (*TransactionResult, error)

	// GetAccount returns account data for an address.
	GetAccount(ctx context.Context, address string) (*AccountResult, error)

	// GetBalance returns balance for an address as a big integer.
	GetBalance(ctx context.Context, address string) (*big.Int, error)

	// GetContract returns contract metadata and code hash.
	GetContract(ctx context.Context, address string) (*ContractResult, error)

	// CallContract executes a contract call locally without state change.
	CallContract(ctx context.Context, address, entrypoint, data string, gasLimit uint64) (*CallResult, error)

	// EstimateGas estimates the gas needed for a contract call.
	EstimateGas(ctx context.Context, address, data string) (uint64, error)

	// SendTransaction submits a transaction to the mempool.
	SendTransaction(ctx context.Context, txJSON string) (string, error)

	// GetEvents returns events matching the given filter.
	GetEvents(ctx context.Context, filter EventFilter) ([]EventResult, error)

	// GetValidators returns validator list for the given epoch.
	GetValidators(ctx context.Context, epoch *uint64) ([]ValidatorResult, error)

	// GetValidator returns validator info for a given operator address.
	GetValidator(ctx context.Context, address string) (*ValidatorResult, error)

	// GetSupply returns token supply metrics.
	GetSupply(ctx context.Context) (*SupplyResult, error)

	// GetContractState returns contract storage value for a given key.
	GetContractState(ctx context.Context, address, key string) (string, error)

	// GetTransactionReceipt returns the receipt for a transaction.
	GetTransactionReceipt(ctx context.Context, txHash string) (*TransactionReceipt, error)

	// GetStateRoot returns the current state root hash.
	GetStateRoot(ctx context.Context) (*StateRootResult, error)

	// GetPendingTxs returns pending transactions from the mempool.
	GetPendingTxs(ctx context.Context) ([]TransactionResult, error)
}

// CallRequest represents a contract call request.
type CallRequest struct {
	To       string
	Data     string
	GasLimit uint64
}

// EstimateRequest represents a gas estimation request.
type EstimateRequest struct {
	To   string
	Data string
}

// EventFilterParams represents event filter parameters from RPC.
type EventFilterParams struct {
	Contract  string
	Topics    []string
	FromBlock *uint64
	ToBlock   *uint64
	Offset    *uint64
	Limit     *uint64
}

// ToEventFilter converts params to EventFilter with defaults.
func (p *EventFilterParams) ToEventFilter() EventFilter {
	filter := EventFilter{
		Contract: p.Contract,
		Topics:   p.Topics,
	}

	if p.FromBlock != nil {
		filter.FromBlock = *p.FromBlock
	}
	if p.ToBlock != nil {
		filter.ToBlock = *p.ToBlock
	}

	pagination := DefaultPagination()
	if p.Offset != nil {
		pagination.Offset = *p.Offset
	}
	if p.Limit != nil {
		pagination.Limit = *p.Limit
	}
	pagination.Normalize()
	filter.Offset = pagination.Offset
	filter.Limit = pagination.Limit

	return filter
}
