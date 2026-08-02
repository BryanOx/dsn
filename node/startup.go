package node

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/genesis"
	"github.com/dsn/dsn/network"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"go.etcd.io/bbolt"
)

// StartupPhase represents a step in the node startup sequence.
type StartupPhase int

const (
	PhaseLoadConfig StartupPhase = iota
	PhaseValidateConfig
	PhaseLoadGenesis
	PhaseValidateGenesis
	PhaseHashGenesis
	PhaseInitBoltDB
	PhaseInitGenesisState
	PhaseInitValidatorRegistry
	PhaseInitConsensusEngine
	PhaseInitNetworking
	PhaseInitRPC
	PhaseInitMetrics
	PhaseEnterSyncMode
	PhaseTransitionToLive
)

// String returns a human-readable name for the startup phase.
func (p StartupPhase) String() string {
	switch p {
	case PhaseLoadConfig:
		return "LoadConfig"
	case PhaseValidateConfig:
		return "ValidateConfig"
	case PhaseLoadGenesis:
		return "LoadGenesis"
	case PhaseValidateGenesis:
		return "ValidateGenesis"
	case PhaseHashGenesis:
		return "HashGenesis"
	case PhaseInitBoltDB:
		return "InitBoltDB"
	case PhaseInitGenesisState:
		return "InitGenesisState"
	case PhaseInitValidatorRegistry:
		return "InitValidatorRegistry"
	case PhaseInitConsensusEngine:
		return "InitConsensusEngine"
	case PhaseInitNetworking:
		return "InitNetworking"
	case PhaseInitRPC:
		return "InitRPC"
	case PhaseInitMetrics:
		return "InitMetrics"
	case PhaseEnterSyncMode:
		return "EnterSyncMode"
	case PhaseTransitionToLive:
		return "TransitionToLive"
	default:
		return "Unknown"
	}
}

// StartupError captures which phase failed during startup.
type StartupError struct {
	Phase   StartupPhase
	Message string
	Cause   error
}

func (e *StartupError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("startup failed at %s: %s: %v", e.Phase, e.Message, e.Cause)
	}
	return fmt.Sprintf("startup failed at %s: %s", e.Phase, e.Message)
}

func (e *StartupError) Unwrap() error {
	return e.Cause
}

// Start starts the node with the full 14-step startup sequence.
// It accepts a context for cancellation.
// This is the main entry point for production node startup.
func (n *Node) Start(ctx context.Context) error {
	// Use sync.Once to ensure Start can only be called once
	n.startOnce.Do(func() {
		n.started = true
		n.shutdownCh = make(chan struct{})
	})

	if !n.started {
		return fmt.Errorf("node already started or failed to start")
	}

	// Run startup sequence - each step fails fast on error
	if err := n.startupPhase1_2_LoadValidateConfig(ctx); err != nil {
		return err
	}

	if err := n.startupPhase3_4_LoadValidateGenesis(ctx); err != nil {
		return err
	}

	if err := n.startupPhase5_HashGenesis(ctx); err != nil {
		return err
	}

	if err := n.startupPhase6_InitBoltDB(ctx); err != nil {
		return err
	}

	if err := n.startupPhase7_InitGenesisState(ctx); err != nil {
		return err
	}

	if err := n.startupPhase8_InitValidatorRegistry(ctx); err != nil {
		return err
	}

	if err := n.startupPhase9_InitConsensusEngine(ctx); err != nil {
		return err
	}

	if err := n.startupPhase10_InitNetworking(ctx); err != nil {
		return err
	}

	if err := n.startupPhase11_InitRPC(ctx); err != nil {
		return err
	}

	if err := n.startupPhase12_InitMetrics(ctx); err != nil {
		return err
	}

	if err := n.startupPhase13_EnterSyncMode(ctx); err != nil {
		return err
	}

	if err := n.startupPhase14_TransitionToLive(ctx); err != nil {
		return err
	}

	return nil
}

