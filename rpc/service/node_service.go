package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("not found")
	// ErrInvalidParams is returned for invalid parameters.
	ErrInvalidParams = errors.New("invalid parameters")
)

// nodeService implements NodeService by delegating to the underlying node.
type nodeService struct {
	node *node.Node
}

// NewNodeService creates a new NodeService implementation.
func NewNodeService(n *node.Node) NodeService {
	return &nodeService{node: n}
}

// GetBlock returns block by number or hash.
// Queries the node's block store for actual block data.
// Returns a not-found error if the block doesn't exist.
func (s *nodeService) GetBlock(ctx context.Context, blockNumber *uint64, blockHash *string) (*BlockResult, error) {
	if blockNumber == nil && (blockHash == nil || *blockHash == "") {
		return nil, errors.New("block number or hash is required")
	}

	var height uint64
	if blockNumber != nil {
		height = *blockNumber
	} else {
		// Block hash lookup would require indexer - return error for now
		return nil, errors.New("block lookup by hash not available: requires block indexer")
	}

	// Query the block store for actual block data
	block, err := s.node.GetBlock(height)
	if err != nil {
		return nil, errors.New("block store not available: full block indexing requires an active node with synchronized state")
	}

	// Compute block hash from header
	var headerHash types.Hash
	headerHash, err = block.Header.HeaderHash(types.SHA256Hasher{})
	if err != nil {
		return nil, fmt.Errorf("failed to compute block hash: %w", err)
	}

	// Convert block to result - Transactions is []Transaction (values), need pointers
	txs := make([]TransactionResult, len(block.Transactions))
	for i := range block.Transactions {
		txs[i] = TransactionToResult(&block.Transactions[i], 0, 0, block.Header.Height, block.Header.PreviousHash)
	}

	return &BlockResult{
		Number:       block.Header.Height,
		Hash:         hashToHex(headerHash),
		ParentHash:   hashToHex(block.Header.PreviousHash),
		Timestamp:    block.Header.Timestamp,
		Transactions: txs,
		Events:       []EventResult{},
	}, nil
}

// GetTransaction returns transaction by hash.
func (s *nodeService) GetTransaction(ctx context.Context, txHash string) (*TransactionResult, error) {
	// Check mempool first - PendingTxs returns []*types.Transaction (pointers)
	txs := s.node.Mempool().PendingTxs()
	for _, tx := range txs {
		if hashToHex(tx.IntentID) == txHash {
			result := TransactionToResult(tx, 0, 0, 0, types.Hash{})
			return &result, nil
		}
	}

	// Full implementation would check indexed transactions
	return nil, ErrNotFound
}

// GetAccount returns account data for an address.
func (s *nodeService) GetAccount(ctx context.Context, address string) (*AccountResult, error) {
	addr, err := types.ParseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	acc, err := s.node.State().GetAccount(addr)
	if err != nil {
		// Account doesn't exist - return empty account
		result := AccountResult{
			Address:     address,
			Balance:     "0",
			Nonce:       0,
			CodeHash:    "0x",
			StorageRoot: "0x",
		}
		return &result, nil
	}

	return &AccountResult{
		Address:     address,
		Balance:     acc.Balance.String(),
		Nonce:       acc.Nonce,
		CodeHash:    hashToHex(acc.CodeHash),
		StorageRoot: hashToHex(acc.StorageRoot),
	}, nil
}

// GetBalance returns balance for an address as a big integer.
func (s *nodeService) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	addr, err := types.ParseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	acc, err := s.node.State().GetAccount(addr)
	if err != nil || acc == nil {
		return big.NewInt(0), nil
	}

	bal, _ := new(big.Int).SetString(acc.Balance.String(), 10)
	return bal, nil
}

// GetContract returns contract metadata and code hash.
func (s *nodeService) GetContract(ctx context.Context, address string) (*ContractResult, error) {
	addr, err := types.ParseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	acc, err := s.node.State().GetAccount(addr)
	if err != nil || acc == nil || acc.CodeHash == (types.Hash{}) {
		return nil, ErrNotFound
	}

	// Get contract metadata from state (if available) - uses CodeHash as contract ID
	meta, _ := s.node.State().GetContractMeta(acc.CodeHash)

	result := ContractToResult(address, meta, acc.CodeHash)
	return &result, nil
}

