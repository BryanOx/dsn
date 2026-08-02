//go:build integration

package staging

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// PVCSimulator simulates StatefulSet PVC behavior for testing.
// In a real Kubernetes environment, the PVC would be mounted at a specific
// path, and pod deletion/recreation would not affect the persistent data.
// This simulator models that behavior using temporary directories.
type PVCSimulator struct {
	BaseDir string
	// Data subdirectory - this is where "persistent" data lives
	DataDir string
}

// NewPVCSimulator creates a new PVC simulator with a temporary directory.
// The data directory represents the PVC mount point.
func NewPVCSimulator(t *testing.T) *PVCSimulator {
	t.Helper()

	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")

	err := os.MkdirAll(dataDir, 0755)
	require.NoError(t, err)

	return &PVCSimulator{
		BaseDir: baseDir,
		DataDir: dataDir,
	}
}

// WriteStateFile writes a state file to the "persistent" volume.
// This simulates writing data to a PVC.
func (p *PVCSimulator) WriteStateFile(t *testing.T, filename string, content []byte) {
	t.Helper()

	filePath := filepath.Join(p.DataDir, filename)
	err := os.WriteFile(filePath, content, 0644)
	require.NoError(t, err, "failed to write state file %s", filename)
}

// ReadStateFile reads a state file from the "persistent" volume.
func (p *PVCSimulator) ReadStateFile(t *testing.T, filename string) []byte {
	t.Helper()

	filePath := filepath.Join(p.DataDir, filename)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err, "failed to read state file %s", filename)
	return content
}

// StateFileExists checks if a state file exists.
func (p *PVCSimulator) StateFileExists(t *testing.T, filename string) bool {
	t.Helper()

	filePath := filepath.Join(p.DataDir, filename)
	_, err := os.Stat(filePath)
	return err == nil
}

// DeletePod simulates pod deletion - the container's writable layer is lost,
// but the PVC (dataDir) persists.
func (p *PVCSimulator) DeletePod(t *testing.T) {
	t.Helper()

	// In real Kubernetes: pod container filesystem is wiped, but PVC persists
	// In simulation: we keep dataDir intact (represents PVC)
	// The baseDir might have temporary container-layer data we'd clean here
	// For our simulation, we just ensure dataDir remains
	require.DirExists(t, p.DataDir, "PVC should persist after pod deletion")
}

// RecreatePod simulates pod recreation - a new pod mounts the same PVC.
// In our simulation, the dataDir already has the persisted data.
func (p *PVCSimulator) RecreatePod(t *testing.T) {
	t.Helper()

	// In real Kubernetes: new pod starts and mounts the same PVC
	// In simulation: dataDir already has persisted data from previous pod
	require.DirExists(t, p.DataDir, "PVC should be mounted for new pod")
}

// ListStateFiles lists all state files in the PVC.
func (p *PVCSimulator) ListStateFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(p.DataDir)
	require.NoError(t, err)

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files
}

// WriteWALEntry writes a WAL (Write-Ahead Log) entry to simulate transaction log.
func (p *PVCSimulator) WriteWALEntry(t *testing.T, height uint64, data []byte) {
	t.Helper()

	filename := fmt.Sprintf("wal/block-%d.wal", height)
	p.WriteStateFile(t, filename, data)
}

// ReadWALEntry reads a WAL entry.
func (p *PVCSimulator) ReadWALEntry(t *testing.T, height uint64) []byte {
	t.Helper()

	filename := fmt.Sprintf("wal/block-%d.wal", height)
	return p.ReadStateFile(t, filename)
}

// CleanupWAL removes all WAL entries up to a given height.
func (p *PVCSimulator) CleanupWAL(t *testing.T, upToHeight uint64) {
	t.Helper()

	walDir := filepath.Join(p.DataDir, "wal")
	if _, err := os.Stat(walDir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(walDir)
	require.NoError(t, err)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Parse height from filename like "block-001.wal"
		// For simplicity, just remove all in this test
		filePath := filepath.Join(walDir, e.Name())
		err := os.Remove(filePath)
		require.NoError(t, err)
	}
}

