package state

import (
	"encoding/binary"
	"fmt"

	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

// Checkpoint represents periodic checkpoint metadata for fast sync entry points.
type Checkpoint struct {
	Height           uint64     // Block height
	BlockHash        types.Hash // Hash of the block header
	StateRoot        types.Hash // State root at this height
	SnapshotHash     types.Hash // Hash of the full state snapshot
	ValidatorSetHash types.Hash // Active validator set hash
	Epoch            uint64     // Current epoch
	Timestamp        uint64     // Block timestamp
}

// Bucket names for checkpoint and snapshot persistence.
var (
	checkpointsBucket   = []byte("checkpoints")
	snapshotsBucket     = []byte("snapshots")
	latestCheckpointKey = []byte("latest")
)

// encodeCheckpoint serializes a Checkpoint into a deterministic byte slice.
// Format: 8+32+32+32+32+8+8 = 152 bytes total
// - Height: 8 bytes BE
// - BlockHash: 32 bytes
// - StateRoot: 32 bytes
// - SnapshotHash: 32 bytes
// - ValidatorSetHash: 32 bytes
// - Epoch: 8 bytes BE
// - Timestamp: 8 bytes BE
func encodeCheckpoint(cp *Checkpoint) []byte {
	buf := make([]byte, 152)
	offset := 0

	// Height: 8 bytes big-endian
	binary.BigEndian.PutUint64(buf[offset:offset+8], cp.Height)
	offset += 8

	// BlockHash: 32 bytes
	copy(buf[offset:offset+32], cp.BlockHash[:])
	offset += 32

	// StateRoot: 32 bytes
	copy(buf[offset:offset+32], cp.StateRoot[:])
	offset += 32

	// SnapshotHash: 32 bytes
	copy(buf[offset:offset+32], cp.SnapshotHash[:])
	offset += 32

	// ValidatorSetHash: 32 bytes
	copy(buf[offset:offset+32], cp.ValidatorSetHash[:])
	offset += 32

	// Epoch: 8 bytes big-endian
	binary.BigEndian.PutUint64(buf[offset:offset+8], cp.Epoch)
	offset += 8

	// Timestamp: 8 bytes big-endian
	binary.BigEndian.PutUint64(buf[offset:offset+8], cp.Timestamp)

	return buf
}

// decodeCheckpoint deserializes a Checkpoint from byte slice.
// Returns nil if data is nil or has incorrect length.
func decodeCheckpoint(data []byte) *Checkpoint {
	if data == nil || len(data) != 152 {
		return nil
	}

	cp := &Checkpoint{}
	offset := 0

	// Height: 8 bytes big-endian
	cp.Height = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// BlockHash: 32 bytes
	copy(cp.BlockHash[:], data[offset:offset+32])
	offset += 32

	// StateRoot: 32 bytes
	copy(cp.StateRoot[:], data[offset:offset+32])
	offset += 32

	// SnapshotHash: 32 bytes
	copy(cp.SnapshotHash[:], data[offset:offset+32])
	offset += 32

	// ValidatorSetHash: 32 bytes
	copy(cp.ValidatorSetHash[:], data[offset:offset+32])
	offset += 32

	// Epoch: 8 bytes big-endian
	cp.Epoch = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Timestamp: 8 bytes big-endian
	cp.Timestamp = binary.BigEndian.Uint64(data[offset : offset+8])

	return cp
}

// heightKey returns the 8-byte big-endian encoding of a height for use as a BoltDB key.
func heightKey(height uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, height)
	return key
}

// StoreCheckpoint persists a checkpoint to the database.
// It stores the checkpoint by height and also updates the "latest" key.
func StoreCheckpoint(ps *PersistentState, cp *Checkpoint) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(checkpointsBucket)
		if err != nil {
			return fmt.Errorf("failed to create checkpoints bucket: %w", err)
		}

		key := heightKey(cp.Height)
		value := encodeCheckpoint(cp)

		// Store by height
		if err := bucket.Put(key, value); err != nil {
			return fmt.Errorf("failed to store checkpoint at height %d: %w", cp.Height, err)
		}

		// Update latest checkpoint
		if err := bucket.Put(latestCheckpointKey, value); err != nil {
			return fmt.Errorf("failed to update latest checkpoint: %w", err)
		}

		return nil
	})
}

// LoadCheckpoint retrieves a checkpoint by height.
// Returns an error if not found.
func LoadCheckpoint(ps *PersistentState, height uint64) (*Checkpoint, error) {
	var checkpoint *Checkpoint

	err := ps.DB().View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(checkpointsBucket)
		if bucket == nil {
			return fmt.Errorf("checkpoints bucket not found")
		}

		data := bucket.Get(heightKey(height))
		if data == nil {
			return fmt.Errorf("checkpoint not found at height %d", height)
		}

		checkpoint = decodeCheckpoint(data)
		if checkpoint == nil {
			return fmt.Errorf("corrupt checkpoint data at height %d", height)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return checkpoint, nil
}

// LatestCheckpoint retrieves the most recent checkpoint.
// Returns an error if no checkpoints exist.
func LatestCheckpoint(ps *PersistentState) (*Checkpoint, error) {
	var checkpoint *Checkpoint

	err := ps.DB().View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(checkpointsBucket)
		if bucket == nil {
			return fmt.Errorf("checkpoint not found")
		}

		data := bucket.Get(latestCheckpointKey)
		if data == nil {
			return fmt.Errorf("checkpoint not found")
		}

		checkpoint = decodeCheckpoint(data)
		if checkpoint == nil {
			return fmt.Errorf("corrupt latest checkpoint data")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return checkpoint, nil
}

// SnapshotExists checks if a snapshot exists for the given height.
func SnapshotExists(ps *PersistentState, height uint64) bool {
	exists := false

	ps.DB().View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(snapshotsBucket)
		if bucket == nil {
			return nil
		}

		exists = bucket.Get(heightKey(height)) != nil
		return nil
	})

	return exists
}

// StoreSnapshot persists snapshot data for a given height.
func StoreSnapshot(ps *PersistentState, height uint64, data []byte) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(snapshotsBucket)
		if err != nil {
			return fmt.Errorf("failed to create snapshots bucket: %w", err)
		}

		key := heightKey(height)
		if err := bucket.Put(key, data); err != nil {
			return fmt.Errorf("failed to store snapshot at height %d: %w", height, err)
		}

		return nil
	})
}

// LoadSnapshot retrieves snapshot data for a given height.
// Returns an error if not found.
func LoadSnapshot(ps *PersistentState, height uint64) ([]byte, error) {
	var data []byte

	err := ps.DB().View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(snapshotsBucket)
		if bucket == nil {
			return fmt.Errorf("snapshot not found at height %d", height)
		}

		data = bucket.Get(heightKey(height))
		if data == nil {
			return fmt.Errorf("snapshot not found at height %d", height)
		}

		// Return a copy to prevent modification of internal buffer
		data = append([]byte{}, data...)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return data, nil
}

// DeleteSnapshot removes a snapshot for the given height.
func DeleteSnapshot(ps *PersistentState, height uint64) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(snapshotsBucket)
		if bucket == nil {
			return nil // Already gone
		}

		key := heightKey(height)
		return bucket.Delete(key)
	})
}
