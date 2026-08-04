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

// TestFallBehind_BlockSyncCatchUp verifies the offline-gap recovery contract
// (7.5): a node that falls far behind (its offline gap reaches well past its
// finalized height, k=6) reconnects and block-syncs only the missing tail —
// resuming at tip+1 — without reorging or re-applying anything at or below its
// finalized height. The stale-range guard itself is pinned by
// TestApplySyncedBlock_AlreadyFinalizedSkipped (S1); this test proves the
// resume-at-tip+1 path over real P2P with a 50-block offline gap (spec
// scenario: "a live node whose tip falls 50 blocks below a peer mid-epoch").
func TestFallBehind_BlockSyncCatchUp(t *testing.T) {
	kpA, err := wallet.GenerateKey()
	require.NoError(t, err)
	kpB, err := wallet.GenerateKey()
	require.NoError(t, err)

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

	registerValidatorsInState(t, a.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})
	registerValidatorsInState(t, b.State(), []types.Address{kpA.Address()}, []*wallet.KeyPair{kpA})

	// A mines 12 blocks with transactions so the chain (and finalized height,
	// k=6 → 6) is meaningful beyond the genesis root. Blocks are produced by A's
	// real consensus loop (MineBlockWithTxs only builds — it does not store), so
	// A's mempool is pre-loaded with the txs and the loop batches them into the
	// chain as it advances to height 12. All txs share the same wall-clock
	// timestamp (±5s live freshness window applies to each one).
	hasher := types.SHA256Hasher{}
	ts := uint64(time.Now().Unix())
	a.StartConsensus()
	for round := 0; round < 12; round++ {
		tx := types.NewTransaction(
			1, 0, kpA.Address(), uint64(round+1),
			types.EncodeTransferPayload(kpA.Address(), 1), nil, 100, 1000,
			ts,
		)
		require.NoError(t, kpA.Sign(tx, hasher))
		require.NoError(t, a.SubmitTx(tx), "A submit tx %d", round+1)
	}
	require.Eventually(t, func() bool { return a.CurrentHeight() >= 12 },
		20*time.Second, 100*time.Millisecond, "A did not reach height 12 (at %d)", a.CurrentHeight())
	a.StopConsensus()
	tipA := a.CurrentHeight()

	// B joins and block-syncs 1..tipA; its finalized height reaches k.
	require.NoError(t, b.P2P().Connect(a.P2P().Addr()))
	require.Eventually(t, func() bool {
		ba := b.P2P().PeerManager().GetPeerByAddr(a.P2P().Addr())
		return ba != nil && ba.Conn != nil && ba.SyncHeight >= tipA
	}, 5*time.Second, 10*time.Millisecond, "P2P handshake did not complete")
	b.StartConsensus()
	require.Eventually(t, func() bool { return b.CurrentHeight() >= tipA },
		20*time.Second, 100*time.Millisecond, "B did not block-sync to %d (at %d)", tipA, b.CurrentHeight())
	CompareStateRoots(t, []*node.Node{a, b})
	cfgB := *b.Config()

	// B goes offline while A extends the chain by 50 blocks (tipA → tipA+50),
	// matching the spec scenario ("falls 50 blocks below a peer mid-epoch"):
	// a gap well past B's finalized height 6.
	b.Close()
	a.StartConsensus()
	require.Eventually(t, func() bool { return a.CurrentHeight() >= tipA+50 },
		30*time.Second, 100*time.Millisecond, "A did not reach %d (at %d)", tipA+50, a.CurrentHeight())
	a.StopConsensus()

	// B restarts and must resume at tip+1 (tipA+1), never re-applying its
	// finalized range [1..k]. Same redial caveat as the restart-resume test.
	b2, err := node.New(cfgB)
	require.NoError(t, err)
	defer b2.Close()
	b2.SetWallet(kpB)
	require.NoError(t, b2.LoadFromPersistent())
	recovered := b2.CurrentHeight()
	require.GreaterOrEqual(t, recovered, tipA, "restarted node must recover its tip from disk")
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
		"B did not catch up the 50-block gap and converge on A's tip %d (B at %d)", a.CurrentHeight(), b2.CurrentHeight())

	CompareStateRoots(t, []*node.Node{a, b2})
	require.Equal(t, a.CurrentHeight(), b2.CurrentHeight())
}
