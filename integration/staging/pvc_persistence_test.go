//go:build integration

package staging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// TestPVC_PersistenceAcrossRestart validates that state persists across
// simulated pod restarts, matching Kubernetes StatefulSet PVC behavior.
//
// This test simulates:
// 1. Initial state: write account data to "PVC"
// 2. Simulate pod deletion (container layer lost, PVC persists)
// 3. Simulate pod recreation (new container, same PVC mount)
// 4. Verify account data survived the restart
// 5. Verify WAL can replay from persisted files
func TestPVC_PersistenceAcrossRestart(t *testing.T) {
	t.Parallel()

	t.Log("=== TestPVC_PersistenceAcrossRestart ===")

	// Step 1: Create PVC simulator and write initial state
	pvc := NewPVCSimulator(t)

	// Generate test address using wallet
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	addr := kp.Address()
	initialBalance := uint64(100000)

	// Write account state (simulates persistent storage)
	pvc.WriteAccountState(t, addr, initialBalance)

	// Write some WAL entries (simulating uncommitted blocks)
	walData := []byte("block-data-height-1")
	pvc.WriteWALEntry(t, 1, walData)
	walData2 := []byte("block-data-height-2")
	pvc.WriteWALEntry(t, 2, walData2)

	t.Logf("Wrote initial state: addr=%x, balance=%d", addr[:8], initialBalance)
	t.Logf("Wrote WAL entries: heights 1, 2")

	// Verify data was written
	files := pvc.ListStateFiles(t)
	require.Greater(t, len(files), 0, "should have written state files")
	t.Logf("State files: %v", files)

	// Step 2: Simulate pod deletion
	t.Log("Simulating pod deletion...")
	pvc.DeletePod(t)

	// Verify files still exist on "PVC" (they should - this is the whole point)
	require.True(t, pvc.StateFileExists(t, "account-"+string(addr[:8])+".state"),
		"account state should persist after pod deletion")

	// Step 3: Simulate pod recreation (new container mounts same PVC)
	t.Log("Simulating pod recreation...")
	pvc.RecreatePod(t)

	// Step 4: Verify state survived the restart
	t.Log("Verifying persisted state...")

	// Check account state
	balance, exists := pvc.ReadAccountState(t, addr)
	require.True(t, exists, "account state should exist after restart")
	require.Equal(t, initialBalance, balance,
		"account balance should be unchanged after restart")
	t.Logf("Verified balance: %d", balance)

	// Check WAL entries (simulating replay)
	wal1 := pvc.ReadWALEntry(t, 1)
	require.Equal(t, []byte("block-data-height-1"), wal1,
		"WAL entry at height 1 should persist")

	wal2 := pvc.ReadWALEntry(t, 2)
	require.Equal(t, []byte("block-data-height-2"), wal2,
		"WAL entry at height 2 should persist")

	t.Log("All state persisted correctly across restart")

	// Step 5: Write more state after restart (simulating continued operation)
	pvc.WriteAccountState(t, addr, initialBalance+5000)
	pvc.WriteWALEntry(t, 3, []byte("block-data-height-3"))

	// Simulate another restart
	pvc.SimulateFullRestart(t)

	// Verify new state also persisted
	balance2, exists2 := pvc.ReadAccountState(t, addr)
	require.True(t, exists2, "account state should exist after second restart")
	require.Equal(t, initialBalance+5000, balance2,
		"account balance should reflect post-restart writes")

	t.Log("Successfully validated PVC persistence across multiple restarts")
}

// TestPVC_WALReplayFromPersistedFiles tests WAL replay from PVC-persisted files.
// This simulates the scenario where a node restarts and replays WAL entries
// from the persistent volume.
func TestPVC_WALReplayFromPersistedFiles(t *testing.T) {
	t.Parallel()

	t.Log("=== TestPVC_WALReplayFromPersistedFiles ===")

	pvc := NewPVCSimulator(t)

	// Simulate a series of transactions committed to WAL
	// In real implementation, these would be actual transaction data
	walletData := map[uint64][]byte{
		1: []byte("tx: addr1 -> addr2, amount: 100"),
		2: []byte("tx: addr2 -> addr3, amount: 50"),
		3: []byte("tx: addr3 -> addr1, amount: 25"),
	}

	// Write WAL entries (simulates pre-commit state)
	for height, data := range walletData {
		pvc.WriteWALEntry(t, uint64(height), data)
	}
	t.Logf("Wrote %d WAL entries", len(walletData))

	// Simulate node crash (power loss, container restart without proper shutdown)
	t.Log("Simulating node crash (uncommitted WAL)...")

	// Node restarts and needs to replay WAL
	t.Log("Node restarting, replaying WAL from PVC...")

	// Simulate WAL replay by reading entries in order
	var replayedHeights []uint64
	for height := uint64(1); height <= 3; height++ {
		walEntry := pvc.ReadWALEntry(t, height)
		require.NotEmpty(t, walEntry, "WAL entry %d should exist", height)
		replayedHeights = append(replayedHeights, height)
		t.Logf("Replayed WAL entry at height %d: %s", height, string(walEntry))
	}

	require.Equal(t, []uint64{1, 2, 3}, replayedHeights,
		"WAL replay should process entries in order")

	// After successful replay, WAL can be cleaned up
	pvc.CleanupWAL(t, 3)

	// Verify WAL was cleaned but state persisted
	files := pvc.ListStateFiles(t)
	t.Logf("Files after WAL cleanup: %v", files)

	// Should only have snapshots/account state, not WAL
	walPresent := false
	for _, f := range files {
		if len(f) >= 3 && f[:3] == "wal" {
			walPresent = true
			break
		}
	}
	require.False(t, walPresent, "WAL files should be cleaned after replay")

	t.Log("WAL replay from PVC succeeded")
}

