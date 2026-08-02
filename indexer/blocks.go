package indexer

import (
	"encoding/json"

	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// BlockData represents the serialized block data stored in BoltDB.
type BlockData struct {
	Header     *types.BlockHeader `json:"header"`
	TxCount    int                `json:"tx_count"`
	EventCount int                `json:"event_count"`
}

// indexBlock stores the block header and metadata in the blocks bucket.
func indexBlock(db *bbolt.DB, ib *IndexableBlock) error {
	return db.Update(func(tx *bbolt.Tx) error {
		blocks, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}

		// Serialize block data (header + tx count + event count)
		data := BlockData{
			Header:     ib.Header,
			TxCount:    len(ib.Txns),
			EventCount: len(ib.Events),
		}

		serialized, err := json.Marshal(data)
		if err != nil {
			return err
		}

		key := encodeBlockNumber(ib.Number)
		return blocks.Put(key, serialized)
	})
}

// GetBlock retrieves a block by height from the indexer DB.
func (idx *Indexer) GetBlock(height uint64) (*BlockData, error) {
	var data *BlockData
	err := idx.db.View(func(tx *bbolt.Tx) error {
		blocks := tx.Bucket(blocksBucket)
		if blocks == nil {
			return nil
		}

		key := encodeBlockNumber(height)
		serialized := blocks.Get(key)
		if serialized == nil {
			return nil
		}

		var bd BlockData
		if err := json.Unmarshal(serialized, &bd); err != nil {
			return err
		}
		data = &bd
		return nil
	})
	return data, err
}

// GetLatestHeight returns the latest indexed block height.
func (idx *Indexer) GetLatestHeight() uint64 {
	return idx.height
}
