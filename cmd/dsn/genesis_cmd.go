package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BryanOx/dsn/dev/localnet"
	"github.com/BryanOx/dsn/genesis"
	"github.com/BryanOx/dsn/types"
	"github.com/spf13/cobra"
)

// GenesisFlags holds flags for genesis commands.
type GenesisFlags struct {
	outputFile    string
	chainID       string
	chainIDNum    uint32
	validators    string
	validatorsNum int
	allocations   string
	devnet        bool
	numNodes      int
}

var genesisFlags GenesisFlags

// genesisCmd represents the genesis command
var genesisCmd = &cobra.Command{
	Use:   "genesis",
	Short: "Genesis file operations",
	Long: `Manage genesis files for DSN networks.

Examples:
  dsn genesis init
  dsn genesis init --devnet
  dsn genesis validate
  dsn genesis devnet`,
}

// genesisInitCmd initializes a new genesis file
var genesisInitCmd = &cobra.Command{
	Use:   "init [flags]",
	Short: "Initialize a new genesis file",
	Long: `Create a new genesis file for a DSN network.

Examples:
  dsn genesis init
  dsn genesis init --devnet
  dsn genesis init --output genesis.json --chain-id 1`,
	RunE: runGenesisInit,
}

// genesisValidateCmd validates a genesis file
var genesisValidateCmd = &cobra.Command{
	Use:   "validate [flags]",
	Short: "Validate a genesis file",
	Long: `Validate a genesis file and check its integrity.

Examples:
  dsn genesis validate
  dsn genesis validate genesis.json`,
	RunE: runGenesisValidate,
}

// genesisDevnetCmd creates a local devnet with multiple validator nodes
var genesisDevnetCmd = &cobra.Command{
	Use:   "devnet [flags]",
	Short: "Create a local devnet with multiple validator nodes",
	Long: `Create a local development network with multiple validator nodes.

This command generates:
  - Validator keys for each node
  - Genesis.json with initial validator set
  - Per-node config files

Examples:
  dsn genesis devnet
  dsn genesis devnet --validators 5 --output dev/localnet/tmp`,
	RunE: runGenesisDevnet,
}

// genesisInspectCmd inspects a genesis file and displays detailed information
var genesisInspectCmd = &cobra.Command{
	Use:   "inspect [flags]",
	Short: "Inspect a genesis file and display detailed information",
	Long: `Display detailed information about a genesis file including:
  - Genesis version and chain ID
  - Validator set with stake and commission
  - Initial allocations
  - Treasury information
  - Consensus and epoch parameters
  - Genesis hash

Examples:
  dsn genesis inspect
  dsn genesis inspect genesis.json`,
	RunE: runGenesisInspect,
}

func init() {
	genesisCmd.AddCommand(genesisInitCmd)
	genesisCmd.AddCommand(genesisValidateCmd)
	genesisCmd.AddCommand(genesisDevnetCmd)
	genesisCmd.AddCommand(genesisInspectCmd)

	// genesis init flags
	genesisInitCmd.Flags().StringVar(&genesisFlags.outputFile, "output", "genesis.json", "output genesis file path")
	genesisInitCmd.Flags().StringVar(&genesisFlags.chainID, "chain-id", "dsn-localnet-1", "chain ID (string)")
	genesisInitCmd.Flags().IntVarP(&genesisFlags.validatorsNum, "validators", "n", 3, "number of validators (for devnet mode)")
	genesisInitCmd.Flags().StringVar(&genesisFlags.validators, "validator-addresses", "", "comma-separated list of validator addresses (non-devnet mode)")
	genesisInitCmd.Flags().StringVar(&genesisFlags.allocations, "allocations", "", "initial token allocations (JSON)")
	genesisInitCmd.Flags().BoolVar(&genesisFlags.devnet, "devnet", false, "generate devnet with auto-generated validator keys")

	// genesis validate uses a positional argument
	genesisValidateCmd.Flags().StringVar(&genesisFlags.outputFile, "file", "genesis.json", "genesis file to validate")

	// genesis devnet flags
	genesisDevnetCmd.Flags().IntVar(&genesisFlags.numNodes, "validators", 3, "number of validator nodes")
	genesisDevnetCmd.Flags().StringVar(&genesisFlags.outputFile, "output", "dev/localnet/tmp", "output directory")

	// genesis inspect flags
	genesisInspectCmd.Flags().StringVar(&genesisFlags.outputFile, "file", "genesis.json", "genesis file to inspect")
}

