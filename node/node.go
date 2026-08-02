package node

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/genesis"
	"github.com/dsn/dsn/indexer"
	"github.com/dsn/dsn/mempool"
	"github.com/dsn/dsn/network"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
	"github.com/dsn/dsn/wallet"
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
	// pending round: voting (VotingState), pendingBlock and pendingHash.
	voteMu        sync.Mutex
	voting        *consensus.VotingState
	pendingBlock  *types.Block
	pendingHash   types.Hash
	pendingHeight uint64

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
			return state.RestoreFromSnapshot(persistent, data, expectedHash)
		})
		n.fastSync.SetReplayHandler(func(fromHeight uint64) error {
			return n.ReplayBlocks(fromHeight, n.currentHeight.Load()) // tip will be determined by FastSyncEngine
		})

		// Register with P2PNode
		p2pNode.SetFastSyncEngine(n.fastSync)
		p2pNode.SetBlockSyncEngine(n.blockSync)
		p2pNode.SetGossipEngine(n.gossip)

		// Create PeerDiscovery if bootstrap peers configured
		if len(cfg.BootstrapPeers) > 0 {
			n.discovery = network.NewPeerDiscovery(n.p2p.PeerManager(), cfg.BootstrapPeers, p2pNode)
		}
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
	if n.wallet == nil {
		return types.Address{}
	}
	active, err := staking.GetActiveValidators(n.State())
	if err != nil || len(active) == 0 {
		return types.Address{}
	}
	return consensus.WeightedProposerAtHeight(height, active)
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

		// Fast sync: replace stub with FastSyncEngine
		if n.cfg.FastSyncEnabled && n.fastSync != nil {
			// Set up state change handler to detect when sync completes
			n.fastSync.SetStateChangeHandler(func(oldState, newState network.SyncState) {
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
		proposer := n.consensusProposerAtHeight(height)

		if proposer != (types.Address{}) && proposer == n.consensusID() {
			// I am the proposer — run the two-phase proposal round: build and
			// broadcast a proof-less proposal, collect precommits, then attach
			// the commit proof and finalize once a 2/3 majority exists.
			n.runProposerRound(height, proposer)
		}

		// Wait before next check
		select {
		case <-n.consensusStopCh:
			return
		case <-time.After(n.cfg.ProposerTimeout):
		}
	}
}

// runProposerRound runs the two-phase consensus round when this node is the
// proposer for the given height. Phase 1 builds and broadcasts a proposal (a
// signed block WITHOUT a commit proof) and votes for it. Phase 2 attaches the
// commit proof once a 2/3 precommit majority is collected and broadcasts the
// final block (see tryFinalizeLocked).
//
// BuildBlock applies the block's state mutations in place, so the proposal is
// self-validated with the Snapshot → build → revert → validate pattern: the
// candidate is built, the state is reverted to before the build, and the block
// is replayed through ValidateBlockProposal. Validation both proves the block
// is acceptable to peers and leaves the state at the post-block height — the
// replay IS the application. On failure the state is reverted and the height
// is skipped.
func (n *Node) runProposerRound(height uint64, proposer types.Address) {
	if n.wallet == nil {
		return
	}

	n.voteMu.Lock()
	if n.pendingHeight == height {
		n.voteMu.Unlock()
		return // already proposed this height
	}
	n.voteMu.Unlock()

	snapID := n.state.Snapshot()

	signer := &walletSigner{kp: n.wallet}
	// TODO: Get evidence from evidence pool
	var evidence []types.Evidence
	block, err := consensus.BuildBlock(n.state, n.vm, n.mempool, height, n.GetTipHash(),
		proposer, signer, n.hasher, n.cfg.MaxTxPerBlock, evidence, n.cfg.BlockTimeSec)
	if err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return
	}

	// Revert the build, then replay the block through the exact pipeline peers
	// will use. If the proposal is valid the replay re-applies the block and
	// the state is left at the post-block height.
	if err := n.state.RevertToSnapshot(snapID); err != nil {
		return
	}
	parentHeader := &types.BlockHeader{
		Height:       height - 1,
		PreviousHash: block.Header.PreviousHash,
	}
	if err := consensus.ValidateBlockProposal(block, parentHeader, n.GetTipHash(),
		n.state, n.hasher, n.vm, n.cfg.BlockTimeSec); err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return
	}

	// The height may have been finalized by a concurrent goroutine while we
	// were building — drop the candidate in that case.
	if n.currentHeight.Load() >= height {
		_ = n.state.RevertToSnapshot(snapID)
		return
	}

	hash, _ := block.HeaderHash(n.hasher)

	n.voteMu.Lock()
	if n.pendingHeight == height {
		n.voteMu.Unlock()
		_ = n.state.RevertToSnapshot(snapID)
		return
	}
	// The block was built by BeginBlock in the epoch carried on its header;
	// that is the epoch whose snapshot the block commits against.
	epoch := block.Header.Epoch
	snap, err := staking.GetSnapshot(n.state, epoch)
	if err != nil || snap == nil {
		n.voteMu.Unlock()
		_ = n.state.RevertToSnapshot(snapID)
		return
	}
	vs := consensus.NewVotingState(height, 0, hash, snap)
	prevote := n.newVote(height, hash, types.VotePrevote)
	precommit := n.newVote(height, hash, types.VotePrecommit)
	_ = vs.AddPrevote(prevote)
	_ = vs.AddPrecommit(precommit)
	n.voting = vs
	n.pendingBlock = block
	n.pendingHash = hash
	n.pendingHeight = height
	n.voteMu.Unlock()

	// Phase 1: broadcast the proposal (no commit proof yet) and our votes.
	data, err := consensus.EncodeBlockMessage(block)
	if err == nil && n.p2p != nil {
		n.p2p.Broadcast(data)
	}
	n.broadcastVote(prevote)
	n.broadcastVote(precommit)

	// Phase 2: if our own votes already reach the 2/3 precommit majority (a
	// single-validator network), finalize immediately.
	n.voteMu.Lock()
	n.tryFinalizeLocked()
	n.voteMu.Unlock()
}

