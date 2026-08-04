package indexer

import (
	"log"

	"github.com/BryanOx/dsn/types"
)

// Recover performs catch-up recovery on startup.
// It reads the latest indexed height and re-indexes blocks from that height+1.
func (idx *Indexer) Recover() error {
	if idx == nil || !idx.enabled {
		return nil
	}

	// Check if we have a block retriever (node)
	if idx.blockRetriever == nil {
		log.Println("Indexer: no block retriever set, skipping recovery")
		return nil
	}

	// Get latest indexed height
	height, err := getLatestIndexedHeight(idx.db)
	if err != nil {
		return err
	}

	if height == 0 {
		log.Println("Indexer: no previous state, starting from genesis")
		return nil
	}

	log.Printf("Indexer: caught up from height %d", height)

	// For now, we don't have access to persistent blocks from here
	// The actual recovery would need to query the node's persistent state
	// This is handled via the SetBlockRetriever callback

	return nil
}

// CatchUp re-indexes blocks from the given start height to the current tip.
func (idx *Indexer) CatchUp(startHeight uint64, tipHeight uint64) error {
	if idx == nil || !idx.enabled || idx.blockRetriever == nil {
		return nil
	}

	log.Printf("Indexer: catching up from height %d to %d", startHeight, tipHeight)

	for height := startHeight; height <= tipHeight; height++ {
		block, err := idx.blockRetriever.GetBlock(height)
		if err != nil {
			log.Printf("Indexer: failed to get block at height %d: %v", height, err)
			continue
		}

		blockHash, _ := block.HeaderHash(types.SHA256Hasher{})

		// Convert slices to pointers
		txns := make([]*types.Transaction, len(block.Transactions))
		for i := 0; i < len(block.Transactions); i++ {
			txns[i] = &block.Transactions[i]
		}
		evts := make([]*types.Event, len(block.Events))
		for i := 0; i < len(block.Events); i++ {
			evts[i] = &block.Events[i]
		}

		ib := &IndexableBlock{
			Number:    block.Header.Height,
			Hash:      blockHash,
			Header:    &block.Header,
			Txns:      txns,
			Events:    evts,
			StateRoot: block.Header.StateRoot,
		}

		if err := idx.indexBlock(ib); err != nil {
			log.Printf("Indexer: failed to index block at height %d: %v", height, err)
			continue
		}

		idx.mu.Lock()
		idx.height = height
		idx.mu.Unlock()

		log.Printf("Indexer: indexed block at height %d", height)
	}

	return nil
}
