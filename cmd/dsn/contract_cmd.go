package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
	wallet2 "github.com/dsn/dsn/wallet"
	"github.com/spf13/cobra"
)

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Smart contract operations",
	Long: `Smart contract operations for DSN:

  deploy     Deploy a WASM contract
  call       Call a contract method
  estimate   Estimate gas for a contract call

Examples:
  dsn contract deploy mycontract.wasm
  dsn contract call 0x123... increment --data 0x01
  dsn contract estimate 0x123... increment --data 0x01`,
}

var contractDeployCmd = &cobra.Command{
	Use:   "deploy <wasm-path>",
	Short: "Deploy a WASM contract",
	Long: `Deploy a smart contract from a WASM binary file.

Example:
  dsn contract deploy mycontract.wasm
  dsn contract deploy mycontract.wasm --abi contract.abi.json
  dsn contract deploy mycontract.wasm --gas-limit 100000 --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(1),
	RunE: runContractDeploy,
}

var contractCallCmd = &cobra.Command{
	Use:   "call <address> <entrypoint>",
	Short: "Call a contract method",
	Long: `Call a deployed contract method locally (read-only).

Example:
  dsn contract call 0x123... increment --data 0x01
  dsn contract call 0x123... get --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(2),
	RunE: runContractCall,
}

var contractEstimateCmd = &cobra.Command{
	Use:   "estimate <address> <entrypoint>",
	Short: "Estimate gas for a contract call",
	Long: `Estimate the gas required for a contract call.

Example:
  dsn contract estimate 0x123... increment --data 0x01
  dsn contract estimate 0x123... get --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(2),
	RunE: runContractEstimate,
}

var contractQueryCmd = &cobra.Command{
	Use:   "query <address> <key>",
	Short: "Query contract storage",
	Long: `Query contract storage for a specific key.

Example:
  dsn contract query 0x123... balance
  dsn contract query 0x123... mykey --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(2),
	RunE: runContractQuery,
}

