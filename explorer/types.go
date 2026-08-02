package explorer

import (
	"encoding/hex"

	"github.com/dsn/dsn/types"
)

// BlockSummary represents a summary of a block for list views.
type BlockSummary struct {
	Number    uint64 `json:"number"`
	Hash      string `json:"hash"`
	Timestamp uint64 `json:"timestamp"`
	TxCount   int    `json:"txCount"`
}

// BlockDetail represents detailed block information.
type BlockDetail struct {
	BlockSummary
	ParentHash   string            `json:"parentHash"`
	StateRoot    string            `json:"stateRoot"`
	Transactions []TransactionItem `json:"transactions"`
}

// TransactionItem represents a transaction in explorer responses.
type TransactionItem struct {
	Hash     string `json:"hash"`
	Sender   string `json:"sender"`
	Nonce    uint64 `json:"nonce"`
	Status   uint8  `json:"status"`
	GasUsed  uint64 `json:"gasUsed"`
	BlockNum uint64 `json:"blockNumber"`
}

// AccountInfo represents account information in explorer.
type AccountInfo struct {
	Address    string   `json:"address"`
	Balance    string   `json:"balance"`
	Nonce      uint64   `json:"nonce"`
	IsContract bool     `json:"isContract"`
	RecentTxs  []string `json:"recentTxs,omitempty"`
}

// EventItem represents an event in explorer responses.
type EventItem struct {
	Contract    string   `json:"contract"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber uint64   `json:"blockNumber"`
}

// ValidatorInfo represents validator information.
type ValidatorInfo struct {
	Address    string `json:"address"`
	Power      uint64 `json:"power"`
	Commission uint16 `json:"commission"`
}

// SupplyInfo represents token supply metrics.
type SupplyInfo struct {
	Total       string `json:"total"`
	Circulating string `json:"circulating"`
	Staked      string `json:"staked"`
}

// ContractInfo represents contract metadata in explorer.
type ContractInfo struct {
	Address      string        `json:"address"`
	CodeHash     string        `json:"codeHash"`
	Metadata     *ContractMeta `json:"metadata,omitempty"`
	RecentEvents []EventItem   `json:"recentEvents,omitempty"`
}

// ContractMeta represents contract metadata.
type ContractMeta struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Offset     uint64      `json:"offset"`
	Limit      uint64      `json:"limit"`
	TotalCount uint64      `json:"totalCount,omitempty"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// hashToHex converts a types.Hash to hex string.
func hashToHex(h types.Hash) string {
	return "0x" + hex.EncodeToString(h[:])
}

// addressToHex converts a types.Address to hex string.
func addressToHex(a types.Address) string {
	return "0x" + hex.EncodeToString(a[:])
}
