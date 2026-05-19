package indexer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"

	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// EventData represents event data stored in the indexer.
type EventData struct {
	Event       *types.Event `json:"event"`
	BlockNumber uint64       `json:"block_number"`
}

// indexEvent stores an event in the events bucket with composite key.
func indexEvent(db *bbolt.DB, event *types.Event) error {
	return db.Update(func(tx *bbolt.Tx) error {
		events, err := tx.CreateBucketIfNotExists(eventsBucket)
		if err != nil {
			return err
		}

		data := EventData{
			Event:       event,
			BlockNumber: event.BlockHeight,
		}

		serialized, err := json.Marshal(data)
		if err != nil {
			return err
		}

		// Key: contract_address(20) + sha256(topic)[:32] + block_height(8BE) + tx_index(4BE) = 64 bytes
		key := EventsIndexKey(event.ContractID, event.Topic, event.BlockHeight, event.TxIndex)
		return events.Put(key, serialized)
	})
}

// EventFilter represents filter criteria for event queries.
type EventFilter struct {
	Contract   *types.Hash
	Topic0     string
	FromBlock  uint64
	ToBlock    uint64
}

// GetEvents retrieves events matching the given filter using prefix scan for efficiency.
func (idx *Indexer) GetEvents(filter EventFilter) ([]*EventData, error) {
	var results []*EventData
	err := idx.db.View(func(tx *bbolt.Tx) error {
		events := tx.Bucket(eventsBucket)
		if events == nil {
			return nil
		}

		c := events.Cursor()

		// Build prefix based on filters
		var prefix []byte
		if filter.Contract != nil {
			prefix = make([]byte, 20)
			copy(prefix, filter.Contract[:20])

			// If both contract and topic are set, add topic hash to prefix
			if filter.Topic0 != "" {
				topicHash := sha256.Sum256([]byte(filter.Topic0))
				prefix = append(prefix, topicHash[:32]...)
			}
		}

		// Use prefix scan when contract filter is active
		var startKey []byte
		if prefix != nil {
			startKey = prefix
		}

		for k, v := c.Seek(startKey); k != nil; k, v = c.Next() {
			// Stop if we have a prefix and key doesn't start with it
			if prefix != nil && !bytes.HasPrefix(k, prefix) {
				break
			}

			// Key format: contract(20) + topicHash(32) + blockHeight(8) + txIndex(4) = 64 bytes
			if len(k) < 64 {
				continue
			}

			// Parse block number from key [52:60]
			blockNum := BigEndianToUint64(k[52:60])

			// Apply block range filter
			if filter.FromBlock > 0 && blockNum < filter.FromBlock {
				continue
			}
			if filter.ToBlock > 0 && blockNum > filter.ToBlock {
				continue
			}

			// If only topic filter is set (no contract), we need to filter manually
			if filter.Contract == nil && filter.Topic0 != "" {
				// Topic hash is in k[20:52]
				expectedTopicHash := sha256.Sum256([]byte(filter.Topic0))
				if !bytes.Equal(k[20:52], expectedTopicHash[:]) {
					continue
				}
			}

			var ed EventData
			if err := json.Unmarshal(v, &ed); err != nil {
				continue
			}

			results = append(results, &ed)
		}
		return nil
	})
	return results, err
}

// GetEventsByBlock returns all events for a given block.
func (idx *Indexer) GetEventsByBlock(blockNumber uint64) ([]*EventData, error) {
	return idx.GetEvents(EventFilter{FromBlock: blockNumber, ToBlock: blockNumber})
}