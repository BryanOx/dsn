package rpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dsn/dsn/rpc/service"
)

// Handler wraps a NodeService for HTTP handling.
type Handler struct {
	service service.NodeService
}

// NewHandler creates a new Handler with the given service.
func NewHandler(svc service.NodeService) *Handler {
	return &Handler{service: svc}
}

// Request types for RPC methods

type GetBlockRequest struct {
	BlockNumber *uint64 `json:"blockNumber,omitempty"`
	BlockHash   *string `json:"blockHash,omitempty"`
}

type GetTransactionRequest struct {
	TxHash string `json:"txHash"`
}

type GetAccountRequest struct {
	Address string `json:"address"`
}

type GetBalanceRequest struct {
	Address string `json:"address"`
}

type GetContractRequest struct {
	Address string `json:"address"`
}

type CallContractRequest struct {
	Address    string `json:"address"`
	Entrypoint string `json:"entrypoint"`
	Data       string `json:"data"`
	GasLimit   uint64 `json:"gasLimit,omitempty"`
}

type EstimateGasRequest struct {
	Address string `json:"address"`
	Data    string `json:"data"`
}

type SendTransactionRequest struct {
	TxJSON string `json:"tx"`
}

type GetEventsRequest struct {
	Filter service.EventFilterParams `json:"filter"`
}

type GetValidatorRequest struct {
	Address string `json:"address"`
}

type GetValidatorsRequest struct {
	Epoch *uint64 `json:"epoch,omitempty"`
}

type GetSupplyRequest struct{}

type GetContractStateRequest struct {
	Address string `json:"address"`
	Key     string `json:"key"`
}

type GetTransactionReceiptRequest struct {
	TxHash string `json:"txHash"`
}

// Handler methods that delegate to NodeService

// handleGetBlock implements dsn_getBlock.
func (h *Handler) handleGetBlock(ctx context.Context, req *GetBlockRequest) (*service.BlockResult, error) {
	return h.service.GetBlock(ctx, req.BlockNumber, req.BlockHash)
}

// handleGetTransaction implements dsn_getTransaction.
func (h *Handler) handleGetTransaction(ctx context.Context, req *GetTransactionRequest) (*service.TransactionResult, error) {
	if req.TxHash == "" {
		return nil, fmt.Errorf("%w: txHash is required", service.ErrInvalidParams)
	}
	return h.service.GetTransaction(ctx, req.TxHash)
}

// handleGetAccount implements dsn_getAccount.
func (h *Handler) handleGetAccount(ctx context.Context, req *GetAccountRequest) (*service.AccountResult, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	return h.service.GetAccount(ctx, req.Address)
}