func runGenesisInit(cmd *cobra.Command, args []string) error {
	fmt.Println("=== Genesis ===")

	outputPath := genesisFlags.outputFile
	if outputPath == "" {
		outputPath = "genesis.json"
	}

	chainID := genesisFlags.chainID
	if chainID == "" {
		chainID = "dsn-localnet-1"
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("file already exists: %s (use --output to specify different path)", outputPath)
	}

	var doc *genesis.GenesisDoc

	if genesisFlags.devnet {
		// Devnet mode: auto-generate validator keys
		fmt.Printf("Generating devnet genesis with %d validators...\n", genesisFlags.validatorsNum)
		doc = generateDevnetGenesis(chainID, genesisFlags.validatorsNum)
	} else {
		// Non-devnet mode: use provided validator addresses or create template
		if genesisFlags.validators != "" {
			fmt.Println("Generating genesis with specified validators...")
			doc = generateGenesisWithValidators(chainID, genesisFlags.validators, genesisFlags.allocations)
		} else {
			fmt.Println("Generating genesis template...")
			doc = generateGenesisTemplate(chainID)
		}
	}

	// Write genesis file
	data, err := genesis.MarshalGenesis(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal genesis: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	fmt.Println()
	fmt.Printf("Genesis file created: %s\n", outputPath)
	fmt.Printf("Chain ID: %s\n", doc.ChainID)
	fmt.Printf("Initial validators: %d\n", len(doc.InitialValidators))

	// Validate the genesis
	if err := genesis.ValidateGenesis(doc); err != nil {
		return fmt.Errorf("genesis validation FAILED: %w", err)
	} else {
		fmt.Println("Genesis validation: PASSED")
	}

	// Compute and display genesis hash
	hash, err := genesis.HashGenesis(doc)
	if err == nil {
		fmt.Printf("Genesis hash: %s\n", hex.EncodeToString(hash[:]))
	}

	return nil
}

// generateDevnetGenesis creates a complete genesis with auto-generated validator keys.
func generateDevnetGenesis(chainID string, numValidators int) *genesis.GenesisDoc {
	validators := make([]genesis.ValidatorEntry, numValidators)
	balances := make([]genesis.BalanceEntry, numValidators+1) // +1 for treasury

	// Generate validator keys
	for i := 0; i < numValidators; i++ {
		pub, _, err := ed25519.GenerateKey(nil)
		if err != nil {
			panic(fmt.Sprintf("failed to generate validator key: %v", err))
		}

		// Derive address from public key (sha256 first 20 bytes)
		hash := sha256.Sum256(pub)
		var addr types.Address
		copy(addr[:], hash[:20])

		// Large stake for devnet (100M tokens, each token = 10^8 units)
		stake := uint64(100_000_000) * 10_000_000

		validators[i] = genesis.ValidatorEntry{
			Address:      "0x" + hex.EncodeToString(addr[:]),
			PubKey:       "0x" + hex.EncodeToString(pub),
			ConsensusKey: "0x" + hex.EncodeToString(pub), // Same as pubkey for Ed25519
			Stake:        stake,
			Commission:   "1000", // 10%
		}

		// Give each validator some initial balance
		balances[i] = genesis.BalanceEntry{
			Address: "0x" + hex.EncodeToString(addr[:]),
			Amount:  uint64(1_000_000) * 10_000_000, // 1M tokens
		}
	}

	// Treasury account
	treasuryAddr := types.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	balances[numValidators] = genesis.BalanceEntry{
		Address: "0x" + hex.EncodeToString(treasuryAddr[:]),
		Amount:  uint64(1_000_000_000) * 10_000_000, // 1B tokens
	}

	return &genesis.GenesisDoc{
		GenesisVersion: genesis.GenesisVersion,
		GenesisTime:    time.Now().UTC().Truncate(time.Second),
		ChainID:        chainID,
		InitialHeight:  1,
		ConsensusParams: genesis.ConsensusParams{
			MaxTxPerBlock:    1000,
			MaxBytesPerBlock: 2 * 1024 * 1024, // 2MB
			MaxGasPerBlock:   100_000_000,     // 100M gas
		},
		EpochParams: genesis.EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 2,
			MaxValidators:         100,
			MinimumStake:          uint64(10_000) * 10_000_000, // 10K tokens minimum
		},
		InflationParams: genesis.InflationParams{
			Enabled: false, // Devnet has no inflation
		},
		InitialValidators: validators,
		InitialBalances:   balances,
		Treasury: genesis.TreasuryEntry{
			Address:        "0x" + hex.EncodeToString(treasuryAddr[:]),
			InitialBalance: uint64(1_000_000_000) * 10_000_000,
		},
	}
}

