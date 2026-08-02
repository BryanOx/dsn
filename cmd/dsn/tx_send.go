package main

import (
	"encoding/json"
	"fmt"

	"github.com/dsn/dsn/internal/txfile"
	"github.com/spf13/cobra"
)

type txSendConfig struct {
	rpcURL string
}

var txSendFlags txSendConfig

var txSendCmd = &cobra.Command{
	Use:   "send <signed-tx-file>",
	Short: "Send a signed transaction to a node",
	Long: `Submit a signed transaction JSON file to a DSN node via RPC.

The file must contain a signed transaction (with signature and intentId fields).
Use "dsn tx sign" to sign an unsigned transaction first.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTxSend(args[0])
	},
}

func init() {
	txCmd.AddCommand(txSendCmd)
	txSendCmd.Flags().StringVar(&txSendFlags.rpcURL, "rpc", "http://localhost:8545", "RPC URL")
}

func runTxSend(path string) error {
	tf, err := txfile.ParseFile(path)
	if err != nil {
		return fmt.Errorf("reading tx file: %w", err)
	}

	if !tf.IsSigned() {
		return fmt.Errorf("transaction file %s is not signed. Use 'dsn tx sign' first", path)
	}

	tfJSON, err := json.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshaling transaction: %w", err)
	}

	result, err := callRPC(txSendFlags.rpcURL, "dsn_sendTransaction", map[string]string{"tx": string(tfJSON)})
	if err != nil {
		return fmt.Errorf("RPC call failed: %w", err)
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(output))
	return nil
}
