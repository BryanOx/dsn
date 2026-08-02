//go:build integration

package integration

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// createCheckpointAndSnapshot retrieves the latest checkpoint and snapshot data for a node.
// Since n.persistent is private, we use the DataDir to create a separate PersistentState
// instance to access checkpoint and snapshot data.
func createCheckpointAndSnapshot(t *testing.T, n *node.Node, height uint64) (*state.Checkpoint, []byte) {
	t.Helper()

	dataDir := n.Config().DataDir
	dbPath := filepath.Join(dataDir, "dsn.db")
	ps, err := state.NewPersistentState(dbPath, n.Hasher(), false)
	require.NoError(t, err)
	defer ps.Close()

	cp, err := state.LatestCheckpoint(ps)
	require.NoError(t, err)
	require.Equal(t, height, cp.Height)

	snapData, err := state.LoadSnapshot(ps, height)
	require.NoError(t, err)
	require.NotNil(t, snapData)

	return cp, snapData
}

// generateHeavyLoad creates a batch of transfer transactions for stress testing.
// Each validator sends transactions in round-robin fashion.
func generateHeavyLoad(t *testing.T, kps []*wallet.KeyPair, hasher types.Hasher, round int, txCount int) []*types.Transaction {
	t.Helper()

	txs := make([]*types.Transaction, 0, txCount)

	// Each sender sends txCount/len(kps) transactions per round
	// The base nonce for each round is round * (txCount/len(kps)) + 1
	txsPerSender := txCount / len(kps)
	baseNonce := uint64(round * txsPerSender)

	for i := 0; i < txCount; i++ {
		senderIdx := i % len(kps)
		kp := kps[senderIdx]

		// Nonce continues from previous rounds
		nonce := baseNonce + uint64((i/len(kps))+1)

		// Sender must be the funded validator (kp.Address()), not a random recipient
		tx := types.NewTransaction(
			1,            // version
			0,            // chainID
			kp.Address(), // sender = funded validator
			nonce,        // nonce - sequential per sender
			types.EncodeTransferPayload(kp.Address(), 0),
			nil,  // constraints
			100,  // maxFee
			1000, // gasLimit
			uint64(time.Now().Unix()+int64(round)),
		)
		require.NoError(t, kp.Sign(tx, hasher))
		txs = append(txs, tx)
	}

	return txs
}

// fundAllAccounts funds all accounts on all nodes with the specified amount.
func fundAllAccounts(t *testing.T, nodes []*node.Node, kps []*wallet.KeyPair, amount uint64) {
	t.Helper()

	for _, n := range nodes {
		for _, kp := range kps {
			FundAccount(t, n, kp.Address(), kp.PublicKey, amount)
		}
	}
}

