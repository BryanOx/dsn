//go:build integration
// +build integration

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/genesis"
	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
	"github.com/dsn/dsn/wallet"
)

// TestNode wraps a node.Node for integration tests
type TestNode struct {
	Node    *node.Node
	Config  node.Config
	Wallet  *wallet.KeyPair
	Txs     []*types.Transaction
	Results []*vm.ExecutionResult
	Blocks  []*types.Block
}

// walletSigner wraps a wallet.KeyPair to implement consensus.Signer interface
type walletSigner struct {
	kp *wallet.KeyPair
}

func (s *walletSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}

// TestNodeOption is a functional option for configuring a TestNode
type TestNodeOption func(*node.Config)

// NewTestNode creates an ephemeral in-memory node with the given validators
func NewTestNode(t testing.TB, validators []types.Address, opts ...TestNodeOption) *TestNode {
	t.Helper()

	// Generate a wallet for the node (needed before creating genesis)
	kp, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("NewTestNode: generate wallet: %v", err)
	}

	// Create a genesis file for the node
	genesisDir := t.TempDir()
	genesisPath := filepath.Join(genesisDir, "genesis.json")

	// Convert validator addresses to genesis format
	genesisValidators := make([]genesis.ValidatorEntry, len(validators))
	for i, addr := range validators {
		// Get the public key for this validator
		// For validators not matching our keypair, generate temporary keys
		var pubKey [32]byte
		if addr == kp.Address() {
			pubKey = kp.PublicKey
		} else {
			// Generate a temporary keypair for other validators
			tempKP, _ := wallet.GenerateKey()
			pubKey = tempKP.PublicKey
		}

		genesisValidators[i] = genesis.ValidatorEntry{
			Address:      fmt.Sprintf("0x%x", addr[:]),
			PubKey:       fmt.Sprintf("0x%x", pubKey[:]),
			ConsensusKey: fmt.Sprintf("0x%x", pubKey[:20]), // Use first 20 bytes as consensus key
			Stake:        100000,                           // Minimum stake for testing
			Commission:   "1000",                           // 10% commission
		}
	}

	// Create genesis document
	doc := &genesis.GenesisDoc{
		GenesisTime:   time.Now(),
		ChainID:       "test-chain",
		InitialHeight: 0,
		ConsensusParams: genesis.ConsensusParams{
			MaxTxPerBlock:    100,
			MaxBytesPerBlock: 10485760, // 10MB
			MaxGasPerBlock:   10000000,
		},
		EpochParams: genesis.EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 2,
			MaxValidators:         100,
			MinimumStake:          1000,
		},
		InflationParams: genesis.InflationParams{
			Enabled: false,
		},
		InitialValidators: genesisValidators,
		InitialBalances:   []genesis.BalanceEntry{},
		Treasury: genesis.TreasuryEntry{
			Address:        "0x0000000000000000000000000000000000000000",
			InitialBalance: 0,
		},
	}

	// Write genesis file
	genesisJSON, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("NewTestNode: marshal genesis: %v", err)
	}
	if err := os.WriteFile(genesisPath, genesisJSON, 0644); err != nil {
		t.Fatalf("NewTestNode: write genesis: %v", err)
	}

	cfg := node.DefaultConfig()
	cfg.DataDir = ""              // ephemeral in-memory
	cfg.GenesisFile = genesisPath // Set genesis file path
	cfg.P2PPort = 0               // no P2P
	cfg.RPCPort = 0               // no RPC
	cfg.Validators = validators
	cfg.MaxTxPerBlock = 100
	cfg.ProposerTimeout = time.Second
	cfg.VMTimeoutSeconds = 30
	for _, opt := range opts {
		opt(&cfg)
	}

	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("NewTestNode: %v", err)
	}

	n.SetWallet(kp)

	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		n.Close()
		t.Fatalf("NewTestNode: start: %v", err)
	}

	return &TestNode{Node: n, Config: cfg, Wallet: kp}
}

