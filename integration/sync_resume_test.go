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

// TestRestartResume_AfterMidSyncGap verifies the restart-resume contract end
// to end (7.4): a node that synced part of the chain, shut down, and restarted
// from the same data directory must resume at tip+1 — never re-requesting the
// already-applied range — and converge on the serving node's tip. This is the
// harness test for the apply gate (`-run TestRestartResume`).
//
// The exact crash-window semantics (applied-but-unpersisted progress, W5) are
// pinned at unit level by TestBlockSyncEngine_ResumeTipFloorAfterCrash and
// TestBlockSyncEngine_InvalidBlockPenalizesStopsAndResumes; this test proves
// the restart path holds on real nodes over real P2P.
func TestRestartResume_AfterMidSyncGap(t *testing.T) {
	kpA, err := wallet.GenerateKey()
	require.NoError(t, err)
	kpB, err := wallet.GenerateKey()
	require.NoError(t, err)

	// A is the only active validator; B is passive and block-syncs from A.
	// FastSyncEnabled=false routes B to block sync instead of snapshot sync.
	newNode := func(kp *wallet.KeyPair) *node.Node {
		cfg := node.Config{
			DataDir:          t.TempDir(),
			P2PPort:          freePort(t),
			MaxTxPerBlock:    100,
			ProposerTimeout:  50 * time.Millisecond,
			MempoolMaxSize:   10000,
			MempoolTTL:       300 * time.Second,
			FastSyncEnabled:  false,
			Validators:       []types.Address{kpA.Address()},
		}
		n, err := node.New(cfg)
		require.NoError(t, err)
		n.SetWallet(kp)
		return n
	}

	a := newNode(kpA)
	defer a.Close()
	b := newNode(kpB)
	cfgB := *b.Config() // captured for the restart

	registerValidatorsInState(t, a.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})
	registerValidatorsInState(t, b.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})

	// A mines to height 4.
	a.StartConsensus()
	require.Eventually(t, func() bool { return a.CurrentHeight() >= 4 },
		15*time.Second, 100*time.Millisecond, "A did not reach height 4 (at %d)", a.CurrentHeight())
	a.StopConsensus()

	// B joins and block-syncs 1..4 from A.
	require.NoError(t, b.P2P().Connect(a.P2P().Addr()))
	require.Eventually(t, func() bool {
		ba := b.P2P().PeerManager().GetPeerByAddr(a.P2P().Addr())
		return ba != nil && ba.Conn != nil && ba.SyncHeight >= 4
	}, 5*time.Second, 10*time.Millisecond, "P2P handshake did not complete")
	b.StartConsensus()
	require.Eventually(t, func() bool { return b.CurrentHeight() >= 4 },
		20*time.Second, 100*time.Millisecond, "B did not block-sync to 4 (at %d)", b.CurrentHeight())
	CompareStateRoots(t, []*node.Node{a, b})
	require.Equal(t, a.CurrentHeight(), b.CurrentHeight())

	// B shuts down mid-stream; A advances to 8 while B is offline.
	b.Close()
	a.StartConsensus()
	require.Eventually(t, func() bool { return a.CurrentHeight() >= 8 },
		15*time.Second, 100*time.Millisecond, "A did not reach height 8 (at %d)", a.CurrentHeight())
	a.StopConsensus()

	// B restarts from the same data directory and must resume at tip+1 (5),
	// converging on A without re-syncing 1..4. The peer manager persisted on
	// shutdown, so B may already be redialing A during node.New: only the
	// recovered-tip floor is asserted here (never above A's tip).
	b2, err := node.New(cfgB)
	require.NoError(t, err)
	defer b2.Close()
	b2.SetWallet(kpB)
	require.NoError(t, b2.LoadFromPersistent())
	recovered := b2.CurrentHeight()
	require.GreaterOrEqual(t, recovered, uint64(4), "restarted node must recover its tip from disk")
	require.LessOrEqual(t, recovered, a.CurrentHeight(), "restarted node must not exceed the serving tip")
	// The restarted node must start its engine before connecting so it reacts
	// to the handshake announcement (node.New does not run the Phase14 startup
	// transition; production calls it via Run(), tests call it explicitly).
	b2.StartConsensus()
	require.NoError(t, b2.P2P().Connect(a.P2P().Addr()))

	require.Eventually(t, func() bool {
		br, ar := b2.State().GetStateRoot(), a.State().GetStateRoot()
		return b2.CurrentHeight() >= a.CurrentHeight() && br == ar
	}, 30*time.Second, 100*time.Millisecond,
		"B did not resume from tip+1 and converge on A's tip %d (B at %d)", a.CurrentHeight(), b2.CurrentHeight())

	CompareStateRoots(t, []*node.Node{a, b2})
	require.Equal(t, a.CurrentHeight(), b2.CurrentHeight())
}
