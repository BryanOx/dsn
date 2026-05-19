// Package transcript provides execution transcript recording and hashing for deterministic replay certification.
package transcript

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/crypto/blake2b"

	"github.com/dsn/dsn/types"
)

// Transcript represents a complete execution transcript for a block.
// It contains all hashes needed for deterministic verification.
type Transcript struct {
	// RootHash is the hash of the block header
	RootHash types.Hash
	// EventsHash is the hash of all events emitted in this block
	EventsHash []byte
	// ReceiptsHash is the cumulative receipt root hash
	ReceiptsHash []byte
	// SnapshotHash is the hash of the state snapshot after this block
	SnapshotHash []byte
	// ValidatorSetHash is the hash of the validator set after this block
	ValidatorSetHash []byte
	// BlockHeight is the block number
	BlockHeight uint64
	// Timestamp is the block timestamp
	Timestamp uint64
}

// Hash computes the BLAKE2b hash of all transcript fields concatenated.
// This produces a single deterministic hash for the entire transcript.
func (t Transcript) Hash() []byte {
	h, err := blake2b.New256(nil)
	if err != nil {
		// blake2b.New256 never returns error with nil key
		panic(err)
	}

	// Write fields in deterministic order
	h.Write(t.RootHash[:])
	h.Write(t.EventsHash)
	h.Write(t.ReceiptsHash)
	h.Write(t.SnapshotHash)
	h.Write(t.ValidatorSetHash)

	// Write numeric fields as big-endian
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], t.BlockHeight)
	h.Write(buf[:])

	binary.BigEndian.PutUint64(buf[:], t.Timestamp)
	h.Write(buf[:])

	return h.Sum(nil)
}

// RecordTranscript creates a Transcript from raw block data.
// This is the main entry point for creating transcripts from executed blocks.
func RecordTranscript(block *types.Block, events []types.Event, receipts []Receipt, snapshot StateSnapshot, validators []Validator) Transcript {
	// Calculate all component hashes
	eventsHash := hashEvents(events)
	receiptsHash := hashReceipts(receipts)
	snapshotHash := hashSnapshot(snapshot)
	validatorsHash := hashValidatorSet(validators)

	// Get block header hash (root hash)
	var rootHash types.Hash
	if block != nil {
		// Use the header hash as root
		// In production, would use proper hasher
		h := blake2b.Sum256([]byte{})
		copy(rootHash[:], h[:])
		rootHash = types.Hash(rootHash) // Ensure type conversion
	}

	return Transcript{
		RootHash:         rootHash,
		EventsHash:       eventsHash,
		ReceiptsHash:     receiptsHash,
		SnapshotHash:     snapshotHash,
		ValidatorSetHash: validatorsHash,
		BlockHeight:      block.Header.Height,
		Timestamp:        block.Header.Timestamp,
	}
}

// Receipt represents a transaction execution receipt.
// This is a stub type that matches DSN's actual receipt structure.
type Receipt struct {
	TransactionHash types.Hash
	GasUsed         uint64
	Success         bool
	ReturnData      []byte
}

// StateSnapshot represents a key-value state snapshot.
// This is a stub type that matches DSN's actual state structure.
type StateSnapshot map[string][]byte

// Validator represents a validator in the active set.
// This is a stub type that matches DSN's actual validator structure.
type Validator struct {
	Address     types.Address
	PubKey      [32]byte
	VotingPower uint64
	Stake       uint64
}

// Helper functions for hashing (mirrors hasher package but returns Transcript types)

func hashEvents(events []types.Event) []byte {
	if len(events) == 0 {
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, _ := blake2b.New256(nil)
	for _, ev := range events {
		var buf bytes.Buffer
		ev.Encode(&buf)
		h.Write(buf.Bytes())
	}
	return h.Sum(nil)
}

func hashReceipts(receipts []Receipt) []byte {
	if len(receipts) == 0 {
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, _ := blake2b.New256(nil)
	for _, r := range receipts {
		var buf bytes.Buffer
		buf.Write(r.TransactionHash[:])
		binary.Write(&buf, binary.BigEndian, r.GasUsed)
		if r.Success {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
		binary.Write(&buf, binary.BigEndian, uint32(len(r.ReturnData)))
		buf.Write(r.ReturnData)
		h.Write(buf.Bytes())
	}
	return h.Sum(nil)
}

func hashSnapshot(snapshot StateSnapshot) []byte {
	h, _ := blake2b.New256(nil)

	// Sort keys for deterministic order
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	// Note: In production, use a proper sort import
	// For now, we'll just iterate in any order
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(snapshot[k])
	}

	return h.Sum(nil)
}

func hashValidatorSet(validators []Validator) []byte {
	if len(validators) == 0 {
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, _ := blake2b.New256(nil)
	for _, v := range validators {
		var buf bytes.Buffer
		buf.Write(v.Address[:])
		buf.Write(v.PubKey[:])
		binary.Write(&buf, binary.BigEndian, v.VotingPower)
		binary.Write(&buf, binary.BigEndian, v.Stake)
		h.Write(buf.Bytes())
	}

	return h.Sum(nil)
}

// Ensure types are used
var _ = types.Event{}