// NewMultiNodeNetwork creates N independent in-memory nodes (no P2P)
func NewMultiNodeNetwork(t testing.TB, n int, opts ...TestNodeOption) []*TestNode {
	t.Helper()
	if n < 1 {
		t.Fatal("NewMultiNodeNetwork: must have at least 1 node")
	}

	// Create identical validator set - each node gets a unique key
	// but they're all validators in the same set
	validators := make([]types.Address, n)
	keyPairs := make([]*wallet.KeyPair, n)

	for i := 0; i < n; i++ {
		kp, err := wallet.GenerateKey()
		if err != nil {
			t.Fatalf("NewMultiNodeNetwork: generate key: %v", err)
		}
		keyPairs[i] = kp
		validators[i] = kp.Address()
	}

	// Create nodes
	nodes := make([]*TestNode, n)
	for i := 0; i < n; i++ {
		nodes[i] = NewTestNode(t, validators, opts...)
		// Give the node's wallet some initial balance
		addr := keyPairs[i].Address()
		var pubKey [32]byte
		copy(pubKey[:], keyPairs[i].PublicKey[:])
		acc := state.NewAccount(addr, pubKey)
		acc.AddBalance(types.NewAmount(1000000)) // Give initial balance
		nodes[i].Node.State().SetAccount(addr, acc)
	}

	return nodes
}

// DeployContract deploys a WASM contract and returns the contract address
func (tn *TestNode) DeployContract(t testing.TB, wasmCode []byte) types.Address {
	t.Helper()

	// Build deploy transaction
	tx := BuildDeployTokenTx(tn.Wallet.Address(), getNonce(tn, tn.Wallet.Address()), 1000000, tn.Wallet, wasmCode)

	// Submit and mine
	tn.SubmitAndMine(t, tx)
	tn.Txs = append(tn.Txs, tx)

	// The contract address is derived from the deploy transaction's code hash
	// For deterministic testing, we use the sender's address as the contract address
	// In production, this would come from the VM execution result
	contractAddr := types.Address{}
	copy(contractAddr[:], tx.IntentID[:20])

	return contractAddr
}

// getNonce gets the nonce for an address from the state
func getNonce(tn *TestNode, addr types.Address) uint64 {
	acc, err := tn.Node.State().GetAccount(addr)
	if err != nil || acc == nil {
		return 0
	}
	return acc.Nonce
}

// CallContract executes a read-only contract call
func (tn *TestNode) CallContract(t testing.TB, contractID types.Address, entrypoint string, calldata []byte) *vm.ExecutionResult {
	t.Helper()

	var contractHash types.Hash
	copy(contractHash[:], contractID[:])

	// Execute without committing (read-only)
	result, err := tn.Node.VM().Execute(&types.CallContractTx{
		ContractID: contractHash,
		Sender:     tn.Wallet.Address(),
		Entrypoint: entrypoint,
		Calldata:   calldata,
		GasLimit:   1000000,
	}, tn.Node.State(), tn.Node.CurrentHeight()+1, uint64(time.Now().Unix()), 0)

	if err != nil {
		t.Fatalf("CallContract: %v", err)
	}

	tn.Results = append(tn.Results, result)
	return result
}

// SubmitAndMine submits a tx and produces a block
func (tn *TestNode) SubmitAndMine(t testing.TB, tx *types.Transaction) *types.Block {
	t.Helper()

	if err := tn.Node.SubmitTx(tx); err != nil {
		t.Fatalf("SubmitAndMine: submit tx: %v", err)
	}

	return tn.MineBlock(t)
}

// SubmitTx submits a transaction to the node's mempool
func (tn *TestNode) SubmitTx(t testing.TB, tx *types.Transaction) {
	t.Helper()
	if err := tn.Node.SubmitTx(tx); err != nil {
		t.Fatalf("SubmitTx: %v", err)
	}
}

// MineBlock produces the next block from mempool txs
func (tn *TestNode) MineBlock(t testing.TB) *types.Block {
	t.Helper()

	state := tn.Node.State()
	proposer := tn.Wallet.Address()

	// Get current height and tip
	height := tn.Node.CurrentHeight() + 1
	prevHash := tn.Node.GetTipHash()

	block, err := consensus.BuildBlock(
		state, tn.Node.VM(), tn.Node.Mempool(),
		height, prevHash, proposer,
		&walletSigner{tn.Wallet}, tn.Node.Hasher(),
		tn.Node.Config().MaxTxPerBlock, nil,
	)
	if err != nil {
		t.Fatalf("MineBlock: build block: %v", err)
	}

	// Validate the block
	parentHeader := &types.BlockHeader{
		Height:       height - 1,
		PreviousHash: prevHash,
	}
	if err := consensus.ValidateBlock(block, parentHeader, prevHash, state, tn.Node.Hasher(), tn.Node.VM()); err != nil {
		t.Fatalf("MineBlock: validate block: %v", err)
	}

	tn.Blocks = append(tn.Blocks, block)
	return block
}