// restartNode closes a node and recreates it with the same config, then loads from persistent.
func restartNode(t *testing.T, n *node.Node) *node.Node {
	t.Helper()

	dataDir := n.Config().DataDir
	n.Close()

	n2, err := node.New(node.Config{
		DataDir:        dataDir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)

	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	return n2
}

// TestConvergence_ConsensusLoop is the end-to-end two-phase voting regression.
// Three nodes run the REAL consensus loop: the proposer broadcasts a proof-less
// proposal, every validator prevotes/precommits over P2P, the proposer attaches
// the 2/3 commit proof and broadcasts the final block, and all nodes apply it.
// Convergence on identical state roots proves the receivers only accepted
// proof-carrying blocks — ValidateBlock rejects nil commit proofs, so a block
// without one would never be applied and the nodes would stall at height 0.
func TestConvergence_ConsensusLoop(t *testing.T) {
	nodes, kps := NewConsensusLoopNetwork(t, 3)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 100000)

	// Submit transfers to every node's mempool — whichever node is proposer
	// for height 1 includes them, and every node cleans its mempool on apply.
	hasher := types.SHA256Hasher{}
	var txs []*types.Transaction
	for _, kp := range kps {
		tx := types.NewTransaction(1, 0, kp.Address(), 1,
			types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
			uint64(time.Now().Unix()))
		require.NoError(t, kp.Sign(tx, hasher))
		txs = append(txs, tx)
	}
	for _, n := range nodes {
		for _, tx := range txs {
			require.NoError(t, n.Mempool().Submit(tx))
		}
	}

	for _, n := range nodes {
		n.StartConsensus()
	}
	defer func() {
		for _, n := range nodes {
			n.StopConsensus()
		}
	}()

	const wantHeight = uint64(5)
	require.Eventually(t, func() bool {
		for _, n := range nodes {
			if n.CurrentHeight() < wantHeight {
				return false
			}
		}
		return true
	}, 30*time.Second, 100*time.Millisecond)

	// All nodes applied the same proof-carrying blocks.
	CompareStateRoots(t, nodes)

	// Every block applied by a non-proposer carries a valid commit proof. The
	// proposer stores headers only, so skip the proposer for each height.
	activeVals, err := staking.GetActiveValidators(nodes[0].State())
	require.NoError(t, err)
	for height := uint64(1); height <= wantHeight; height++ {
		proposer := consensus.WeightedProposerAtHeight(height, activeVals)
		for i, n := range nodes {
			if types.DeriveConsensusID(kps[i].PublicKey) == proposer {
				continue
			}
			block, err := n.GetBlock(height)
			require.NoError(t, err, "node %d must have applied height %d", i, height)
			require.NotNil(t, block.CommitProof, "block %d must carry a commit proof", height)
			require.Equal(t, height, block.CommitProof.Height)
			headerHash, err := block.HeaderHash(n.Hasher())
			require.NoError(t, err)
			require.Equal(t, headerHash, block.CommitProof.BlockHash,
				"block %d commit proof must reference the block hash", height)
		}
	}
}

// TestConvergence_3Node tests basic state convergence with 3 nodes over 5 rounds.
func TestConvergence_3Node(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 100000)

	// 5 rounds of basic transactions
	hasher := types.SHA256Hasher{}
	for round := 0; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}
}

// TestConvergence_5Node tests state convergence with 5 nodes over 3 rounds.
func TestConvergence_5Node(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 5)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 100000)

	hasher := types.SHA256Hasher{}
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}
}

