package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/rpc"
	"github.com/BryanOx/dsn/rpc/service"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
)

func main() {
	// Check if any subcommand was provided
	if len(os.Args) > 1 {
		// Use cobra for subcommands
		if err := Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	// Legacy flag-based mode (backward compatibility)
	legacyMain()
}

// legacyMain handles the original flag-based behavior
func legacyMain() {
	// CLI flags
	walletPath := flag.String("wallet", "validator_key.json", "path to wallet key file")
	rpcAddr := flag.String("rpc", ":8545", "RPC server address")
	genesis := flag.Bool("genesis", false, "create genesis account (first run)")
	flag.Parse()

	// Load config from env (overrides defaults)
	cfg := node.ConfigFromEnv()

	// Override RPC port from CLI flag if provided
	if *rpcAddr != ":8545" {
		// use the flag value
	}

	n, err := node.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	printBanner()

	// Wallet setup
	var kp *wallet.KeyPair
	if *genesis {
		kp, err = wallet.GenerateKey()
		if err != nil {
			log.Fatal(err)
		}
		if err := wallet.SaveKey(*walletPath, kp); err != nil {
			log.Fatal(err)
		}

		// Create genesis account with funds
		var pubKey [32]byte
		copy(pubKey[:], kp.PublicKey[:])
		genesisAcc := state.NewAccount(kp.Address(), pubKey)
		genesisAcc.AddBalance(types.NewAmount(100000000)) // 100M DSN
		n.State().SetAccount(kp.Address(), genesisAcc)
		n.CommitState()

		fmt.Printf("Genesis wallet created: %s\n", *walletPath)
		fmt.Printf("   Address: %s\n", kp.Address().String())
		fmt.Printf("   Balance: 100,000,000 DSN\n")
	} else {
		kp, err = wallet.LoadKeyFile(*walletPath, nil) // nil provider — daemon stays read-only, never hangs on prompt
		if err != nil {
			log.Printf("No wallet found at %s (run with -genesis to create one)", *walletPath)
			// Continue without wallet (read-only mode)
		} else {
			fmt.Printf("Loaded wallet: %s\n", kp.Address().String())
		}
	}

	// Start RPC server
	var rpcServer *rpc.Server
	secCfg := rpc.SecurityConfig{
		TLSCertFile:     cfg.TLSCertFile,
		TLSKeyFile:      cfg.TLSKeyFile,
		RPCApiKey:       cfg.RPCApiKey,
		RPCApiKeyHeader: cfg.RPCApiKeyHeader,
	}

	// Create service layer
	svc := service.NewNodeService(n)

	// Use security config if TLS or API key is configured
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.RPCApiKey != "" {
		rpcServer = rpc.NewWithSecurityConfig(svc, secCfg)
	} else {
		rpcServer = rpc.NewWithService(svc)
	}

	go func() {
		// Check if TLS is configured
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			fmt.Printf("RPC server listening on https://%s (TLS enabled)\n", *rpcAddr)
			if err := rpcServer.ServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile, *rpcAddr); err != nil {
				log.Fatal(err)
			}
		} else {
			fmt.Printf("RPC server listening on %s\n", *rpcAddr)
			if err := rpcServer.Serve(*rpcAddr); err != nil {
				log.Fatal(err)
			}
		}
	}()

	// P2P networking
	if n.P2P() != nil {
		fmt.Printf("P2P listening on /ip4/0.0.0.0/tcp/%d\n", cfg.P2PPort)
		fmt.Printf("   Peer ID: %s\n", n.P2P().ID())
	}

	// Genesis state root
	root, _ := n.CommitState()
	fmt.Printf("Genesis state root: %x\n", root[:])
	fmt.Println()
	fmt.Println("Node is running. Press Ctrl+C to stop.")
	fmt.Println()
	fmt.Println("Available RPC methods:")
	fmt.Println("  dsn_getBalance     -> curl -X POST -d '{\"method\":\"dsn_getBalance\",\"params\":[\"address\"]}' http://localhost:8545")
	fmt.Println("  dsn_getStateRoot   -> curl -X POST -d '{\"method\":\"dsn_getStateRoot\"}' http://localhost:8545")
	fmt.Println("  dsn_getPendingTxs  -> curl -X POST -d '{\"method\":\"dsn_getPendingTxs\"}' http://localhost:8545")
	fmt.Println("  dsn_getAccount     -> curl -X POST -d '{\"method\":\"dsn_getAccount\",\"params\":[\"address\"]}' http://localhost:8545")

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("\nShutting down...")
	if err := n.Close(); err != nil {
		log.Printf("Error closing node: %v", err)
	}
	fmt.Println("Node stopped.")
}
