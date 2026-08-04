package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/BryanOx/dsn/internal/bip39"
	"github.com/BryanOx/dsn/internal/passphrase"
	"github.com/BryanOx/dsn/types"
	wallet2 "github.com/BryanOx/dsn/wallet"
	"github.com/spf13/cobra"
)

// lastMnemonic holds the recovery phrase of the wallet created or restored in
// the current process only. It is never written to disk; export-mnemonic reads
// it and a fresh process starts empty.
var lastMnemonic string

// passphraseProvider supplies keystore passphrases. It is nil in production,
// where walletPassphrase returns a TTY prompt; tests inject a deterministic
// source.
var passphraseProvider wallet2.PassphraseFunc

// walletPassphrase returns the passphrase provider for keystore creation and
// unlock, preferring an injected provider over the real TTY prompt.
func walletPassphrase() wallet2.PassphraseFunc {
	if passphraseProvider != nil {
		return passphraseProvider
	}
	return ttyWalletPassphrase
}

// ttyWalletPassphrase prompts on the TTY with double entry.
func ttyWalletPassphrase() (string, error) {
	return passphrase.PromptTwice("wallet passphrase")
}

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Wallet management commands",
	Long: `Wallet management for DSN:

  generate         Generate a new wallet (encrypted keystore by default)
  restore          Restore a wallet from a BIP-39 recovery phrase
  export-mnemonic  Print the recovery phrase held in this process
  migrate          Encrypt a legacy plaintext wallet in place
  sign             Sign a transaction from a file
  nonce            Get the current nonce for an address

Examples:
  dsn wallet generate
  dsn wallet generate --output wallets/mywallet.json
  dsn wallet restore --mnemonic "abandon abandon ... about"
  dsn wallet sign transactions/tx.json --key wallets/mywallet.json
  dsn wallet nonce 0x1234567890abcdef`,
}

var walletGenerateCmd = &cobra.Command{
	Use:   "generate [flags]",
	Short: "Generate a new wallet",
	Long: `Generate a new Ed25519 key pair and save it as an encrypted keystore.

The recovery mnemonic is printed exactly once on stdout after the wallet is
written.  Use --legacy-plaintext to write the old plaintext format without
prompting (no mnemonic, no passphrase).

Examples:
  dsn wallet generate
  dsn wallet generate --output wallets/mywallet.json
  dsn wallet generate --legacy-plaintext`,
	RunE: runWalletGenerate,
}

var walletRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a wallet from a recovery phrase",
	Long: `Restore an encrypted wallet from a BIP-39 12-word recovery phrase.

The mnemonic is validated before any prompt or file write. A new passphrase is
created for the encrypted keystore. The recovery mnemonic is printed again
after the wallet is written (in-process only).

Example:
  dsn wallet restore --mnemonic "abandon abandon ... about"`,
	RunE: runWalletRestore,
}

var walletExportCmd = &cobra.Command{
	Use:           "export-mnemonic",
	Short:         "Print the current recovery phrase",
	Long: `Print the BIP-39 recovery phrase of the wallet generated or restored earlier
in this same process. The phrase is never stored on disk; once the process
exits it cannot be recovered.`,
	RunE: runWalletExportMnemonic,
}

var walletMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Encrypt a legacy plaintext wallet in place",
	Long: `Encrypt a legacy plaintext wallet file with the new keystore format.

A backup of the original is saved alongside as <path>.bak. The migration is
verified after writing — if the encrypted file does not decrypt the original
is preserved.

Example:
  dsn wallet migrate --key wallets/wallet.json`,
	RunE: runWalletMigrate,
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
	output          string
	key             string
	rpc             string
	passphrase      string
	legacyPlaintext bool
	mnemonic        string
}

var walletFlags WalletFlags

func init() {
	rootCmd.AddCommand(walletCmd)

	// wallet generate flags
	walletGenerateCmd.Flags().StringVarP(&walletFlags.output, "output", "o", "wallets/wallet.json", "output file path")
	walletGenerateCmd.Flags().StringVar(&walletFlags.passphrase, "passphrase", "", "UNSUPPORTED: passphrases are prompted on the TTY only")
	walletGenerateCmd.Flags().BoolVar(&walletFlags.legacyPlaintext, "legacy-plaintext", false, "write the legacy plaintext format (no prompt, no mnemonic)")
	walletCmd.AddCommand(walletGenerateCmd)

	// wallet restore flags
	walletRestoreCmd.Flags().StringVar(&walletFlags.mnemonic, "mnemonic", "", "12-word BIP-39 recovery phrase")
	walletRestoreCmd.Flags().StringVarP(&walletFlags.output, "output", "o", "wallets/wallet.json", "output file path")
	walletCmd.AddCommand(walletRestoreCmd)

	// wallet export-mnemonic (no flags)
	walletCmd.AddCommand(walletExportCmd)

	// wallet migrate flags
	walletMigrateCmd.Flags().StringVarP(&walletFlags.key, "key", "k", "wallets/wallet.json", "legacy wallet key file to encrypt")
	walletCmd.AddCommand(walletMigrateCmd)

	// wallet sign flags
	walletSignCmd.Flags().StringVarP(&walletFlags.key, "key", "k", "wallets/wallet.json", "wallet key file")
	walletCmd.AddCommand(walletSignCmd)

	// wallet nonce flags
	walletNonceCmd.Flags().StringVarP(&walletFlags.rpc, "rpc", "r", "http://localhost:8545", "RPC server URL")
	walletCmd.AddCommand(walletNonceCmd)
}

