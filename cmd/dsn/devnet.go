package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/explorer"
	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/rpc"
	"github.com/dsn/dsn/rpc/service"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/telemetry"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

// FaucetConfig holds configuration for the devnet faucet
type FaucetConfig struct {
	Amount         uint64        // Amount to dispense per request
	RateLimitSecs  time.Duration // Minimum time between requests
	MaxDailyAmount uint64        // Maximum amount per day (optional)
}

// DevnetFlags holds command-line flags for devnet
type DevnetFlags struct {
	rpcPort      int
	explorerPort int
	indexer      bool
	reset        bool
	verbose      bool
}

var devnetFlags DevnetFlags

var devnetCmd = &cobra.Command{
	Use:   "devnet",
	Short: "Start a local devnet node with RPC, indexer, and explorer",
	Long: `Start an in-memory devnet node for local development and testing.

Features:
- In-memory blockchain state (restarts clean)
- JSON-RPC server on port 8545
- Explorer REST API on port 8080
- Optional indexer for historical data
- Pre-funded validator account (1,000,000 DSN)
- Built-in faucet for test token distribution

Example:
  dsn devnet
  dsn devnet --indexer
  dsn devnet --reset`,
	RunE: runDevnet,
}

func init() {
	rootCmd.AddCommand(devnetCmd)

	devnetCmd.Flags().IntVar(&devnetFlags.rpcPort, "rpc-port", 8545, "RPC server port")
	devnetCmd.Flags().IntVar(&devnetFlags.explorerPort, "explorer-port", 8080, "Explorer server port")
	devnetCmd.Flags().BoolVar(&devnetFlags.indexer, "indexer", false, "Enable block indexer")
	devnetCmd.Flags().BoolVar(&devnetFlags.reset, "reset", false, "Reset devnet state (clear indexer DB)")
	devnetCmd.Flags().BoolVarP(&devnetFlags.verbose, "verbose", "v", false, "verbose output")
}

// devnetSigner implements consensus.Signer using a wallet key pair.
type devnetSigner struct {
	kp *wallet.KeyPair
}

func (s *devnetSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}

