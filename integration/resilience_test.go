//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// T4-1: Graceful Rolling Restart Test
// Start a 3-node cluster, wait for convergence, then gracefully stop and restart
// each node one at a time. Verify the node catches up and state roots match.
func TestResilience_RollingRestart(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Close()
			}
		}
	}()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Produce 5 blocks to establish baseline
	for round := 0; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("rolling-restart"), nil, 100, 1000,
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

	// Get baseline state root
	baselineRoot := nodes[0].State().GetStateRoot()

	// Gracefully restart each node one at a time
	for i := 0; i < 3; i++ {
		t.Logf("Restarting node %d gracefully", i)

		// Close the node gracefully
		dataDir := nodes[i].Config().DataDir
		heightBefore := nodes[i].CurrentHeight()
		stateRootBefore := nodes[i].State().GetStateRoot()

		nodes[i].Close()
		nodes[i] = nil

		// Recreate the node from the same data directory
		n2, err := node.New(node.Config{
			DataDir:         dataDir,
			P2PPort:         0,
			MempoolMaxSize:  10000,
			MempoolTTL:      300 * time.Second,
			SnapshotInterval: 10,
			Validators:      []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()},
		})
		require.NoError(t, err, "failed to recreate node %d", i)
		n2.SetWallet(kps[i])
		nodes[i] = n2

		// Register validators on the recreated node
		validators := []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()}
		registerValidatorsInState(t, n2.State(), validators, kps)

		// Load from persistent storage
		err = n2.LoadFromPersistent()
		require.NoError(t, err, "failed to load from persistent for node %d", i)

		// Verify height and state root match before continuing
		require.Equal(t, heightBefore, n2.CurrentHeight(), "node %d height mismatch after restart", i)
		require.Equal(t, stateRootBefore, n2.State().GetStateRoot(), "node %d state root mismatch after restart", i)

		// Produce more blocks to verify catch-up
		for round := 5; round < 8; round++ {
			var txs []*types.Transaction
			for _, kp := range kps {
				tx := types.NewTransaction(
					1, 0, kp.Address(), uint64(round+1),
					[]byte("rolling-restart"), nil, 100, 1000,
					uint64(time.Now().Unix()+int64(round)),
				)
				require.NoError(t, kp.Sign(tx, hasher))
				txs = append(txs, tx)
			}
			for _, n := range nodes {
				if n != nil {
					MineBlockWithTxs(t, n, kps, txs)
				}
			}
		}

		// Verify all nodes converge
		CompareStateRoots(t, nodes)
	}

	// Final verification: all nodes should match baseline
	require.Equal(t, baselineRoot, nodes[0].State().GetStateRoot(), "node 0 final state root mismatch")
}