// CallContract executes a contract call locally without state change.
// It creates a snapshot of the state, executes the call, and rolls back the snapshot.
func (s *nodeService) CallContract(ctx context.Context, address, entrypoint, data string, gasLimit uint64) (*CallResult, error) {
	// Gas limit enforcement per SC-SEC-006
	if gasLimit == 0 {
		gasLimit = 1000000 // default
	}
	if gasLimit > 1000000 {
		return nil, fmt.Errorf("%w: gas limit exceeds maximum (1,000,000)", ErrInvalidParams)
	}

	// Validate address
	addr, err := types.ParseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	// Get contract ID from address (first 20 bytes)
	var contractID types.Hash
	copy(contractID[:], addr.Bytes())

	// Get state and create snapshot for rollback
	st := s.node.State()
	snapID := st.Snapshot()

	// Decode calldata from hex
	calldata := []byte{}
	if data != "" {
		calldata, err = hex.DecodeString(strings.TrimPrefix(data, "0x"))
		if err != nil {
			st.RevertToSnapshot(snapID)
			return nil, fmt.Errorf("%w: invalid calldata: %v", ErrInvalidParams, err)
		}
	}

	// Build call transaction with the specified entrypoint
	tx := &types.CallContractTx{
		ContractID:  contractID,
		Sender:      types.Address{}, // zero address for read-only simulation
		Entrypoint:  entrypoint,
		Calldata:    calldata,
		GasLimit:    gasLimit,
	}

	// Execute via VM with current block context (use 0 for simulation)
	blockHeight := uint64(0)
	blockTimestamp := uint64(0)

	result, err := s.node.VM().Execute(tx, st, blockHeight, blockTimestamp, 0)
	// Always rollback - do NOT persist anything
	st.RevertToSnapshot(snapID)

	if err != nil {
		return nil, err
	}

	return &CallResult{
		Data:       hex.EncodeToString(result.ReturnData),
		GasUsed:    result.GasUsed,
		Reverted:   result.Reverted,
		Events:     eventsToResults(result.Events),
	}, nil
}

// EstimateGas estimates the gas needed for a contract call using binary search.
func (s *nodeService) EstimateGas(ctx context.Context, address, data string) (uint64, error) {
	// Validate address
	_, err := types.ParseAddress(address)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	// Binary search for minimum gas limit
	const (
		gasLimitMin = uint64(21000)
		gasLimitMax = uint64(10_000_000)
	)

	lo, hi := gasLimitMin, gasLimitMax
	var lastSuccess uint64

	for lo <= hi {
		mid := (lo + hi) / 2
		result, err := s.CallContract(ctx, address, "call", data, mid)
		if err != nil {
			// Error during execution - try higher gas
			lo = mid + 1
			continue
		}
		if result.Reverted {
			// Reverted - not enough gas, try higher
			lo = mid + 1
		} else {
			// Success - try lower to find minimum
			lastSuccess = mid
			if mid == gasLimitMin {
				break
			}
			hi = mid - 1
		}
	}

	if lastSuccess == 0 {
		return 0, errors.New("gas estimation failed: call reverts at all gas limits")
	}

	return lastSuccess, nil
}

// SendTransaction submits a transaction to the mempool.
func (s *nodeService) SendTransaction(ctx context.Context, txJSON string) (string, error) {
	// Parse JSON into a temporary struct
	var rawTx struct {
		Sender    string `json:"sender"`
		Nonce     uint64 `json:"nonce"`
		ChainID   uint32 `json:"chainId"`
		Payload   string `json:"payload,omitempty"`
		Constraints string `json:"constraints,omitempty"`
		GasLimit  uint64 `json:"gasLimit"`
		MaxFee    uint64 `json:"maxFee"`
		Timestamp uint64 `json:"timestamp"`
		Signature string `json:"signature"`
		TxType    uint8  `json:"txType,omitempty"`
	}

	if err := json.Unmarshal([]byte(txJSON), &rawTx); err != nil {
		return "", fmt.Errorf("failed to parse transaction JSON: %w", err)
	}

	// Parse sender address (supports both hex 0x... and Base58)
	var sender types.Address
	var err error

	// Try hex format first (0x...)
	if strings.HasPrefix(rawTx.Sender, "0x") {
		addrBytes, err := hex.DecodeString(strings.TrimPrefix(rawTx.Sender, "0x"))
		if err != nil {
			return "", fmt.Errorf("invalid sender address: %w", err)
		}
		sender, err = types.AddressFromBytes(addrBytes)
		if err != nil {
			return "", fmt.Errorf("invalid sender address: %w", err)
		}
	} else {
		// Try Base58 format
		sender, err = types.ParseAddress(rawTx.Sender)
		if err != nil {
			return "", fmt.Errorf("invalid sender address: %w", err)
		}
	}

	// Parse signature from hex
	signature, err := hex.DecodeString(strings.TrimPrefix(rawTx.Signature, "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid signature: %w", err)
	}

	// Parse payload from hex (optional)
	var payload []byte
	if rawTx.Payload != "" {
		payload, err = hex.DecodeString(strings.TrimPrefix(rawTx.Payload, "0x"))
		if err != nil {
			return "", fmt.Errorf("invalid payload: %w", err)
		}
	}

	// Parse constraints from hex (optional)
	var constraints []byte
	if rawTx.Constraints != "" {
		constraints, err = hex.DecodeString(strings.TrimPrefix(rawTx.Constraints, "0x"))
		if err != nil {
			return "", fmt.Errorf("invalid constraints: %w", err)
		}
	}

	// Build transaction
	tx := &types.Transaction{
		Sender:      sender,
		Nonce:       rawTx.Nonce,
		ChainID:     rawTx.ChainID,
		Payload:     payload,
		Constraints: constraints,
		GasLimit:    rawTx.GasLimit,
		MaxFee:       rawTx.MaxFee,
		Timestamp:   rawTx.Timestamp,
		Signature:   signature,
		TxType:      types.TxType(rawTx.TxType),
	}

	// Compute IntentID using SHA256Hasher
	hasher := types.SHA256Hasher{}
	intentID, err := tx.ComputeIntentID(hasher)
	if err != nil {
		return "", fmt.Errorf("failed to compute intent ID: %w", err)
	}
	tx.IntentID = intentID

	// Validate transaction
	if err := tx.Validate(); err != nil {
		return "", fmt.Errorf("transaction validation failed: %w", err)
	}

	// Submit to mempool
	if err := s.node.Mempool().Submit(tx); err != nil {
		return "", fmt.Errorf("failed to submit transaction to mempool: %w", err)
	}

	// Return the transaction hash (IntentID) with 0x prefix
	return "0x" + hex.EncodeToString(intentID[:]), nil
}

