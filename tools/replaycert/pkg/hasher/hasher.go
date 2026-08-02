// Package hasher provides deterministic transcript hashing utilities using BLAKE2b.
// This package is designed for cross-platform deterministic replay certification.
package hasher

import (
	"bytes"
	"encoding/binary"
	"sort"

	"golang.org/x/crypto/blake2b"

	"github.com/dsn/dsn/types"
)

// HashTranscript computes the BLAKE2b hash of raw transaction execution data.
// This is used for deterministic verification of transaction execution.
func HashTranscript(data []byte) []byte {
	h, err := blake2b.New256(nil)
	if err != nil {
		// blake2b.New256 never returns error with nil key
		panic(err)
	}
	h.Write(data)
	return h.Sum(nil)
}

// HashEvents computes the BLAKE2b hash of an ordered list of events.
// Events must be in the order they were emitted during block execution.
func HashEvents(events []types.Event) []byte {
	if len(events) == 0 {
		// Return deterministic empty hash
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	for _, ev := range events {
		// Encode each event deterministically
		var buf bytes.Buffer
		if err := ev.Encode(&buf); err != nil {
			continue // Skip invalid events
		}
		h.Write(buf.Bytes())
	}

	return h.Sum(nil)
}

// HashReceipts computes the BLAKE2b hash of a list of receipts.
// Returns the cumulative receipt root hash.
func HashReceipts(receipts []Receipt) []byte {
	if len(receipts) == 0 {
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	for _, r := range receipts {
		data := r.Bytes()
		h.Write(data)
	}

	return h.Sum(nil)
}

// HashSnapshot computes the BLAKE2b hash of a state snapshot.
// Keys are sorted alphabetically to ensure deterministic hashing.
func HashSnapshot(snapshot StateSnapshot) []byte {
	h, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Write each key-value pair in sorted order
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(snapshot[k])
	}

	return h.Sum(nil)
}

// HashValidatorSet computes the BLAKE2b hash of a validator set.
// Validators are ordered by their consensus address for deterministic hashing.
func HashValidatorSet(validators []Validator) []byte {
	if len(validators) == 0 {
		h, _ := blake2b.New256(nil)
		return h.Sum(nil)
	}

	h, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	// Sort validators by address for deterministic order
	sorted := make([]Validator, len(validators))
	copy(sorted, validators)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i].Address[:], sorted[j].Address[:]) < 0
	})

	// Hash each validator in sorted order
	for _, v := range sorted {
		h.Write(v.Bytes())
	}

	return h.Sum(nil)
}

// HashBlockTranscript computes the BLAKE2b hash of a complete block transcript.
// This concatenates all component hashes in a deterministic order.
func HashBlockTranscript(rootHash, eventsHash, receiptsHash, snapshotHash, validatorSetHash []byte) []byte {
	h, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	// Write hashes in fixed order
	h.Write(rootHash)
	h.Write(eventsHash)
	h.Write(receiptsHash)
	h.Write(snapshotHash)
	h.Write(validatorSetHash)

	return h.Sum(nil)
}

// Receipt represents a transaction execution receipt.
// This is a stub type that matches DSN's actual receipt structure.
type Receipt struct {
	TransactionHash types.Hash
	GasUsed         uint64
	Success         bool
	ReturnData      []byte
	Logs            []EventLog
}

// EventLog represents a logged event from contract execution.
type EventLog struct {
	Address types.Address
	Topics  []types.Hash
	Data    []byte
}

// Bytes returns the deterministic binary representation of the receipt.
func (r *Receipt) Bytes() []byte {
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

	binary.Write(&buf, binary.BigEndian, uint32(len(r.Logs)))
	for _, log := range r.Logs {
		buf.Write(log.Address[:])
		binary.Write(&buf, binary.BigEndian, uint32(len(log.Topics)))
		for _, t := range log.Topics {
			buf.Write(t[:])
		}
		binary.Write(&buf, binary.BigEndian, uint32(len(log.Data)))
		buf.Write(log.Data)
	}

	return buf.Bytes()
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

// Bytes returns the deterministic binary representation of the validator.
func (v *Validator) Bytes() []byte {
	var buf bytes.Buffer

	buf.Write(v.Address[:])
	buf.Write(v.PubKey[:])
	binary.Write(&buf, binary.BigEndian, v.VotingPower)
	binary.Write(&buf, binary.BigEndian, v.Stake)

	return buf.Bytes()
}

// Ensure types are used
var _ = types.Hash{}
