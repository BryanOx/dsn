package indexer

import (
	"go.etcd.io/bbolt"
)

// Bucket names for the indexer database.
var (
	blocksBucket       = []byte("blocks")
	transactionsBucket = []byte("transactions")
	eventsBucket       = []byte("events")
	validatorsBucket   = []byte("validators")
	stateBucket        = []byte("state")

	// State keys
	latestIndexedHeightKey = []byte("latest_indexed_height")
)

// initSchema creates all required buckets if they don't exist.
func initSchema(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		// Create blocks bucket
		_, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}

		// Create transactions bucket
		_, err = tx.CreateBucketIfNotExists(transactionsBucket)
		if err != nil {
			return err
		}

		// Create events bucket
		_, err = tx.CreateBucketIfNotExists(eventsBucket)
		if err != nil {
			return err
		}

		// Create validators bucket
		_, err = tx.CreateBucketIfNotExists(validatorsBucket)
		if err != nil {
			return err
		}

		// Create state bucket
		_, err = tx.CreateBucketIfNotExists(stateBucket)
		if err != nil {
			return err
		}

		return nil
	})
}

// getLatestIndexedHeight returns the latest indexed block height from the state bucket.
func getLatestIndexedHeight(db *bbolt.DB) (uint64, error) {
	var height uint64
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(stateBucket)
		if b == nil {
			return nil
		}
		data := b.Get(latestIndexedHeightKey)
		if data != nil && len(data) == 8 {
			height = BigEndianToUint64(data)
		}
		return nil
	})
	return height, err
}

// setLatestIndexedHeight stores the latest indexed block height in the state bucket.
func setLatestIndexedHeight(db *bbolt.DB, height uint64) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(stateBucket)
		if err != nil {
			return err
		}
		return b.Put(latestIndexedHeightKey, Uint64ToBigEndian(height))
	})
}