// TestConvergence_ValidatorChurn tests state convergence when validators join/leave.
func TestConvergence_ValidatorChurn(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 200000)
	hasher := types.SHA256Hasher{}

	// Round 1-3: normal blocks
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}

	// Create new validator
	newKP, err := wallet.GenerateKey()
	require.NoError(t, err)
	newAddr := newKP.Address()

	// Register on ALL nodes
	for _, n := range nodes {
		s := n.State()
		acc := state.NewAccount(newAddr, newKP.PublicKey)
		acc.AddBalance(types.NewAmount(200000))
		require.NoError(t, s.SetAccount(newAddr, acc))

		_, err := staking.RegisterValidator(s, newKP.PublicKey, newAddr,
			types.NewAmount(100000), 0, 0)
		require.NoError(t, err)
	}

	// Round 4-5: post-churn blocks with new validator
	updatedKPs := append(kps, newKP)
	for round := 3; round < 5; round++ {
		var txs []*types.Transaction
		for i, kp := range updatedKPs {
			var nonce uint64
			if i < len(kps) {
				// Existing validators: nonce continues from where it left off (1,2,3 -> 4,5)
				nonce = uint64(round + 1)
			} else {
				// New validator: starts at nonce 1 in first post-churn round
				nonce = uint64(round - 2) // round 3 -> nonce 1, round 4 -> nonce 2
			}
			tx := types.NewTransaction(
				1, 0, kp.Address(), nonce,
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}
}

// TestConvergence_RestartMidEpoch tests state convergence after node restart mid-epoch.
func TestConvergence_RestartMidEpoch(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer nodes[0].Close()
	defer nodes[1].Close()
	defer nodes[2].Close()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// 3 rounds on all nodes
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}

	// Commit and restart node 0
	_, err := nodes[0].CommitState()
	require.NoError(t, err)
	preRoot := nodes[0].State().GetStateRoot()
	dataDir := nodes[0].Config().DataDir
	nodes[0].Close()

	n2, err := node.New(node.Config{
		DataDir:        dataDir,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()
	err = n2.LoadFromPersistent()
	require.NoError(t, err)
	nodes[0] = n2

	// Verify state root matches
	require.Equal(t, preRoot, n2.State().GetStateRoot())

	// 2 more rounds - use correct nonce continuing from where we left off
	// After restart, node 0 has currentHeight=0 while nodes 1,2 are at height 3.
	// We need to mine on nodes 1,2 first, then manually build blocks for node 0.
	for round := 3; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		// Mine on nodes 1,2 first - they are at correct height
		MineBlockWithTxs(t, nodes[1], kps, txs)
		MineBlockWithTxs(t, nodes[2], kps, txs)
		expectedRoot := nodes[1].State().GetStateRoot()

		// Now manually build block for node 0 at the correct height
		height := uint64(round + 1) // heights 4,5
		prevHash := nodes[1].GetTipHash()

		// Get proposer for this height
		activeVals, _ := staking.GetActiveValidators(nodes[1].State())
		proposer := consensus.WeightedProposerAtHeight(height, activeVals)

		// Find the key pair for this proposer
		var proposerKP *wallet.KeyPair
		for _, kp := range kps {
			if types.DeriveConsensusID(kp.PublicKey) == proposer {
				proposerKP = kp
				break
			}
		}
		require.NotNil(t, proposerKP, "proposer keypair not found for %v", proposer)

		block, err := consensus.BuildBlock(
			nodes[0].State(), nodes[0].VM(),
			&testMempool{txs: txs},
			height, prevHash, proposer,
			&consensusSigner{kp: proposerKP},
			nodes[0].Hasher(), 100, nil, 1,
		)
		require.NoError(t, err)

		// Apply block to node 0's state
		require.Equal(t, expectedRoot, block.Header.StateRoot,
			"state root mismatch at round %d", round)
	}

	// Now all 3 should converge
	CompareStateRoots(t, nodes)
}

// TestConvergence_CatchUp tests that a node can catch up to the state of another node.
func TestConvergence_CatchUp(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Node 0 mines 5 blocks, recording state
	var recordedRoots []types.Hash
	for round := 0; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		MineBlockWithTxs(t, nodes[0], kps, txs)
		recordedRoots = append(recordedRoots, nodes[0].State().GetStateRoot())
	}

	// Now apply same blocks to node 1
	for round := 0; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		MineBlockWithTxs(t, nodes[1], kps, txs)
		require.Equal(t, recordedRoots[round], nodes[1].State().GetStateRoot(),
			"state root mismatch at block %d", round+1)
	}
}

// TestConvergence_FastSync tests fast sync from checkpoint functionality.
func TestConvergence_FastSync(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	defer nodes[0].Close()
	defer nodes[1].Close()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Node 0 mines 10 blocks
	for round := 0; round < 10; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		MineBlockWithTxs(t, nodes[0], kps, txs)
	}

	// Build checkpoint and snapshot from node 0's in-memory state
	// (instead of opening a new PersistentState which causes locking issues)
	stateRoot := nodes[0].State().GetStateRoot()

	// Get validator set hash from staking snapshot
	stakingSnap, err := staking.CreateSnapshot(nodes[0].State(), 1)
	require.NoError(t, err)
	require.NotNil(t, stakingSnap)
	valSetHash := stakingSnap.SetHash

	// Create snapshot from in-memory state
	snap, err := nodes[0].State().CreateSnapshot(10, 1, valSetHash, uint64(time.Now().Unix()))
	require.NoError(t, err)

	// Serialize snapshot
	snapData, err := state.SerializeSnapshot(snap)
	require.NoError(t, err)

	// Build checkpoint
	cp := &state.Checkpoint{
		Height:           10,
		BlockHash:        nodes[0].GetTipHash(),
		StateRoot:        stateRoot,
		SnapshotHash:     state.SnapshotHash(snapData),
		ValidatorSetHash: valSetHash,
		Epoch:            1,
		Timestamp:        uint64(time.Now().Unix()),
	}

	// Create a new node and do fast sync from checkpoint
	dataDir := nodes[1].Config().DataDir
	nodes[1].Close()

	n2, err := node.New(node.Config{
		DataDir:          dataDir,
		P2PPort:          0,
		MempoolMaxSize:   10000,
		MempoolTTL:       300 * time.Second,
		SnapshotInterval: 10,
		Validators:       []types.Address{kps[0].Address(), kps[1].Address()},
	})
	require.NoError(t, err)
	defer n2.Close()
	n2.SetWallet(kps[1])

	// Register validators on new node
	registerValidatorsInState(t, n2.State(), []types.Address{kps[0].Address(), kps[1].Address()}, kps)

	// Fast sync from checkpoint at height 10, targeting height 10 (no replay needed)
	err = n2.FastSyncFromCheckpoint(cp, snapData, 10)
	require.NoError(t, err)

	// Verify state root matches
	require.Equal(t, nodes[0].State().GetStateRoot(), n2.State().GetStateRoot())
}

