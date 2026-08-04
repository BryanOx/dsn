package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BryanOx/dsn/internal/txfile"
	"github.com/BryanOx/dsn/types"
	"github.com/spf13/cobra"
)

type txCreateConfig struct {
	sender    string
	to        string
	amount    uint64
	payload   string
	gasLimit  uint64
	maxFee    uint64
	nonce     uint64
	rpcURL    string
	outputDir string
	chainID   uint32
	txType    uint8
	force     bool
}

var txCreateFlags txCreateConfig

var txCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an unsigned transaction file",
	Long: `Create a new unsigned transaction JSON file with auto-numbering.

The output file is named tx-NNN.json where NNN is auto-incremented.
Use --to and --amount for a simple transfer (builds the payload automatically).
Use --payload for custom payload data (hex-encoded).
Use --nonce 0 (default) to auto-resolve from the node via --rpc.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTxCreate()
	},
}

func init() {
	txCmd.AddCommand(txCreateCmd)

	f := txCreateCmd.Flags()
	f.StringVarP(&txCreateFlags.sender, "sender", "s", "", "Sender address (required)")
	f.StringVar(&txCreateFlags.to, "to", "", "Recipient address (for transfer)")
	f.Uint64VarP(&txCreateFlags.amount, "amount", "a", 0, "Transfer amount")
	f.StringVar(&txCreateFlags.payload, "payload", "", "Transaction payload (hex with or without 0x)")
	f.Uint64Var(&txCreateFlags.gasLimit, "gas-limit", 100000, "Gas limit")
	f.Uint64Var(&txCreateFlags.maxFee, "max-fee", 10, "Max fee per gas")
	f.Uint64Var(&txCreateFlags.nonce, "nonce", 0, "Nonce (0 = auto-resolve from RPC)")
	f.StringVar(&txCreateFlags.rpcURL, "rpc", "http://localhost:8545", "RPC URL for nonce lookup")
	f.StringVarP(&txCreateFlags.outputDir, "output-dir", "o", "transactions", "Output directory for tx files")
	f.Uint32Var(&txCreateFlags.chainID, "chain-id", 0, "Chain ID")
	f.Uint8Var(&txCreateFlags.txType, "tx-type", 0, "Transaction type (0=Standard, 1=Deploy, 2=Call, 3=ValidatorReg)")
	f.BoolVar(&txCreateFlags.force, "force", false, "Overwrite existing file if it exists")
}

func runTxCreate() error {
	if txCreateFlags.sender == "" {
		return fmt.Errorf("--sender is required")
	}
	if _, err := types.ParseAddress(txCreateFlags.sender); err != nil {
		return fmt.Errorf("invalid sender address %q: %w", txCreateFlags.sender, err)
	}

	payload := txCreateFlags.payload
	if txCreateFlags.to != "" && txCreateFlags.amount > 0 {
		builtPayload, err := txfile.BuildTransferPayload(txCreateFlags.to, txCreateFlags.amount)
		if err != nil {
			return fmt.Errorf("building transfer payload: %w", err)
		}
		if payload != "" {
			fmt.Fprintf(os.Stderr, "Warning: --payload ignored because --to and --amount are set\n")
		}
		payload = builtPayload
	}

	if payload == "" {
		payload = "0x"
	}

	if !strings.HasPrefix(payload, "0x") && !strings.HasPrefix(payload, "0X") {
		payload = "0x" + payload
	}

	cleanPayload := strings.TrimPrefix(payload, "0x")
	cleanPayload = strings.TrimPrefix(cleanPayload, "0X")
	if _, err := hex.DecodeString(cleanPayload); err != nil {
		return fmt.Errorf("invalid payload hex: %w", err)
	}

	nonce := txCreateFlags.nonce
	if nonce == 0 && txCreateFlags.rpcURL != "" {
		resolvedNonce, err := resolveNonceFromRPC(txCreateFlags.rpcURL, txCreateFlags.sender)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve nonce from RPC (%v). Using nonce=0.\n", err)
		} else {
			nonce = resolvedNonce
			fmt.Fprintf(os.Stderr, "Resolved nonce: %d\n", nonce)
		}
	}

	if err := os.MkdirAll(txCreateFlags.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	txPath, _, err := txfile.AutoNumber(txCreateFlags.outputDir)
	if err != nil {
		return fmt.Errorf("auto-numbering: %w", err)
	}

	if !txCreateFlags.force {
		if _, err := os.Stat(txPath); err == nil {
			return fmt.Errorf("file %s already exists (use --force to overwrite)", txPath)
		}
	}

	tf := &txfile.TransactionFile{
		Version:   1,
		ChainID:   txCreateFlags.chainID,
		Sender:    txCreateFlags.sender,
		Nonce:     nonce,
		Payload:   payload,
		MaxFee:    txCreateFlags.maxFee,
		GasLimit:  txCreateFlags.gasLimit,
		Timestamp: uint64(time.Now().Unix()),
		TxType:    txCreateFlags.txType,
	}

	if err := txfile.WriteFile(tf, txPath); err != nil {
		return fmt.Errorf("writing tx file: %w", err)
	}

	fmt.Printf("Created: %s\n", txPath)
	return nil
}

func resolveNonceFromRPC(rpcURL, sender string) (uint64, error) {
	result, err := callRPC(rpcURL, "dsn_getAccount", []string{sender})
	if err != nil {
		return 0, err
	}

	nonceFloat, ok := result["nonce"].(float64)
	if !ok {
		return 0, fmt.Errorf("could not parse nonce from response: %v", result)
	}
	return uint64(nonceFloat), nil
}
