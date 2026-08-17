package indexer

import (
	"encoding/json"

	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

// TransactionReceipt represents transaction data stored in the indexer.
type TransactionReceipt struct {
	Hash            types.Hash         `json:"hash"`
	Data            *types.Transaction `json:"data"`
	Status          bool               `json:"status"`
	GasUsed         uint64             `json:"gas_used"`
	BlockNumber     uint64             `json:"block_number"`
	Events          []*types.Event     `json:"events"`
	ContractAddress string             `json:"contract_address,omitempty"`
	ReturnData      string             `json:"return_data,omitempty"`
}

// indexTransaction stores a transaction and its receipt with real execution results.
func indexTransaction(db *bbolt.DB, tx *types.Transaction, blockNumber uint64, events []*types.Event, gasUsed uint64, success bool, contractAddress string, returnData []byte) error {
	return db.Update(func(boltTx *bbolt.Tx) error {
		transactions, err := boltTx.CreateBucketIfNotExists(transactionsBucket)
		if err != nil {
			return err
		}

		receipt := TransactionReceipt{
			Hash:            tx.IntentID,
			Data:            tx,
			Status:          success,
			GasUsed:         gasUsed,
			BlockNumber:     blockNumber,
			Events:          events,
			ContractAddress: contractAddress,
			ReturnData:      string(returnData),
		}

		serialized, err := json.Marshal(receipt)
		if err != nil {
			return err
		}

		// Key is the transaction hash (32 bytes)
		key := tx.IntentID[:]
		return transactions.Put(key, serialized)
	})
}

// GetTransaction retrieves a transaction receipt by hash.
func (idx *Indexer) GetTransaction(hash types.Hash) (*TransactionReceipt, error) {
	var receipt *TransactionReceipt
	err := idx.db.View(func(tx *bbolt.Tx) error {
		transactions := tx.Bucket(transactionsBucket)
		if transactions == nil {
			return nil
		}

		serialized := transactions.Get(hash[:])
		if serialized == nil {
			return nil
		}

		var r TransactionReceipt
		if err := json.Unmarshal(serialized, &r); err != nil {
			return err
		}
		receipt = &r
		return nil
	})
	return receipt, err
}

// GetTransactionsByBlock returns all transactions for a given block.
func (idx *Indexer) GetTransactionsByBlock(blockNumber uint64) ([]*TransactionReceipt, error) {
	var results []*TransactionReceipt
	err := idx.db.View(func(tx *bbolt.Tx) error {
		transactions := tx.Bucket(transactionsBucket)
		if transactions == nil {
			return nil
		}

		c := transactions.Cursor()
		k, v := c.First()
		for k != nil {
			var r TransactionReceipt
			if err := json.Unmarshal(v, &r); err == nil && r.BlockNumber == blockNumber {
				results = append(results, &r)
			}
			k, v = c.Next()
		}
		return nil
	})
	return results, err
}
