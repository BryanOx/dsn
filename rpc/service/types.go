package service

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/BryanOx/dsn/types"
)

// BlockResult represents block data returned by RPC.
type BlockResult struct {
	Number       uint64              `json:"number"`
	Hash         string              `json:"hash"`
	ParentHash   string              `json:"parentHash"`
	Timestamp    uint64              `json:"timestamp"`
	Transactions []TransactionResult `json:"transactions"`
	Events       []EventResult       `json:"events"`
}

// TransactionResult represents transaction data with receipt.
type TransactionResult struct {
	Hash      string         `json:"hash"`
	Sender    string         `json:"sender"`
	Recipient string         `json:"recipient"`
	IntentID  string         `json:"intentId"`
	Nonce     uint64         `json:"nonce"`
	Value     string         `json:"value"`
	MaxFee    string         `json:"maxFee"`
	GasLimit  uint64         `json:"gasLimit"`
	Data      string         `json:"data"`
	Receipt   *ReceiptResult `json:"receipt,omitempty"`
}

// ReceiptResult represents transaction receipt.
type ReceiptResult struct {
	Status      string `json:"status"`
	GasUsed     uint64 `json:"gasUsed"`
	BlockNumber uint64 `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
}

// AccountResult represents account data.
type AccountResult struct {
	Address     string `json:"address"`
	Balance     string `json:"balance"`
	Nonce       uint64 `json:"nonce"`
	CodeHash    string `json:"codeHash"`
	StorageRoot string `json:"storageRoot"`
}

// ContractResult represents contract data.
type ContractResult struct {
	Address  string        `json:"address"`
	Metadata *ContractMeta `json:"metadata,omitempty"`
	CodeHash string        `json:"codeHash"`
}

// ContractMeta represents contract metadata.
type ContractMeta struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`
}

// CallResult represents result of a local contract call.
type CallResult struct {
	Data     string        `json:"data"`
	GasUsed  uint64        `json:"gasUsed"`
	Reverted bool          `json:"reverted"`
	Events   []EventResult `json:"events"`
}

// EstimateResult represents gas estimation.
type EstimateResult struct {
	Gas uint64 `json:"gas"`
}

// TransactionReceipt represents a transaction receipt.
type TransactionReceipt struct {
	TxHash          string        `json:"txHash"`
	BlockNumber     uint64        `json:"blockNumber"`
	BlockHash       string        `json:"blockHash"`
	GasUsed         uint64        `json:"gasUsed"`
	GasLimit        uint64        `json:"gasLimit"`
	Status          string        `json:"status"`
	ContractAddress string        `json:"contractAddress,omitempty"`
	Events          []EventResult `json:"events,omitempty"`
	ReturnData      string        `json:"returnData"`
}

// SendTxResult represents result of transaction submission.
type SendTxResult struct {
	TxHash string `json:"txHash"`
}

