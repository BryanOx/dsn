package network

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/dsn/dsn/consensus"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// BlockSyncEngine handles block synchronization with peers using range requests.
type BlockSyncEngine struct {
	pm         *PeerManager
	p2p        *P2PNode
	persistent *state.PersistentState
	mu         sync.RWMutex
	stopCh     chan struct{}
	notifyCh   chan struct{}

	// Sync state
	lastSyncedHeight uint64
	targetHeight     uint64
	isSyncing        bool
	targetPeerAddr   string // peer currently serving the active catch-up

	// Callbacks
	onBlock func(data []byte) (bool, error) // validate and apply block, returns (accepted, error)

	// Pending requests tracking
	pendingRequests map[uint64]time.Time // height -> request time for timeout handling
}

// blockSyncBucket is the BoltDB bucket name for block sync progress persistence.
var blockSyncBucket = []byte("block_sync")

// lastSyncedHeightKey is the key for storing last synced height.
var lastSyncedHeightKey = []byte("last_synced_height")

// NewBlockSyncEngine creates a new BlockSyncEngine.
func NewBlockSyncEngine(pm *PeerManager, p2p *P2PNode, persistent *state.PersistentState) *BlockSyncEngine {
	return &BlockSyncEngine{
		pm:               pm,
		p2p:              p2p,
		persistent:       persistent,
		stopCh:           make(chan struct{}),
		notifyCh:         make(chan struct{}, 1),
		lastSyncedHeight: 0,
		targetHeight:     0,
		isSyncing:        false,
		pendingRequests:  make(map[uint64]time.Time),
	}
}

// Start begins the block sync engine background goroutines.
func (e *BlockSyncEngine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Start from the last applied height: max(on-disk tip, persisted progress).
	e.lastSyncedHeight = e.resumeFrom() - 1

	// Start background sync checker
	go e.syncLoop()
}

// Stop shuts down the block sync engine.
func (e *BlockSyncEngine) Stop() {
	close(e.stopCh)
}

// IsSyncing returns whether the engine is currently syncing.
func (e *BlockSyncEngine) IsSyncing() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isSyncing
}

// LastSyncedHeight returns the height of the last synced block.
func (e *BlockSyncEngine) LastSyncedHeight() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastSyncedHeight
}

// SetBlockHandler registers a callback for processing incoming blocks.
func (e *BlockSyncEngine) SetBlockHandler(h func(data []byte) (bool, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onBlock = h
}

// syncLoop periodically checks if we need to catch up with peers, and also
// reacts immediately to fresh SyncHeight announcements via NotifyPeerHeight.
func (e *BlockSyncEngine) syncLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.startCatchUp()
		case <-e.notifyCh:
			e.startCatchUp()
		}
	}
}

// NotifyPeerHeight wakes the sync loop so catch-up starts without waiting for
// the next 30s tick. Coalesced: a wake-up already pending is not duplicated.
func (e *BlockSyncEngine) NotifyPeerHeight(height uint64) {
	select {
	case e.notifyCh <- struct{}{}:
	default:
	}
}

// MaxPeerHeight returns the highest SyncHeight advertised by any peer with a
// live connection.
func (e *BlockSyncEngine) MaxPeerHeight() uint64 {
	if p := e.bestPeer(); p != nil {
		p.mu.RLock()
		defer p.mu.RUnlock()
		return p.SyncHeight
	}
	return 0
}

// bestPeer returns the live peer advertising the highest SyncHeight, or nil.
func (e *BlockSyncEngine) bestPeer() *Peer {
	e.pm.mu.RLock()
	defer e.pm.mu.RUnlock()

	var best *Peer
	var max uint64
	for _, p := range e.pm.peers {
		p.mu.RLock()
		if p.Conn != nil && p.SyncHeight > max {
			max = p.SyncHeight
			best = p
		}
		p.mu.RUnlock()
	}
	return best
}

// resumeFrom returns the next height to fetch: max(on-disk tip, persisted
// progress) + 1. It never re-requests a block at or below the applied tip.
func (e *BlockSyncEngine) resumeFrom() uint64 {
	tip := uint64(0)
	if e.persistent != nil && e.persistent.DB() != nil {
		if tipHeader, err := consensus.LoadTip(e.persistent); err == nil && tipHeader != nil {
			tip = tipHeader.Height
		}
	}
	progress := e.loadProgress()
	resume := tip
	if progress > resume {
		resume = progress
	}
	return resume + 1
}