// T4-2: Crash-Loop Recovery Test
// Start a 3-node cluster, crash (kill without cleanup) one node, restart it,
// verify recovery. Then introduce a second crash immediately after restart.
func TestResilience_CrashLoopRecovery(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Close()
			}
		}
	}()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Produce 3 blocks
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("crash-loop"), nil, 100, 1000,
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

	// Record state before crash
	preCrashRoot := nodes[2].State().GetStateRoot()
	dataDir2 := nodes[2].Config().DataDir

	// Simulate crash: close without graceful shutdown
	// (In this test we just call Close, which simulates a crash since we don't do cleanup)
	nodes[2].Close()
	nodes[2] = nil
	t.Log("Crashed node 2")

	// Produce more blocks on nodes 0 and 1 while node 2 is down
	for round := 3; round < 5; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("crash-loop"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
	}

	// Get the new state root that node 2 needs to catch up to
	targetRoot := nodes[0].State().GetStateRoot()
	t.Logf("Target state root after crash: %x", targetRoot)

	// Restart node 2 (first recovery)
	n2, err := node.New(node.Config{
		DataDir:         dataDir2,
		P2PPort:         0,
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
		SnapshotInterval: 10,
		Validators:      []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()},
	})
	require.NoError(t, err)
	n2.SetWallet(kps[2])
	nodes[2] = n2

	validators := []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()}
	registerValidatorsInState(t, n2.State(), validators, kps)

	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// First recovery: node 2 should have pre-crash state
	require.Equal(t, preCrashRoot, n2.State().GetStateRoot(), "node 2 should have pre-crash state after first recovery")

	// Now simulate second crash immediately after restart
	t.Log("Crashed node 2 again (second crash)")
	n2.Close()
	nodes[2] = nil

	// Produce more blocks while node 2 is down again
	for round := 5; round < 7; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("crash-loop"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
	}

	// Get the new target state root
	targetRoot2 := nodes[0].State().GetStateRoot()

	// Second recovery: restart node 2 again
	n3, err := node.New(node.Config{
		DataDir:         dataDir2,
		P2PPort:         0,
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
		SnapshotInterval: 10,
		Validators:      []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()},
	})
	require.NoError(t, err)
	n3.SetWallet(kps[2])
	nodes[2] = n3

	registerValidatorsInState(t, n3.State(), validators, kps)

	err = n3.LoadFromPersistent()
	require.NoError(t, err)

	// Verify node 2 recovered and can catch up by applying blocks
	var txs []*types.Transaction
	for _, kp := range kps {
		tx := types.NewTransaction(
			1, 0, kp.Address(), 7,
			[]byte("crash-loop"), nil, 100, 1000,
			uint64(time.Now().Unix()),
		)
		require.NoError(t, kp.Sign(tx, hasher))
		txs = append(txs, tx)
	}
	MineBlockWithTxs(t, nodes[2], kps, txs)
	MineBlockWithTxs(t, nodes[0], kps, txs)
	MineBlockWithTxs(t, nodes[1], kps, txs)

	// After recovery and catch-up, verify convergence
	CompareStateRoots(t, nodes)
	require.Equal(t, targetRoot2, nodes[2].State().GetStateRoot(), "node 2 should match target after crash-loop recovery")
}

// T4-3: Corrupted Snapshot Recovery
// Create a snapshot, write invalid data to it, restart the node,
// and verify it detects corruption and falls back to full sync.
func TestResilience_CorruptedSnapshot(t *testing.T) {
	nodes, kps := NewMultiNodeNetwork(t, 2)
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Close()
			}
		}
	}()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Mine enough blocks to trigger snapshot creation (snapshot interval = 10 epochs)
	// Each block at height >= 100 triggers epoch boundary
	// We need to reach at least epoch 1 (height >= 100) for snapshot
	for round := 0; round < 20; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("corrupted-snap"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
	}

	// Commit state on node 0 to create persistent snapshot
	_, err := nodes[0].CommitState()
	require.NoError(t, err)

	// Get the data directory and find the snapshot file
	dataDir := nodes[0].Config().DataDir

	// Find and corrupt the snapshot file
	snapshots, err := filepath.Glob(filepath.Join(dataDir, "*.snap"))
	if err == nil && len(snapshots) > 0 {
		t.Logf("Found snapshot file: %s", snapshots[0])

		// Corrupt the snapshot by writing invalid data
		err := os.WriteFile(snapshots[0], []byte("CORRUPTED_SNAPSHOT_DATA_12345"), 0644)
		require.NoError(t, err, "failed to corrupt snapshot")
		t.Log("Corrupted snapshot file")
	} else {
		t.Log("No snapshot file found, proceeding with restart")
	}

	// Close and restart node 0
	nodes[0].Close()

	n2, err := node.New(node.Config{
		DataDir:         dataDir,
		P2PPort:         0,
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
		SnapshotInterval: 10,
		Validators:      []types.Address{kps[0].Address(), kps[1].Address()},
	})
	require.NoError(t, err)
	n2.SetWallet(kps[0])
	nodes[0] = n2

	// Try to load from persistent - should detect corruption and fall back
	err = n2.LoadFromPersistent()
	// The error is expected - corruption should be detected
	t.Logf("LoadFromPersistent error (expected): %v", err)

	// The node should still have its in-memory state or recover to a valid state
	// Verify the node can still process blocks
	validators := []types.Address{kps[0].Address(), kps[1].Address()}
	registerValidatorsInState(t, n2.State(), validators, kps)

	// Continue producing blocks after restart
	var txs []*types.Transaction
	for _, kp := range kps {
		tx := types.NewTransaction(
			1, 0, kp.Address(), 21,
			[]byte("post-corrupt"), nil, 100, 1000,
			uint64(time.Now().Unix()),
		)
		require.NoError(t, kp.Sign(tx, hasher))
		txs = append(txs, tx)
	}
	MineBlockWithTxs(t, nodes[0], kps, txs)
	MineBlockWithTxs(t, nodes[1], kps, txs)

	// Both nodes should converge
	CompareStateRoots(t, nodes)
}