var contractReceiptCmd = &cobra.Command{
	Use:   "receipt <txid>",
	Short: "Get transaction receipt",
	Long: `Get the receipt for a transaction.

Example:
  dsn contract receipt 0xabc123...
  dsn contract receipt 0xabc123... --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(1),
	RunE: runContractReceipt,
}

// ContractFlags holds contract command flags
type ContractFlags struct {
	abi       string
	gasLimit  uint64
	rpc       string
	data      string
	key       string
}

var contractFlags ContractFlags

func init() {
	rootCmd.AddCommand(contractCmd)

	// contract deploy flags
	contractDeployCmd.Flags().StringVar(&contractFlags.abi, "abi", "", "contract ABI file (optional)")
	contractDeployCmd.Flags().Uint64Var(&contractFlags.gasLimit, "gas-limit", 1000000, "gas limit")
	contractDeployCmd.Flags().StringVarP(&contractFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	contractDeployCmd.Flags().StringVarP(&contractFlags.key, "key", "k", "wallet.json", "wallet key file")
	contractCmd.AddCommand(contractDeployCmd)

	// contract call flags
	contractCallCmd.Flags().StringVar(&contractFlags.data, "data", "", "call data (hex)")
	contractCallCmd.Flags().Uint64Var(&contractFlags.gasLimit, "gas-limit", 1000000, "gas limit")
	contractCallCmd.Flags().StringVarP(&contractFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	contractCmd.AddCommand(contractCallCmd)

	// contract estimate flags
	contractEstimateCmd.Flags().StringVar(&contractFlags.data, "data", "", "call data (hex)")
	contractEstimateCmd.Flags().StringVarP(&contractFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	contractCmd.AddCommand(contractEstimateCmd)

	// contract query flags
	contractQueryCmd.Flags().StringVarP(&contractFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	contractCmd.AddCommand(contractQueryCmd)

	// contract receipt flags
	contractReceiptCmd.Flags().StringVarP(&contractFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	contractCmd.AddCommand(contractReceiptCmd)
}

// DeployRequest represents a contract deploy request
type DeployRequest struct {
	WASMPath  string `json:"wasmPath"`
	ABI       string `json:"abi,omitempty"`
	GasLimit  uint64 `json:"gasLimit"`
	WalletKey string `json:"walletKey"`
}

// DeployResponse represents a contract deploy response
type DeployResponse struct {
	Address   string `json:"address"`
	TxHash    string `json:"txHash,omitempty"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// CallRequest represents a contract call request
type CallRequest struct {
	Address    string `json:"address"`
	Entrypoint string `json:"entrypoint"`
	Data      string `json:"data,omitempty"`
	GasLimit   uint64 `json:"gasLimit"`
}

// CallResponse represents a contract call response
type CallResponse struct {
	Result string `json:"result"`
	Success bool  `json:"success"`
	Error  string `json:"error,omitempty"`
}

// EstimateRequest represents a gas estimate request
type EstimateRequest struct {
	Address    string `json:"address"`
	Entrypoint string `json:"entrypoint"`
	Data      string `json:"data,omitempty"`
}

// EstimateResponse represents a gas estimate response
type EstimateResponse struct {
	Gas   uint64 `json:"gas"`
	Error string `json:"error,omitempty"`
}

func runContractDeploy(cmd *cobra.Command, args []string) error {
	wasmPath := args[0]

	// Read WASM file
	wasmData, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("failed to read WASM file: %w", err)
	}

	// Compute codeHash = sha256(wasmCode)
	codeHash := types.Hash(sha256.Sum256(wasmData))

	// Load wallet
	kp, err := wallet2.LoadKey(contractFlags.key)
	if err != nil {
		return fmt.Errorf("failed to load wallet: %w", err)
	}

	sender := kp.Address()

	// Get sender's nonce from RPC
	nonce, err := getAccountNonce(contractFlags.rpc, sender.String())
	if err != nil {
		// If we can't get nonce, default to 0
		nonce = 0
	}

	// Create DeployContractTx
	deployTx := &types.DeployContractTx{
		Sender:   sender,
		Nonce:    nonce,
		WasmCode: wasmData,
		CodeHash: codeHash,
		MaxFee:   1000000, // Default max fee
		GasLimit: contractFlags.gasLimit,
	}

	// Encode DeployContractTx to bytes for payload
	var txBuf bytes.Buffer
	if err := deployTx.Encode(&txBuf); err != nil {
		return fmt.Errorf("failed to encode deploy transaction: %w", err)
	}

	// Create Transaction with DeployContractTx as payload
	tx := &types.Transaction{
		Sender:   sender,
		Nonce:    nonce,
		ChainID:  1, // Default chain ID
		Payload:  txBuf.Bytes(),
		GasLimit: contractFlags.gasLimit,
		MaxFee:   1000000,
		TxType:   types.TxTypeDeployContract,
	}

	// Sign the transaction
	if err := kp.Sign(tx, types.SHA256Hasher{}); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Build transaction JSON for RPC
	txJSON := buildTransactionJSON(tx)

	// Send transaction via RPC
	result, err := callRPC(contractFlags.rpc, "dsn_sendTransaction", map[string]interface{}{
		"tx": txJSON,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy contract: %w", err)
	}

	// Parse response for tx hash
	txHash, ok := result["txHash"].(string)
	if !ok {
		return fmt.Errorf("invalid response from RPC: no txHash")
	}

	// Compute contract address using vm.DeriveContractID
	contractAddr := deriveContractAddress(sender, nonce, codeHash)

	fmt.Println("Contract deployed successfully!")
	fmt.Println()
	fmt.Printf("Contract Address: %s\n", contractAddr.String())
	fmt.Printf("Transaction Hash: %s\n", txHash)

	return nil
}

// getAccountNonce retrieves the nonce for an account via RPC
func getAccountNonce(rpcURL, address string) (uint64, error) {
	result, err := callRPC(rpcURL, "dsn_getAccount", map[string]interface{}{
		"address": address,
	})
	if err != nil {
		return 0, err
	}

	nonce, ok := result["nonce"].(float64)
	if !ok {
		return 0, fmt.Errorf("failed to get nonce from account")
	}

	return uint64(nonce), nil
}

// buildTransactionJSON builds a JSON string from a Transaction
func buildTransactionJSON(tx *types.Transaction) string {
	txData := map[string]interface{}{
		"sender":    tx.Sender.String(),
		"nonce":     tx.Nonce,
		"chainId":   tx.ChainID,
		"payload":   hex.EncodeToString(tx.Payload),
		"gasLimit":  tx.GasLimit,
		"maxFee":    tx.MaxFee,
		"timestamp": tx.Timestamp,
		"signature": hex.EncodeToString(tx.Signature),
		"txType":    uint8(tx.TxType),
	}
	jsonBytes, _ := json.Marshal(txData)
	return string(jsonBytes)
}

// deriveContractAddress derives contract address from sender, nonce, and codeHash
// Uses vm.DeriveContractID for deterministic address generation
func deriveContractAddress(sender types.Address, nonce uint64, codeHash types.Hash) types.Address {
	// Use VM's DeriveContractID for deterministic address derivation
	id := vm.DeriveContractID(sender, nonce, codeHash)
	var addr types.Address
	copy(addr[:], id[:20])
	return addr
}

func runContractCall(cmd *cobra.Command, args []string) error {
	address := args[0]
	entrypoint := args[1]

	// Parse call data
	var callData []byte
	if contractFlags.data != "" {
		data, err := hex.DecodeString(strings.TrimPrefix(contractFlags.data, "0x"))
		if err != nil {
			return fmt.Errorf("invalid call data: %w", err)
		}
		callData = data
	}

	// Build request params
	params := map[string]interface{}{
		"address":    address,
		"entrypoint": entrypoint,
		"gasLimit":   contractFlags.gasLimit,
	}
	if len(callData) > 0 {
		params["data"] = hex.EncodeToString(callData)
	}

	// Call RPC
	result, err := callRPC(contractFlags.rpc, "dsn_callContract", params)
	if err != nil {
		return fmt.Errorf("contract call failed: %w", err)
	}

	// Print result
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	fmt.Println(string(output))

	return nil
}

func runContractEstimate(cmd *cobra.Command, args []string) error {
	address := args[0]
	entrypoint := args[1]

	// Parse call data
	var callData string
	if contractFlags.data != "" {
		callData = strings.TrimPrefix(contractFlags.data, "0x")
	}

	// Build request params
	params := map[string]interface{}{
		"address":    address,
		"entrypoint": entrypoint,
	}
	if callData != "" {
		params["data"] = callData
	}

	// Call RPC
	result, err := callRPC(contractFlags.rpc, "dsn_estimateGas", params)
	if err != nil {
		return fmt.Errorf("gas estimation failed: %w", err)
	}

	// Extract gas value
	gas, ok := result["gas"].(float64)
	if !ok {
		return fmt.Errorf("invalid response from RPC")
	}

	fmt.Printf("Estimated Gas: %.0f\n", gas)

	return nil
}

func runContractQuery(cmd *cobra.Command, args []string) error {
	address := args[0]
	key := args[1]

	// Build request params
	params := map[string]interface{}{
		"address": address,
		"key":     key,
	}

	// Call RPC
	result, err := callRPC(contractFlags.rpc, "dsn_getContractState", params)
	if err != nil {
		return fmt.Errorf("contract query failed: %w", err)
	}

	// Display result - get the "value" key
	if val, ok := result["value"].(string); ok && val != "" {
		fmt.Println(val)
		return nil
	}

	fmt.Println("null")
	return nil
}

func runContractReceipt(cmd *cobra.Command, args []string) error {
	txHash := args[0]

	// Build request params
	params := map[string]interface{}{
		"txHash": txHash,
	}

	// Call RPC
	result, err := callRPC(contractFlags.rpc, "dsn_getTransactionReceipt", params)
	if err != nil {
		return fmt.Errorf("failed to get receipt: %w", err)
	}

	// Print result as JSON
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal receipt: %w", err)
	}

	fmt.Println(string(output))

	return nil
}

// Use the wallet package from earlier
// This is a workaround since wallet is in the same package
func init() {
	// Import is handled at the top of the file
}