// handleGetBalance implements dsn_getBalance.
func (h *Handler) handleGetBalance(ctx context.Context, req *GetBalanceRequest) (string, error) {
	if req.Address == "" {
		return "0", fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	balance, err := h.service.GetBalance(ctx, req.Address)
	if err != nil {
		return "0", err
	}
	return balance.String(), nil
}

// handleGetContract implements dsn_getContract.
func (h *Handler) handleGetContract(ctx context.Context, req *GetContractRequest) (*service.ContractResult, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	return h.service.GetContract(ctx, req.Address)
}

// handleCallContract implements dsn_callContract.
func (h *Handler) handleCallContract(ctx context.Context, req *CallContractRequest) (*service.CallResult, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	entrypoint := req.Entrypoint
	if entrypoint == "" {
		entrypoint = "call" // default entrypoint
	}
	gasLimit := req.GasLimit
	if gasLimit == 0 {
		gasLimit = 1000000 // default
	}
	return h.service.CallContract(ctx, req.Address, entrypoint, req.Data, gasLimit)
}

// handleEstimateGas implements dsn_estimateGas.
func (h *Handler) handleEstimateGas(ctx context.Context, req *EstimateGasRequest) (uint64, error) {
	if req.Address == "" {
		return 0, fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	return h.service.EstimateGas(ctx, req.Address, req.Data)
}

// handleSendTransaction implements dsn_sendTransaction.
func (h *Handler) handleSendTransaction(ctx context.Context, req *SendTransactionRequest) (string, error) {
	if req.TxJSON == "" {
		return "", fmt.Errorf("%w: transaction is required", service.ErrInvalidParams)
	}
	return h.service.SendTransaction(ctx, req.TxJSON)
}

// handleGetEvents implements dsn_getEvents.
func (h *Handler) handleGetEvents(ctx context.Context, req *GetEventsRequest) ([]service.EventResult, error) {
	filter := req.Filter.ToEventFilter()
	return h.service.GetEvents(ctx, filter)
}

// handleGetValidator implements dsn_getValidator.
func (h *Handler) handleGetValidator(ctx context.Context, req *GetValidatorRequest) (*service.ValidatorResult, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	return h.service.GetValidator(ctx, req.Address)
}

// handleGetValidators implements dsn_getValidators.
func (h *Handler) handleGetValidators(ctx context.Context, req *GetValidatorsRequest) ([]service.ValidatorResult, error) {
	return h.service.GetValidators(ctx, req.Epoch)
}

// handleGetSupply implements dsn_getSupply.
func (h *Handler) handleGetSupply(ctx context.Context) (*service.SupplyResult, error) {
	return h.service.GetSupply(ctx)
}

// handleGetContractState implements dsn_getContractState.
func (h *Handler) handleGetContractState(ctx context.Context, req *GetContractStateRequest) (string, error) {
	if req.Address == "" {
		return "", fmt.Errorf("%w: address is required", service.ErrInvalidParams)
	}
	if req.Key == "" {
		return "", fmt.Errorf("%w: key is required", service.ErrInvalidParams)
	}
	return h.service.GetContractState(ctx, req.Address, req.Key)
}

// handleGetTransactionReceipt implements dsn_getTransactionReceipt.
func (h *Handler) handleGetTransactionReceipt(ctx context.Context, req *GetTransactionReceiptRequest) (*service.TransactionReceipt, error) {
	if req.TxHash == "" {
		return nil, fmt.Errorf("%w: txHash is required", service.ErrInvalidParams)
	}
	return h.service.GetTransactionReceipt(ctx, req.TxHash)
}

// handleGetStateRoot implements dsn_getStateRoot.
func (h *Handler) handleGetStateRoot(ctx context.Context) (*service.StateRootResult, error) {
	return h.service.GetStateRoot(ctx)
}

// handleGetPendingTxs implements dsn_getPendingTxs.
func (h *Handler) handleGetPendingTxs(ctx context.Context) ([]service.TransactionResult, error) {
	return h.service.GetPendingTxs(ctx)
}

// Helper to parse request parameters

func parseGetBlockRequest(params json.RawMessage) (*GetBlockRequest, error) {
	var req GetBlockRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseGetTransactionRequest(params json.RawMessage) (*GetTransactionRequest, error) {
	var req GetTransactionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseGetAccountRequest(params json.RawMessage) (*GetAccountRequest, error) {
	var req GetAccountRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseGetBalanceRequest(params json.RawMessage) (*GetBalanceRequest, error) {
	// Handle both positional and named params
	var req GetBalanceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Try positional - first param is address
		var args []interface{}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		if len(args) > 0 {
			if addr, ok := args[0].(string); ok {
				req.Address = addr
			}
		}
	}
	return &req, nil
}

func parseGetContractRequest(params json.RawMessage) (*GetContractRequest, error) {
	var req GetContractRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseCallContractRequest(params json.RawMessage) (*CallContractRequest, error) {
	var req CallContractRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseEstimateGasRequest(params json.RawMessage) (*EstimateGasRequest, error) {
	var req EstimateGasRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseSendTransactionRequest(params json.RawMessage) (*SendTransactionRequest, error) {
	var req SendTransactionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Try positional
		var args []json.RawMessage
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		if len(args) > 0 {
			req.TxJSON = string(args[0])
		}
	}
	return &req, nil
}

func parseGetEventsRequest(params json.RawMessage) (*GetEventsRequest, error) {
	var req GetEventsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseGetValidatorRequest(params json.RawMessage) (*GetValidatorRequest, error) {
	var req GetValidatorRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Try positional
		var args []interface{}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		if len(args) > 0 {
			if addr, ok := args[0].(string); ok {
				req.Address = addr
			}
		}
	}
	return &req, nil
}

func parseGetValidatorsRequest(params json.RawMessage) (*GetValidatorsRequest, error) {
	var req GetValidatorsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Try positional
		var args []interface{}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		if len(args) > 0 {
			if epoch, ok := args[0].(float64); ok {
				epochUint := uint64(epoch)
				req.Epoch = &epochUint
			}
		}
	}
	return &req, nil
}

func parseGetContractStateRequest(params json.RawMessage) (*GetContractStateRequest, error) {
	var req GetContractStateRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func parseGetTransactionReceiptRequest(params json.RawMessage) (*GetTransactionReceiptRequest, error) {
	var req GetTransactionReceiptRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Try positional params
		var args []interface{}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}
		if len(args) > 0 {
			if txHash, ok := args[0].(string); ok {
				req.TxHash = txHash
			}
		}
	}
	return &req, nil
}

// Utility functions

func hexToBytes(s string) ([]byte, error) {
	if s == "" || s == "0x" {
		return nil, nil
	}
	// Remove 0x prefix if present
	if len(s) > 2 && s[0:2] == "0x" {
		s = s[2:]
	}
	return hex.DecodeString(s)
}

// ReadBody reads and returns the request body.
func ReadBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// parseRequestID parses the request ID from JSON-RPC request.
func parseRequestID(id interface{}) interface{} {
	switch v := id.(type) {
	case float64:
		return int(v)
	case string:
		return v
	case nil:
		return nil
	default:
		return nil
	}
}