// EventResult represents a logged event.
type EventResult struct {
	Contract    string   `json:"contract"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber uint64   `json:"blockNumber"`
	TxHash      string   `json:"txHash"`
}

// ValidatorResult represents validator info.
type ValidatorResult struct {
	ConsensusID     string `json:"consensusId"`
	PublicKey       string `json:"publicKey"`
	Address         string `json:"address"`
	RewardAddress   string `json:"rewardAddress"`
	BondedStake     string `json:"bondedStake"`
	Status          string `json:"status"`
	VotingPower     uint64 `json:"votingPower"`
	Commission      uint16 `json:"commission"`
	JailedUntil     uint64 `json:"jailedUntil"`
	ActivationEpoch uint64 `json:"activationEpoch"`
	UnstakeEpoch    uint64 `json:"unstakeEpoch"`
}

// SupplyResult represents token supply metrics.
type SupplyResult struct {
	Total       string `json:"total"`
	Circulating string `json:"circulating"`
	Staked      string `json:"staked"`
}

// StateRootResult represents the current state root hash.
type StateRootResult struct {
	StateRoot string `json:"state_root"`
}

// EventFilter represents filter for event queries.
type EventFilter struct {
	Contract  string
	Topics    []string
	FromBlock uint64
	ToBlock   uint64
	Offset    uint64
	Limit     uint64
}

// PaginationParams represents pagination for list queries.
type PaginationParams struct {
	Offset uint64
	Limit  uint64
}

// DefaultPagination returns default pagination settings.
func DefaultPagination() PaginationParams {
	return PaginationParams{
		Offset: 0,
		Limit:  20,
	}
}

// Normalize caps limit at 100.
func (p *PaginationParams) Normalize() {
	if p.Limit == 0 {
		p.Limit = 20
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset > 0 && p.Limit == 0 {
		p.Limit = 20
	}
}

// Helper functions to convert types to RPC DTOs

// BlockToResult converts a types.Block to BlockResult.
func BlockToResult(block *types.Block) BlockResult {
	blockHash, _ := block.HeaderHash(types.SHA256Hasher{})

	result := BlockResult{
		Number:       block.Header.Height,
		Hash:         hashToHex(blockHash),
		ParentHash:   hashToHex(block.Header.PreviousHash),
		Timestamp:    block.Header.Timestamp,
		Transactions: make([]TransactionResult, len(block.Transactions)),
		Events:       make([]EventResult, 0),
	}

	for i, tx := range block.Transactions {
		txCopy := tx
		txResult := TransactionToResult(&txCopy, 0, 0, 0, types.Hash{})
		result.Transactions[i] = txResult
	}

	return result
}

// TransactionToResult converts a types.Transaction to TransactionResult.
func TransactionToResult(tx *types.Transaction, receiptStatus uint8, gasUsed, blockNumber uint64, blockHash types.Hash) TransactionResult {
	result := TransactionResult{
		Hash:      hashToHex(tx.IntentID),
		Sender:    tx.Sender.String(),
		Recipient: "", // Transaction doesn't have explicit recipient field
		IntentID:  hashToHex(tx.IntentID),
		Nonce:     tx.Nonce,
		Value:     "", // Value in payload, need to parse
		MaxFee:    fmt.Sprintf("%d", tx.MaxFee),
		GasLimit:  tx.GasLimit,
		Data:      bytesToHex(tx.Payload),
	}

	if blockNumber > 0 {
		result.Receipt = &ReceiptResult{
			Status:      receiptStatusToString(receiptStatus),
			GasUsed:     gasUsed,
			BlockNumber: blockNumber,
			BlockHash:   hashToHex(blockHash),
		}
	}

	return result
}

// AccountToResult converts account fields to AccountResult.
func AccountToResult(addr string, balance string, nonce uint64, codeHash, storageRoot types.Hash) AccountResult {
	return AccountResult{
		Address:     addr,
		Balance:     balance,
		Nonce:       nonce,
		CodeHash:    hashToHex(codeHash),
		StorageRoot: hashToHex(storageRoot),
	}
}

// ContractToResult converts contract metadata to ContractResult.
func ContractToResult(addr string, meta *types.ContractMetadata, codeHash types.Hash) ContractResult {
	result := ContractResult{
		Address:  addr,
		CodeHash: hashToHex(codeHash),
	}

	if meta != nil {
		result.Metadata = &ContractMeta{
			Name:       hashToHex(meta.ContractID),
			Version:    fmt.Sprintf("%d", meta.BlockHeight),
			Entrypoint: "",
		}
	}

	return result
}

// EventToResult converts types.Event to EventResult.
func EventToResult(evt *types.Event) EventResult {
	return EventResult{
		Contract:    hashToHex(evt.ContractID),
		Topics:      []string{evt.Topic},
		Data:        bytesToHex(evt.Data),
		BlockNumber: evt.BlockHeight,
		TxHash:      "", // Not available in current Event type
	}
}

// SupplyToResult converts big.Int supply values to SupplyResult.
func SupplyToResult(total, circulating, staked *big.Int) *SupplyResult {
	return &SupplyResult{
		Total:       bigIntToString(total),
		Circulating: bigIntToString(circulating),
		Staked:      bigIntToString(staked),
	}
}

// Helper functions

func hashToHex(h types.Hash) string {
	return "0x" + hex.EncodeToString(h[:])
}

func bytesToHex(b []byte) string {
	if len(b) == 0 {
		return "0x"
	}
	return "0x" + hex.EncodeToString(b)
}

func receiptStatusToString(status uint8) string {
	if status == 1 {
		return "success"
	}
	return "failure"
}

func bigIntToString(bi *big.Int) string {
	if bi == nil {
		return "0"
	}
	return bi.String()
}