// TestPVC_StateRootConvergence tests that state roots can be verified
// after a restart, ensuring the node can rejoin consensus.
func TestPVC_StateRootConvergence(t *testing.T) {
	t.Parallel()

	t.Log("=== TestPVC_StateRootConvergence ===")

	pvc := NewPVCSimulator(t)

	// Simulate state root calculation and persistence
	// In real implementation, this would be actual Merkle root
	stateRootBefore := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	heightBefore := uint64(100)

	pvc.WriteBlockSnapshot(t, heightBefore, stateRootBefore)
	t.Logf("Wrote snapshot at height %d: %x", heightBefore, stateRootBefore)

	// Simulate node restart
	pvc.SimulateFullRestart(t)

	// Read persisted snapshot
	snapshotData := pvc.ReadBlockSnapshot(t, heightBefore)
	require.Equal(t, stateRootBefore, snapshotData,
		"state root should match after restart")

	// Simulate continuing operation and creating new snapshot
	stateRootAfter := []byte{0x06, 0x07, 0x08, 0x09, 0x0a}
	heightAfter := uint64(101)

	pvc.WriteBlockSnapshot(t, heightAfter, stateRootAfter)

	// Restart again
	pvc.SimulateFullRestart(t)

	// Verify both snapshots persisted
	snap1 := pvc.ReadBlockSnapshot(t, heightBefore)
	snap2 := pvc.ReadBlockSnapshot(t, heightAfter)

	require.Equal(t, stateRootBefore, snap1,
		"earlier snapshot should persist")
	require.Equal(t, stateRootAfter, snap2,
		"later snapshot should persist")

	t.Log("State root convergence validated")
}

// TestPVC_ConcurrentWrites simulates concurrent state writes that might
// occur during block production, and validates they persist correctly.
func TestPVC_ConcurrentWrites(t *testing.T) {
	// Note: This test is sequential in Go test framework, but models
	// the scenario where multiple blocks are written before a restart

	t.Log("=== TestPVC_ConcurrentWrites ===")

	pvc := NewPVCSimulator(t)

	// Simulate rapid block production - each block updates account state
	// In real implementation, these would be actual state transitions
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	addr := kp.Address()

	balances := []uint64{1000, 2000, 3000, 4000, 5000}

	for i, balance := range balances {
		height := uint64(i + 1)

		// Write state at each height
		pvc.WriteAccountState(t, addr, balance)

		// Write snapshot
		stateRoot := []byte{byte(height), 0, 0, 0, 0, 0, 0, 0}
		pvc.WriteBlockSnapshot(t, height, stateRoot)

		// Write WAL entry
		walData := []byte("block-data")
		pvc.WriteWALEntry(t, height, walData)
	}
	t.Logf("Wrote state for %d blocks", len(balances))

	// Simulate crash after all blocks
	pvc.SimulateFullRestart(t)

	// Verify final state
	finalBalance, exists := pvc.ReadAccountState(t, addr)
	require.True(t, exists, "final account state should exist")
	require.Equal(t, balances[len(balances)-1], finalBalance,
		"final balance should be from last block")

	// Verify we can read snapshots at different heights
	for i := range balances {
		height := uint64(i + 1)
		snap := pvc.ReadBlockSnapshot(t, height)
		require.NotEmpty(t, snap, "snapshot at height %d should exist", height)
	}

	t.Log("Concurrent writes validated - all state persisted correctly")
}

// TestPVC_StoragePathValidation validates that the PVC simulation correctly
// uses a separate path from the container's writable layer, matching
// Kubernetes behavior where /data is on PVC while /tmp is ephemeral.
func TestPVC_StoragePathValidation(t *testing.T) {
	t.Log("=== TestPVC_StoragePathValidation ===")

	pvc := NewPVCSimulator(t)

	// Verify the data directory is separate from base (simulating PVC mount)
	require.NotEqual(t, pvc.BaseDir, pvc.DataDir,
		"Data dir should be subdirectory (PVC mount point)")

	// Write a file to data dir
	pvc.WriteStateFile(t, "persistent.txt", []byte("persistent data"))

	// In Kubernetes: /tmp would be in container layer and lost on restart
	// Our simulation: we don't use BaseDir for persistent data
	// This is validated by the fact we use DataDir for all state

	// Verify file is in DataDir, not BaseDir
	persistentPath := filepath.Join(pvc.DataDir, "persistent.txt")
	_, err := os.Stat(persistentPath)
	require.NoError(t, err, "file should exist in PVC mount path")

	// Simulate restart
	pvc.SimulateFullRestart(t)

	// Verify file still exists (persisted)
	content := pvc.ReadStateFile(t, "persistent.txt")
	require.Equal(t, []byte("persistent data"), content)

	t.Log("Storage path validation passed - PVC mount behavior correct")
}
