package indexer

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/dsn/dsn/telemetry"
	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// Indexer is a BoltDB-backed indexer for DSN blocks, transactions, and events.
type Indexer struct {
	db      *bbolt.DB
	ch      chan *IndexableBlock
	done    chan struct{}
	height  uint64 // latest indexed height
	enabled bool
	mu      sync.RWMutex

	// blockRetriever is used for recovery/catch-up to fetch blocks from node
	blockRetriever BlockRetriever
}

// BlockRetriever interface for fetching blocks during catch-up.
type BlockRetriever interface {
	GetBlock(height uint64) (*types.Block, error)
}

// New creates a new Indexer instance.
// Returns nil if dataDir is empty (indexer disabled).
func New(dataDir string, enabled bool) (*Indexer, error) {
	if !enabled || dataDir == "" {
		return nil, nil
	}

	dbPath := filepath.Join(dataDir, "indexer.db")
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open indexer db: %w", err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init indexer schema: %w", err)
	}

	// Get latest indexed height
	height, _ := getLatestIndexedHeight(db)

	idx := &Indexer{
		db:      db,
		ch:      make(chan *IndexableBlock, 100), // buffer of 100
		done:    make(chan struct{}),
		height:  height,
		enabled: true,
	}

	return idx, nil
}

// Start launches the indexing goroutine.
func (idx *Indexer) Start() {
	if idx == nil || !idx.enabled {
		return
	}

	go idx.run()
	log.Printf("Indexer started at height %d", idx.height)
}

// Stop gracefully shuts down the indexer.
func (idx *Indexer) Stop() {
	if idx == nil || !idx.enabled {
		return
	}

	close(idx.ch)
	<-idx.done

	if err := idx.db.Close(); err != nil {
		log.Printf("Error closing indexer DB: %v", err)
	}
	log.Printf("Indexer stopped at height %d", idx.height)
}

// Enqueue adds a block to the indexing queue.
// Non-blocking: if channel is full, the call will block temporarily.
func (idx *Indexer) Enqueue(ib *IndexableBlock) {
	if idx == nil || !idx.enabled {
		return
	}
	idx.ch <- ib
}

// run is the main indexing loop that processes blocks from the channel.
func (idx *Indexer) run() {
	defer close(idx.done)

	for {
		select {
		case <-idx.done:
			return
		case ib, ok := <-idx.ch:
			if !ok {
				// Channel closed, exit
				return
			}

			// Index the block
			if err := idx.indexBlock(ib); err != nil {
				log.Printf("Error indexing block %d: %v", ib.Number, err)
				continue
			}

			// Update height
			idx.mu.Lock()
			if ib.Number > idx.height {
				idx.height = ib.Number
				// Update metrics
				telemetry.IndexerHeight.Set(float64(ib.Number))
			}
			idx.mu.Unlock()

			// Update indexer lag if node height is available
			if idx.blockRetriever != nil {
				if _, err := idx.blockRetriever.GetBlock(ib.Number); err == nil {
					// Node height is the same as this block since we just indexed it
					// For more accurate lag, we'd need to get current node height
					// For now, set lag to 0 when we're caught up
					telemetry.IndexerLag.Set(0)
				}
			}
		}
	}
}

// indexBlock indexes all data for a single block in a single transaction.
func (idx *Indexer) indexBlock(ib *IndexableBlock) error {
	return idx.db.Update(func(tx *bbolt.Tx) error {
		// 1. Index block header
		blocks, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}

		data := BlockData{
			Header:     ib.Header,
			TxCount:    len(ib.Txns),
			EventCount: len(ib.Events),
		}

		serialized, err := json.Marshal(data)
		if err != nil {
			return err
		}

		blockKey := encodeBlockNumber(ib.Number)
		if err := blocks.Put(blockKey, serialized); err != nil {
			return err
		}

		// 2. Index transactions
		transactions, err := tx.CreateBucketIfNotExists(transactionsBucket)
		if err != nil {
			return err
		}

		for _, tx := range ib.Txns {
			receipt := TransactionReceipt{
				Hash:        tx.IntentID,
				Data:        tx,
				Status:      true,
				GasUsed:     tx.GasLimit,
				BlockNumber: ib.Number,
			}

			txSerialized, err := json.Marshal(receipt)
			if err != nil {
				return err
			}

			if err := transactions.Put(tx.IntentID[:], txSerialized); err != nil {
				return err
			}
		}

		// 3. Index events
		events, err := tx.CreateBucketIfNotExists(eventsBucket)
		if err != nil {
			return err
		}

		for _, event := range ib.Events {
			eventData := EventData{
				Event:       event,
				BlockNumber: ib.Number,
			}

			eventSerialized, err := json.Marshal(eventData)
			if err != nil {
				return err
			}

			eventKey := EventsIndexKey(event.ContractID, event.Topic, ib.Number, event.TxIndex)
			if err := events.Put(eventKey, eventSerialized); err != nil {
				return err
			}
		}

		// 4. Update latest indexed height
		state, err := tx.CreateBucketIfNotExists(stateBucket)
		if err != nil {
			return err
		}

		return state.Put(latestIndexedHeightKey, Uint64ToBigEndian(ib.Number))
	})
}

// SetBlockRetriever sets the block retriever for catch-up recovery.
func (idx *Indexer) SetBlockRetriever(br BlockRetriever) {
	if idx != nil {
		idx.blockRetriever = br
	}
}

// IsEnabled returns whether the indexer is enabled.
func (idx *Indexer) IsEnabled() bool {
	if idx == nil {
		return false
	}
	return idx.enabled
}

// Height returns the latest indexed height.
func (idx *Indexer) Height() uint64 {
	if idx == nil {
		return 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.height
}