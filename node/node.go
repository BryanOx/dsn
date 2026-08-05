package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/genesis"
	"github.com/BryanOx/dsn/indexer"
	"github.com/BryanOx/dsn/mempool"
	"github.com/BryanOx/dsn/network"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/vm"
	"github.com/BryanOx/dsn/wallet"
)

// Node represents a DSN node that ties together state, mempool, and networking.
type Node struct {
	cfg        Config
	state      *state.InMemoryState
	persistent *state.PersistentState
	mempool    *mempool.Mempool
	hasher     types.SHA256Hasher
	p2p        *network.P2PNode
	vm         *vm.VM // WASM VM for contract execution
	indexer    *indexer.Indexer

	// Sync mode for fast sync / recovery
	syncMode SyncMode // NEW

	// Engine references for Phase 3B
	fastSync  *network.FastSyncEngine
	blockSync *network.BlockSyncEngine
	gossip    *network.GossipEngine
	discovery *network.PeerDiscovery

	// Fork handling
	finalizedHeight uint64 // k=6 finality
	reorgBuffer     []ReorgEntry

	// Consensus fields
	consensusRunning bool
	consensusStopCh  chan struct{}
	wallet           *wallet.KeyPair
	currentHeight    atomic.Uint64
	currentTipHash   atomic.Pointer[types.Hash] // atomic.Pointer because types.Hash is an array (sync/atomic can't store arrays)
	currentEpoch     atomic.Uint64              // tracks current epoch for boundary detection
	bestChainHeight  uint64                     // tracks best chain height for fork choice

	// Two-phase voting state for the pending proposal round. voteMu guards the
	// pending round: voting (VotingState), pendingBlock, pendingHash and the
	// view-change bookkeeping (pendingRound, pendingRoundTicks, precommitSent).
	voteMu            sync.Mutex
	voting            *consensus.VotingState
	pendingBlock      *types.Block
	pendingHash       types.Hash
	pendingHeight     uint64
	pendingRound      uint32
	pendingRoundTicks uint64
	precommitSent     bool

	// Production Node Runtime fields (Phase 5B)
	genesisDoc      *genesis.GenesisDoc
	genesisHash     types.Hash
	started         bool
	startOnce       sync.Once
	stopOnce        sync.Once
	shutdownCh      chan struct{}
	validators      []types.Address
	consensusParams *genesis.ConsensusParams
	epochParams     *genesis.EpochParams
	metricsServer   *MetricsServer
}

// finalityK is the finality threshold (k=6)
const finalityK = 6

// forkWeight returns the effective height for fork comparison.
// Blocks older than (tip - k) are finalized and cannot be reorged.
func forkWeight(h, tip uint64) uint64 {
	if tip < finalityK {
		return h // early chain — no finality
	}
	if h > tip-finalityK {
		return tip - finalityK // not yet finalized
	}
	return h
}

// ReorgEntry stores block info for potential rollback on chain reorganization.
type ReorgEntry struct {
	Height uint64
	Hash   types.Hash
}

// New creates a new DSN node with the given configuration.
// If cfg.DataDir is empty, uses in-memory state only.
// If cfg.DataDir is set, persists state to a BoltDB database in that directory.
func New(cfg Config) (*Node, error) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	var persistent *state.PersistentState
	if cfg.DataDir != "" {
		dbPath := filepath.Join(cfg.DataDir, "dsn.db")
		ps, err := state.NewPersistentState(dbPath, hasher, cfg.FSync)
		if err != nil {
			return nil, fmt.Errorf("failed to open persistent state: %w", err)
		}
		persistent = ps

		// Load accounts from persistent storage into in-memory state
		// Try to recover any account that has a state root
		if ps.GetStateRoot() != (types.Hash{}) {
			// Accounts are already loaded into ps.cache during NewPersistentState
			// For now, user must explicitly call n.LoadFromPersistent()
		}
	}

	mp := mempool.New(cfg.MempoolMaxSize, cfg.MempoolTTL, s)

	// Initialize P2P if port is configured
	var p2pNode *network.P2PNode
	if cfg.P2PPort > 0 {
		var err error
		p2pNode, err = network.NewP2PNode(cfg.P2PPort)
		if err != nil {
			return nil, fmt.Errorf("failed to create p2p node: %w", err)
		}

		// Auto-submit gossiped transactions to mempool
		p2pNode.SetTxHandler(func(tx *types.Transaction) {
			mp.Submit(tx)
		})
	}

	n := &Node{
		cfg:        cfg,
		state:      s,
		persistent: persistent,
		mempool:    mp,
		hasher:     hasher,
		p2p:        p2pNode,
	}

	// Create WASM VM for contract execution
	v, err := vm.NewVM(hasher)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}
	n.vm = v

	// Create indexer if data directory is configured
	if cfg.DataDir != "" && cfg.IndexerEnabled {
		idx, err := indexer.New(cfg.DataDir, cfg.IndexerEnabled)
		if err != nil {
			return nil, fmt.Errorf("failed to create indexer: %w", err)
		}
		n.indexer = idx

		// Set the indexer's block retriever to ourselves
		if n.indexer != nil {
			n.indexer.SetBlockRetriever(n)
			n.indexer.Start()
		}
	}

	// Create and wire engines (Phase 3B)
	if p2pNode != nil && persistent != nil {
		// Create engines
		n.fastSync = network.NewFastSyncEngine(n.p2p.PeerManager(), n.p2p, persistent)
		n.blockSync = network.NewBlockSyncEngine(n.p2p.PeerManager(), n.p2p, persistent)
		n.gossip = network.NewGossipEngine(n.p2p.PeerManager(), n.p2p)

		// Set handlers on FastSyncEngine
		n.fastSync.SetRestoreHandler(func(data []byte, expectedHash types.Hash) error {
			// Verify + persist the snapshot, rebuilding the persistent SMT
			// (state root checked against the snapshot inside).
			if err := state.RestoreFromSnapshot(persistent, data, expectedHash); err != nil {
				return err
			}

			// Rebuild in-memory state from persistent storage and commit to
			// compute the in-memory SMT root.
			if err := persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
				return n.state.SetAccount(addr, acc)
			}); err != nil {
				return fmt.Errorf("rebuild in-memory accounts: %w", err)
			}
			if err := persistent.ForEachKV(func(k string, v []byte) error {
				return n.state.SetBytes(k, v)
			}); err != nil {
				return fmt.Errorf("rebuild in-memory kvstore: %w", err)
			}
			if _, err := n.state.Commit(); err != nil {
				return fmt.Errorf("commit in-memory state: %w", err)
			}

			// Verify the rebuilt in-memory root matches the restored root.
			gotRoot := n.state.GetStateRoot()
			wantRoot := persistent.GetStateRoot()
			if gotRoot != wantRoot {
				return fmt.Errorf("state root mismatch after restore: got %x, expected %x",
					gotRoot[:], wantRoot[:])
			}

			// Adopt the snapshot's tip and persist it so crash recovery can
			// resume the replay from here. The snapshot itself carries the
			// height/root/timestamp (no block hash — a fresh node has none).
			snap, err := state.DeserializeSnapshot(data)
			if err != nil {
				return fmt.Errorf("parse restored snapshot: %w", err)
			}
			n.setTip(snap.Height, types.Hash{})
			if err := consensus.StoreTip(persistent, &types.BlockHeader{
				Height:       snap.Height,
				PreviousHash: types.Hash{},
				StateRoot:    snap.StateRoot,
				Timestamp:    snap.Timestamp,
			}); err != nil {
				return fmt.Errorf("store tip after restore: %w", err)
			}
			return nil
		})
		n.fastSync.SetReplayHandler(func(fromHeight uint64) error {
			// Replay target: max(peer height) — a fresh node has no local tip
			// past the restored snapshot, so the range must extend to what the
			// network advertises.
			target := n.currentHeight.Load()
			if n.blockSync != nil {
				if mh := n.blockSync.MaxPeerHeight(); mh > target {
					target = mh
				}
			}
			return n.ReplayBlocks(fromHeight, target)
		})

		// Register with P2PNode
		p2pNode.SetFastSyncEngine(n.fastSync)
		p2pNode.SetBlockSyncEngine(n.blockSync)
		p2pNode.SetGossipEngine(n.gossip)

		// Adapter (Task 5.4): the block-sync engine applies served blocks
		// through the node's validation pipeline (decodeSyncedBlock →
		// ValidateBlock → applyAcceptedBlock), never the gossip wire format.
		n.blockSync.SetBlockHandler(n.applySyncedBlock)

		// Height provider: announce our tip to every new peer (late-joiner
		// visibility) without the node coupling into p2p connection handling.
		p2pNode.SetSyncHeightProvider(func() uint64 { return n.currentHeight.Load() })

		// Create PeerDiscovery for every node so the read loop can answer
		// pings/PEX even for a seed node with no bootstrap peers. node0 in the
		// localnet has an empty bootstrap list; without a discovery instance
		// its read loop would drop inbound pings and never pong, tearing
		// down the connections of the peers that bootstrap to it.
		n.discovery = network.NewPeerDiscovery(n.p2p.PeerManager(), cfg.BootstrapPeers, p2pNode)
		p2pNode.SetDiscovery(n.discovery)
	}

	// Perform crash recovery if persistent storage is available
	if persistent != nil {
		if err := n.Recover(); err != nil {
			return nil, fmt.Errorf("crash recovery: %w", err)
		}
	}

	return n, nil
}