func runWalletGenerate(cmd *cobra.Command, args []string) error {
	if cmd.Flags().Changed("passphrase") {
		return errors.New("--passphrase is not supported: passphrases are prompted on the TTY only and must never appear on the command line")
	}

	if walletFlags.legacyPlaintext {
		return runLegacyGenerate()
	}

	provider := walletPassphrase()
	pass, err := provider()
	if err != nil {
		return fmt.Errorf("passphrase prompt failed: %w", err)
	}

	mnemonic, err := bip39.GenerateMnemonic()
	if err != nil {
		return fmt.Errorf("failed to generate mnemonic: %w", err)
	}
	kp, err := wallet2.KeyFromMnemonic(mnemonic)
	if err != nil {
		return fmt.Errorf("failed to derive key from mnemonic: %w", err)
	}
	if err := wallet2.SaveKeystore(walletFlags.output, pass, kp); err != nil {
		return fmt.Errorf("failed to save encrypted keystore: %w", err)
	}

	lastMnemonic = mnemonic

	fmt.Println("Encrypted wallet created successfully!")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address().String())
	fmt.Printf("Saved to:   %s\n", walletFlags.output)
	fmt.Println()
	fmt.Printf("Recovery mnemonic: %s\n", mnemonic)
	fmt.Fprintln(os.Stderr, "warning: write down this mnemonic and keep it safe; it is shown once and never stored on disk")
	return nil
}

// runLegacyGenerate is the pre-change generate path — no prompt, no mnemonic.
func runLegacyGenerate() error {
	kp, err := wallet2.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	if err := wallet2.SaveKey(walletFlags.output, kp); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}
	fmt.Println("Wallet generated successfully!")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address().String())
	fmt.Printf("Public Key: %s\n", hex.EncodeToString(kp.PublicKey[:]))
	fmt.Printf("Saved to:   %s\n", walletFlags.output)
	return nil
}

func runWalletRestore(cmd *cobra.Command, args []string) error {
	phrase := strings.TrimSpace(walletFlags.mnemonic)
	if phrase == "" {
		return errors.New("--mnemonic is required")
	}
	if err := bip39.ValidateMnemonic(phrase); err != nil {
		return fmt.Errorf("invalid mnemonic: %w", err)
	}

	provider := walletPassphrase()
	pass, err := provider()
	if err != nil {
		return fmt.Errorf("passphrase prompt failed: %w", err)
	}
	kp, err := wallet2.KeyFromMnemonic(phrase)
	if err != nil {
		return fmt.Errorf("failed to derive key from mnemonic: %w", err)
	}
	if err := wallet2.SaveKeystore(walletFlags.output, pass, kp); err != nil {
		return fmt.Errorf("failed to save encrypted keystore: %w", err)
	}

	lastMnemonic = phrase

	fmt.Println("Wallet restored successfully!")
	fmt.Println()
	fmt.Printf("Address:    %s\n", kp.Address().String())
	fmt.Printf("Saved to:   %s\n", walletFlags.output)
	fmt.Println()
	fmt.Printf("Recovery mnemonic: %s\n", phrase)
	fmt.Fprintln(os.Stderr, "warning: write down this mnemonic and keep it safe; it is shown once and never stored on disk")
	return nil
}

func runWalletExportMnemonic(cmd *cobra.Command, args []string) error {
	if lastMnemonic == "" {
		return errors.New("no mnemonic available: the recovery phrase is held only in process memory for the session that generated or restored it; it can never be recovered from a keystore on disk")
	}
	fmt.Printf("Recovery mnemonic: %s\n", lastMnemonic)
	fmt.Fprintln(os.Stderr, "warning: this mnemonic is shown once and never stored on disk; write it down and keep it safe")
	return nil
}

func runWalletMigrate(cmd *cobra.Command, args []string) error {
	provider := walletPassphrase()
	pass, err := provider()
	if err != nil {
		return fmt.Errorf("passphrase prompt failed: %w", err)
	}
	if err := wallet2.Migrate(walletFlags.key, pass); err != nil {
		return fmt.Errorf("failed to migrate wallet: %w", err)
	}
	fmt.Printf("Migrated %s to an encrypted keystore; original preserved as %s.bak\n", walletFlags.key, walletFlags.key)
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

	// Load wallet key (encrypted keystore prompts for passphrase)
	kp, err := wallet2.LoadKeyFile(walletFlags.key, walletPassphrase())
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