func runDevnet(cmd *cobra.Command, args []string) error {
	printBanner()

	// Create temp data directory for indexer persistence
	dataDir := filepath.Join(os.TempDir(), "dsn-devnet")

	// Handle reset flag
	if devnetFlags.reset {
		fmt.Println("Resetting devnet state...")
		if err := os.RemoveAll(dataDir); err != nil {
			log.Printf("Warning: could not remove data dir: %v", err)
		}
	}

	// Create data directory if needed
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Build node config
	cfg := node.DefaultConfig()
	cfg.DataDir = dataDir
	cfg.IndexerEnabled = devnetFlags.indexer

	// Create node
	n, err := node.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	// Generate validator key and pre-fund account
	validatorKey, err := wallet.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate validator key: %w", err)
	}

	// Create genesis account with 1M DSN
	var pubKey [32]byte
	copy(pubKey[:], validatorKey.PublicKey[:])
	genesisAcc := state.NewAccount(validatorKey.Address(), pubKey)
	genesisAcc.AddBalance(types.NewAmount(1000000)) // 1,000,000 DSN
	n.State().SetAccount(validatorKey.Address(), genesisAcc)

	// Commit initial state
	_, err = n.CommitState()
	if err != nil {
		return fmt.Errorf("failed to commit genesis state: %w", err)
	}

	// Set node wallet so nodeAddress() matches our validator
	n.SetWallet(validatorKey)

	// Start devnet block producer goroutine
	go func() {
		height := uint64(1)
		prevHash := types.Hash{}
		signer := &devnetSigner{kp: validatorKey}
		proposer := validatorKey.Address()
		hasher := types.SHA256Hasher{}
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Build block from mempool
				block, err := consensus.BuildBlock(
					n.State(),
					n.VM(), // WASM VM for contract execution
					n.Mempool(),
					height,
					prevHash,
					proposer,
					signer,
					hasher,
					100, // max txs per block
					nil, // no evidence
					3,   // matches the 3s devnet block ticker
				)
				if err != nil {
					log.Printf("Block production error at height %d: %v", height, err)
					continue
				}

				// Commit state changes
				_, err = n.CommitState()
				if err != nil {
					log.Printf("State commit error at height %d: %v", height, err)
					continue
				}

				// Compute this block's hash for next iteration
				blockHash, err := block.HeaderHash(hasher)
				if err != nil {
					log.Printf("Header hash error at height %d: %v", height, err)
					continue
				}

				// Clean processed txs from mempool
				for _, tx := range block.Transactions {
					n.Mempool().Remove(tx.IntentID)
				}

				log.Printf("Block produced at height %d with %d transactions", height, len(block.Transactions))
				height++
				prevHash = blockHash
			}
		}
	}()

	// Create service layer
	svc := service.NewNodeService(n)

	// Start RPC server
	rpcServer := rpc.NewWithService(svc)
	rpcAddr := fmt.Sprintf(":%d", devnetFlags.rpcPort)
	go func() {
		fmt.Printf("RPC server listening on http://localhost:%d\n", devnetFlags.rpcPort)
		if err := rpcServer.Serve(rpcAddr); err != nil {
			log.Printf("RPC server error: %v", err)
		}
	}()

	// Start indexer if enabled
	if devnetFlags.indexer && n.Indexer() != nil {
		n.Indexer().Start()
		fmt.Println("Indexer enabled")
	}

	// Create and start explorer server (background mode)
	explorerServer := explorer.NewServer(n.Indexer(), svc)
	explorerAddr := fmt.Sprintf(":%d", devnetFlags.explorerPort)
	stopExplorer := explorerServer.ServeBackground(explorerAddr)
	fmt.Printf("Explorer API listening on http://localhost:%d/api/v1\n", devnetFlags.explorerPort)

	// Add faucet endpoint to explorer
	faucetCfg := FaucetConfig{
		Amount:        100, // 100 DSN per request
		RateLimitSecs: 60,  // 60 seconds between requests
	}
	faucetHandler := NewDevnetFaucet(validatorKey, faucetCfg, svc)
	explorerServer.Router().HandleFunc("/api/v1/faucet", faucetHandler.HandleFaucet).Methods("POST", "OPTIONS")
	fmt.Printf("Faucet available at http://localhost:%d/api/v1/faucet\n", devnetFlags.explorerPort)

	// Initialize logger
	logger := telemetry.NewLogger("info")
	telemetry.SetLogger(logger)
	logger.Info("Devnet logger initialized", "level", "info")

	// Start Prometheus metrics server on port 9464
	metricsAddr := ":9464"
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logger.Info("Starting metrics server", "addr", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			logger.Warn("Metrics server error", "error", err.Error())
		}
	}()
	fmt.Printf("Prometheus metrics available at http://localhost%s/metrics\n", metricsAddr)

	// Start background metrics updater
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			// Update mempool count
			telemetry.MempoolTxCount.Set(float64(n.Mempool().Count()))
			// Update peer count if P2P is enabled
			if n.P2P() != nil && n.P2P().PeerManager() != nil {
				telemetry.PeerCount.Set(float64(n.P2P().PeerManager().NumConnected()))
			}
		}
	}()

	// Print devnet info
	fmt.Println()
	fmt.Println("Devnet is running!")
	fmt.Println("------------------")
	fmt.Printf("Validator Address: %s\n", validatorKey.Address().String())
	fmt.Printf("Validator Balance: 1,000,000 DSN\n")
	fmt.Println()
	fmt.Println("Useful endpoints:")
	fmt.Printf("  JSON-RPC: http://localhost:%d\n", devnetFlags.rpcPort)
	fmt.Printf("  Explorer: http://localhost:%d/api/v1\n", devnetFlags.explorerPort)
	fmt.Printf("  Faucet:  POST http://localhost:%d/api/v1/faucet with {\"address\":\"0x...\"}\n", devnetFlags.explorerPort)
	fmt.Printf("  Metrics: http://localhost:9464/metrics\n")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down devnet...")

	// Stop explorer
	stopExplorer()

	// Close node
	if err := n.Close(); err != nil {
		log.Printf("Error closing node: %v", err)
	}

	fmt.Println("Devnet stopped.")
	return nil
}

// DevnetFaucet handles faucet requests
type DevnetFaucet struct {
	mu           sync.Mutex
	lastRequest  map[string]time.Time
	rateLimitSec time.Duration
	amount       uint64
	signer       *wallet.KeyPair
	service      service.NodeService
	stopCleanup  chan struct{}
}

// NewDevnetFaucet creates a new faucet handler
func NewDevnetFaucet(key *wallet.KeyPair, cfg FaucetConfig, svc service.NodeService) *DevnetFaucet {
	f := &DevnetFaucet{
		lastRequest:  make(map[string]time.Time),
		rateLimitSec: cfg.RateLimitSecs,
		amount:       cfg.Amount,
		signer:       key,
		service:      svc,
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup goroutine to remove old entries
	go f.cleanupOldEntries()

	return f
}

// cleanupOldEntries periodically removes old rate limit entries
func (f *DevnetFaucet) cleanupOldEntries() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCleanup:
			return
		case <-ticker.C:
			f.mu.Lock()
			now := time.Now()
			for ip, lastReq := range f.lastRequest {
				// Remove entries older than 10 minutes
				if now.Sub(lastReq) > 10*time.Minute {
					delete(f.lastRequest, ip)
				}
			}
			f.mu.Unlock()
		}
	}
}