// LoadFromPersistent copies all accounts from persistent storage into in-memory state.
func (n *Node) LoadFromPersistent() error {
	if n.persistent == nil {
		return nil
	}
	// Load accounts from persistent cache into in-memory state
	if err := n.persistent.ForEachAccount(func(addr types.Address, acc *state.Account) error {
		return n.state.SetAccount(addr, acc)
	}); err != nil {
		return fmt.Errorf("load accounts from persistent: %w", err)
	}
	// Load kvstore entries from persistent cache into in-memory state
	if err := n.persistent.ForEachKV(func(k string, v []byte) error {
		return n.state.SetBytes(k, v)
	}); err != nil {
		return fmt.Errorf("load kvstore from persistent: %w", err)
	}
	// Rebuild SMT in memory after loading data
	if _, err := n.state.Commit(); err != nil {
		return fmt.Errorf("rebuild SMT after load from persistent: %w", err)
	}
	return nil
}

// SaveToPersistent flushes in-memory state to persistent storage.
func (n *Node) SaveToPersistent() (types.Hash, error) {
	return n.CommitState()
}

// State returns the node's state database.
func (n *Node) State() *state.InMemoryState {
	return n.state
}

// Mempool returns the node's transaction pool.
func (n *Node) Mempool() *mempool.Mempool {
	return n.mempool
}

// P2P returns the node's P2P networking layer, or nil if P2P is disabled.
func (n *Node) P2P() *network.P2PNode {
	return n.p2p
}

// Indexer returns the node's indexer, or nil if indexer is disabled.
func (n *Node) Indexer() *indexer.Indexer {
	return n.indexer
}

// VM returns the node's WASM VM for contract execution.
func (n *Node) VM() *vm.VM {
	return n.vm
}

// GetBlock retrieves a block by height from the persistent store.
func (n *Node) GetBlock(height uint64) (*types.Block, error) {
	if n.persistent == nil {
		return nil, fmt.Errorf("persistent state not available")
	}
	return consensus.LoadBlock(n.persistent, height)
}

// SubmitTx submits a transaction to the mempool for validation and inclusion.
func (n *Node) SubmitTx(tx *types.Transaction) error {
	return n.mempool.Submit(tx)
}

// CommitState finalizes pending state changes and returns the new state root.
func (n *Node) CommitState() (types.Hash, error) {
	// Sync accounts from InMemoryState to PersistentState
	if n.persistent != nil {
		if err := n.state.ForEachAccount(func(addr types.Address, acc *state.Account) error {
			return n.persistent.SetAccount(addr, acc)
		}); err != nil {
			return types.Hash{}, fmt.Errorf("sync accounts to persistent: %w", err)
		}
		if err := n.state.ForEachKV(func(k string, v []byte) error {
			return n.persistent.SetBytes(k, v)
		}); err != nil {
			return types.Hash{}, fmt.Errorf("sync kvstore to persistent: %w", err)
		}
		// Persist to BoltDB
		if _, err := n.persistent.Commit(); err != nil {
			return types.Hash{}, fmt.Errorf("persistent commit: %w", err)
		}
	}
	// Return the InMemoryState root
	return n.state.Commit()
}

// PendingTxs returns transactions ready for block inclusion.
func (n *Node) PendingTxs() []*types.Transaction {
	return n.mempool.PendingTxs()
}

