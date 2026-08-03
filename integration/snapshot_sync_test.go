//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// TestSnapshotSync_FreshNodeAutoSync verifies the automatic snapshot path end
// to end: A (a validator) mines consensus blocks and seeds a state snapshot +
// checkpoint at its tip; B (a fresh node with FastSyncEnabled) joins and,
// without operator action, auto-discovers the snapshot over P2P, downloads +
// verifies + restores it, then replays the tail blocks A produced after the
// snapshot and converges on A's state root.
//
// The snapshot is seeded from A's live state via node.StoreStateSnapshot rather
// than mined across an epoch boundary: the shared registration helper advances
// the epoch counter (transition at height 100), so the first boundary the chain
// reaches is always invalid under the staking epoch math. Snapshot creation at
// epoch boundaries is covered by unit tests; this test exercises the sync path.
func TestSnapshotSync_FreshNodeAutoSync(t *testing.T) {
	kpA, err := wallet.GenerateKey()
	require.NoError(t, err)
	kpB, err := wallet.GenerateKey()
	require.NoError(t, err)

	// A and B share the same genesis: only A is an active validator, so B's
	// consensus loop never proposes and B stays passive while syncing.
	newNetworkNode := func(kp *wallet.KeyPair, fastSync bool) *node.Node {
		cfg := node.Config{
			DataDir:          t.TempDir(),
			P2PPort:          freePort(t),
			MaxTxPerBlock:    100,
			ProposerTimeout:  50 * time.Millisecond,
			MempoolMaxSize:   10000,
			MempoolTTL:       300 * time.Second,
			FastSyncEnabled:  fastSync,
			Validators:       []types.Address{kpA.Address()},
		}
		n, err := node.New(cfg)
		require.NoError(t, err)
		n.SetWallet(kp)
		return n
	}

	a := newNetworkNode(kpA, false)
	defer a.Close()
	b := newNetworkNode(kpB, true)
	defer b.Close()

	registerValidatorsInState(t, a.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})
	registerValidatorsInState(t, b.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})

	// A mines real consensus blocks to height 4 (below the first epoch
	// boundary, which this harness cannot cross).
	a.StartConsensus()
	require.Eventually(t, func() bool { return a.CurrentHeight() >= 4 },
		15*time.Second, 100*time.Millisecond, "A did not reach height 4 (at %d)", a.CurrentHeight())
	a.StopConsensus()

	// Freeze A's tip and seed the snapshot + checkpoint at that exact height;
	// all metadata comes from the block header so B's verify passes.
	cp, err := a.StoreStateSnapshot()
	require.NoError(t, err)
	t.Logf("seeded snapshot at height %d (root %x)", cp.Height, cp.StateRoot)

	// A resumes and mines past the snapshot so B has a tail to replay.
	a.StartConsensus()
	require.Eventually(t, func() bool { return a.CurrentHeight() >= 8 },
		15*time.Second, 100*time.Millisecond, "A did not reach height 8 (at %d)", a.CurrentHeight())
	a.StopConsensus()
	t.Logf("A produced %d blocks above the snapshot at %d", a.CurrentHeight()-cp.Height, cp.Height)

	// Fresh B connects BEFORE its engine queries so the first broadcast reaches A.
	require.NoError(t, b.P2P().Connect(a.P2P().Addr()))

	// Wait for B to hold A's connection and process A's 0x43 height
	// announcement. The announcement is sent from A's registerConn (accept
	// path), so receiving it proves A wired the inbound connection and its
	// read loop is starting — the snapshot query will reach it. The accept
	// side never registers the peer under B's listen address (it uses the
	// ephemeral source addr), so only B's view is polled.
	require.Eventually(t, func() bool {
		ba := b.P2P().PeerManager().GetPeerByAddr(a.P2P().Addr())
		return ba != nil && ba.Conn != nil && ba.SyncHeight >= 4
	}, 5*time.Second, 10*time.Millisecond,
		"P2P handshake did not complete before B started (sync=%d)", func() uint64 {
			if ba := b.P2P().PeerManager().GetPeerByAddr(a.P2P().Addr()); ba != nil {
				return ba.SyncHeight
			}
			return 0
		}())

	// B starts consensus: empty state + FastSyncEnabled must auto-start
	// snapshot sync, restore, replay the tail and converge on A.
	b.StartConsensus()

	require.Eventually(t, func() bool {
		br, ar := b.State().GetStateRoot(), a.State().GetStateRoot()
		return b.CurrentHeight() >= a.CurrentHeight() && br == ar
	}, 45*time.Second, 100*time.Millisecond,
		"B did not snapshot-sync + replay to A's tip %d (B at %d)", a.CurrentHeight(), b.CurrentHeight())

	CompareStateRoots(t, []*node.Node{a, b})
	require.Equal(t, a.CurrentHeight(), b.CurrentHeight())
}
