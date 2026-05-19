package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	wallet2 "github.com/dsn/dsn/wallet"
	"github.com/spf13/cobra"

	"github.com/dsn/dsn/types"
)

// ValidatorFlags holds flags for validator commands.
type ValidatorFlags struct {
	keyFile     string
	stakeAmount uint64
	commission  uint64
	outputFile  string
	rpcURL      string
	nodeURL     string
}

var validatorFlags ValidatorFlags

// validatorCmd represents the validator command
var validatorCmd = &cobra.Command{
	Use:   "validator",
	Short: "Validator operations",
	Long: `Manage validator identity and registration.

Examples:
  dsn validator init
  dsn validator register
  dsn validator status`,
}

// validatorInitCmd initializes a new validator key
var validatorInitCmd = &cobra.Command{
	Use:   "init [flags]",
	Short: "Initialize a new validator key",
	Long: `Generate a new validator key pair and save to a file.

The validator key is used for:
  - Consensus signing (block proposal/voting)
  - Validator registration
  - Identifying the validator on the network

Examples:
  dsn validator init
  dsn validator init --output ~/.dsn/validator.key`,
	RunE: runValidatorInit,
}

// validatorRegisterCmd registers the validator with the network
var validatorRegisterCmd = &cobra.Command{
	Use:   "register [flags]",
	Short: "Register validator with the network",
	Long: `Register this node as a validator on the network.

This command requires:
  - Validator key file (generated with 'dsn validator init')
  - Sufficient stake amount (minimum 100,000 DSN)
  - Running RPC server

Examples:
  dsn validator register --key ~/.dsn/validator.key --stake 1000000 --node http://localhost:8545`,
	RunE: runValidatorRegister,
}

// validatorStatusCmd shows validator status
var validatorStatusCmd = &cobra.Command{
	Use:     "status [flags]",
	Aliases: []string{"info"},
	Short:   "Show validator status",
	Long: `Query the network for validator status.

Displays:
  - Validator address
  - Public key
  - Registration state (if node is running)

Examples:
  dsn validator status
  dsn validator info
  dsn validator status --key ~/.dsn/validator.key --node http://localhost:8545`,
	RunE: runValidatorStatus,
}

// defaultValidatorKeyPath returns the default validator key path
func defaultValidatorKeyPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home != "" {
		return filepath.Join(home, ".dsn", "validator.key")
	}
	return "~/.dsn/validator.key"
}

func init() {
	validatorCmd.AddCommand(validatorInitCmd)
	validatorCmd.AddCommand(validatorRegisterCmd)
	validatorCmd.AddCommand(validatorStatusCmd)

	defaultKeyPath := defaultValidatorKeyPath()

	// validator init flags
	validatorInitCmd.Flags().StringVar(&validatorFlags.outputFile, "output", defaultKeyPath, "output key file path")

	// validator register flags
	validatorRegisterCmd.Flags().StringVar(&validatorFlags.keyFile, "key", defaultKeyPath, "validator key file")
	validatorRegisterCmd.Flags().Uint64Var(&validatorFlags.stakeAmount, "stake", 0, "stake amount in DSN (minimum 100000)")
	validatorRegisterCmd.Flags().Uint64Var(&validatorFlags.commission, "commission", 1000, "commission rate (0-10000 = 0-100%)")
	validatorRegisterCmd.Flags().StringVar(&validatorFlags.nodeURL, "node", "http://localhost:8545", "node RPC URL")
	validatorRegisterCmd.Flags().StringVar(&validatorFlags.rpcURL, "rpc", "http://localhost:8545", "RPC server URL (alias for --node)")

	// validator status flags
	validatorStatusCmd.Flags().StringVar(&validatorFlags.keyFile, "key", defaultKeyPath, "validator key file")
	validatorStatusCmd.Flags().StringVar(&validatorFlags.nodeURL, "node", "http://localhost:8545", "node RPC URL")
	validatorStatusCmd.Flags().StringVar(&validatorFlags.rpcURL, "rpc", "http://localhost:8545", "RPC server URL (alias for --node)")
}