// Close gracefully shuts down the node, closing all resources.
func (n *Node) Close() error {
	n.StopConsensus()

	// Stop indexer first
	if n.indexer != nil {
		n.indexer.Stop()
	}

	// Stop all engines
	if n.fastSync != nil {
		n.fastSync.Stop()
	}
	if n.blockSync != nil {
		n.blockSync.Stop()
	}
	if n.discovery != nil {
		n.discovery.Stop()
	}

	// Flush peer DB and stop PeerManager
	if n.p2p != nil {
		if n.p2p.PeerManager() != nil {
			n.p2p.PeerManager().Stop()
			n.p2p.PeerManager().Persist() // final flush
		}
		if err := n.p2p.Close(); err != nil {
			return fmt.Errorf("failed to close p2p node: %w", err)
		}
	}

	// Close VM
	if n.vm != nil {
		if err := n.vm.Close(); err != nil {
			return fmt.Errorf("failed to close VM: %w", err)
		}
	}

	if n.persistent != nil {
		return n.persistent.Close()
	}
	return nil
}

// SetWallet sets the wallet for block signing.
func (n *Node) SetWallet(kp *wallet.KeyPair) {
	n.wallet = kp
}

// walletSigner wraps a wallet.KeyPair to implement consensus.Signer interface.
type walletSigner struct {
	kp *wallet.KeyPair
}

func (s *walletSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}

// nodeAddress returns the node's address from its wallet.
func (n *Node) nodeAddress() types.Address {
	if n.wallet != nil {
		return n.wallet.Address()
	}
	return types.Address{}
}

// consensusID returns the node's canonical ConsensusID derived from its wallet
// public key (SHA256(pubkey)[:20]), the identity used by the staking registry
// and block validation. It is distinct from the operator Address().
func (n *Node) consensusID() types.Address {
	if n.wallet == nil {
		return types.Address{}
	}
	return types.DeriveConsensusID(n.wallet.PublicKey)
}

// consensusProposerAtHeight selects the block proposer for height from the
// ACTIVE staking registry using weighted voting power — the same selection the
// block validator applies (consensus.WeightedProposerAtHeight over
// staking.GetActiveValidators). It returns the proposer's ConsensusID, or the
// zero address when the node has no wallet, the registry read fails, or the
// active validator set is empty.
func (n *Node) consensusProposerAtHeight(height uint64) types.Address {
	return n.consensusProposerAtHeightAndRound(height, 0)
}

// consensusProposerAtHeightAndRound selects the proposer for (height, round)
// from the ACTIVE staking registry using the round-aware weighted schedule
// (consensus.WeightedProposerAtHeightAndRound over staking.GetActiveValidators).
// Round 0 reproduces the height-only schedule exactly. Returns the zero
// address when the node has no wallet, the registry read fails, or the active
// validator set is empty.
func (n *Node) consensusProposerAtHeightAndRound(height uint64, round uint32) types.Address {
	if n.wallet == nil {
		return types.Address{}
	}
	active, err := staking.GetActiveValidators(n.State())
	if err != nil || len(active) == 0 {
		return types.Address{}
	}
	return consensus.WeightedProposerAtHeightAndRound(height, round, active)
}

// currentRound returns the round this node is currently pending on for the
// given height, or 0 when the node has no pending round for it (the pending
// round is only established once a proposal is built or adopted).
func (n *Node) currentRound(height uint64) uint32 {
	n.voteMu.Lock()
	defer n.voteMu.Unlock()
	if n.pendingHeight != height {
		return 0
	}
	return n.pendingRound
}

// maybeAdvanceRound drives the view change: each consensus-loop tick while the
// pending round stays unfinalized counts toward the round's deadline, and once
// ProposerTimeout×(1+r) elapses the node advances to round r+1. The advance
// abandons the old round's vote set (they are stale) and resets the precommit
// gate, but keeps the pending block: the new round's proposer either rebuilds
// fresh (proposeForHeight with replace) or a peer's higher-round proposal is
// adopted (handleProposal). Round r must satisfy r+1 <= MaxRound; at the cap
// the node waits and re-broadcasts instead of advancing.
func (n *Node) maybeAdvanceRound(height uint64) {
	n.voteMu.Lock()
	defer n.voteMu.Unlock()
	if n.currentHeight.Load() >= height || n.pendingHeight != height {
		return
	}
	if n.pendingRound >= n.cfg.MaxRound {
		return
	}
	n.pendingRoundTicks++
	if n.pendingRoundTicks >= uint64(n.pendingRound)+1 {
		n.pendingRound++
		n.pendingRoundTicks = 0
		n.precommitSent = false
		n.voting = nil
	}
}

// CurrentHeight returns the current chain height.
func (n *Node) CurrentHeight() uint64 {
	return n.currentHeight.Load()
}

// GetTipHash returns the hash of the current tip block.
func (n *Node) GetTipHash() types.Hash {
	h := n.currentTipHash.Load()
	if h == nil {
		return types.Hash{}
	}
	return *h
}

// setTip atomically records the current chain tip. The header hash is stored
// as an immutable copy because types.Hash is an array and sync/atomic cannot
// operate on arrays directly.
func (n *Node) setTip(height uint64, hash types.Hash) {
	n.currentHeight.Store(height)
	n.currentTipHash.Store(&hash)
}

// Hasher returns the node's SHA256 hasher.
func (n *Node) Hasher() types.SHA256Hasher {
	return n.hasher
}

// Config returns the node's configuration.
func (n *Node) Config() *Config {
	return &n.cfg
}

// StartConsensus starts the consensus loop if P2P is enabled and validators exist.
func (n *Node) StartConsensus() {
	if n.consensusRunning {
		return
	}
	// Only start if P2P is enabled AND the staking registry has active validators
	// (proposer selection reads from the registry, not the legacy n.validators set).
	if n.p2p == nil {
		return
	}
	active, err := staking.GetActiveValidators(n.State())
	if err != nil || len(active) == 0 {
		return
	}

	n.consensusRunning = true
	n.consensusStopCh = make(chan struct{})

	// Load tip from persistent storage
	if n.persistent != nil {
		tip, err := consensus.LoadTip(n.persistent)
		if err == nil {
			hash, _ := tip.HeaderHash(n.hasher)
			n.setTip(tip.Height, hash)
			n.currentEpoch.Store(tip.Epoch)
		}

		// Fast sync: snapshot sync for fresh (empty-state) nodes; nodes with
		// existing state always use block-range catch-up (automatic).
		if n.cfg.FastSyncEnabled && n.fastSync != nil && n.persistent.GetStateRoot() == (types.Hash{}) {
			// Set up state change handler to detect when sync completes
			n.fastSync.SetStateChangeHandler(func(_, newState network.SyncState) {
				if newState == network.SyncLive {
					// Fast sync complete — start block sync if needed
					tip, _ := consensus.LoadTip(n.persistent)
					if tip != nil {
						hash, _ := tip.HeaderHash(n.hasher)
						n.setTip(tip.Height, hash)
						n.currentEpoch.Store(tip.Epoch)
					}

					// Start block sync catch-up
					if n.blockSync != nil {
						n.blockSync.Start()
					}

					// Start discovery and gossip
					if n.discovery != nil {
						n.discovery.Start()
					}
					// gossip is passive, triggered by block/tx handlers
				}
			})

			// Start fast sync
			n.fastSync.Start()
		} else if n.blockSync != nil {
			n.blockSync.Start()
		}
	}

	// Start discovery immediately (even without fast sync)
	if n.discovery != nil {
		n.discovery.Start()
	}

	// Register block handler on P2P
	n.p2p.SetBlockHandler(n.handleBlockMessage)

	// Register vote handler on P2P (two-phase voting)
	n.p2p.SetVoteHandler(n.handleVoteMessage)

	go n.runConsensusLoop()
}

