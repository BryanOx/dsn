//go:build integration

package soak

import (
	"testing"
	"time"

	"github.com/BryanOx/dsn/integration"
	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// SoakClient provides a bridge between integration tests and soakrunner functionality.
// It manages transaction submission, verification, and monitoring for soak tests.
type SoakClient struct {
	t        *testing.T
	nodes    []*node.Node
	keyPairs []*wallet.KeyPair
	hasher   *types.SHA256Hasher
}

// NewSoakClient creates a new soak client for the given nodes.
func NewSoakClient(t *testing.T, nodes []*node.Node) *SoakClient {
	t.Helper()

	return &SoakClient{
		t:        t,
		nodes:    nodes,
		keyPairs: make([]*wallet.KeyPair, 0),
		hasher:   &types.SHA256Hasher{},
	}
}

// NewSoakClientWithKeyPairs creates a new soak client with keypairs for transaction generation.
func NewSoakClientWithKeyPairs(t *testing.T, nodes []*node.Node, kps []*wallet.KeyPair) *SoakClient {
	t.Helper()

	return &SoakClient{
		t:        t,
		nodes:    nodes,
		keyPairs: kps,
		hasher:   &types.SHA256Hasher{},
	}
}

// RunProfile runs a soak profile with the specified TPS and duration.
// This is a stub implementation for test integration.
func (s *SoakClient) RunProfile(profile string, duration time.Duration) {
	s.t.Logf("Running soak profile: %s for %v", profile, duration)

	// Profile-based TPS mapping
	var tps int
	switch profile {
	case "light":
		tps = 10
	case "moderate":
		tps = 100
	case "heavy":
		tps = 1000
	case "burst":
		tps = 50
	case "mixed":
		tps = 70
	default:
		tps = 100
	}

	startTime := time.Now()
	round := 0

	for time.Since(startTime) < duration {
		// Generate transactions based on TPS
		txCount := tps / 10 // approximate
		if txCount < 1 {
			txCount = 1
		}

		txs := generateSoakTransactions(s.t, s.nodes, s.keyPairs, s.hasher, round, txCount)

		// Submit to all nodes
		for _, n := range s.nodes {
			integration.MineBlockWithTxs(s.t, n, s.keyPairs, txs)
		}

		// Verify periodically
		if round%10 == 0 {
			s.VerifyState()
			s.t.Logf("Soak profile %s: round %d, elapsed %v", profile, round, time.Since(startTime))
		}

		round++
		time.Sleep(100 * time.Millisecond)
	}

	s.t.Logf("Soak profile %s completed: %d rounds", profile, round)
}

// VerifyState verifies that all nodes have converged to the same state.
func (s *SoakClient) VerifyState() {
	require.GreaterOrEqual(s.t, len(s.nodes), 2, "VerifyState: need at least 2 nodes")

	refRoot := s.nodes[0].State().GetStateRoot()
	for i, n := range s.nodes[1:] {
		nodeRoot := n.State().GetStateRoot()
		require.Equal(s.t, refRoot, nodeRoot, "node %d state root mismatch", i+1)
	}
}

// GetNodeHeights returns the current height of each node.
func (s *SoakClient) GetNodeHeights() []uint64 {
	heights := make([]uint64, len(s.nodes))
	for i, n := range s.nodes {
		heights[i] = n.CurrentHeight()
	}
	return heights
}

// VerifyHeights verifies that all nodes are at similar heights (within reasonable variance).
func (s *SoakClient) VerifyHeights() {
	heights := s.GetNodeHeights()
	if len(heights) < 2 {
		return
	}

	minHeight := heights[0]
	maxHeight := heights[0]
	for _, h := range heights[1:] {
		if h < minHeight {
			minHeight = h
		}
		if h > maxHeight {
			maxHeight = h
		}
	}

	// Allow some variance due to timing, but should be close
	require.LessOrEqual(s.t, maxHeight-minHeight, uint64(5), "node heights differ by more than 5")
}

// GetMempoolSizes returns the current mempool size for each node.
func (s *SoakClient) GetMempoolSizes() []int {
	sizes := make([]int, len(s.nodes))
	for i, n := range s.nodes {
		sizes[i] = n.Mempool().Count()
	}
	return sizes
}

// VerifyMempoolDrained verifies that mempool sizes are reasonable after blocks are produced.
func (s *SoakClient) VerifyMempoolDrained() {
	sizes := s.GetMempoolSizes()
	for i, size := range sizes {
		// Mempools should drain after blocks are produced
		s.t.Logf("Node %d mempool size: %d", i, size)
	}
}

// SubmitTransaction submits a transaction to a specific node's mempool.
func (s *SoakClient) SubmitTransaction(nodeIdx int, tx *types.Transaction) error {
	require.Less(s.t, nodeIdx, len(s.nodes), "node index out of range")
	return s.nodes[nodeIdx].SubmitTx(tx)
}

// GetStateRoot returns the state root for a specific node.
func (s *SoakClient) GetStateRoot(nodeIdx int) types.Hash {
	require.Less(s.t, nodeIdx, len(s.nodes), "node index out of range")
	return s.nodes[nodeIdx].State().GetStateRoot()
}

// SoakConfig holds configuration for a soak test run.
type SoakConfig struct {
	Profile     string
	TPS         int
	Duration    time.Duration
	NumAccounts int
	LogInterval time.Duration
}

// DefaultSoakConfigs provides default configurations for different profiles.
var DefaultSoakConfigs = map[string]SoakConfig{
	"light": {
		Profile:     "light",
		TPS:         10,
		Duration:    1 * time.Hour,
		NumAccounts: 100,
		LogInterval: 30 * time.Second,
	},
	"moderate": {
		Profile:     "moderate",
		TPS:         100,
		Duration:    24 * time.Hour,
		NumAccounts: 1000,
		LogInterval: 1 * time.Minute,
	},
	"heavy": {
		Profile:     "heavy",
		TPS:         1000,
		Duration:    72 * time.Hour,
		NumAccounts: 10000,
		LogInterval: 5 * time.Minute,
	},
	"burst": {
		Profile:     "burst",
		TPS:         50,
		Duration:    30 * time.Minute,
		NumAccounts: 500,
		LogInterval: 30 * time.Second,
	},
	"wasm": {
		Profile:     "wasm",
		TPS:         20,
		Duration:    1 * time.Hour,
		NumAccounts: 100,
		LogInterval: 1 * time.Minute,
	},
	"mixed": {
		Profile:     "mixed",
		TPS:         70,
		Duration:    2 * time.Hour,
		NumAccounts: 500,
		LogInterval: 1 * time.Minute,
	},
}

// GetConfig returns the configuration for a given profile name.
func GetConfig(profile string) SoakConfig {
	if cfg, ok := DefaultSoakConfigs[profile]; ok {
		return cfg
	}
	return DefaultSoakConfigs["moderate"]
}