// generateGenesisTemplate creates a genesis template with placeholder validators.
func generateGenesisTemplate(chainID string) *genesis.GenesisDoc {
	// Default treasury address
	treasuryAddr := types.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	return &genesis.GenesisDoc{
		GenesisVersion: genesis.GenesisVersion,
		GenesisTime:    time.Now().UTC().Truncate(time.Second),
		ChainID:        chainID,
		InitialHeight:  1,
		ConsensusParams: genesis.ConsensusParams{
			MaxTxPerBlock:    1000,
			MaxBytesPerBlock: 2 * 1024 * 1024,
			MaxGasPerBlock:   100_000_000,
		},
		EpochParams: genesis.EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 2,
			MaxValidators:         100,
			MinimumStake:          uint64(10_000) * 10_000_000,
		},
		InflationParams: genesis.InflationParams{
			Enabled: false,
		},
		InitialValidators: []genesis.ValidatorEntry{
			{
				Address:      "0x" + hex.EncodeToString(make([]byte, 20)),
				PubKey:       "0x" + hex.EncodeToString(make([]byte, 32)),
				ConsensusKey: "0x" + hex.EncodeToString(make([]byte, 32)),
				Stake:        uint64(100_000_000) * 10_000_000,
				Commission:   "1000",
			},
		},
		InitialBalances: []genesis.BalanceEntry{
			{
				Address: "0x" + hex.EncodeToString(make([]byte, 20)),
				Amount:  uint64(1_000_000) * 10_000_000,
			},
		},
		Treasury: genesis.TreasuryEntry{
			Address:        "0x" + hex.EncodeToString(treasuryAddr[:]),
			InitialBalance: uint64(1_000_000_000) * 10_000_000,
		},
	}
}

// generateGenesisWithValidators creates a genesis with user-specified validator addresses.
func generateGenesisWithValidators(chainID string, validatorsFlag string, allocationsFlag string) *genesis.GenesisDoc {
	// Parse validator addresses
	parts := strings.Split(validatorsFlag, ",")
	validators := make([]genesis.ValidatorEntry, 0, len(parts))
	balances := make([]genesis.BalanceEntry, 0)

	treasuryAddr := types.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	for _, addrHex := range parts {
		addrHex = strings.TrimSpace(addrHex)
		if addrHex == "" {
			continue
		}

		var addr types.Address
		addrBytes, err := hex.DecodeString(strings.TrimPrefix(addrHex, "0x"))
		if err != nil {
			fmt.Printf("Warning: invalid address '%s', skipping: %v\n", addrHex, err)
			continue
		}
		copy(addr[:], addrBytes)

		validators = append(validators, genesis.ValidatorEntry{
			Address:      "0x" + hex.EncodeToString(addr[:]),
			PubKey:       "0x" + hex.EncodeToString(make([]byte, 32)), // placeholder
			ConsensusKey: "0x" + hex.EncodeToString(make([]byte, 32)), // placeholder
			Stake:        uint64(100_000_000) * 10_000_000,
			Commission:   "1000",
		})

		balances = append(balances, genesis.BalanceEntry{
			Address: "0x" + hex.EncodeToString(addr[:]),
			Amount:  uint64(1_000_000) * 10_000_000,
		})
	}

	// Parse allocations if provided
	if allocationsFlag != "" {
		// JSON format: [{"address":"0x...","amount":1000000}]
		var allocs []struct {
			Address string `json:"address"`
			Amount  uint64 `json:"amount"`
		}
		if err := json.Unmarshal([]byte(allocationsFlag), &allocs); err != nil {
			fmt.Printf("Warning: failed to parse allocations JSON: %v\n", err)
		} else {
			for _, a := range allocs {
				balances = append(balances, genesis.BalanceEntry{
					Address: a.Address,
					Amount:  a.Amount,
				})
			}
		}
	}

	// Fallback: ensure at least one validator exists
	if len(validators) == 0 {
		fmt.Println("Warning: no valid validators provided, creating template...")
		return generateGenesisTemplate(chainID)
	}

	// Add treasury balance
	balances = append(balances, genesis.BalanceEntry{
		Address: "0x" + hex.EncodeToString(treasuryAddr[:]),
		Amount:  uint64(1_000_000_000) * 10_000_000,
	})

	return &genesis.GenesisDoc{
		GenesisVersion: genesis.GenesisVersion,
		GenesisTime:    time.Now().UTC().Truncate(time.Second),
		ChainID:        chainID,
		InitialHeight:  1,
		ConsensusParams: genesis.ConsensusParams{
			MaxTxPerBlock:    1000,
			MaxBytesPerBlock: 2 * 1024 * 1024,
			MaxGasPerBlock:   100_000_000,
		},
		EpochParams: genesis.EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 2,
			MaxValidators:         100,
			MinimumStake:          uint64(10_000) * 10_000_000,
		},
		InflationParams: genesis.InflationParams{
			Enabled: false,
		},
		InitialValidators: validators,
		InitialBalances:   balances,
		Treasury: genesis.TreasuryEntry{
			Address:        "0x" + hex.EncodeToString(treasuryAddr[:]),
			InitialBalance: uint64(1_000_000_000) * 10_000_000,
		},
	}
}