// StopConsensus stops the consensus loop.
func (n *Node) StopConsensus() {
	if n.consensusRunning {
		close(n.consensusStopCh)
		n.consensusRunning = false
	}
}

// runConsensusLoop is the main consensus loop running in a goroutine.
func (n *Node) runConsensusLoop() {
	for {
		select {
		case <-n.consensusStopCh:
			return
		default:
		}

		// The target height is derived from the current tip at the start of
		// every iteration instead of being incremented in place. This keeps
		// the loop in sync with blocks applied by other goroutines
		// (applyAcceptedBlock) and guarantees every height is produced
		// exactly once — previously a second height++ in the idle wait
		// skipped even heights, so epoch boundaries were never produced.
		height := n.currentHeight.Load() + 1

		// F1: select the proposer exactly like block validation does — from the
		// ACTIVE staking registry via WeightedProposerAtHeight over ConsensusIDs.
		// The old static operator-address set (n.validators) + ProposerAtHeight
		// never matched validation, so every block this node built was rejected
		// with ErrWrongProposer.
		//
		// PR2: the schedule is round-aware (WeightedProposerAtHeightAndRound);
		// round 0 reproduces the height-only schedule exactly.
		round := n.currentRound(height)
		proposer := n.consensusProposerAtHeightAndRound(height, round)

		if proposer != (types.Address{}) && proposer == n.consensusID() {
			// I am the proposer — run the two-phase proposal round: build and
			// broadcast a proof-less proposal, collect precommits, then attach
			// the commit proof and finalize once a 2/3 majority exists.
			// runProposerRound broadcasts the fresh proposal on its first
			// entry and returns false on later ticks while the round is still
			// pending; those ticks re-broadcast the stored proposal so peers
			// that missed the initial send can still vote on it.
			if !n.runProposerRound(height, proposer, round) {
				n.rebroadcastPendingProposal()
			}
		}

		// PR2 view change: while the round stays unfinalized, the node
		// advances its pending round on the linear ProposerTimeout×(1+r)
		// deadline, capped at MaxRound. Advancing is independent of the
		// proposer role — the schedule decides who proposes each round.
		//
		// The advance runs AFTER the wait so the tick that created the round
		// does not immediately age it: round r lives at least (r+1)×
		// ProposerTimeout from when it became pending.
		select {
		case <-n.consensusStopCh:
			return
		case <-time.After(n.cfg.ProposerTimeout):
		}
		n.maybeAdvanceRound(height)

		// Missed-proposal detection: the height this iteration targeted never
		// materialized while a peer advertises a longer chain — the node is
		// likely behind and should catch up over block ranges instead of
		// waiting for the block-sync engine's periodic tick.
		if n.blockSync != nil && n.currentHeight.Load() < height {
			if peerHeight := n.blockSync.MaxPeerHeight(); peerHeight > n.currentHeight.Load() {
				n.blockSync.NotifyPeerHeight(peerHeight)
			}
		}
	}
}

// runProposerRound runs one two-phase consensus round when this node is the
// proposer for the given height and round. Phase 1 builds and broadcasts a
// proposal (a signed block WITHOUT a commit proof) and prevotes for it. Phase
// 2 attaches the commit proof once a 2/3 precommit majority is collected and
// broadcasts the final block (see tryFinalizeLocked). The node's own precommit
// is gated on a 2/3 prevote majority (see tryPrecommitLocked).
//
// It returns true when this call broadcast a fresh proposal, and false when
// the round is already pending at or above `round` (or could not be started),
// so the consensus loop can re-broadcast the stored proposal on later ticks.
func (n *Node) runProposerRound(height uint64, proposer types.Address, round uint32) bool {
	if n.wallet == nil {
		return false
	}

	n.voteMu.Lock()
	if n.pendingHeight == height && n.pendingBlock != nil && n.pendingBlock.Header.Round >= round {
		n.voteMu.Unlock()
		return false // already proposed this height at this or a higher round
	}
	replace := n.pendingHeight == height
	n.voteMu.Unlock()

	return n.proposeForHeight(height, proposer, round, replace)
}

