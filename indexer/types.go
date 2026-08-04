package indexer

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/BryanOx/dsn/types"
)

// IndexableBlock represents a block ready for indexing.
type IndexableBlock struct {
	Number    uint64
	Hash      types.Hash
	Header    *types.BlockHeader
	Txns      []*types.Transaction
	Events    []*types.Event
	StateRoot types.Hash
}

// Uint64ToBigEndian converts a uint64 to big-endian bytes.
func Uint64ToBigEndian(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

// BigEndianToUint64 converts big-endian bytes to a uint64.
func BigEndianToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

// encodeBlockNumber encodes a block number as a key for BoltDB.
func encodeBlockNumber(height uint64) []byte {
	return Uint64ToBigEndian(height)
}

// EventsIndexKey creates a composite key for event indexing.
// Format: contract_address(20) + sha256(topic)[:32] + block_height(8BE) + tx_index(4BE) = 64 bytes
func EventsIndexKey(contractID types.Hash, topic string, blockNumber uint64, txIndex uint32) []byte {
	key := make([]byte, 20+32+8+4)
	// contract address: first 20 bytes of hash
	copy(key[:20], contractID[:20])
	// topic hash: sha256 of topic string, first 32 bytes
	topicHash := sha256.Sum256([]byte(topic))
	copy(key[20:52], topicHash[:32])
	// block height: 8 bytes big-endian
	binary.BigEndian.PutUint64(key[52:60], blockNumber)
	// tx index: 4 bytes big-endian
	binary.BigEndian.PutUint32(key[60:64], txIndex)
	return key
}