// handleVoteMessage processes an incoming consensus vote from a peer. Only the
// proposer tracks votes in its VotingState; other validators vote and wait for
// the final block. Votes for unknown or already-finalized rounds are ignored.
func (n *Node) handleVoteMessage(vote *types.Vote) {
	if vote == nil {
		return
	}
	n.voteMu.Lock()
	defer n.voteMu.Unlock()

	if n.voting == nil || n.pendingHeight != vote.Height {
		return
	}

	switch vote.VoteType {
	case types.VotePrevote:
		_ = n.voting.AddPrevote(vote)
	case types.VotePrecommit:
		if n.voting.AddPrecommit(vote) == nil {
			n.tryFinalizeLocked()
		}
	}
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
}

// newVote builds and signs a vote for the given height/block hash with this
// node's consensus identity.
func (n *Node) newVote(height uint64, blockHash types.Hash, voteType types.VoteType) *types.Vote {
	vote := &types.Vote{
		VoteType:  voteType,
		Height:    height,
		Round:     0,
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
func (n *Node) handleProposal(block *types.Block) {
	n.voteMu.Lock()
	if n.pendingHeight == block.Header.Height {
		n.voteMu.Unlock()
		return // already voted for this round
	}
	hash, _ := block.HeaderHash(n.hasher)
	n.pendingHeight = block.Header.Height
	n.pendingBlock = block
	n.pendingHash = hash
	n.voteMu.Unlock()

	if n.gossip != nil {
		if data, err := consensus.EncodeBlockMessage(block); err == nil {
			n.gossip.GossipBlock(data)
		}
	}

	if n.wallet == nil {
		return
	}
	n.broadcastVote(n.newVote(block.Header.Height, hash, types.VotePrevote))
	n.broadcastVote(n.newVote(block.Header.Height, hash, types.VotePrecommit))
}

// finalizeLocalBlock persists a proposer-produced block, updates the chain tip
// and handles epoch-boundary bookkeeping. The block's state changes are already
// applied in n.state (by BuildBlock plus the self-validation replay), so this
// only finalizes headers, persistence and the tip.
func (n *Node) finalizeLocalBlock(block *types.Block) {
	if n.persistent != nil {
		consensus.StoreBlockHeader(n.persistent, &block.Header)
		consensus.StoreTip(n.persistent, &block.Header)
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
		// The proposal check must not corrupt the working state — always revert.
		_ = n.state.RevertToSnapshot(snapID)
		if err != nil {
			return
		}
		n.handleProposal(block)
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
}