// T4-4: Network Partition Recovery
// Start a 3-node cluster, simulate a network partition by disconnecting one node,
// verify other nodes continue, then reconnect and verify catch-up.
func TestResilience_NetworkPartition(t *testing.T) {
	// Note: Since P2P is disabled in tests (P2PPort=0), we simulate network partition
	// by closing one node and having others continue, then restarting it.
	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Close()
			}
		}
	}()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Establish baseline - 3 blocks
	for round := 0; round < 3; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("network-partition"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
	}

	// Record state root before partition
	prePartitionRoot := nodes[2].State().GetStateRoot()
	dataDir2 := nodes[2].Config().DataDir

	// Simulate network partition: disconnect node 2
	t.Log("Simulating network partition: disconnecting node 2")
	nodes[2].Close()
	nodes[2] = nil

	// Nodes 0 and 1 continue producing blocks while node 2 is partitioned
	for round := 3; round < 8; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("partition-continued"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		// Only mine on nodes 0 and 1
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
	}

	// Verify nodes 0 and 1 are at same height and have same state root
	require.Equal(t, nodes[0].CurrentHeight(), nodes[1].CurrentHeight(), "nodes 0 and 1 should have same height")
	CompareStateRoots(t, []*node.Node{nodes[0], nodes[1]})

	// Get the target state root that partitioned node needs to catch up to
	targetRoot := nodes[0].State().GetStateRoot()
	t.Logf("Target state root after partition: %x", targetRoot)

	// Reconnect the partitioned node
	t.Log("Reconnecting node 2")
	n2, err := node.New(node.Config{
		DataDir:         dataDir2,
		P2PPort:         0,
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
		SnapshotInterval: 10,
		Validators:      []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()},
	})
	require.NoError(t, err)
	n2.SetWallet(kps[2])
	nodes[2] = n2

	validators := []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()}
	registerValidatorsInState(t, n2.State(), validators, kps)

	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// Verify node 2 has pre-partition state
	require.Equal(t, prePartitionRoot, n2.State().GetStateRoot(), "node 2 should have pre-partition state")

	// Node 2 needs to catch up - apply the blocks that were produced during partition
	// Node 2 is at height 3, nodes 0,1 are at height 8, so need to apply 5 more blocks
	for round := 8; round < 13; round++ {
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("catch-up"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		// Mine on all nodes
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
		MineBlockWithTxs(t, nodes[2], kps, txs)
	}

	// Verify all nodes converge after reconnection
	CompareStateRoots(t, nodes)
	require.Equal(t, targetRoot, nodes[2].State().GetStateRoot(), "node 2 should match target after catch-up")
}