// expandPath expands ~ to home directory.
// Works on both Unix-like systems (uses HOME) and Windows (uses USERPROFILE).
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		if home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func runValidatorInit(cmd *cobra.Command, args []string) error {
	outputPath := expandPath(validatorFlags.outputFile)

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Printf("Warning: %s already exists\n", outputPath)
		fmt.Print("Do you want to overwrite? (y/N): ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Generate new validator key
	kp, err := wallet2.GenerateValidatorKey()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Ensure parent directory exists
	dir := outputPath
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			dir = dir[:i]
			break
		}
	}
	if dir != "" && dir != outputPath {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Save key to file
	if err := wallet2.SaveValidatorKey(outputPath, kp.PrivateKey, kp.PublicKey); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	// Print results
	fmt.Println("Validator key generated successfully!")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address.String())
	fmt.Printf("Public Key: %s\n", hex.EncodeToString(kp.PublicKey))
	fmt.Printf("Saved to:   %s\n", outputPath)
	fmt.Println()
	fmt.Println("IMPORTANT: Keep this key file secure!")

	return nil
}

func runValidatorRegister(cmd *cobra.Command, args []string) error {
	keyPath := expandPath(validatorFlags.keyFile)
	rpcURL := validatorFlags.nodeURL
	if validatorFlags.rpcURL != "http://localhost:8545" {
		rpcURL = validatorFlags.rpcURL
	}

	// Validate stake amount
	if validatorFlags.stakeAmount < 100000 {
		return fmt.Errorf("minimum stake is 100,000 DSN, got %d", validatorFlags.stakeAmount)
	}

	// Validate commission
	if validatorFlags.commission > 10000 {
		return fmt.Errorf("commission must be 0-10000 (0-100%%), got %d", validatorFlags.commission)
	}

	// Load validator key
	kp, err := wallet2.LoadValidatorKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load validator key from '%s': %w\nRun 'dsn validator init' to generate a new validator key", keyPath, err)
	}

	fmt.Println("Validator key loaded:")
	fmt.Printf("  Address:    %s\n", kp.Address.String())
	fmt.Printf("  Public Key: %s\n", hex.EncodeToString(kp.PublicKey))
	fmt.Println()

	// Get current nonce for the sender
	nonce, err := getNonce(rpcURL, kp.Address.String())
	if err != nil {
		return fmt.Errorf("failed to get nonce: %w", err)
	}

	// Build validator registration message
	msg := &validatorRegistrationMsg{
		Address:    kp.Address,
		PubKey:     []byte(kp.PublicKey),
		Stake:      validatorFlags.stakeAmount,
		Commission: validatorFlags.commission,
	}

	// Create registration transaction
	registrationTx, err := newValidatorRegistrationTx(kp.Address, nonce, msg, 1000, uint64(time.Now().Unix()), 1)
	if err != nil {
		return fmt.Errorf("failed to create registration transaction: %w", err)
	}

	// Sign transaction
	hasher := types.SHA256Hasher{}
	intentID, err := registrationTx.ComputeIntentID(&hasher)
	if err != nil {
		return fmt.Errorf("failed to compute intent ID: %w", err)
	}
	registrationTx.IntentID = intentID
	registrationTx.Signature = ed25519.Sign(kp.PrivateKey, intentID[:])

	// Submit to RPC
	fmt.Printf("Submitting registration transaction...\n")
	fmt.Printf("  Stake:      %d DSN\n", validatorFlags.stakeAmount)
	fmt.Printf("  Commission: %.1f%%\n", float64(validatorFlags.commission)/100)
	fmt.Println()

	result, err := submitTransaction(rpcURL, registrationTx)
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %w", err)
	}

	fmt.Println("Registration submitted successfully!")
	fmt.Printf("Intent ID: %s\n", hex.EncodeToString(registrationTx.IntentID[:]))

	if result != nil {
		if encoded, err := json.MarshalIndent(result, "", "  "); err == nil {
			fmt.Println("\nResponse:")
			fmt.Println(string(encoded))
		}
	}

	return nil
}

// validatorRegistrationMsg is a local wrapper for the types.ValidatorRegistrationMsg
type validatorRegistrationMsg struct {
	Address    types.Address
	PubKey     []byte
	Stake      uint64
	Commission uint64
}