// HandleBlockRangeRequest processes an incoming block range request.
// Returns serialized response payload containing the requested blocks.
func (e *BlockSyncEngine) HandleBlockRangeRequest(payload []byte, from PeerID) []byte {
	startHeight, endHeight, err := decodeBlockRangeRequest(payload)
	if err != nil {
		e.pm.ReportFailure(from, 2) // malformed request
		return nil
	}

	// Validate request
	if startHeight > endHeight {
		e.pm.ReportFailure(from, 2) // malformed request
		return nil
	}
	if endHeight-startHeight >= uint64(MaxBlocksPerRangeResponse) {
		return nil
	}

	// Load blocks from persistent state
	var blocks [][]byte
	for height := startHeight; height <= endHeight; height++ {
		block, err := consensus.LoadBlock(e.persistent, height)
		if err != nil {
			// Block not available, skip
			continue
		}

		// Encode block as wire format
		blockData, err := encodeBlockAsWireFromBlock(block)
		if err != nil {
			continue
		}
		blocks = append(blocks, blockData)
	}

	// Serialize block list
	resp := encodeBlockList(blocks)
	return resp
}

// HandleBlockRangeResponse processes an incoming block range response.
func (e *BlockSyncEngine) HandleBlockRangeResponse(payload []byte, from PeerID) {
	blocks, err := decodeBlockList(payload)
	if err != nil {
		e.pm.ReportFailure(from, 2) // malformed message
		return
	}

	e.mu.RLock()
	handler := e.onBlock
	target := e.targetHeight
	prev := e.lastSyncedHeight
	e.mu.RUnlock()

	if handler == nil {
		return
	}

	applied := 0

	for _, blockData := range blocks {
		accepted, err := handler(blockData)
		if err != nil || !accepted {
			// Validation failed - penalize peer and stop processing
			e.pm.ReportFailure(from, 5) // invalid block
			e.mu.Lock()
			e.isSyncing = false
			e.mu.Unlock()
			return
		}
		applied++
	}

	if applied == 0 {
		// Nothing advanced this round; stop so the next ticker or peer
		// announcement re-evaluates with fresh targets.
		e.mu.Lock()
		e.isSyncing = false
		e.mu.Unlock()
		return
	}

	e.pm.ReportSuccess(from, MsgTypeBlock)

	// D3: progress is the on-disk tip height after applying, not a block count.
	if e.persistent != nil && e.persistent.DB() != nil {
		if tip, err := consensus.LoadTip(e.persistent); err == nil && tip != nil {
			e.mu.Lock()
			e.lastSyncedHeight = tip.Height
			e.mu.Unlock()
			// Persist per batch (at most MaxBlocksPerRangeResponse blocks).
			e.persistProgress()
		}
	}

	e.mu.RLock()
	last := e.lastSyncedHeight
	syncing := e.isSyncing
	addr := e.targetPeerAddr
	e.mu.RUnlock()

	if !syncing {
		return
	}

	if last >= target {
		// Catch-up complete: clear persisted progress and stop syncing.
		e.mu.Lock()
		e.isSyncing = false
		e.mu.Unlock()
		e.clearProgress()
		return
	}

	if last == prev {
		// No tip advance (e.g. no persistent state): cannot make progress,
		// stop so a later tick re-evaluates instead of hot-looping.
		e.mu.Lock()
		e.isSyncing = false
		e.mu.Unlock()
		return
	}

	// Request the next window from the peer serving the catch-up.
	if addr != "" {
		e.requestWindow(addr, last+1, target)
	}
}

// startCatchUp begins the block sync process. It is async: it picks the resume
// height and target, then requests one window; each response advances the
// window until the target is reached.
func (e *BlockSyncEngine) startCatchUp() {
	e.mu.Lock()
	if e.isSyncing {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	// Resume from max(on-disk tip, persisted progress) + 1.
	resume := e.resumeFrom()

	// Pick the peer advertising the highest height.
	targetPeer := e.bestPeer()
	if targetPeer == nil {
		return
	}
	targetPeer.mu.RLock()
	target := targetPeer.SyncHeight
	targetPeer.mu.RUnlock()

	if target < resume {
		return // already caught up
	}

	e.mu.Lock()
	e.targetHeight = target
	e.targetPeerAddr = targetPeer.Address
	e.isSyncing = true
	e.mu.Unlock()

	// Request the first window [resume, resume+99].
	e.requestWindow(targetPeer.Address, resume, target)
}

// requestWindow sends a block range request for the next window of at most 100
// blocks, capped at the target height.
func (e *BlockSyncEngine) requestWindow(addr string, from, target uint64) {
	to := from + (uint64(MaxBlocksPerRangeResponse) - 1)
	if to > target {
		to = target
	}
	_ = e.requestBlockRange(addr, from, to)
}

// requestBlockRange sends a block range request to a peer.
func (e *BlockSyncEngine) requestBlockRange(addr string, startHeight, endHeight uint64) error {
	payload := encodeBlockRangeRequest(startHeight, endHeight)
	msg := make([]byte, 1+len(payload))
	msg[0] = MsgTypeBlockRangeRequest
	copy(msg[1:], payload)
	return e.p2p.SendTo(addr, msg)
}

// persistProgress saves the last synced height to persistent state.
func (e *BlockSyncEngine) persistProgress() {
	if e.persistent == nil || e.persistent.DB() == nil {
		return
	}

	e.mu.RLock()
	height := e.lastSyncedHeight
	e.mu.RUnlock()

	e.persistent.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(blockSyncBucket)
		if err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, height)
		return b.Put(lastSyncedHeightKey, key)
	})
}

