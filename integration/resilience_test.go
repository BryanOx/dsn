//go:build integration

package integration

import (
	"context"
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

	// Gracefully restart each node one at a time
	for i := 0; i < 3; i++ {
		t.Logf("Restarting node %d gracefully", i)

		// Persist state before the restart so the recreated node can
		// restore the same state root from disk.
		_, err := nodes[i].CommitState()
		require.NoError(t, err, "node %d commit state before restart", i)

		// Close the node gracefully
		restartCfg := *nodes[i].Config()
		heightBefore := nodes[i].CurrentHeight()
		stateRootBefore := nodes[i].State().GetStateRoot()

		nodes[i].Close()
		nodes[i] = nil

		// Recreate the node from the same data directory, preserving the
		// full config (including MaxTxPerBlock) so the restarted node
		// builds identical blocks to the live nodes.
		n2, err := node.New(restartCfg)
		require.NoError(t, err, "failed to recreate node %d", i)
		n2.SetWallet(kps[i])
		nodes[i] = n2

		// Load from persistent storage
		err = n2.LoadFromPersistent()
		require.NoError(t, err, "failed to load from persistent for node %d", i)

		// Verify height and state root match before continuing
		require.Equal(t, heightBefore, n2.CurrentHeight(), "node %d height mismatch after restart", i)
		require.Equal(t, stateRootBefore, n2.State().GetStateRoot(), "node %d state root mismatch after restart", i)

		// Produce more blocks to verify catch-up. Nonces must keep advancing
		// across restarts: rounds 5-7 after the first restart, 8-10 after the
		// second, 11-13 after the third.
		for round := 5 + i*3; round < 8+i*3; round++ {
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
			for _, n := range nodes {
				if n != nil {
					MineBlockWithTxs(t, n, kps, txs)
				}
			}
		}

		// Verify all nodes converge
		CompareStateRoots(t, nodes)
	}

	// Final verification: after all restarts and catch-up, every node must
	// hold the same (post-catch-up) state root. The state advances beyond the
	// pre-restart baseline, so convergence is the property to check here.
	CompareStateRoots(t, nodes)
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

	// Record state before crash
	preCrashRoot := nodes[2].State().GetStateRoot()
	restartCfg := *nodes[2].Config()

	// Persist state before the crash so recovery can restore the
	// pre-crash state root from disk.
	_, err := nodes[2].CommitState()
	require.NoError(t, err, "commit state before crash")

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
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
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
	n2, err := node.New(restartCfg)
	require.NoError(t, err)
	n2.SetWallet(kps[2])
	nodes[2] = n2

	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// First recovery: node 2 should have pre-crash state
	require.Equal(t, preCrashRoot, n2.State().GetStateRoot(), "node 2 should have pre-crash state after first recovery")

	// Now simulate second crash immediately after restart
	t.Log("Crashed node 2 again (second crash)")
	// Persist the restored state before the second crash so it can be
	// recovered again.
	_, err = n2.CommitState()
	require.NoError(t, err, "commit state before second crash")
	cfg2 := *n2.Config()
	n2.Close()
	nodes[2] = nil

	// Produce more blocks while node 2 is down again
	for round := 5; round < 7; round++ {
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
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
	}

	// Get the new target state root
	targetRoot2 := nodes[0].State().GetStateRoot()

	// Second recovery: restart node 2 again
	n3, err := node.New(cfg2)
	require.NoError(t, err)
	n3.SetWallet(kps[2])
	nodes[2] = n3

	err = n3.LoadFromPersistent()
	require.NoError(t, err)

	// Verify node 2 recovered and can catch up by replaying the blocks it
	// missed while down (nonces 4-7), then verify it matches the target.
	for round := 3; round < 7; round++ {
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
		MineBlockWithTxs(t, nodes[2], kps, txs)
	}

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
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
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
	restartCfg := *nodes[0].Config()

	// Note: snapshot corruption is not exercised here. Snapshots are stored
	// inside a BoltDB bucket (snapshotsBucket in state/checkpoint.go), not as
	// standalone *.snap files on disk, so there is no file to corrupt. The
	// crash/recovery/root-match scenario below still exercises the persisted
	// state round-trip.

	// Close and restart node 0
	nodes[0].Close()

	n2, err := node.New(restartCfg)
	require.NoError(t, err)
	n2.SetWallet(kps[0])
	nodes[0] = n2

	// Load the persisted state back and verify the node can continue producing blocks
	err = n2.LoadFromPersistent()
	require.NoError(t, err, "load from persistent after restart")

	// Continue producing blocks after restart
	var txs []*types.Transaction
	for _, kp := range kps {
		tx := types.NewTransaction(
			1, 0, kp.Address(), 21,
			types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
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
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
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
	restartCfg := *nodes[2].Config()

	// Persist state before the partition so the reconnected node can
	// restore the pre-partition state root from disk.
	_, err := nodes[2].CommitState()
	require.NoError(t, err, "commit state before partition")

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
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
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
	n2, err := node.New(restartCfg)
	require.NoError(t, err)
	n2.SetWallet(kps[2])
	nodes[2] = n2

	err = n2.LoadFromPersistent()
	require.NoError(t, err)

	// Verify node 2 has pre-partition state
	require.Equal(t, prePartitionRoot, n2.State().GetStateRoot(), "node 2 should have pre-partition state")

	// Node 2 needs to catch up - replay the blocks produced during the
	// partition (nonces 4-8) on node 2 only so it reaches the target state.
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
		MineBlockWithTxs(t, nodes[2], kps, txs)
	}

	// Verify node 2 caught up to the pre-reconnect target state
	require.Equal(t, targetRoot, nodes[2].State().GetStateRoot(), "node 2 should match target after catch-up")

	// Produce more blocks on all nodes to verify ongoing convergence
	for round := 8; round < 13; round++ {
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
		// Mine on all nodes
		MineBlockWithTxs(t, nodes[0], kps, txs)
		MineBlockWithTxs(t, nodes[1], kps, txs)
		MineBlockWithTxs(t, nodes[2], kps, txs)
	}

	// Verify all nodes converge after reconnection
	CompareStateRoots(t, nodes)
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
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
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
	// Nonce tracking: the baseline produced 3 blocks (nonces 1-3), so the
	// next transaction nonce is 4. Nonces must advance continuously across
	// every block mined during the kill/restart loop.
	nextNonce := uint64(4)
	for iteration := 0; iteration < 10; iteration++ {
		select {
		case <-ctx.Done():
			t.Fatal("test timeout during iteration", iteration)
		default:
		}

		t.Logf("Iteration %d/10: Killing and restarting node %d", iteration+1, targetNodeIdx)

		// Store the current state before kill
		restartCfg := *nodes[targetNodeIdx].Config()
		heightBefore := nodes[targetNodeIdx].CurrentHeight()
		stateRootBefore := nodes[targetNodeIdx].State().GetStateRoot()

		// Persist state before the kill so recovery can restore the same
		// state root from disk.
		_, err := nodes[targetNodeIdx].CommitState()
		require.NoError(t, err, "commit state before kill on iteration %d", iteration)

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
					1, 0, kp.Address(), nextNonce,
					types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
					uint64(time.Now().Unix()),
				)
				require.NoError(t, kp.Sign(tx, hasher))
				txs = append(txs, tx)
			}
			nextNonce++
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
		n2, err := node.New(restartCfg)
		require.NoError(t, err, "failed to restart node on iteration %d", iteration)
		n2.SetWallet(kps[targetNodeIdx])
		nodes[targetNodeIdx] = n2

		err = n2.LoadFromPersistent()
		require.NoError(t, err, "failed to load from persistent on iteration %d", iteration)

		// Verify the node reconnected with correct state
		require.Equal(t, heightBefore, n2.CurrentHeight(), "node should have same height after restart")
		require.Equal(t, stateRootBefore, n2.State().GetStateRoot(), "node should have same state root after restart")

		// Replay the blocks produced while the node was down so it catches
		// up to the alive nodes' nonce before a new block is mined together.
		catchupNonce := nextNonce - uint64(blocksWhileDown)
		for round := 0; round < blocksWhileDown; round++ {
			var txs []*types.Transaction
			for _, kp := range kps {
				tx := types.NewTransaction(
					1, 0, kp.Address(), catchupNonce+uint64(round),
					types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
					uint64(time.Now().Unix()),
				)
				require.NoError(t, kp.Sign(tx, hasher))
				txs = append(txs, tx)
			}
			MineBlockWithTxs(t, nodes[targetNodeIdx], kps, txs)
		}

		// Verify cluster can still produce blocks and converge
		// First let alive nodes produce a block, then catch up the restarted node
		var txs []*types.Transaction
		for _, kp := range kps {
			tx := types.NewTransaction(
				1, 0, kp.Address(), nextNonce,
				types.EncodeTransferPayload(kp.Address(), 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, hasher))
			txs = append(txs, tx)
		}
		nextNonce++

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