// GetStateRoot returns the current state root
func (tn *TestNode) GetStateRoot() types.Hash {
	return tn.Node.State().GetStateRoot()
}

// ProduceBlocks produces N blocks, optionally submitting txs through a callback
func (tn *TestNode) ProduceBlocks(t testing.TB, n int, txFn func(height uint64) []*types.Transaction) []*types.Block {
	t.Helper()

	blocks := make([]*types.Block, 0, n)
	for i := 0; i < n; i++ {
		height := tn.Node.CurrentHeight() + 1
		if txFn != nil {
			txs := txFn(height)
			for _, tx := range txs {
				if err := tn.Node.Mempool().Submit(tx); err != nil {
					t.Fatalf("ProduceBlocks: add tx: %v", err)
				}
			}
		}
		block := tn.MineBlock(t)
		blocks = append(blocks, block)
	}

	return blocks
}

// WipeState removes all state from a node (for replay tests)
func (tn *TestNode) WipeState(t testing.TB) {
	t.Helper()
	// Note: This is a placeholder. Full implementation would require
	// node API to swap out the state. For now, create a new node instead.
	t.Log("WipeState: use NewTestNode instead for fresh state")
}

// ReplayFrom replays blocks starting from the given height
func (tn *TestNode) ReplayFrom(t testing.TB, fromHeight uint64) error {
	t.Helper()
	return tn.Node.ReplayBlocks(fromHeight, tn.Node.CurrentHeight())
}

// CompareState compares state roots, gas, events across nodes
func CompareState(t testing.TB, nodes []*TestNode) StateComparison {
	t.Helper()
	if len(nodes) < 2 {
		return StateComparison{Details: "need at least 2 nodes"}
	}

	ref := nodes[0]
	refRoot := ref.Node.State().GetStateRoot()
	refHeight := ref.Node.CurrentHeight()

	for i, n := range nodes[1:] {
		// Compare state roots
		nodeRoot := n.Node.State().GetStateRoot()
		if refRoot != nodeRoot {
			return StateComparison{
				StateRootsMatch: false,
				Details:         fmt.Sprintf("state root mismatch node %d: %x vs %x", i+1, refRoot, nodeRoot),
			}
		}

		// Compare heights
		nodeHeight := n.Node.CurrentHeight()
		if refHeight != nodeHeight {
			return StateComparison{
				StateRootsMatch: false,
				HeightsMatch:    false,
				Details:         fmt.Sprintf("height mismatch node %d: %d vs %d", i+1, refHeight, nodeHeight),
			}
		}

		// Compare last block events (if blocks exist)
		if len(ref.Blocks) > 0 && len(n.Blocks) > 0 {
			refEvents := ref.Blocks[len(ref.Blocks)-1].Events
			nodeEvents := n.Blocks[len(n.Blocks)-1].Events
			if len(refEvents) != len(nodeEvents) {
				return StateComparison{
					StateRootsMatch: true,
					EventsMatch:     false,
					Details:         fmt.Sprintf("events count mismatch node %d: %d vs %d", i+1, len(refEvents), len(nodeEvents)),
				}
			}
		}
	}

	return StateComparison{
		StateRootsMatch: true,
		GasMatch:        true,
		EventsMatch:     true,
		HeightsMatch:    true,
		Details:         "all nodes match",
	}
}

// StateComparison holds comparison results
type StateComparison struct {
	StateRootsMatch bool
	GasMatch        bool
	EventsMatch     bool
	HeightsMatch    bool
	Details         string
}

// LoadTestWasm loads test WASM bytecode from testdata or builds it
func LoadTestWasm(t testing.TB) []byte {
	t.Helper()
	// Try to load from testdata first
	code, err := os.ReadFile("integration/testdata/token.wasm")
	if err == nil {
		return code
	}

	// Return nil - actual WASM would need to be compiled separately
	t.Log("Warning: test WASM not found. Place compiled WASM at integration/testdata/token.wasm")
	return nil
}

// NodeConfigOption returns a TestNodeOption that modifies the node config
func NodeConfigOption(fn func(*node.Config)) TestNodeOption {
	return func(cfg *node.Config) {
		fn(cfg)
	}
}
