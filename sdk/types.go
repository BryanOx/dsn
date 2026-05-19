package sdk

import (
	"encoding/json"
)

// JSON-RPC request/response types

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Block represents block data from RPC.
type Block struct {
	Number       uint64          `json:"number"`
	Hash         string          `json:"hash"`
	ParentHash   string          `json:"parentHash"`
	Timestamp    uint64          `json:"timestamp"`
	Transactions []Transaction   `json:"transactions"`
	Events       []Event         `json:"events"`
}

// BlockHeader represents a block header (for subscriptions).
type BlockHeader struct {
	Number     uint64 `json:"number"`
	Hash       string `json:"hash"`
	ParentHash string `json:"parentHash"`
	Timestamp  uint64 `json:"timestamp"`
}

// Transaction represents transaction data from RPC.
type Transaction struct {
	Hash       string   `json:"hash"`
	Sender     string   `json:"sender"`
	Recipient  string   `json:"recipient"`
	IntentID   string   `json:"intentId"`
	Nonce      uint64   `json:"nonce"`
	Value      string   `json:"value"`
	MaxFee     string   `json:"maxFee"`
	GasLimit   uint64   `json:"gasLimit"`
	Data       string   `json:"data"`
	Receipt    *Receipt `json:"receipt,omitempty"`
}

// Receipt represents transaction receipt.
type Receipt struct {
	Status      string `json:"status"`
	GasUsed     uint64 `json:"gasUsed"`
	BlockNumber uint64 `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
}

// Account represents account data from RPC.
type Account struct {
	Address     string `json:"address"`
	Balance     string `json:"balance"`
	Nonce       uint64 `json:"nonce"`
	CodeHash    string `json:"codeHash"`
	StorageRoot string `json:"storageRoot"`
}

// Contract represents contract data from RPC.
type Contract struct {
	Address   string         `json:"address"`
	Metadata  *ContractMeta `json:"metadata,omitempty"`
	CodeHash  string        `json:"codeHash"`
}

// ContractMeta represents contract metadata.
type ContractMeta struct {
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Entrypoint  string `json:"entrypoint,omitempty"`
}

// CallResult represents result of a local contract call.
type CallResult struct {
	Data string `json:"data"`
}

// EstimateResult represents gas estimation.
type EstimateResult struct {
	Gas uint64 `json:"gas"`
}

// SendTxResult represents result of transaction submission.
type SendTxResult struct {
	TxHash string `json:"txHash"`
}

// Event represents a logged event from RPC.
type Event struct {
	Contract    string   `json:"contract"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber uint64   `json:"blockNumber"`
	TxHash      string   `json:"txHash"`
}

// Validator represents validator info from RPC.
type Validator struct {
	Address    string `json:"address"`
	Power      uint64 `json:"power"`
	Commission uint64 `json:"commission"`
}

// Supply represents token supply metrics from RPC.
type Supply struct {
	Total       string `json:"total"`
	Circulating string `json:"circulating"`
	Staked      string `json:"staked"`
}

// EventFilter represents filter for event queries.
type EventFilter struct {
	Contract   string   `json:"contract,omitempty"`
	Topics     []string `json:"topics,omitempty"`
	FromBlock  uint64   `json:"fromBlock,omitempty"`
	ToBlock    uint64   `json:"toBlock,omitempty"`
	Offset     uint64   `json:"offset,omitempty"`
	Limit      uint64   `json:"limit,omitempty"`
}

// BlockFilter represents filter for getting blocks.
type BlockFilter struct {
	BlockNumber *uint64 `json:"blockNumber,omitempty"`
	BlockHash   *string `json:"blockHash,omitempty"`
}

// GetBlockParams params for dsn_getBlock.
type GetBlockParams struct {
	BlockNumber *uint64 `json:"blockNumber,omitempty"`
	BlockHash   *string `json:"blockHash,omitempty"`
}

// GetTransactionParams params for dsn_getTransaction.
type GetTransactionParams struct {
	TxHash string `json:"txHash"`
}

// GetAccountParams params for dsn_getAccount.
type GetAccountParams struct {
	Address string `json:"address"`
}

// GetBalanceParams params for dsn_getBalance.
type GetBalanceParams struct {
	Address string `json:"address"`
}

// GetContractParams params for dsn_getContract.
type GetContractParams struct {
	Address string `json:"address"`
}

// CallContractParams params for dsn_callContract.
type CallContractParams struct {
	Address  string `json:"address"`
	Data     string `json:"data"`
	GasLimit uint64 `json:"gasLimit,omitempty"`
}

// EstimateGasParams params for dsn_estimateGas.
type EstimateGasParams struct {
	Address string `json:"address"`
	Data    string `json:"data"`
}

// SendTransactionParams params for dsn_sendTransaction.
type SendTransactionParams struct {
	Transaction Transaction `json:"transaction"`
}

// GetEventsParams params for dsn_getEvents.
type GetEventsParams struct {
	EventFilter `json:",inline"`
}

// GetValidatorsParams params for dsn_getValidators.
type GetValidatorsParams struct {
	Epoch *uint64 `json:"epoch,omitempty"`
}

