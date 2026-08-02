package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/rpc/middleware"
	"github.com/dsn/dsn/rpc/service"
	"github.com/dsn/dsn/rpc/ws"
)

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Server handles JSON-RPC requests.
type Server struct {
	service      service.NodeService
	handler      *Handler
	router       *Router
	subscription *ws.SubscriptionHandler
}

// New creates a new RPC server from a node (backward-compatible).
func New(n *node.Node) *Server {
	svc := service.NewNodeService(n)
	return NewWithService(svc)
}

// NewWithService creates a new RPC server with a NodeService.
// This is the backward-compatible constructor.
func NewWithService(svc service.NodeService) *Server {
	return NewWithServiceAndConfig(svc, DefaultRouterConfig())
}

// NewWithServiceAndConfig creates a new RPC server with a NodeService and config.
func NewWithServiceAndConfig(svc service.NodeService, cfg RouterConfig) *Server {
	router := NewRouter(svc, cfg)
	return &Server{
		service:      svc,
		handler:      NewHandler(svc),
		router:       router,
		subscription: router.SubscriptionHandler(),
	}
}

// SecurityConfig holds security configuration for the RPC server.
type SecurityConfig struct {
	TLSCertFile     string
	TLSKeyFile      string
	RPCApiKey       string
	RPCApiKeyHeader string
}

// NewWithSecurityConfig creates a new RPC server with a NodeService and security config.
// This is used when TLS or API key authentication is required.
func NewWithSecurityConfig(svc service.NodeService, secCfg SecurityConfig) *Server {
	routerCfg := DefaultRouterConfig()
	if secCfg.TLSCertFile != "" && secCfg.TLSKeyFile != "" {
		routerCfg.TLSEnabled = true
	}
	if secCfg.RPCApiKey != "" {
		routerCfg.ApiKey = secCfg.RPCApiKey
	}
	if secCfg.RPCApiKeyHeader != "" {
		routerCfg.ApiKeyHeader = secCfg.RPCApiKeyHeader
	} else {
		routerCfg.ApiKeyHeader = "X-API-Key"
	}

	router := NewRouter(svc, routerCfg)
	return &Server{
		service:      svc,
		handler:      NewHandler(svc),
		router:       router,
		subscription: router.SubscriptionHandler(),
	}
}

// SubscriptionHandler returns the WebSocket subscription handler
// for broadcasting events from the indexer/node.
func (s *Server) SubscriptionHandler() *ws.SubscriptionHandler {
	return s.subscription
}

// ServeHTTP handles HTTP JSON-RPC requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, -32600, "only POST allowed", nil)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, -32700, "read error", nil)
		return
	}

	// Check for batch request (reject in v1)
	if batchErr := middleware.ValidateBatchRequest(body); batchErr != nil {
		writeError(w, batchErr.Code, batchErr.Message, nil)
		return
	}

	var req RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, -32700, "parse error: "+err.Error(), nil)
		return
	}

	// Validate request
	if validationErr := middleware.ValidateRequest(middleware.RPCRequestFields{
		JSONRPC: req.JSONRPC,
		Method:  req.Method,
	}); validationErr != nil {
		writeError(w, validationErr.Code, validationErr.Message, req.ID)
		return
	}

	// Handle the request
	resp := s.handleRequest(context.Background(), &req)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleRequest routes and processes a single RPC request.
func (s *Server) handleRequest(ctx context.Context, req *RPCRequest) *RPCResponse {
	switch req.Method {
	// Block methods
	case "dsn_getBlock":
		return s.handleRPCGetBlock(ctx, req)
	// Transaction methods
	case "dsn_getTransaction":
		return s.handleRPCGetTransaction(ctx, req)
	case "dsn_sendTransaction":
		return s.handleRPCSendTransaction(ctx, req)
	// Account methods
	case "dsn_getAccount":
		return s.handleRPCGetAccount(ctx, req)
	case "dsn_getBalance":
		return s.handleRPCGetBalance(ctx, req)
	// Contract methods
	case "dsn_getContract":
		return s.handleRPCGetContract(ctx, req)
	case "dsn_callContract":
		return s.handleRPCCallContract(ctx, req)
	case "dsn_estimateGas":
		return s.handleRPCEstimateGas(ctx, req)
	case "dsn_getContractState":
		return s.handleRPCGetContractState(ctx, req)
	case "dsn_getTransactionReceipt":
		return s.handleRPCGetTransactionReceipt(ctx, req)
	// Event methods
	case "dsn_getEvents":
		return s.handleRPCGetEvents(ctx, req)
	// Validator methods
	case "dsn_getValidator":
		return s.handleRPCGetValidator(ctx, req)
	case "dsn_getValidators":
		return s.handleRPCGetValidators(ctx, req)
	// Supply methods
	case "dsn_getSupply":
		return s.handleRPCGetSupply(ctx, req)
	// Legacy methods (backward compatibility)
	case "dsn_getStateRoot":
		return s.handleLegacyGetStateRoot(ctx, req)
	case "dsn_getPendingTxs":
		return s.handleLegacyGetPendingTxs(ctx, req)
	default:
		return &RPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
			ID:      req.ID,
		}
	}
}