// proposeForHeight builds a fresh proposal for the given height and round,
// self-validates it through the exact pipeline peers use, (re)establishes the
// pending round and broadcasts the proposal plus this node's prevote. The
// node's own precommit is emitted later, gated on a 2/3 prevote majority
// (tryPrecommitLocked), so a proposer alone can never precommit below quorum.
//
// The proposal is validated with the Snapshot → build → revert → replay
// pattern: the candidate is built in place, the state is reverted, and the
// block is replayed through ValidateBlockProposal. On success the replay's
// state changes are ALSO reverted, so the proposer's state never advances
// ahead of currentHeight — a boundary-height epoch transition is applied
// exactly once, at finalization (finalizeLocalBlock), just like the receiver
// path applies a final commit-proof block.
//
// When replace is true the call replaces an existing pending round for the same
// height instead of skipping it. That is only allowed while the pending
// proposal is still unvoted (no external votes), otherwise the replacement
// would orphan peer votes and deadlock the round.
func (n *Node) proposeForHeight(height uint64, proposer types.Address, round uint32, replace bool) bool {
	if n.wallet == nil {
		return false
	}

	snapID := n.state.Snapshot()

	signer := &walletSigner{kp: n.wallet}
	// TODO: Get evidence from evidence pool
	var evidence []types.Evidence
	block, err := consensus.BuildBlock(n.state, n.vm, n.mempool, height, n.GetTipHash(),
		proposer, signer, n.hasher, n.cfg.MaxTxPerBlock, evidence, n.cfg.BlockTimeSec)
	if err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}
	// Stamp the proposal with its round. Round is part of HeaderHash, so the
	// stamp must happen before the header is hashed/signed — otherwise the
	// proposal would not validate against its own HeaderHash.
	if err := consensus.SetProposalRound(block, round, signer, n.hasher); err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}

	// Revert the build, then replay the block through the exact pipeline peers
	// will use. The replay re-applies the block to prove it validates. The
	// build revert truncates the snapshot list to the snapID slot, so a fresh
	// snapshot must be taken here to keep a valid id for the post-replay
	// revert (RevertToSnapshot rejects out-of-range ids and no-ops).
	if err := n.state.RevertToSnapshot(snapID); err != nil {
		return false
	}
	snapID = n.state.Snapshot()
	parentHeader := &types.BlockHeader{
		Height:       height - 1,
		PreviousHash: block.Header.PreviousHash,
	}
	if err := consensus.ValidateBlockProposal(block, parentHeader, n.GetTipHash(),
		n.state, n.hasher, n.vm, n.cfg.BlockTimeSec); err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}

	// The height may have been finalized by a concurrent goroutine while we
	// were building — drop the candidate in that case.
	if n.currentHeight.Load() >= height {
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}

	hash, _ := block.HeaderHash(n.hasher)

	n.voteMu.Lock()
	if !replace && n.pendingHeight == height {
		n.voteMu.Unlock()
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}
	if replace {
		if n.pendingHeight != height {
			n.voteMu.Unlock()
			_ = n.state.RevertToSnapshot(snapID)
			return false
		}
		// A SAME-round refresh would orphan peer votes on the old hash, so it
		// is blocked while any external vote exists. A ROUND-advance rebuild
		// abandons the old round entirely — those votes are stale and must not
		// block the new proposer (the pendingRound deadline already advanced
		// the view), so it always proceeds.
		sameRound := n.pendingBlock != nil && n.pendingBlock.Header.Round == round
		if sameRound && n.votingHasExternalVotes(n.voting) {
			// A peer voted on the current proposal; refreshing the hash would
			// orphan that vote. Keep the pending round as-is.
			n.voteMu.Unlock()
			_ = n.state.RevertToSnapshot(snapID)
			return false
		}
	}
	// The block was built by BeginBlock in the epoch carried on its header;
	// that is the epoch whose snapshot the block commits against. Capture that
	// snapshot while the replay has it in the working state.
	epoch := block.Header.Epoch
	snap, err := staking.GetSnapshot(n.state, epoch)
	if err != nil || snap == nil {
		n.voteMu.Unlock()
		_ = n.state.RevertToSnapshot(snapID)
		return false
	}
	// Undo the self-validation replay: the proposal must not leave the
	// proposer's state ahead of currentHeight. At a boundary height that would
	// advance the epoch before finalization, so re-validating the echoed
	// proposal — or refreshing it — would run the epoch transition a second
	// time and fail with "epoch mismatch". The block is applied exactly once,
	// in finalizeLocalBlock, mirroring how the receiver path applies a final
	// commit-proof block.
	if err := n.state.RevertToSnapshot(snapID); err != nil {
		n.voteMu.Unlock()
		return false
	}
	vs := consensus.NewVotingState(height, round, hash, snap)
	prevote := n.newVote(height, hash, round, types.VotePrevote)
	_ = vs.AddPrevote(prevote)
	n.voting = vs
	n.pendingBlock = block
	n.pendingHash = hash
	n.pendingHeight = height
	n.pendingRound = round
	n.pendingRoundTicks = 0
	n.precommitSent = false
	n.voteMu.Unlock()

	// Phase 1: broadcast the proposal (no commit proof yet) and our prevote.
	data, err := consensus.EncodeBlockMessage(block)
	if err == nil && n.p2p != nil {
		n.p2p.Broadcast(data)
	}
	n.broadcastVote(prevote)

	// Phase 2: if our own prevote already reaches the 2/3 prevote majority (a
	// single-validator network), precommit and finalize immediately.
	n.voteMu.Lock()
	n.tryPrecommitLocked()
	n.tryFinalizeLocked()
	n.voteMu.Unlock()
	return true
}

// votingHasExternalVotes reports whether any validator other than this node has
// voted in the given voting state. Must be called with voteMu held.
func (n *Node) votingHasExternalVotes(vs *consensus.VotingState) bool {
	if vs == nil {
		return false
	}
	self := n.consensusID()
	for _, v := range vs.Prevotes() {
		if v.Validator != self {
			return true
		}
	}
	for _, v := range vs.Precommits() {
		if v.Validator != self {
			return true
		}
	}
	return false
}

// publishSyncHeight announces this node's current tip to all connected peers.
// Peers use the announcement to start catch-up without waiting for their
// periodic tick; connect-time announcements are handled in p2p registerConn
// via the sync-height provider.
func (n *Node) publishSyncHeight() {
	if n.p2p == nil {
		return
	}
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, n.currentHeight.Load())
	msg := make([]byte, 1+len(payload))
	msg[0] = network.MsgTypeSyncHeight
	copy(msg[1:], payload)
	n.p2p.Broadcast(msg)
}

// rebroadcastPendingProposal re-sends the proposal this node is currently
// waiting on, so peers that connected after the initial broadcast can still
// vote on it. Only the proposer re-broadcasts: the proposer is the one that
// created the VotingState for the pending round (receivers only fill
// pendingBlock/pendingHeight — see handleProposal), so a non-proposer with a
// stored proposal is a voter waiting for the final block, not a broadcaster.
// Re-sending the same block is safe because receivers treat a repeated
// proposal idempotently (handleProposal skips a height it already voted on).
// It is a no-op once the height is finalized or when there is no pending round.
//
// A proposal whose timestamp has left the freshness window can no longer be
// validated by peers (types.ValidateTimestamp rejects blocks older than the
// skew bound), so a same-bytes re-send would be dropped. When that happens and
// nobody has voted on the pending round yet, the proposal is rebuilt with a
// fresh timestamp and hash instead, giving late-connecting peers a proposal
// they can actually vote on.
func (n *Node) rebroadcastPendingProposal() {
	n.voteMu.Lock()
	height := n.pendingHeight
	round := n.pendingRound
	block := n.pendingBlock
	hash := n.pendingHash
	isProposer := n.voting != nil
	stale := block != nil && types.ValidateTimestamp(block.Header.Timestamp) != nil
	hasExternalVotes := n.votingHasExternalVotes(n.voting)
	n.voteMu.Unlock()

	if height == 0 || block == nil || !isProposer {
		return
	}
	if n.currentHeight.Load() >= height {
		return // already finalized
	}

	// A stale, unvoted proposal is useless to peers — rebuild it fresh at the
	// CURRENT pending round (the round may have advanced while this proposal
	// was waiting). The rebuild is only reached from the loop when this node is
	// the round's proposer, so the fresh block is built with the round-aware
	// proposer schedule. The atomic guard in proposeForHeight re-checks the
	// vote set under voteMu, so a vote racing in between this check and the
	// rebuild cannot be orphaned.
	if stale && !hasExternalVotes {
		if n.proposeForHeight(height, n.consensusProposerAtHeightAndRound(height, round), round, true) {
			return
		}
	}

	data, err := consensus.EncodeBlockMessage(block)
	if err != nil || n.p2p == nil {
		return
	}
	log.Printf("[consensus] re-broadcasting proposal for height %d round %d", height, round)
	n.p2p.Broadcast(data)

	// Re-broadcast this node's own prevote alongside the block: the prevote
	// was broadcast only once at proposal time, so a peer that connected (or
	// lost) the initial send never observes the 2/3 prevote majority its
	// precommit is gated on (S7) — and without that precommit this round can
	// never finalize. Vote re-sends are idempotent for receivers (duplicates
	// are dropped), so broadcasting on every pending tick is safe.
	if n.wallet != nil {
		n.broadcastVote(n.newVote(height, hash, round, types.VotePrevote))
	}
}

