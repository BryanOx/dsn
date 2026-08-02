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

	// Sync state
	lastSyncedHeight uint64
	targetHeight     uint64
	isSyncing        bool

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

	// Load persisted progress
	e.lastSyncedHeight = e.loadProgress()

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

// syncLoop periodically checks if we need to catch up with peers.
func (e *BlockSyncEngine) syncLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.startCatchUp()
		}
	}
}

// HandleBlockRangeRequest processes an incoming block range request.
// Returns serialized response payload containing the requested blocks.
func (e *BlockSyncEngine) HandleBlockRangeRequest(payload []byte) []byte {
	startHeight, endHeight, err := decodeBlockRangeRequest(payload)
	if err != nil {
		return nil
	}

	// Validate request
	if startHeight > endHeight {
		return nil
	}
	if endHeight-startHeight > 100 {
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
	return encodeBlockList(blocks)
}

// HandleBlockRangeResponse processes an incoming block range response.
func (e *BlockSyncEngine) HandleBlockRangeResponse(payload []byte, from PeerID) {
	blocks, err := decodeBlockList(payload)
	if err != nil {
		e.pm.ReportFailure(from, 2) // malformed message
		return
	}

	e.mu.Lock()
	handler := e.onBlock
	e.mu.Unlock()

	if handler == nil {
		return
	}

	successCount := 0

	for _, blockData := range blocks {
		accepted, err := handler(blockData)
		if err != nil || !accepted {
			// Validation failed - penalize peer and stop processing
			e.pm.ReportFailure(from, 5) // invalid block
			return
		}
		successCount++

		// Try to extract height from block for tracking
		if len(blockData) > 10 {
			// Block data starts with type (1) + len (4) + data
			// The actual block structure varies, so we'll track by count
		}
	}

	if successCount > 0 {
		e.pm.ReportSuccess(from, MsgTypeBlock)
		e.mu.Lock()
		// Update lastSyncedHeight based on number of blocks received
		e.lastSyncedHeight += uint64(successCount)
		e.mu.Unlock()

		// Persist progress every 100 blocks
		if e.lastSyncedHeight%100 == 0 {
			e.persistProgress()
		}
	}
}

// startCatchUp begins the block sync process.
func (e *BlockSyncEngine) startCatchUp() {
	e.mu.Lock()
	if e.isSyncing {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	// Get current tip height
	tip, err := consensus.LoadTip(e.persistent)
	currentHeight := uint64(0)
	if err == nil && tip != nil {
		currentHeight = tip.Height
	}

	// Get best peers and their heights
	bestPeers := e.pm.GetBestPeers(10)
	if len(bestPeers) == 0 {
		return
	}

	// Find highest peer height
	maxPeerHeight := currentHeight
	var targetPeer *Peer
	for _, p := range bestPeers {
		p.mu.RLock()
		if p.SyncHeight > maxPeerHeight {
			maxPeerHeight = p.SyncHeight
			targetPeer = p
		}
		p.mu.RUnlock()
	}

	if maxPeerHeight <= currentHeight {
		return // already synced
	}

	e.mu.Lock()
	e.targetHeight = maxPeerHeight
	e.isSyncing = true
	e.mu.Unlock()

	// Request batches from peer
	for from := currentHeight + 1; from <= maxPeerHeight; from += 100 {
		to := from + 99
		if to > maxPeerHeight {
			to = maxPeerHeight
		}

		// Request from best available peer
		if targetPeer != nil {
			e.requestBlockRange(targetPeer.Address, from, to)
		}

		// Wait a bit between batches
		select {
		case <-e.stopCh:
			e.mu.Lock()
			e.isSyncing = false
			e.mu.Unlock()
			return
		case <-time.After(500 * time.Millisecond):
		}
	}

	e.mu.Lock()
	e.lastSyncedHeight = e.targetHeight
	e.isSyncing = false
	e.mu.Unlock()

	// Persist final progress
	e.persistProgress()
}

// requestBlockRange sends a block range request to a peer.
func (e *BlockSyncEngine) requestBlockRange(addr string, startHeight, endHeight uint64) error {
	payload := encodeBlockRangeRequest(startHeight, endHeight)
	msg := FrameMessage(MsgTypeBlockRangeRequest, payload)
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