// startupPhase1_2_LoadValidateConfig loads and validates the config.
// Config loading is handled by CLI before calling Start(), so we just validate.
func (n *Node) startupPhase1_2_LoadValidateConfig(ctx context.Context) error {
	// Config is already loaded by CLI and passed to node.New()
	// Here we do any additional runtime validation
	if n.cfg.DataDir != "" {
		// Check if DataDir is writable
		if err := os.MkdirAll(n.cfg.DataDir, 0750); err != nil {
			return &StartupError{
				Phase:   PhaseValidateConfig,
				Message: "data directory is not writable",
				Cause:   err,
			}
		}
	}

	// Validate ports
	if n.cfg.P2PPort > 0 && (n.cfg.P2PPort < 1024 || n.cfg.P2PPort > 65535) {
		return &StartupError{
			Phase:   PhaseValidateConfig,
			Message: fmt.Sprintf("P2P port %d out of valid range (1024-65535)", n.cfg.P2PPort),
		}
	}

	if n.cfg.RPCPort > 0 && (n.cfg.RPCPort < 1024 || n.cfg.RPCPort > 65535) {
		return &StartupError{
			Phase:   PhaseValidateConfig,
			Message: fmt.Sprintf("RPC port %d out of valid range (1024-65535)", n.cfg.RPCPort),
		}
	}

	if n.cfg.MaxPeers <= 0 {
		return &StartupError{
			Phase:   PhaseValidateConfig,
			Message: fmt.Sprintf("max peers %d must be greater than 0", n.cfg.MaxPeers),
		}
	}

	return nil
}

// startupPhase3_4_LoadValidateGenesis loads and validates the genesis document.
func (n *Node) startupPhase3_4_LoadValidateGenesis(ctx context.Context) error {
	// Load genesis from file
	doc, err := genesis.LoadGenesis(n.cfg.GenesisFile)
	if err != nil {
		return &StartupError{
			Phase:   PhaseLoadGenesis,
			Message: "failed to load genesis file",
			Cause:   err,
		}
	}

	// Validate genesis
	if err := genesis.ValidateGenesis(doc); err != nil {
		return &StartupError{
			Phase:   PhaseValidateGenesis,
			Message: "genesis validation failed",
			Cause:   err,
		}
	}

	n.genesisDoc = doc
	return nil
}

// startupPhase5_HashGenesis computes genesis hash and verifies against stored.
func (n *Node) startupPhase5_HashGenesis(ctx context.Context) error {
	// Compute genesis hash
	genesisHash, err := genesis.HashGenesis(n.genesisDoc)
	if err != nil {
		return &StartupError{
			Phase:   PhaseHashGenesis,
			Message: "failed to compute genesis hash",
			Cause:   err,
		}
	}
	n.genesisHash = genesisHash

	// If DB exists, verify genesis hash matches stored
	if n.persistent != nil {
		storedHash, err := n.loadStoredGenesisHash(n.persistent.DB())
		if err == nil && storedHash != (types.Hash{}) {
			if storedHash != genesisHash {
				return &StartupError{
					Phase:   PhaseHashGenesis,
					Message: "genesis hash mismatch: node was started with different genesis",
				}
			}
		}
	}

	return nil
}

// loadStoredGenesisHash loads the stored genesis hash from BoltDB.
func (n *Node) loadStoredGenesisHash(db *bbolt.DB) (types.Hash, error) {
	var storedHash types.Hash
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("genesis"))
		if bucket == nil {
			return nil
		}
		data := bucket.Get([]byte("genesis_hash"))
		if data != nil && len(data) == 32 {
			copy(storedHash[:], data)
		}
		return nil
	})
	return storedHash, err
}

// startupPhase6_InitBoltDB opens/creates the BoltDB database.
// This is called after node.New() which may have already created the DB.
// This step ensures the DB is properly initialized and handles fresh vs existing.
func (n *Node) startupPhase6_InitBoltDB(ctx context.Context) error {
	// BoltDB is already initialized in node.New()
	// Here we ensure the genesis bucket exists for storing genesis hash
	if n.persistent != nil {
		err := n.persistent.DB().Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte("genesis"))
			if err != nil {
				return err
			}
			// Create/check meta bucket for DB versioning
			metaBucket, err := tx.CreateBucketIfNotExists([]byte("meta"))
			if err != nil {
				return err
			}
			// Check DB version
			versionBytes := metaBucket.Get([]byte("db_version"))
			if versionBytes == nil {
				// Fresh DB or pre-versioning DB — set version to 0 (legacy)
				return metaBucket.Put([]byte("db_version"), []byte{0, 0, 0, 0, 0, 0, 0, 0})
			}
			return nil
		})
		if err != nil {
			return &StartupError{
				Phase:   PhaseInitBoltDB,
				Message: "failed to initialize meta bucket",
				Cause:   err,
			}
		}
	}
	return nil
}