// handleVoteMessage processes an incoming consensus vote from a peer. Only the
// proposer tracks votes in its VotingState; other validators vote and wait for
// the final block. Votes for unknown heights or rounds (mismatched with the
// pending round) are ignored. A peer precommit only triggers finalization when
// it matches the pending round.
func (n *Node) handleVoteMessage(vote *types.Vote) {
	if vote == nil {
		return
	}
	n.voteMu.Lock()
	defer n.voteMu.Unlock()

	if n.voting == nil || n.pendingHeight != vote.Height || n.pendingRound != vote.Round {
		return
	}

	switch vote.VoteType {
	case types.VotePrevote:
		_ = n.voting.AddPrevote(vote)
		// A peer prevote can raise this node's own prevote majority: precommit
		// (and possibly finalize) if the gate is now satisfied.
		n.tryPrecommitLocked()
	case types.VotePrecommit:
		if err := n.voting.AddPrecommit(vote); err != nil {
			log.Printf("[consensus] precommit rejected for height %d round %d: %v", vote.Height, vote.Round, err)
		} else {
			n.tryFinalizeLocked()
		}
	}
}

// tryPrecommitLocked emits this node's precommit for the pending round once a
// 2/3 prevote majority exists (prevote-gated precommit). It is a no-op when
// the gate is unsatisfied or this node already precommitted, so a proposer
// alone can never precommit below quorum. Must be called with voteMu held.
func (n *Node) tryPrecommitLocked() {
	if n.voting == nil || n.pendingBlock == nil {
		return
	}
	if n.precommitSent {
		return
	}
	if !n.voting.HasPrevoteMajority() {
		return
	}
	height := n.pendingHeight
	round := n.pendingRound
	hash := n.pendingHash
	precommit := n.newVote(height, hash, round, types.VotePrecommit)
	if err := n.voting.AddPrecommit(precommit); err != nil {
		return
	}
	n.precommitSent = true
	n.voteMu.Unlock()
	n.broadcastVote(precommit)
	n.voteMu.Lock()
	n.tryFinalizeLocked()
}

// tryFinalizeLocked attaches the commit proof to the pending block and
// finalizes it once a 2/3 precommit majority exists. Only the proposer
// finalizes, which avoids duplicate final-block broadcasts. Must be called
// with voteMu held.
func (n *Node) tryFinalizeLocked() {
	vs := n.voting
	if vs == nil || n.pendingBlock == nil {
		return
	}
	if n.pendingBlock.Header.Proposer != n.consensusID() {
		return // only the proposer finalizes
	}
	if n.currentHeight.Load() >= n.pendingBlock.Header.Height {
		n.resetPendingLocked()
		return // already finalized by another path
	}
	if !vs.HasPrecommitMajority() {
		return
	}

	proof, err := vs.BuildCommitProof()
	if err != nil {
		return
	}

	block := n.pendingBlock
	block.CommitProof = proof
	n.resetPendingLocked()

	// Broadcast the final block (now carrying the commit proof).
	data, err := consensus.EncodeBlockMessage(block)
	if err == nil && n.p2p != nil {
		n.p2p.Broadcast(data)
	}

	// The block's state changes are already applied by the replay in
	// runProposerRound — this finalizes headers, persistence and the tip.
	n.finalizeLocalBlock(block)
}

// resetPendingLocked clears the pending proposal round. Must be called with
// voteMu held.
func (n *Node) resetPendingLocked() {
	n.voting = nil
	n.pendingBlock = nil
	n.pendingHash = types.Hash{}
	n.pendingHeight = 0
	n.pendingRound = 0
	n.pendingRoundTicks = 0
	n.precommitSent = false
}

// newVote builds and signs a vote for the given height/round/block hash with
// this node's consensus identity. Votes are scoped to (height, round): a vote
// never carries a round it was not created for.
func (n *Node) newVote(height uint64, blockHash types.Hash, round uint32, voteType types.VoteType) *types.Vote {
	vote := &types.Vote{
		VoteType:  voteType,
		Height:    height,
		Round:     round,
		BlockHash: blockHash,
		Validator: n.consensusID(),
	}
	if n.wallet != nil {
		_ = vote.Sign(n.wallet.PrivateKey[:])
	}
	return vote
}

// broadcastVote sends a vote to all connected peers as a framed vote message.
func (n *Node) broadcastVote(vote *types.Vote) {
	if n.p2p == nil || vote == nil {
		return
	}
	var buf bytes.Buffer
	if err := vote.Encode(&buf); err != nil {
		return
	}
	msg := make([]byte, 1, 1+buf.Len())
	msg[0] = network.MsgTypeVote
	n.p2p.Broadcast(append(msg, buf.Bytes()...))
}

// handleProposal records an incoming proof-less proposal, re-gossips it and
// votes for it. Called after the proposal passed ValidateBlockProposal.
//
// Round policy (PR2): proposals are round-scoped. A proposal for a LOWER
// round than the one already pending is ignored. A proposal for a HIGHER
// round is accepted only while the pending round has no votes (prevotes or
// precommits) — once a peer vote is registered, replacing the proposal would
// orphan it and deadlock the round. Accepting a higher round bumps the pending
// round and its view-change deadline so the round deadline restarts.
func (n *Node) handleProposal(block *types.Block) {
	snap, _ := staking.GetSnapshot(n.State(), block.Header.Epoch)
	n.handleProposalWithSnap(block, snap)
}