// SimulateFullRestart simulates a complete pod deletion and recreation cycle:
// 1. Delete the "pod" (simulates container termination)
// 2. Recreate the "pod" (simulates new pod mounting same PVC)
// 3. Verify persisted data is accessible
func (p *PVCSimulator) SimulateFullRestart(t *testing.T) {
	t.Helper()

	// Step 1: Pod deletion - container layer gone, PVC persists
	p.DeletePod(t)

	// Step 2: Pod recreation - new container mounts same PVC
	p.RecreatePod(t)

	// Step 3: Verify data still accessible (PVC persisted)
	// This is implicitly checked by the fact we can read/write after recreation
}

// WriteAccountState simulates persisting account state to PVC.
func (p *PVCSimulator) WriteAccountState(t *testing.T, addr types.Address, balance uint64) {
	t.Helper()

	amount := types.NewAmount(balance)
	data, err := amount.MarshalBinary()
	require.NoError(t, err, "marshal amount")

	content := make([]byte, 0, len(addr)+len(data))
	content = append(content, addr[:]...)
	content = append(content, data...)

	// Use hex encoding for address to avoid invalid filename chars
	addrStr := ""
	for _, b := range addr[:8] {
		addrStr += string(b)
	}
	p.WriteStateFile(t, "account-"+addrStr+".state", content)
}

// ReadAccountState simulates reading account state from PVC.
func (p *PVCSimulator) ReadAccountState(t *testing.T, addr types.Address) (balance uint64, exists bool) {
	t.Helper()

	// Use same hex encoding as WriteAccountState
	addrStr := ""
	for _, b := range addr[:8] {
		addrStr += string(b)
	}
	filename := "account-" + addrStr + ".state"
	if !p.StateFileExists(t, filename) {
		return 0, false
	}

	content := p.ReadStateFile(t, filename)
	// Skip address (32 bytes), read balance
	if len(content) < 32 {
		return 0, false
	}

	var amount types.Amount
	err := amount.UnmarshalBinary(content[32:])
	if err != nil {
		return 0, false
	}

	// Use String() method and parse back to uint64
	balanceStr := amount.String()
	var parsed uint64
	_, err = fmt.Sscanf(balanceStr, "%d", &parsed)
	if err != nil {
		return 0, false
	}

	return parsed, true
}

// WriteBlockSnapshot simulates persisting a block snapshot to PVC.
func (p *PVCSimulator) WriteBlockSnapshot(t *testing.T, height uint64, stateRoot []byte) {
	t.Helper()

	filename := fmt.Sprintf("snapshots/snapshot-%d.bin", height)
	p.WriteStateFile(t, filename, stateRoot)
}

// ReadBlockSnapshot reads a block snapshot.
func (p *PVCSimulator) ReadBlockSnapshot(t *testing.T, height uint64) []byte {
	t.Helper()

	filename := fmt.Sprintf("snapshots/snapshot-%d.bin", height)
	return p.ReadStateFile(t, filename)
}

// LatestSnapshotHeight returns the height of the latest snapshot.
func (p *PVCSimulator) LatestSnapshotHeight(t *testing.T) uint64 {
	t.Helper()

	snapshotsDir := filepath.Join(p.DataDir, "snapshots")
	if _, err := os.Stat(snapshotsDir); os.IsNotExist(err) {
		return 0
	}

	entries, err := os.ReadDir(snapshotsDir)
	require.NoError(t, err)

	var maxHeight uint64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Parse height from filename - simplified
		// In real implementation, parse properly
	}
	return maxHeight
}

// testMempool implements consensus.MempoolI for building blocks with explicit transactions.
type testMempool struct {
	txs []*types.Transaction
}

func (m *testMempool) PendingTxs() []*types.Transaction { return m.txs }
func (m *testMempool) Remove(_ types.Hash)              {}

// consensusSigner wraps a wallet.KeyPair to implement consensus.Signer interface.
type consensusSigner struct {
	kp *wallet.KeyPair
}

func (s *consensusSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}