// startupPhase7_InitGenesisState initializes or loads genesis state.
func (n *Node) startupPhase7_InitGenesisState(ctx context.Context) error {
	if n.persistent == nil {
		return nil // in-memory only, nothing to initialize
	}

	// Check if DB is fresh (no state root)
	currentRoot := n.persistent.GetStateRoot()
	isFreshDB := currentRoot == (types.Hash{})

	if isFreshDB {
		// Initialize genesis state
		root, err := genesis.InitGenesisState(n.genesisDoc, n.state, n.persistent.DB(), n.hasher)
		if err != nil {
			return &StartupError{
				Phase:   PhaseInitGenesisState,
				Message: "failed to initialize genesis state",
				Cause:   err,
			}
		}

		// Store genesis hash for future restart verification
		err = n.persistent.DB().Update(func(tx *bbolt.Tx) error {
			return tx.Bucket([]byte("genesis")).Put([]byte("genesis_hash"), n.genesisHash[:])
		})
		if err != nil {
			return &StartupError{
				Phase:   PhaseInitGenesisState,
				Message: "failed to store genesis hash",
				Cause:   err,
			}
		}

		// Store genesis state root
		if _, err := n.persistent.Commit(); err != nil {
			return &StartupError{
				Phase:   PhaseInitGenesisState,
				Message: "failed to commit genesis state",
				Cause:   err,
			}
		}

		_ = root // genesis state root initialized
		n.currentHeight.Store(0)
		n.currentTipHash.Store(&types.Hash{})
	} else {
		// DB has existing state - load accounts into memory
		if err := n.persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
			return n.state.SetAccount(addr, acc)
		}); err != nil {
			return &StartupError{
				Phase:   PhaseInitGenesisState,
				Message: "failed to load accounts from persistent storage",
				Cause:   err,
			}
		}

		if err := n.persistent.ForEachKV(func(k string, v []byte) error {
			return n.state.SetBytes(k, v)
		}); err != nil {
			return &StartupError{
				Phase:   PhaseInitGenesisState,
				Message: "failed to load kvstore from persistent storage",
				Cause:   err,
			}
		}

		// Load tip from DB
		if tip, err := consensus.LoadTip(n.persistent); err == nil {
			hash, _ := tip.HeaderHash(n.hasher)
			n.setTip(tip.Height, hash)
			n.currentEpoch.Store(tip.Epoch)
		}
	}

	return nil
}

// startupPhase8_InitValidatorRegistry loads validators from staking registry or legacy DB.
func (n *Node) startupPhase8_InitValidatorRegistry(ctx context.Context) error {
	// Load validators from staking registry first, with fallback to legacy BoltDB
	if n.persistent != nil {
		// Try staking registry first
		activeVals, err := staking.GetActiveValidatorAddresses(n.State())
		if err != nil || len(activeVals) == 0 {
			// Fallback: load from old BoltDB bucket (pre-migration DBs)
			validators, err := n.loadValidatorsFromDB()
			if err != nil {
				return &StartupError{
					Phase:   PhaseInitValidatorRegistry,
					Message: "failed to load validators from DB",
					Cause:   err,
				}
			}
			n.validators = validators
		} else {
			n.validators = activeVals
		}

		// Migration check: if we loaded from old BoltDB bucket, log notice
		if n.persistent != nil {
			stakingCount, _ := staking.ValidatorCount(n.State())
			if stakingCount == 0 && len(n.validators) > 0 {
				fmt.Println("Note: validators loaded from legacy storage. Run migration to update staking registry.")
			}
		}
	}

	// If validator key file is configured, load and verify
	if n.cfg.ValidatorKeyFile != "" {
		vk, err := wallet.LoadValidatorKey(n.cfg.ValidatorKeyFile)
		if err != nil {
			return &StartupError{
				Phase:   PhaseInitValidatorRegistry,
				Message: "failed to load validator key",
				Cause:   err,
			}
		}

		// Check if this validator is in the active set
		isActive := false
		isPending := false
		var pendingValidator *staking.Validator
		for _, v := range n.validators {
			if v == vk.Address {
				isActive = true
				break
			}
		}

		if !isActive {
			// Check staking registry for pending status
			if n.State() != nil {
				v, err := staking.GetValidatorByOperator(n.State(), vk.Address)
				if err == nil && v.Status == staking.ValidatorPending {
					isPending = true
					pendingValidator = v
					fmt.Printf("Validator %s found PENDING in staking registry (activation epoch: %d)\n",
						vk.Address.String(), v.ActivationEpoch)
				}
			}
		}

		// Set wallet for signing - convert slices to arrays
		var pubKey [32]byte
		var privKey [64]byte
		copy(pubKey[:], vk.PublicKey)
		copy(privKey[:], vk.PrivateKey)
		n.wallet = &wallet.KeyPair{
			PublicKey:  pubKey,
			PrivateKey: privKey,
		}

		if isActive {
			fmt.Printf("Validator identity restored: %s\n", vk.Address.String())
		} else if isPending && pendingValidator != nil {
			fmt.Printf("Validator pending activation, expected at epoch %d\n", pendingValidator.ActivationEpoch)
		} else {
			fmt.Printf("Warning: validator key loaded but address not in active validator set (read-only mode)\n")
		}
	}

	return nil
}