// TestConvergence_HeavyLoad tests state convergence under heavy transaction load.
func TestConvergence_HeavyLoad(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	for _, n := range nodes {
		defer n.Close()
	}

	fundAllAccounts(t, nodes, kps, 10000000)
	hasher := types.SHA256Hasher{}

	for round := 0; round < 3; round++ {
		txs := generateHeavyLoad(t, kps, hasher, round, 20)
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}
}

// TestConvergence_ReplayAfterReconnect tests state convergence after node disconnect and reconnect.
func TestConvergence_ReplayAfterReconnect(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer nodes[0].Close()
	defer nodes[1].Close()
	defer nodes[2].Close()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// 3 rounds together
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			// Nonce must be round+1 since each round consumes nonce from previous
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
		CompareStateRoots(t, nodes)
	}

	// Save state root before disconnect
	preDisconnectRoot := nodes[2].State().GetStateRoot()
	dataDir2 := nodes[2].Config().DataDir

	// Commit state and close node 2
	_, err := nodes[2].CommitState()
	require.NoError(t, err)
	nodes[2].Close()

	// Restart node 2 with same DataDir
	n2, err := node.New(node.Config{
		DataDir:        dataDir2,
		P2PPort:        0,
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	})
	require.NoError(t, err)
	defer n2.Close()
	nodes[2] = n2
	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// Verify it matches pre-disconnect state
	require.Equal(t, preDisconnectRoot, n2.State().GetStateRoot())

	// Apply same 5 blocks to node 2 - first mine on nodes 0,1 to get the blocks,
	// then manually build equivalent blocks for node 2 at the correct heights
	for round := 3; round < 8; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		// Mine on nodes 0,1 first - they are at correct height
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
		expectedRoot := nodes[0].State().GetStateRoot()

		// Now manually build block for node 2 at the correct height
		height := uint64(round + 1) // heights 4,5,6,7,8
		prevHash := nodes[0].GetTipHash()

		// Get proposer for this height
		activeVals, _ := staking.GetActiveValidators(nodes[0].State())
		proposer := consensus.WeightedProposerAtHeight(height, activeVals)

		// Find the key pair for this proposer
		var proposerKP *wallet.KeyPair
		for _, kp := range kps {
			if types.DeriveConsensusID(kp.PublicKey) == proposer {
				proposerKP = kp
				break
			}
		}
		require.NotNil(t, proposerKP, "proposer keypair not found for %v", proposer)

		block, err := consensus.BuildBlock(
			nodes[2].State(), nodes[2].VM(),
			&testMempool{txs: txs},
			height, prevHash, proposer,
			&consensusSigner{kp: proposerKP},
			nodes[2].Hasher(), 100, nil, 1,
		)
		require.NoError(t, err)

		// Apply block to node 2's state
		require.Equal(t, expectedRoot, block.Header.StateRoot,
			"state root mismatch at round %d", round)
	}

	// Now all 3 should converge
	CompareStateRoots(t, nodes)
}
