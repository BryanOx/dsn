package node

import (
	"net"
	"testing"
	"time"

	"github.com/dsn/dsn/network"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// freeTestPort returns an available TCP port for a P2P node.
func freeTestPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// newNodeWithP2P creates a persistent node with P2P enabled (no validators).
func newNodeWithP2P(t *testing.T) *Node {
	t.Helper()
	cfg := Config{
		DataDir:        t.TempDir(),
		P2PPort:        freeTestPort(t),
		MempoolMaxSize: 10000,
		MempoolTTL:     300 * time.Second,
	}
	n, err := New(cfg)
	require.NoError(t, err)
	return n
}

// markPeerConnected flips the given peer's state so the FastSyncEngine's
// GetPeersByState(PeerConnected) sees it as a download source.
func markPeerConnected(t *testing.T, n *Node, addr string) {
	t.Helper()
	p := n.P2P().PeerManager().GetPeerByAddr(addr)
	require.NotNil(t, p, "peer %s not registered in peer manager", addr)
	p.State = network.PeerConnected
}

// storeSnapshotOnNode writes a valid snapshot + checkpoint at height 100 into
// the node's persistent storage (300 KiB of data => 2 chunks). The serve
// closures registered by New() will advertise and serve it.
func storeSnapshotOnNode(t *testing.T, n *Node) *state.Checkpoint {
	t.Helper()

	hasher := types.SHA256Hasher{}
	src := state.NewInMemoryState(hasher)
	addr := types.Address{0: 0x01}
	acc := state.NewAccount(addr, [32]byte{0: 0x01})
	acc.AddBalance(types.NewAmount(5000))
	src.SetAccount(addr, acc)
	big := make([]byte, 300*1024)
	for i := range big {
		big[i] = byte(i % 251)
	}
	src.SetBytes("big", big)
	root, err := src.Commit()
	require.NoError(t, err)

	snap, err := src.CreateSnapshot(100, 2, types.Hash{0xAA}, 1000000)
	require.NoError(t, err)
	snapData, err := state.SerializeSnapshot(snap)
	require.NoError(t, err)

	cp := &state.Checkpoint{
		Height:           100,
		BlockHash:        types.Hash{0xBB},
		StateRoot:        root,
		SnapshotHash:     state.SnapshotHash(snapData),
		ValidatorSetHash: types.Hash{0xAA},
		Epoch:            2,
		Timestamp:        1000000,
	}
	require.NoError(t, state.StoreSnapshot(n.persistent, 100, snapData))
	require.NoError(t, state.StoreCheckpoint(n.persistent, cp))
	return cp
}

// TestSyncFromNetwork_DownloadAndVerify exercises the full network fetch path:
// discovery, chunk download with re-request, reassembly and hash verification,
// then restore. Before task 5.3 SyncFromNetwork returns "not fully
// implemented", so this test fails (RED).
func TestSyncFromNetwork_DownloadAndVerify(t *testing.T) {
	a := newNodeWithP2P(t)
	defer a.Close()
	cp := storeSnapshotOnNode(t, a)

	b := newNodeWithP2P(t)
	defer b.Close()

	// Fresh node: empty state.
	require.Equal(t, types.Hash{}, b.persistent.GetStateRoot())

	// Connect B to A and register A as a connected peer for the download.
	aAddr := a.P2P().Addr()
	require.NoError(t, b.P2P().Connect(aAddr))
	markPeerConnected(t, b, aAddr)
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, b.SyncFromNetwork(15*time.Second))

	// All chunks downloaded, reassembled, verified and restored: the node's
	// state root must now equal the snapshot's root and the account must exist.
	require.Equal(t, cp.StateRoot, b.persistent.GetStateRoot())
	require.Equal(t, cp.StateRoot, b.State().GetStateRoot())

	acc, err := b.State().GetAccount(types.Address{0: 0x01})
	require.NoError(t, err)
	require.Equal(t, 0, types.NewAmount(5000).Cmp(acc.Balance))
}

// TestSyncFromNetwork_HashMismatch verifies that a snapshot whose reassembled
// hash differs from the advertised hash is rejected: SyncFromNetwork reports an
// error and no state is restored (the caller falls back to range replay).
func TestSyncFromNetwork_HashMismatch(t *testing.T) {
	a := newNodeWithP2P(t)
	defer a.Close()
	cp := storeSnapshotOnNode(t, a)

	// Corrupt the stored snapshot data while keeping the checkpoint's hash:
	// the server advertises the (now wrong) checkpoint hash but serves chunks
	// of the corrupt data, so reassembly hash verification must fail.
	snapData, err := state.LoadSnapshot(a.persistent, 100)
	require.NoError(t, err)
	corrupt := append([]byte{}, snapData...)
	corrupt[0] ^= 0xFF
	require.NoError(t, state.StoreSnapshot(a.persistent, 100, corrupt))

	b := newNodeWithP2P(t)
	defer b.Close()

	aAddr := a.P2P().Addr()
	require.NoError(t, b.P2P().Connect(aAddr))
	markPeerConnected(t, b, aAddr)
	time.Sleep(100 * time.Millisecond)

	err = b.SyncFromNetwork(15 * time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "snapshot")

	// Nothing restored: the node stays empty for the range-replay fallback.
	require.Equal(t, types.Hash{}, b.persistent.GetStateRoot())
	require.Equal(t, cp.SnapshotHash, state.SnapshotHash(snapData), "test setup sanity: advertised hash")
}