func (m *validatorRegistrationMsg) encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if _, err := buf.Write(m.Address[:]); err != nil {
		return nil, err
	}
	if len(m.PubKey) != 32 {
		return nil, types.ErrInvalidEncoding
	}
	if _, err := buf.Write(m.PubKey); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.Stake); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, m.Commission); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newValidatorRegistrationTx(sender types.Address, nonce uint64, msg *validatorRegistrationMsg, maxFee uint64, timestamp uint64, chainID uint32) (*types.Transaction, error) {
	payload, err := msg.encode()
	if err != nil {
		return nil, err
	}

	return &types.Transaction{
		Version:   1,
		ChainID:   chainID,
		Sender:    sender,
		Nonce:     nonce,
		Payload:   payload,
		MaxFee:    maxFee,
		GasLimit:  50000,
		Timestamp: timestamp,
		TxType:    types.TxTypeValidatorRegistration,
	}, nil
}

func runValidatorStatus(cmd *cobra.Command, args []string) error {
	keyPath := expandPath(validatorFlags.keyFile)
	rpcURL := validatorFlags.nodeURL
	if validatorFlags.rpcURL != "http://localhost:8545" {
		rpcURL = validatorFlags.rpcURL
	}

	// Load validator key
	kp, err := wallet2.LoadValidatorKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load validator key from '%s': %w\nRun 'dsn validator init' to generate a new validator key", keyPath, err)
	}

	fmt.Println("Validator Information:")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address.String())
	fmt.Printf("Public Key: %s\n", hex.EncodeToString(kp.PublicKey))

	// Try to query RPC for status
	result, err := getValidatorStatus(rpcURL, kp.Address.String())
	if err != nil {
		fmt.Println("\nNote: Could not connect to RPC server to get status.")
		fmt.Println("The node may not be running.")
		return nil
	}

	if result != nil {
		fmt.Println("\nRegistration Status:")
		if encoded, err := json.MarshalIndent(result, "", "  "); err == nil {
			fmt.Println(string(encoded))
		}
	}

	return nil
}

// getNonce retrieves the current nonce for an address from the RPC
func getNonce(rpcURL, addr string) (uint64, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dsn_getAccount",
		"params":  map[string]string{"address": addr},
		"id":      1,
	}

	resp, err := doRPCRequest(rpcURL, req)
	if err != nil {
		return 0, err
	}

	if result, ok := resp["result"].(map[string]interface{}); ok {
		if nonce, ok := result["nonce"].(float64); ok {
			return uint64(nonce), nil
		}
	}

	// Default to nonce 1 if no account
	return 1, nil
}

// submitTransaction submits a transaction to the RPC
func submitTransaction(rpcURL string, tx *types.Transaction) (map[string]interface{}, error) {
	// Encode transaction to map
	txData := map[string]interface{}{
		"version":   tx.Version,
		"chainId":   tx.ChainID,
		"sender":    tx.Sender.String(),
		"nonce":     tx.Nonce,
		"payload":   hex.EncodeToString(tx.Payload),
		"maxFee":    tx.MaxFee,
		"gasLimit":  tx.GasLimit,
		"timestamp": tx.Timestamp,
		"signature": hex.EncodeToString(tx.Signature),
		"intentId":  hex.EncodeToString(tx.IntentID[:]),
		"txType":    tx.TxType,
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dsn_submitTransaction",
		"params":  txData,
		"id":      1,
	}

	resp, err := doRPCRequest(rpcURL, req)
	if err != nil {
		return nil, err
	}

	return resp["result"].(map[string]interface{}), nil
}

// getValidatorStatus queries the RPC for validator status
func getValidatorStatus(rpcURL, addr string) (map[string]interface{}, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dsn_getValidator",
		"params":  map[string]string{"address": addr},
		"id":      1,
	}

	resp, err := doRPCRequest(rpcURL, req)
	if err != nil {
		return nil, err
	}

	if result, ok := resp["result"].(map[string]interface{}); ok {
		return result, nil
	}
	return nil, nil
}

// doRPCRequest performs a JSON-RPC request
func doRPCRequest(url string, req interface{}) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpResp, err := http.Post(url, "application/json", strings.NewReader(string(jsonBytes)))
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var rpcResp map[string]interface{}
	if err := json.NewDecoder(httpResp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}

	// Handle error response
	if errObj, ok := rpcResp["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("RPC error: %v", errObj)
	}

	result, ok := rpcResp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response: no result")
	}

	return result, nil
}