// loadValidatorsFromDB loads validator entries from BoltDB.
func (n *Node) loadValidatorsFromDB() ([]types.Address, error) {
	var validators []types.Address

	err := n.persistent.DB().View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(genesis.ValidatorBucketName))
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(k, v []byte) error {
			var addr types.Address
			copy(addr[:], k)
			validators = append(validators, addr)
			return nil
		})
	})

	return validators, err
}

// startupPhase9_InitConsensusEngine initializes consensus parameters.
func (n *Node) startupPhase9_InitConsensusEngine(ctx context.Context) error {
	// Consensus params are loaded from genesis doc
	if n.genesisDoc != nil {
		// Store consensus params in node for use during block production
		n.consensusParams = &n.genesisDoc.ConsensusParams
		n.epochParams = &n.genesisDoc.EpochParams
	}

	// Load finality state from DB if exists
	if n.persistent != nil {
		// Finality is already tracked via finalizedHeight in consensus
	}

	return nil
}

// startupPhase10_InitNetworking starts P2P networking.
func (n *Node) startupPhase10_InitNetworking(ctx context.Context) error {
	if n.p2p == nil || n.cfg.P2PPort == 0 {
		fmt.Println("P2P networking disabled")
		return nil
	}

	// Start P2P listener (already started in node.New())
	fmt.Printf("P2P listening on /ip4/0.0.0.0/tcp/%d\n", n.cfg.P2PPort)

	// Connect to bootstrap peers
	if len(n.cfg.BootstrapPeers) > 0 {
		fmt.Printf("Connecting to bootstrap peers: %v\n", n.cfg.BootstrapPeers)
		// Peer connection is handled by discovery engine
	}

	// Start peer discovery
	if n.discovery != nil {
		n.discovery.Start()
	}

	return nil
}

// startupPhase11_InitRPC starts the JSON-RPC server.
// Note: RPC server is now managed by the CLI directly for better lifecycle control.
func (n *Node) startupPhase11_InitRPC(ctx context.Context) error {
	if n.cfg.RPCPort == 0 {
		fmt.Println("RPC server disabled (use --rpc-port to enable)")
		return nil
	}

	fmt.Printf("RPC server available at localhost:%d (started by CLI)\n", n.cfg.RPCPort)
	return nil
}

// startupPhase12_InitMetrics starts Prometheus metrics endpoint.
func (n *Node) startupPhase12_InitMetrics(ctx context.Context) error {
	if n.cfg.MetricsPort == 0 {
		fmt.Println("Metrics endpoint disabled")
		return nil
	}

	// Start metrics server
	n.metricsServer = &MetricsServer{
		node:   n,
		addr:   fmt.Sprintf(":%d", n.cfg.MetricsPort),
		stopCh: make(chan struct{}),
	}

	go n.metricsServer.Start()

	return nil
}

// startupPhase13_EnterSyncMode determines and enters the appropriate sync mode.
func (n *Node) startupPhase13_EnterSyncMode(ctx context.Context) error {
	// Determine initial sync mode based on node state
	if n.persistent == nil {
		// In-memory only, no sync needed
		n.syncMode = SyncModeNormal
		fmt.Println("Node running in-memory only (no sync required)")
		return nil
	}

	// Check if we have existing state
	stateRoot := n.persistent.GetStateRoot()
	if stateRoot == (types.Hash{}) {
		// Fresh genesis - node is at height 0, waiting for initial blocks
		// Start in normal mode but will sync from network
		n.syncMode = SyncModeNormal
		fmt.Println("Node at genesis height, waiting for network sync")
	} else if n.cfg.FastSyncEnabled && n.fastSync != nil {
		// Fast sync enabled - start fast sync
		n.syncMode = SyncModeFastSync
		fmt.Println("Starting fast sync...")
	} else {
		// Check if we're behind network (would need catch-up sync)
		// For now, assume we're current if we have state
		n.syncMode = SyncModeNormal
		fmt.Printf("Node at height %d, sync mode: normal\n", n.currentHeight.Load())
	}

	return nil
}

