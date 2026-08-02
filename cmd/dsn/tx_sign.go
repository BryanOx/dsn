package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsn/dsn/internal/txfile"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/spf13/cobra"
)

type txSignConfig struct {
	keyFile string
	output  string
}

var txSignFlags txSignConfig

var txSignCmd = &cobra.Command{
	Use:   "sign <tx-file>",
	Short: "Sign an unsigned transaction file",
	Long: `Sign an unsigned transaction JSON file with a wallet key.
Creates a new signed file without modifying the original.

The output file is named by appending "-signed" to the input filename
(e.g., tx-001.json -> tx-001-signed.json) unless --output is specified.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTxSign(args[0])
	},
}

func init() {
	txCmd.AddCommand(txSignCmd)

	f := txSignCmd.Flags()
	f.StringVarP(&txSignFlags.keyFile, "key", "k", "wallets/wallet.json", "Wallet key file")
	f.StringVarP(&txSignFlags.output, "output", "o", "", "Output signed file path (default: <input>-signed.json)")
}

func runTxSign(inputPath string) error {
	tf, err := txfile.ParseFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading tx file: %w", err)
	}

	if tf.IsSigned() {
		return fmt.Errorf("transaction file %s is already signed", inputPath)
	}

	kp, err := wallet.LoadKey(txSignFlags.keyFile)
	if err != nil {
		return fmt.Errorf("loading wallet key from %s: %w", txSignFlags.keyFile, err)
	}

	walletAddr := kp.Address().String()
	if tf.Sender != walletAddr {
		fmt.Fprintf(os.Stderr, "Warning: sender in tx file (%s) does not match wallet address (%s)\n",
			tf.Sender, walletAddr)
	}

	hasher := types.SHA256Hasher{}
	tx, err := tf.ToTransaction(hasher)
	if err != nil {
		return fmt.Errorf("converting transaction: %w", err)
	}

	if err := kp.Sign(tx, hasher); err != nil {
		return fmt.Errorf("signing transaction: %w", err)
	}

	signedTF := txfile.FromTransaction(tx)

	outputPath := txSignFlags.output
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := strings.TrimSuffix(inputPath, ext)
		outputPath = base + "-signed" + ext
	}

	if err := txfile.WriteFile(signedTF, outputPath); err != nil {
		return fmt.Errorf("writing signed tx file: %w", err)
	}

	fmt.Printf("Signed: %s\n", outputPath)
	return nil
}