// dsn_getBlock handler
func (s *Server) handleRPCGetBlock(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetBlockRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetBlock(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getTransaction handler
func (s *Server) handleRPCGetTransaction(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetTransactionRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetTransaction(ctx, reqParams)
	if err != nil {
		if err == service.ErrNotFound {
			return errorResponse(req.ID, -32000, "transaction not found")
		}
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_sendTransaction handler
func (s *Server) handleRPCSendTransaction(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseSendTransactionRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	txHash, err := s.handler.handleSendTransaction(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]string{"txHash": txHash})
}

// dsn_getAccount handler
func (s *Server) handleRPCGetAccount(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetAccountRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetAccount(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getBalance handler
func (s *Server) handleRPCGetBalance(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetBalanceRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	balance, err := s.handler.handleGetBalance(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]string{"balance": balance})
}

// dsn_getContract handler
func (s *Server) handleRPCGetContract(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetContractRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetContract(ctx, reqParams)
	if err != nil {
		if err == service.ErrNotFound {
			return errorResponse(req.ID, -32000, "contract not found")
		}
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_callContract handler
func (s *Server) handleRPCCallContract(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseCallContractRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleCallContract(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_estimateGas handler
func (s *Server) handleRPCEstimateGas(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseEstimateGasRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	gas, err := s.handler.handleEstimateGas(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, map[string]uint64{"gas": gas})
}

// dsn_getContractState handler
func (s *Server) handleRPCGetContractState(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetContractStateRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetContractState(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	// Return as a map with "value" key
	return successResponse(req.ID, map[string]string{"value": result})
}

// dsn_getTransactionReceipt handler
func (s *Server) handleRPCGetTransactionReceipt(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetTransactionReceiptRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetTransactionReceipt(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getEvents handler
func (s *Server) handleRPCGetEvents(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetEventsRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetEvents(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getValidator handler
func (s *Server) handleRPCGetValidator(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetValidatorRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetValidator(ctx, reqParams)
	if err != nil {
		if err == service.ErrNotFound {
			return errorResponse(req.ID, -32000, "validator not found")
		}
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getValidators handler
func (s *Server) handleRPCGetValidators(ctx context.Context, req *RPCRequest) *RPCResponse {
	reqParams, err := parseGetValidatorsRequest(req.Params)
	if err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.handler.handleGetValidators(ctx, reqParams)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// dsn_getSupply handler
func (s *Server) handleRPCGetSupply(ctx context.Context, req *RPCRequest) *RPCResponse {
	result, err := s.handler.handleGetSupply(ctx)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return successResponse(req.ID, result)
}

// Legacy handlers for backward compatibility

func (s *Server) handleLegacyGetStateRoot(ctx context.Context, req *RPCRequest) *RPCResponse {
	result, err := s.handler.handleGetStateRoot(ctx)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}
	return successResponse(req.ID, result)
}

func (s *Server) handleLegacyGetPendingTxs(ctx context.Context, req *RPCRequest) *RPCResponse {
	result, err := s.handler.handleGetPendingTxs(ctx)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}
	return successResponse(req.ID, result)
}

// Response helpers

func successResponse(id interface{}, result interface{}) *RPCResponse {
	return &RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

func errorResponse(id interface{}, code int, message string) *RPCResponse {
	return &RPCResponse{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message},
		ID:      id,
	}
}

func writeError(w http.ResponseWriter, code int, msg string, id interface{}) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RPCResponse{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: msg},
		ID:      id,
	})
}

// Serve starts the HTTP RPC server.
func (s *Server) Serve(addr string) error {
	return http.ListenAndServe(addr, s.router)
}

// ServeTLS starts the HTTPS RPC server with TLS.
// If certFile or keyFile is empty, it falls back to Serve (HTTP).
func (s *Server) ServeTLS(certFile, keyFile, addr string) error {
	if certFile == "" || keyFile == "" {
		return s.Serve(addr)
	}
	return http.ListenAndServeTLS(addr, certFile, keyFile, s.router)
}