// startupPhase14_TransitionToLive starts consensus participation.
func (n *Node) startupPhase14_TransitionToLive(ctx context.Context) error {
	// If fast sync was running, wait for it to complete first
	if n.syncMode == SyncModeFastSync && n.fastSync != nil {
		// Fast sync runs asynchronously - set up completion handler
		n.fastSync.SetStateChangeHandler(func(oldState, newState network.SyncState) {
			if newState == network.SyncLive {
				n.syncMode = SyncModeNormal
				fmt.Println("Fast sync complete, transitioning to live consensus")
				// Start block sync catch-up if needed
				if n.blockSync != nil {
					n.blockSync.Start()
				}
				// Start consensus
				n.StartConsensus()
			}
		})
		n.fastSync.Start()
	} else {
		// Start consensus directly
		n.syncMode = SyncModeNormal

		// Only start consensus if we have validators and P2P
		if len(n.validators) > 0 && n.p2p != nil {
			n.StartConsensus()
			fmt.Println("Node is now participating in consensus")
		} else {
			fmt.Println("Node running in read-only mode (no validator key or P2P disabled)")
		}
	}

	return nil
}

// Shutdown gracefully stops the node with proper ordering.
func (n *Node) Shutdown(ctx context.Context) error {
	n.stopOnce.Do(func() {
		fmt.Println("Shutting down node...")

		// Close shutdown channel to signal all goroutines
		close(n.shutdownCh)

		// 1. Stop accepting new work - stop consensus first
		n.StopConsensus()

		// 2. Stop indexer
		if n.indexer != nil {
			n.indexer.Stop()
		}

		// 3. Stop sync engines
		if n.fastSync != nil {
			n.fastSync.Stop()
		}
		if n.blockSync != nil {
			n.blockSync.Stop()
		}
		if n.discovery != nil {
			n.discovery.Stop()
		}

		// 4. Close P2P connections
		if n.p2p != nil {
			if n.p2p.PeerManager() != nil {
				n.p2p.PeerManager().Persist()
				n.p2p.PeerManager().Stop()
			}
			_ = n.p2p.Close()
		}

		// 5. RPC server is managed by CLI - no action needed

		// 6. Stop metrics server
		if n.metricsServer != nil {
			n.metricsServer.Stop()
		}

		// 7. Close VM
		if n.vm != nil {
			_ = n.vm.Close()
		}

		// 8. Flush state to BoltDB and close
		if n.persistent != nil {
			// Persist finalized height
			if n.currentHeight.Load() > 0 {
				_ = consensus.StoreTip(n.persistent, &types.BlockHeader{
					Height:    n.currentHeight.Load(),
					StateRoot: n.persistent.GetStateRoot(),
					Timestamp: uint64(time.Now().Unix()),
				})
			}
			_ = n.persistent.Close()
		}

		fmt.Println("Node stopped")
	})

	return nil
}

// MetricsServer wraps Prometheus metrics HTTP server.
type MetricsServer struct {
	node    *Node
	addr    string
	stopCh  chan struct{}
	server  *http.Server
	mu      sync.Mutex
	running bool
}

// Start starts the metrics HTTP server.
func (ms *MetricsServer) Start() {
	ms.mu.Lock()
	if ms.running {
		ms.mu.Unlock()
		return
	}
	ms.running = true
	ms.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", ms.handleMetrics)
	mux.HandleFunc("/health", ms.handleHealth)

	ms.server = &http.Server{
		Addr:    ms.addr,
		Handler: mux,
	}

	go func() {
		if err := ms.server.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	fmt.Printf("Metrics server listening on %s\n", ms.addr)
}

// Stop stops the metrics HTTP server.
func (ms *MetricsServer) Stop() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.running {
		return
	}

	close(ms.stopCh)
	if ms.server != nil {
		_ = ms.server.Close()
	}
	ms.running = false
}

func (ms *MetricsServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Prometheus metrics would be generated here
	// For now, return basic metrics
	fmt.Fprintf(w, "# HELP dsn_block_height Current block height\n")
	fmt.Fprintf(w, "# TYPE dsn_block_height gauge\n")
	fmt.Fprintf(w, "dsn_block_height %d\n", ms.node.currentHeight.Load())
}

func (ms *MetricsServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}