// Stop stops the faucet cleanup goroutine
func (f *DevnetFaucet) Stop() {
	close(f.stopCleanup)
}

// FaucetRequest represents a faucet request
type FaucetRequest struct {
	Address string `json:"address"`
}

// FaucetResponse represents a faucet response
type FaucetResponse struct {
	Success bool   `json:"success"`
	TxHash  string `json:"txHash,omitempty"`
	Error   string `json:"error,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

// HandleFaucet handles faucet requests
func (f *DevnetFaucet) HandleFaucet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get client IP for rate limiting
	ip := getClientIP(r)

	// Check rate limit
	f.mu.Lock()
	lastReq, exists := f.lastRequest[ip]
	if exists && time.Since(lastReq) < f.rateLimitSec {
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   fmt.Sprintf("rate limited, try again in %.0f seconds", f.rateLimitSec.Seconds()),
		})
		return
	}
	f.lastRequest[ip] = time.Now()
	f.mu.Unlock()

	// Parse request
	var req FaucetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "invalid request body",
		})
		return
	}

	// Validate address
	if req.Address == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "address is required",
		})
		return
	}

	// Parse address
	addr, err := types.ParseAddress(req.Address)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "invalid address format",
		})
		return
	}

	// Get current nonce from the node
	ctx := context.Background()
	account, err := f.service.GetAccount(ctx, f.signer.Address().String())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "failed to get account nonce",
		})
		return
	}
	nonce := account.Nonce + 1

	// Create transfer transaction
	// Encode recipient into payload
	payload := make([]byte, 20, 20+8)
	copy(payload[:20], addr[:])
	// Append amount as big-endian uint64
	amountBytes := make([]byte, 8)
	amountBytes[0] = byte(f.amount >> 56)
	amountBytes[1] = byte(f.amount >> 48)
	amountBytes[2] = byte(f.amount >> 40)
	amountBytes[3] = byte(f.amount >> 32)
	amountBytes[4] = byte(f.amount >> 24)
	amountBytes[5] = byte(f.amount >> 16)
	amountBytes[6] = byte(f.amount >> 8)
	amountBytes[7] = byte(f.amount)
	payload = append(payload, amountBytes...)

	// Build transaction for signing
	tx := &types.Transaction{
		Sender:      f.signer.Address(),
		Nonce:       nonce,
		Payload:     payload,
		GasLimit:    21000,
		MaxFee:      1000, // 0.001 DSN max fee
		Constraints: []byte{},
		Timestamp:   uint64(time.Now().Unix()),
		ChainID:     0,
		Version:     0,
	}

	// Compute IntentID
	hasher := types.SHA256Hasher{}
	intentID, err := tx.ComputeIntentID(hasher)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "failed to compute intent ID",
		})
		return
	}

	// Sign the transaction
	signature, err := f.signer.SignHash(intentID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "failed to sign transaction",
		})
		return
	}

	// Marshal signed transaction to JSON with lowercase fields
	signedTx := struct {
		Sender      string `json:"sender"`
		Nonce       uint64 `json:"nonce"`
		ChainID     uint32 `json:"chainId"`
		GasLimit    uint64 `json:"gasLimit"`
		MaxFee      uint64 `json:"maxFee"`
		Payload     string `json:"payload"`
		Constraints string `json:"constraints"`
		Timestamp   uint64 `json:"timestamp"`
		Signature   string `json:"signature"`
		IntentID    string `json:"intentId"`
	}{
		Sender:      f.signer.Address().String(),
		Nonce:       nonce,
		ChainID:     0,
		GasLimit:    21000,
		MaxFee:      1000,
		Payload:     hex.EncodeToString(payload),
		Constraints: "",
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   hex.EncodeToString(signature),
		IntentID:    hex.EncodeToString(intentID[:]),
	}

	txJSON, err := json.Marshal(signedTx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   "failed to marshal transaction",
		})
		return
	}

	// Send transaction via RPC
	txHash, err := f.service.SendTransaction(ctx, string(txJSON))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaucetResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(FaucetResponse{
		Success: true,
		TxHash:  txHash,
		Amount:  fmt.Sprintf("%d DSN", f.amount),
	})
}

// getClientIP extracts the client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take first IP in chain
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				return ip
			}
		}
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