// clearProgress removes persisted sync progress after catch-up completes, so a
// later restart does not resume from stale progress.
func (e *BlockSyncEngine) clearProgress() {
	if e.persistent == nil || e.persistent.DB() == nil {
		return
	}

	e.persistent.DB().Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(blockSyncBucket)
		if b == nil {
			return nil
		}
		return b.Delete(lastSyncedHeightKey)
	})
}

// loadProgress reads the last synced height from persistent state.
func (e *BlockSyncEngine) loadProgress() uint64 {
	if e.persistent == nil || e.persistent.DB() == nil {
		return 0
	}

	var height uint64
	e.persistent.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(blockSyncBucket)
		if b == nil {
			return nil
		}
		data := b.Get(lastSyncedHeightKey)
		if data != nil && len(data) == 8 {
			height = binary.BigEndian.Uint64(data)
		}
		return nil
	})
	return height
}

// encodeBlockRangeRequest serializes a block range request.
// Format: [startHeight(8)][endHeight(8)] = 16 bytes.
func encodeBlockRangeRequest(startHeight, endHeight uint64) []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:8], startHeight)
	binary.BigEndian.PutUint64(data[8:16], endHeight)
	return data
}

// decodeBlockRangeRequest deserializes a block range request.
func decodeBlockRangeRequest(data []byte) (startHeight, endHeight uint64, err error) {
	if len(data) < 16 {
		return 0, 0, fmt.Errorf("payload too short: need 16 bytes, got %d", len(data))
	}
	startHeight = binary.BigEndian.Uint64(data[0:8])
	endHeight = binary.BigEndian.Uint64(data[8:16])
	return startHeight, endHeight, nil
}

// encodeBlockList serializes a list of blocks for range response.
// Format: [count(4)][len1(4)][block1_data][len2(4)][block2_data]...
func encodeBlockList(blocks [][]byte) []byte {
	// Calculate total size
	totalSize := 4 // count
	for _, block := range blocks {
		totalSize += 4 + len(block) // len + data
	}

	buf := make([]byte, 0, totalSize)

	// Write count
	countBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(countBuf, uint32(len(blocks)))
	buf = append(buf, countBuf...)

	// Write each block with length prefix
	for _, block := range blocks {
		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(block)))
		buf = append(buf, lenBuf...)
		buf = append(buf, block...)
	}

	return buf
}

// decodeBlockList deserializes a block list from range response.
func decodeBlockList(data []byte) ([][]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("payload too short: need at least 4 bytes for count")
	}

	count := binary.BigEndian.Uint32(data[0:4])
	if count > MaxBlocksPerRangeResponse {
		return nil, fmt.Errorf("too many blocks in response: %d", count)
	}

	offset := 4
	blocks := make([][]byte, 0, count)

	for i := 0; i < int(count); i++ {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("truncated block length at index %d", i)
		}

		blockLen := binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4

		if offset+int(blockLen) > len(data) {
			return nil, fmt.Errorf("truncated block data at index %d", i)
		}

		blockData := make([]byte, blockLen)
		copy(blockData, data[offset:offset+int(blockLen)])
		blocks = append(blocks, blockData)
		offset += int(blockLen)
	}

	return blocks, nil
}

// encodeBlockAsWireFromBlock encodes a full block for wire transfer.
// Format: [type(1)][length(4)][data...]
func encodeBlockAsWireFromBlock(block *types.Block) ([]byte, error) {
	// Encode the block using gossip format
	encoded, err := consensus.EncodeBlockMessage(block)
	if err != nil {
		return nil, err
	}
	// Strip the first byte (message type) since we add our own
	if len(encoded) < 1 {
		return nil, fmt.Errorf("encoded block too short")
	}
	return encoded[1:], nil
}