// handleProposalWithSnap records an incoming proof-less proposal, re-gossips it
// and votes for it. Called after the proposal passed ValidateBlockProposal.
// snap is the validator snapshot for the proposal's epoch (captured by the
// caller before the validation replay is reverted — at an epoch boundary the
// working state's snapshot for the new epoch only exists during that replay).
func (n *Node) handleProposalWithSnap(block *types.Block, snap *staking.ValidatorSnapshot) {
	if block == nil {
		return
	}
	n.voteMu.Lock()
	round := block.Header.Round
	adopt := false
	switch {
	case n.pendingHeight != block.Header.Height:
		// fresh height — adopt the proposal
		adopt = true
	case n.pendingBlock != nil:
		if n.pendingBlock.Header.Round >= round {
			// duplicate or stale round
		} else if n.votingHasExternalVotes(n.voting) {
			// a peer voted on the current round; switching the hash would
			// orphan that vote — drop the higher-round proposal
		} else {
			adopt = true
		}
	default:
		// no proposal held (the round advanced by timeout): adopt only rounds
		// at or above the advanced round counter
		adopt = round >= n.pendingRound
	}
	if !adopt {
		n.voteMu.Unlock()
		return
	}
	hash, _ := block.HeaderHash(n.hasher)
	n.pendingHeight = block.Header.Height
	n.pendingRound = round
	n.pendingRoundTicks = 0
	n.pendingBlock = block
	n.pendingHash = hash
	n.precommitSent = false
	// The receiver tracks votes too: precommits are gated on a 2/3 prevote
	// majority (S7), which requires observing peers' prevotes.
	if snap != nil {
		vs := consensus.NewVotingState(block.Header.Height, round, hash, snap)
		n.voting = vs
	}
	prevote := n.newVote(block.Header.Height, hash, round, types.VotePrevote)
	if n.voting != nil {
		_ = n.voting.AddPrevote(prevote)
	}
	n.voteMu.Unlock()

	if n.gossip != nil {
		if data, err := consensus.EncodeBlockMessage(block); err == nil {
			n.gossip.GossipBlock(data)
		}
	}

	if n.wallet == nil {
		return
	}
	n.broadcastVote(prevote)
	// Own prevote alone may already reach the gate on a single-validator
	// network — precommit (but never finalize: only the proposer finalizes).
	n.voteMu.Lock()
	n.tryPrecommitLocked()
	n.voteMu.Unlock()
}

// finalizeLocalBlock applies and persists a proposer-produced block, updates
// the chain tip and handles epoch-boundary bookkeeping. proposeForHeight only
// self-validates proposals on a reverted state and never leaves the state ahead
// of currentHeight, so the block's state changes are applied HERE — through the
// same ValidateBlock pipeline receivers use in handleBlockMessage. This keeps a
// boundary-height epoch transition running exactly once, on the proposer too.
func (n *Node) finalizeLocalBlock(block *types.Block) {
	// Apply the block to the working state exactly like the receiver path does
	// before accepting a final commit-proof block.
	expectedPrevHash := n.GetTipHash()
	if n.persistent != nil && block.Header.Height > 0 {
		parent, err := consensus.LoadBlockHeader(n.persistent, block.Header.Height-1)
		if err == nil {
			expectedPrevHash, _ = parent.HeaderHash(n.hasher)
		} else {
			expectedPrevHash = types.ZeroHash
		}
	}
	parentHeader := &types.BlockHeader{
		Height:       block.Header.Height - 1,
		PreviousHash: block.Header.PreviousHash,
	}
	snapID := n.state.Snapshot()
	if err := consensus.ValidateBlock(block, parentHeader, expectedPrevHash,
		n.state, n.hasher, n.vm, n.cfg.BlockTimeSec); err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return
	}

	if n.persistent != nil {
		// Persist the FULL block (header + body + tip) atomically so this
		// node can serve its own produced blocks to catching-up peers;
		// header-only storage would leave them invisible to block-range
		// requests and block sync could never converge on them.
		consensus.AtomicStoreBlockAndTip(n.persistent, block)
		// CRITICAL: Persist state to avoid losing changes on crash
		if _, err := n.CommitState(); err != nil {
			panic(fmt.Sprintf("commit state failed: %v at height %d", err, block.Header.Height))
		}
	}

	hash, _ := block.HeaderHash(n.hasher)
	n.setTip(block.Header.Height, hash)

	for _, tx := range block.Transactions {
		n.mempool.Remove(tx.IntentID)
	}

	// Create checkpoint and snapshot at epoch boundaries
	if n.persistent != nil && block.Header.Epoch > 0 && block.Header.Epoch != n.currentEpoch.Load() {
		n.currentEpoch.Store(block.Header.Epoch)

		// Create chain checkpoint (always at epoch boundaries)
		_, _ = consensus.CreateCheckpoint(n.persistent,
			block.Header.Height, block.Header.Epoch,
			block.Header.StateRoot, n.GetTipHash(),
			block.Header.ValidatorSetHash)

		// Create state snapshot if snapshot interval matches
		if n.cfg.SnapshotInterval > 0 &&
			block.Header.Epoch%n.cfg.SnapshotInterval == 0 {
			snap, err := n.state.CreateSnapshot(block.Header.Height,
				block.Header.Epoch, block.Header.ValidatorSetHash,
				block.Header.Timestamp)
			if err == nil {
				snapData, serr := state.SerializeSnapshot(snap)
				if serr == nil {
					_ = state.StoreSnapshot(n.persistent, block.Header.Height, snapData)
				}
			}
		}
	}

	// Announce the new tip to peers so catching-up nodes learn our height. The
	// receiver path already publishes in applyAcceptedBlock; the proposer path
	// advances the tip here and must publish too (single-validator networks
	// would otherwise never announce past the connect-time poke).
	n.publishSyncHeight()
}