// GetEvents returns events matching the given filter.
// Requires event indexer (Phase 6) to query indexed events.
func (s *nodeService) GetEvents(ctx context.Context, filter EventFilter) ([]EventResult, error) {
	return nil, errors.New("event querying not available: requires event indexer (Phase 6)")
}

// GetValidators returns validator list for the given epoch.
// Requires active staking state synchronization to query validator set.
func (s *nodeService) GetValidators(ctx context.Context, epoch *uint64) ([]ValidatorResult, error) {
	// Check if state has validator data available
	// StateDB interface doesn't expose validator queries yet
	// Validator set tracking requires staking state integration
	return nil, errors.New("validator set query not available: requires active staking state synchronization")
}

// GetSupply returns token supply metrics.
func (s *nodeService) GetSupply(ctx context.Context) (*SupplyResult, error) {
	// Calculate total supply by iterating all accounts
	total := big.NewInt(0)
	circulating := big.NewInt(0)
	staked := big.NewInt(0)

	// Iterate accounts to calculate supply
	_ = s.node.State().ForEachAccount(func(addr types.Address, acc *state.Account) error {
		accBal, _ := new(big.Int).SetString(acc.Balance.String(), 10)
		total = new(big.Int).Add(total, accBal)
		circulating = new(big.Int).Add(circulating, accBal)
		return nil
	})

	return &SupplyResult{
		Total:       total.String(),
		Circulating: circulating.String(),
		Staked:      staked.String(),
	}, nil
}

// GetCurrentHeight returns the current chain height.
func (s *nodeService) GetCurrentHeight() uint64 {
	// Would be exposed from node in full implementation
	return 0
}

// eventsToResults converts VM events to RPC event results.
func eventsToResults(events []types.Event) []EventResult {
	if len(events) == 0 {
		return nil
	}
	results := make([]EventResult, len(events))
	for i, ev := range events {
		results[i] = EventResult{
			Contract: hex.EncodeToString(ev.ContractID[:]),
			Topics:   []string{ev.Topic},
			Data:     hex.EncodeToString(ev.Data),
		}
	}
	return results
}

// GetContractState returns the contract storage value for a given key.
func (s *nodeService) GetContractState(ctx context.Context, address, key string) (string, error) {
	// Validate address
	addr, err := types.ParseAddress(address)
	if err != nil {
		return "", fmt.Errorf("%w: invalid address: %v", ErrInvalidParams, err)
	}

	// Get contract ID from address (first 20 bytes)
	var contractID types.Hash
	copy(contractID[:], addr.Bytes())

	// Get state and query storage
	st := s.node.State()
	value, err := st.GetContractStorage(contractID, []byte(key))
	if err != nil {
		// Key not found - return empty string (not an error)
		return "", nil
	}

	if len(value) == 0 {
		return "", nil
	}

	return hex.EncodeToString(value), nil
}

// GetTransactionReceipt returns the receipt for a transaction.
// Currently returns a stub - full implementation requires block indexer.
func (s *nodeService) GetTransactionReceipt(ctx context.Context, txHash string) (*TransactionReceipt, error) {
	// Validate tx hash
	if txHash == "" {
		return nil, fmt.Errorf("%w: txHash is required", ErrInvalidParams)
	}

	// Remove 0x prefix if present
	txHash = strings.TrimPrefix(txHash, "0x")

	// Try to find the transaction in the mempool first
	txs := s.node.Mempool().PendingTxs()
	for _, tx := range txs {
		txHashHex := hex.EncodeToString(tx.IntentID[:])
		if txHashHex == txHash {
			// Transaction is in mempool - return pending receipt
			return &TransactionReceipt{
				TxHash:      "0x" + txHash,
				BlockNumber: 0,
				BlockHash:   "",
				GasUsed:     0,
				GasLimit:    tx.GasLimit,
				Status:      "pending",
				ReturnData:  "",
			}, nil
		}
	}

	// For now, return not found - full implementation requires block indexer
	return nil, ErrNotFound
}