func runGenesisValidate(cmd *cobra.Command, args []string) error {
	filePath := genesisFlags.outputFile
	if filePath == "" {
		filePath = "genesis.json"
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("genesis file not found at '%s'\nRun 'dsn genesis init' or 'dsn genesis init --devnet' to create one", filePath)
	}

	fmt.Printf("Loading genesis file: %s\n", filePath)

	// Load genesis
	doc, err := genesis.LoadGenesis(filePath)
	if err != nil {
		return fmt.Errorf("failed to load genesis: %w", err)
	}

	fmt.Printf("Chain ID: %s\n", doc.ChainID)
	fmt.Printf("Genesis Time: %s\n", doc.GenesisTime.Format(time.RFC3339))
	fmt.Printf("Initial Height: %d\n", doc.InitialHeight)
	fmt.Printf("Validators: %d\n", len(doc.InitialValidators))
	fmt.Printf("Initial Balances: %d\n", len(doc.InitialBalances))
	fmt.Printf("Treasury: %s\n", doc.Treasury.Address)

	// Validate genesis
	fmt.Println()
	fmt.Println("Validating genesis...")

	if err := genesis.ValidateGenesis(doc); err != nil {
		return fmt.Errorf("genesis validation FAILED: %w", err)
	}

	fmt.Println("Genesis validation: PASSED")

	// Compute and display genesis hash
	hash, err := genesis.HashGenesis(doc)
	if err != nil {
		return fmt.Errorf("failed to compute genesis hash: %w", err)
	}

	fmt.Printf("Genesis hash: %s\n", hex.EncodeToString(hash[:]))

	return nil
}

func runGenesisDevnet(cmd *cobra.Command, args []string) error {
	fmt.Println("Creating local devnet...")
	fmt.Println()

	cfg := localnet.Config{
		NumValidators: genesisFlags.numNodes,
		OutputDir:     genesisFlags.outputFile,
		ChainID:       "dsn-localnet-1",
	}

	_, err := localnet.GenerateLocalnet(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate localnet: %w", err)
	}

	fmt.Println()
	fmt.Println("Devnet generation complete!")
	fmt.Println("You can now start the devnet with docker-compose.")

	return nil
}