// handleBlockMessage processes an incoming block from the network.
func (n *Node) handleBlockMessage(data []byte) {
	block, err := consensus.DecodeBlockMessage(data)
	if err != nil {
		return
	}

	// Track finalized height (k=6 finality)
	if block.Header.Height > 6 {
		n.finalizedHeight = block.Header.Height - 6
	}

	// Reject blocks below finalized height
	if block.Header.Height <= n.finalizedHeight {
		return
	}

	// Fork detection: check if this block extends our current chain
	expectedPrevHash := n.GetTipHash()
	isFork := block.Header.PreviousHash != expectedPrevHash

	if isFork {
		n.handleFork(block)
		return
	}

	// Validate block
	// Load actual parent block header from persistent store to get real PreviousHash
	if n.persistent != nil && block.Header.Height > 0 {
		parentBlock, err := consensus.LoadBlockHeader(n.persistent, block.Header.Height-1)
		if err == nil {
			expectedPrevHash, _ = parentBlock.HeaderHash(n.hasher)
		} else {
			// If parent not found, fall back to zero hash (shouldn't happen in normal operation)
			expectedPrevHash = types.ZeroHash
		}
	}

	parentHeader := &types.BlockHeader{
		Height:       block.Header.Height - 1,
		PreviousHash: block.Header.PreviousHash,
	}

	// A proof-less block is a phase-1 proposal: validate it without the commit
	// proof requirement, revert the check, then vote for it. The real
	// application happens when the proposer broadcasts the final block that
	// carries the commit proof.
	if block.CommitProof == nil {
		if block.Header.Height <= n.currentHeight.Load() {
			return // stale proposal for an already-applied height
		}
		snapID := n.state.Snapshot()
		err := consensus.ValidateBlockProposal(block, parentHeader, expectedPrevHash,
			n.state, n.hasher, n.vm, n.cfg.BlockTimeSec)
		// Capture the proposal's epoch snapshot while the validation replay
		// has it in the working state: at an epoch boundary the new epoch's
		// snapshot only exists during the replay (BeginBlock transitions the
		// epoch and stores it), so it must be grabbed before the revert.
		snap, _ := staking.GetSnapshot(n.state, block.Header.Epoch)
		// The proposal check must not corrupt the working state — always revert.
		_ = n.state.RevertToSnapshot(snapID)
		if err != nil {
			log.Printf("[consensus] proposal validation failed height=%d: %v", block.Header.Height, err)
			return
		}
		n.handleProposalWithSnap(block, snap)
		return
	}

	// Final block carrying the commit proof: validate and apply.
	snapID := n.state.Snapshot()
	err = consensus.ValidateBlock(block, parentHeader, expectedPrevHash,
		n.state, n.hasher, n.vm, n.cfg.BlockTimeSec)
	if err != nil {
		// Revert the partial state mutations from the failed validation.
		_ = n.state.RevertToSnapshot(snapID)
		return
	}

	// Add to reorg buffer (keep last 10 blocks)
	n.reorgBuffer = append(n.reorgBuffer, ReorgEntry{
		Height: block.Header.Height,
		Hash:   n.GetTipHash(),
	})
	if len(n.reorgBuffer) > 10 {
		n.reorgBuffer = n.reorgBuffer[1:]
	}

	// Accept block
	n.applyAcceptedBlock(block)
}

// handleFork handles a potential chain fork.
func (n *Node) handleFork(block *types.Block) {
	// Compute fork weights for comparison
	// Use currentHeight as bestChainHeight
	weightNew := forkWeight(block.Header.Height, n.currentHeight.Load())
	weightCurr := forkWeight(n.currentHeight.Load(), n.currentHeight.Load())

	// Check if new fork is better using fork weight
	if weightNew < weightCurr {
		// New fork would reorg a finalized block - reject and penalize peer
		// Note: In a full implementation, we'd have peer info to penalize
		// For now, we just reject the fork
		return
	}

	// If weights equal, use height as tiebreaker
	if weightNew == weightCurr && block.Header.Height <= n.currentHeight.Load() {
		return // not a better fork
	}

	// Validate the fork block
	// Load actual parent block header to get real PreviousHash
	var expectedPrevHash types.Hash
	if n.persistent != nil && block.Header.Height > 0 {
		parentBlock, err := consensus.LoadBlockHeader(n.persistent, block.Header.Height-1)
		if err == nil {
			expectedPrevHash, _ = parentBlock.HeaderHash(n.hasher)
		} else {
			expectedPrevHash = types.ZeroHash
		}
	}

	parentHeader := &types.BlockHeader{
		Height:       block.Header.Height - 1,
		PreviousHash: block.Header.PreviousHash,
	}

	err := consensus.ValidateBlock(block, parentHeader, expectedPrevHash, n.state, n.hasher, n.vm, n.cfg.BlockTimeSec)
	if err != nil {
		return
	}

	n.applyAcceptedBlock(block)
}

// applyAcceptedBlock applies a validated block and persists state.
func (n *Node) applyAcceptedBlock(block *types.Block) {
	hash, _ := block.HeaderHash(n.hasher)
	n.setTip(block.Header.Height, hash)

	// Store atomically
	if n.persistent != nil {
		consensus.AtomicStoreBlockAndTip(n.persistent, block)
		// CRITICAL: Persist state to avoid losing changes on crash
		if _, err := n.CommitState(); err != nil {
			panic(fmt.Sprintf("commit state failed: %v at height %d", err, block.Header.Height))
		}
	}

	// Index the block if indexer is enabled
	if n.indexer != nil && n.indexer.IsEnabled() {
		blockHash, _ := block.HeaderHash(n.hasher)
		txns := make([]*types.Transaction, len(block.Transactions))
		for i := 0; i < len(block.Transactions); i++ {
			txns[i] = &block.Transactions[i]
		}
		events := make([]*types.Event, len(block.Events))
		for i := 0; i < len(block.Events); i++ {
			events[i] = &block.Events[i]
		}

		n.indexer.Enqueue(&indexer.IndexableBlock{
			Number:    block.Header.Height,
			Hash:      blockHash,
			Header:    &block.Header,
			Txns:      txns,
			Events:    events,
			StateRoot: block.Header.StateRoot,
		})
	}

	// Clean mempool
	for _, tx := range block.Transactions {
		n.mempool.Remove(tx.IntentID)
	}

	// Gossip block to peers
	if n.gossip != nil {
		data, err := consensus.EncodeBlockMessage(block)
		if err == nil {
			n.gossip.GossipBlock(data)
		}
	}

	// Create checkpoint at epoch boundaries
	if n.persistent != nil && block.Header.Epoch > 0 && block.Header.Epoch != n.currentEpoch.Load() {
		n.currentEpoch.Store(block.Header.Epoch)
		_, _ = consensus.CreateCheckpoint(n.persistent,
			block.Header.Height, block.Header.Epoch,
			block.Header.StateRoot, n.GetTipHash(),
			block.Header.ValidatorSetHash)

		if n.cfg.SnapshotInterval > 0 &&
			block.Header.Epoch%n.cfg.SnapshotInterval == 0 {
			snap, _ := n.state.CreateSnapshot(block.Header.Height,
				block.Header.Epoch, block.Header.ValidatorSetHash,
				block.Header.Timestamp)
			if snap != nil {
				snapData, _ := state.SerializeSnapshot(snap)
				if snapData != nil {
					state.StoreSnapshot(n.persistent, block.Header.Height, snapData)
				}
			}
		}
	}

	// Announce the new tip to peers so late joiners learn our height (spec:
	// publication after applyAcceptedBlock advances the tip). Runs after
	// setTip so the published height is the applied tip.
	n.publishSyncHeight()
}
