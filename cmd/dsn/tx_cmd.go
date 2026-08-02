package main

import (
	"github.com/spf13/cobra"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Manage transactions",
	Long: `Create, sign, and send DSN transactions.

Use "tx create" to generate unsigned transaction files,
"tx sign" to sign them with a wallet key,
and "tx send" to submit signed transactions to a node.`,
}

func init() {
	rootCmd.AddCommand(txCmd)
}