func runGenesisInspect(cmd *cobra.Command, args []string) error {
	filePath := genesisFlags.outputFile
	if filePath == "" {
		filePath = "genesis.json"
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("genesis file not found at '%s'\nRun 'dsn genesis init' to create one", filePath)
	}

	// Load genesis
	doc, err := genesis.LoadGenesis(filePath)
	if err != nil {
		return fmt.Errorf("failed to load genesis: %w", err)
	}

	// Compute genesis hash
	hash, err := genesis.HashGenesis(doc)
	if err != nil {
		return fmt.Errorf("failed to compute genesis hash: %w", err)
	}

	// Calculate total allocation
	var totalAllocation uint64
	for _, b := range doc.InitialBalances {
		totalAllocation += b.Amount
	}

	// Print header
	fmt.Printf("Genesis File: %s\n", filePath)
	fmt.Printf("Genesis Version: %d\n", doc.GenesisVersion)
	fmt.Printf("Chain ID: %s\n", doc.ChainID)
	fmt.Printf("Genesis Time: %s\n", doc.GenesisTime.Format(time.RFC3339))
	fmt.Printf("Initial Height: %d\n", doc.InitialHeight)
	fmt.Println()

	// Print validator set
	fmt.Println("=== Validator Set", fmt.Sprintf("(%d) ===", len(doc.InitialValidators)))
	for _, v := range doc.InitialValidators {
		fmt.Printf("  Address:    %s\n", truncateAddress(v.Address))
		fmt.Printf("  Stake:      %s\n", formatAmount(v.Stake))
		fmt.Printf("  Commission: %s%%\n", formatCommission(v.Commission))
	}
	fmt.Println()

	// Print allocations
	fmt.Printf("=== Allocations (%d accounts, total: %s) ===\n", len(doc.InitialBalances), formatAmount(totalAllocation))
	for _, b := range doc.InitialBalances {
		fmt.Printf("  Address: %s\n", truncateAddress(b.Address))
		fmt.Printf("  Amount:  %s\n", formatAmount(b.Amount))
	}
	fmt.Println()

	// Print treasury
	fmt.Println("=== Treasury ===")
	fmt.Printf("  Address: %s\n", truncateAddress(doc.Treasury.Address))
	fmt.Printf("  Balance: %s\n", formatAmount(doc.Treasury.InitialBalance))
	fmt.Println()

	// Print consensus parameters
	fmt.Println("=== Consensus Parameters ===")
	fmt.Printf("  MaxTxPerBlock:    %d\n", doc.ConsensusParams.MaxTxPerBlock)
	fmt.Printf("  MaxBytesPerBlock: %d\n", doc.ConsensusParams.MaxBytesPerBlock)
	fmt.Printf("  MaxGasPerBlock:   %d\n", doc.ConsensusParams.MaxGasPerBlock)
	fmt.Println()

	// Print epoch parameters
	fmt.Println("=== Epoch Parameters ===")
	fmt.Printf("  BlocksPerEpoch:        %d\n", doc.EpochParams.BlocksPerEpoch)
	fmt.Printf("  UnstakeCooldownEpochs: %d\n", doc.EpochParams.UnstakeCooldownEpochs)
	fmt.Printf("  MaxValidators:         %d\n", doc.EpochParams.MaxValidators)
	fmt.Printf("  MinimumStake:          %s\n", formatAmount(doc.EpochParams.MinimumStake))
	fmt.Println()

	// Print inflation parameters
	fmt.Println("=== Inflation ===")
	fmt.Printf("  Enabled:   %t\n", doc.InflationParams.Enabled)
	if doc.InflationParams.AnnualRate != "" {
		fmt.Printf("  AnnualRate: %s\n", doc.InflationParams.AnnualRate)
	}
	fmt.Println()

	// Print genesis hash
	fmt.Println("=== Genesis Hash ===")
	fmt.Printf("  %s\n", hex.EncodeToString(hash[:]))

	return nil
}

// truncateAddress truncates a hex address for display (e.g., 0xabc...def)
func truncateAddress(addr string) string {
	if len(addr) <= 16 {
		return addr
	}
	return addr[:10] + "..." + addr[len(addr)-6:]
}

// formatAmount formats a token amount for display
func formatAmount(amount uint64) string {
	// Assuming 10^8 units per token
	const units = 10_000_000
	if amount < units {
		return fmt.Sprintf("%d", amount)
	}
	whole := amount / units
	remainder := amount % units
	if remainder == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.%08d", whole, remainder)
}

// formatCommission formats a commission rate from basis points to percentage
func formatCommission(commission string) string {
	// Commission is stored as basis points (e.g., 1000 = 10%)
	var basisPoints uint64
	_, err := fmt.Sscanf(commission, "%d", &basisPoints)
	if err != nil {
		return commission // Return as-is if parsing fails
	}
	return fmt.Sprintf("%.2f", float64(basisPoints)/100)
}
