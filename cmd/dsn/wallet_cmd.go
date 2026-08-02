package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dsn/dsn/types"
	wallet2 "github.com/dsn/dsn/wallet"
	"github.com/spf13/cobra"
)

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Wallet management commands",
	Long: `Wallet management for DSN:

  generate   Generate a new Ed25519 key pair
  sign       Sign a transaction from a file
  nonce      Get the current nonce for an address

Examples:
  dsn wallet generate
  dsn wallet generate --output wallets/mywallet.json
  dsn wallet sign transactions/tx.json --key wallets/mywallet.json
  dsn wallet nonce 0x1234567890abcdef`,
}

var walletGenerateCmd = &cobra.Command{
	Use:   "generate [flags]",
	Short: "Generate a new wallet key pair",
	Long: `Generate a new Ed25519 key pair and save to a file.

The generated key pair includes:
- Private key (hex encoded)
- Public key (hex encoded)
- Address (derived from public key)

Examples:
  dsn wallet generate
  dsn wallet generate --output wallets/mywallet.json`,
	RunE: runWalletGenerate,
}

var walletSignCmd = &cobra.Command{
	Use:   "sign <tx-file> [flags]",
	Short: "Sign a transaction from a file",
	Long: `Sign a transaction from a JSON file using a wallet key.

The transaction file should contain unsigned transaction fields:
{
  "sender": "0x...",
  "nonce": 0,
  "payload": "0x...",
  "gasLimit": 21000,
  "maxFee": 1000
}

Examples:
  dsn wallet sign transactions/tx.json
  dsn wallet sign transactions/tx.json --key wallets/mywallet.json`,
	Args: cobra.ExactArgs(1),
	RunE: runWalletSign,
}

var walletNonceCmd = &cobra.Command{
	Use:   "nonce <address> [flags]",
	Short: "Get the current nonce for an address",
	Long: `Query the RPC server for the current nonce of an address.

Examples:
  dsn wallet nonce 0x1234567890abcdef
  dsn wallet nonce 0x1234567890abcdef --rpc http://localhost:8545`,
	Args: cobra.ExactArgs(1),
	RunE: runWalletNonce,
}

// WalletFlags holds wallet command flags
type WalletFlags struct {
	output string
	key    string
	rpc    string
}

var walletFlags WalletFlags

func init() {
	rootCmd.AddCommand(walletCmd)

	// wallet generate flags
	walletGenerateCmd.Flags().StringVarP(&walletFlags.output, "output", "o", "wallets/wallet.json", "output file path")
	walletCmd.AddCommand(walletGenerateCmd)

	// wallet sign flags
	walletSignCmd.Flags().StringVarP(&walletFlags.key, "key", "k", "wallets/wallet.json", "wallet key file")
	walletCmd.AddCommand(walletSignCmd)

	// wallet nonce flags
	walletNonceCmd.Flags().StringVarP(&walletFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	walletCmd.AddCommand(walletNonceCmd)
}

func runWalletGenerate(cmd *cobra.Command, args []string) error {
	// Generate new key pair
	kp, err := wallet2.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Save to file
	if err := wallet2.SaveKey(walletFlags.output, kp); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	// Print results
	fmt.Println("Wallet generated successfully!")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address().String())
	fmt.Printf("Public Key: %s\n", hex.EncodeToString(kp.PublicKey[:]))
	fmt.Printf("Saved to:   %s\n", walletFlags.output)

	return nil
}

// UnsignedTransaction represents an unsigned transaction from a file
type UnsignedTransaction struct {
	Sender    string `json:"sender"`
	Nonce     uint64 `json:"nonce"`
	Payload   string `json:"payload,omitempty"`
	GasLimit  uint64 `json:"gasLimit"`
	MaxFee    uint64 `json:"maxFee"`
	Timestamp uint64 `json:"timestamp,omitempty"`
}

// SignedTransaction represents a signed transaction
type SignedTransaction struct {
	UnsignedTransaction
	Signature string `json:"signature"`
	IntentID  string `json:"intentId"`
}

func runWalletSign(cmd *cobra.Command, args []string) error {
	txFile := args[0]

	// Read transaction file
	data, err := os.ReadFile(txFile)
	if err != nil {
		return fmt.Errorf("failed to read transaction file: %w", err)
	}

	// Parse unsigned transaction
	var unsignedTx UnsignedTransaction
	if err := json.Unmarshal(data, &unsignedTx); err != nil {
		return fmt.Errorf("failed to parse transaction file: %w", err)
	}

	// Load wallet key
	kp, err := wallet2.LoadKey(walletFlags.key)
	if err != nil {
		return fmt.Errorf("failed to load wallet key: %w", err)
	}

	// Parse payload (hex to bytes)
	var payload []byte
	if unsignedTx.Payload != "" {
		payload, err = hex.DecodeString(strings.TrimPrefix(unsignedTx.Payload, "0x"))
		if err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}
	}

	// Create transaction - use payload for data
	// Always use current timestamp (avoids stale timestamps from script pipelines)
	ts := uint64(time.Now().Unix())
	tx := &types.Transaction{
		Sender:    kp.Address(),
		Nonce:     unsignedTx.Nonce,
		Payload:   payload,
		GasLimit:  unsignedTx.GasLimit,
		MaxFee:    unsignedTx.MaxFee,
		Timestamp: ts,
		TxType:    types.TxTypeStandard,
	}

	// Sign transaction
	hasher := types.SHA256Hasher{}
	if err := kp.Sign(tx, hasher); err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Update unsigned tx fields that were auto-populated
	unsignedTx.Timestamp = ts

	// Create signed transaction output
	signedTx := SignedTransaction{
		UnsignedTransaction: unsignedTx,
		Signature:           hex.EncodeToString(tx.Signature),
		IntentID:            hex.EncodeToString(tx.IntentID[:]),
	}

	// Output signed transaction
	output, err := json.MarshalIndent(signedTx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal signed transaction: %w", err)
	}

	fmt.Println(string(output))

	return nil
}

func runWalletNonce(cmd *cobra.Command, args []string) error {
	address := args[0]

	// Validate address
	addr, err := types.ParseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	// Query RPC for account
	result, err := callRPC(walletFlags.rpc, "dsn_getAccount", map[string]string{"address": addr.String()})
	if err != nil {
		return fmt.Errorf("RPC call failed: %w", err)
	}

	// Extract nonce from result
	if result == nil {
		return fmt.Errorf("empty response from RPC")
	}

	// Print the result nicely
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	fmt.Println(string(output))

	return nil
}

// RPCClient provides a simple RPC client for CLI
type RPCClient struct {
	url string
}

// callRPC makes a JSON-RPC call
func callRPC(url, method string, params interface{}) (map[string]interface{}, error) {
	type RPCRequest struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params"`
		ID      int         `json:"id"`
	}

	type RPCResponse struct {
		JSONRPC string                 `json:"jsonrpc"`
		Result  map[string]interface{} `json:"result"`
		Error   map[string]interface{} `json:"error"`
		ID      int                    `json:"id"`
	}

	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Make HTTP request
	resp, err := http.Post(url, "application/json", strings.NewReader(string(reqData)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decode response
	var rpcResp RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %v", rpcResp.Error)
	}

	return rpcResp.Result, nil
}