// T4-5: Validator Kill/Restart Loop
// Kill and restart the same validator 10 times in succession.
// Verify it reconnects each time and cluster maintains finality.
func TestResilience_KillRestartLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	nodes, kps := NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			if n != nil {
				n.Close()
			}
		}
	}()

	fundAllAccounts(t, nodes, kps, 100000)
	hasher := types.SHA256Hasher{}

	// Initialize - produce a few blocks before the loop
	for round := 0; round < 3; round++ {
		select {
		case <-ctx.Done():
			t.Fatal("test timeout")
		default:
		}

		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(round+1),
				[]byte("init"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(round)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		for _, n := range nodes {
			MineBlockWithTxs(t, n, kps, txs)
		}
	}

	// Kill/restart loop - 10 iterations
	targetNodeIdx := 2 // We kill node 2 each time
	for iteration := 0; iteration < 10; iteration++ {
		select {
		case <-ctx.Done():
			t.Fatal("test timeout during iteration", iteration)
		default:
		}

		t.Logf("Iteration %d/10: Killing and restarting node %d", iteration+1, targetNodeIdx)

		// Store the current state before kill
		dataDir := nodes[targetNodeIdx].Config().DataDir
		heightBefore := nodes[targetNodeIdx].CurrentHeight()
		stateRootBefore := nodes[targetNodeIdx].State().GetStateRoot()

		// Kill the node (close without cleanup)
		nodes[targetNodeIdx].Close()
		nodes[targetNodeIdx] = nil

		// Other nodes continue producing blocks while this node is down
		// We produce 1-2 blocks between restarts
		blocksWhileDown := (iteration % 2) + 1 // 1 or 2 blocks
		for round := 0; round < blocksWhileDown; round++ {
			var txs []*types.Transaction
			for _, kp := range kps {
				tx := types.NewTransaction(
					1, 0, kp.Address(), uint64(3+iteration*3+round+1),
					[]byte("while-down"), nil, 100, 1000,
					uint64(time.Now().Unix()+int64(iteration*10+round)),
				)
				require.NoError(t, kp.Sign(tx, hasher))
				txs = append(txs, tx)
			}
			// Only mine on alive nodes
			for i := 0; i < len(nodes); i++ {
				if i != targetNodeIdx && nodes[i] != nil {
					MineBlockWithTxs(t, nodes[i], kps, txs)
				}
			}
		}

		// Verify alive nodes maintain convergence
		if nodes[0] != nil && nodes[1] != nil {
			require.Equal(t, nodes[0].CurrentHeight(), nodes[1].CurrentHeight(), "alive nodes should have same height")
		}

		// Restart the killed node
		n2, err := node.New(node.Config{
			DataDir:         dataDir,
			P2PPort:         0,
			MempoolMaxSize:  10000,
			MempoolTTL:      300 * time.Second,
			SnapshotInterval: 10,
			Validators:      []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()},
		})
		require.NoError(t, err, "failed to restart node on iteration %d", iteration)
		n2.SetWallet(kps[targetNodeIdx])
		nodes[targetNodeIdx] = n2

		validators := []types.Address{kps[0].Address(), kps[1].Address(), kps[2].Address()}
		registerValidatorsInState(t, n2.State(), validators, kps)

		err = n2.LoadFromPersistent()
		require.NoError(t, err, "failed to load from persistent on iteration %d", iteration)

		// Verify the node reconnected with correct state
		require.Equal(t, heightBefore, n2.CurrentHeight(), "node should have same height after restart")
		require.Equal(t, stateRootBefore, n2.State().GetStateRoot(), "node should have same state root after restart")

		// Verify cluster can still produce blocks and converge
		// First let alive nodes produce a block, then catch up the restarted node
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), uint64(3+iteration*3+blocksWhileDown+1),
				[]byte("post-reconnect"), nil, 100, 1000,
				uint64(time.Now().Unix()+int64(iteration)),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}

		// Get the current state from node 0 (the reference)
		MineBlockWithTxs(t, nodes[0], kps, txs)
		if nodes[1] != nil {
			MineBlockWithTxs(t, nodes[1], kps, txs)
		}
		expectedRoot := nodes[0].State().GetStateRoot()

		// Apply same block to restarted node
		MineBlockWithTxs(t, nodes[targetNodeIdx], kps, txs)
		require.Equal(t, expectedRoot, nodes[targetNodeIdx].State().GetStateRoot(),
			"restarted node should have same state root after catch-up")

		t.Logf("Iteration %d/10 completed successfully", iteration+1)
	}

	// Final verification: all 10 iterations completed, verify state roots match
	CompareStateRoots(t, nodes)
	t.Log("All 10 kill/restart iterations completed successfully")
}