package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

type BlockHeader struct {
	Version           uint64
	Height            uint64
	PreviousHash      Hash
	StateRoot         Hash
	TxRoot            Hash
	ReceiptRoot       Hash
	ValidatorRoot     Hash
	ValidatorSetHash  Hash  // NEW: hash of full active validator set
	Epoch             uint64 // NEW: epoch this block belongs to
	Timestamp         uint64
	Proposer          Address
	EventsRoot        Hash  // NEW: root hash of all events in this block
}

type FeeSummary struct {
	TotalFees      uint64
	ValidatorShare uint64 // 70%
	BurnShare      uint64 // 20%
	TreasuryShare  uint64 // 10%
}

func NewFeeSummary(totalFees uint64) FeeSummary {
	return FeeSummary{
		TotalFees:      totalFees,
		ValidatorShare: totalFees * 70 / 100,
		BurnShare:      totalFees * 20 / 100,
		TreasuryShare:  totalFees * 10 / 100,
	}
}

type Block struct {
	Header       BlockHeader
	Transactions []Transaction
	FeeSummary   FeeSummary
	Signature    []byte
	CommitProof  *CommitProof // may be nil for genesis block
	Events       []Event       // NEW: contract events emitted during block execution

	// Evidence is a list of finalized evidence objects to process in this block.
	// Must be empty for genesis blocks.
	// Each evidence object is validated and applied during BeginBlock.
	Evidence []Evidence
}

func NewGenesisBlock(proposer Address) *Block {
	return &Block{
		Header: BlockHeader{
			Version:          1,
			Height:           0,
			PreviousHash:     Hash{},
			StateRoot:        Hash{},
			TxRoot:           Hash{},
			ReceiptRoot:      Hash{},
			ValidatorRoot:    Hash{},
			ValidatorSetHash: Hash{}, // matches genesis snapshot
			Epoch:            0,
			Timestamp:        uint64(time.Now().Unix()),
			Proposer:         proposer,
			EventsRoot:       Hash{},
		},
		Transactions: []Transaction{},
		FeeSummary:   FeeSummary{},
		CommitProof:  nil, // genesis has no commit proof
	}
}

func (h *BlockHeader) HeaderHash(hasher Hasher) (Hash, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, h.Version); err != nil {
		return Hash{}, err
	}
	if err := binary.Write(buf, binary.BigEndian, h.Height); err != nil {
		return Hash{}, err
	}
	buf.Write(h.PreviousHash[:])
	buf.Write(h.StateRoot[:])
	buf.Write(h.TxRoot[:])
	buf.Write(h.ReceiptRoot[:])
	buf.Write(h.ValidatorRoot[:])
	buf.Write(h.ValidatorSetHash[:]) // Write ValidatorSetHash(32) after ValidatorRoot
	if err := binary.Write(buf, binary.BigEndian, h.Epoch); err != nil { // Write Epoch(8) after ValidatorSetHash
		return Hash{}, err
	}
	if err := binary.Write(buf, binary.BigEndian, h.Timestamp); err != nil {
		return Hash{}, err
	}
	buf.Write(h.Proposer[:])
	buf.Write(h.EventsRoot[:]) // Write EventsRoot(32) after Proposer

	return hasher.Hash(buf.Bytes())
}

func (b *Block) HeaderHash(hasher Hasher) (Hash, error) {
	return b.Header.HeaderHash(hasher)
}

// ComputeEventsRoot calculates the root hash of a list of events.
// It concatenates the SHA-256 hash of each event (in order) and then
// hashes the result. Empty events list returns zero hash.
func ComputeEventsRoot(events []Event) Hash {
	if len(events) == 0 {
		return Hash{}
	}

	hasher := sha256.New()
	for _, ev := range events {
		// Encode each event to bytes
		var buf bytes.Buffer
		if err := ev.Encode(&buf); err != nil {
			// This should never happen - Encode should not fail on valid events
			continue
		}
		evHash := sha256.Sum256(buf.Bytes())
		hasher.Write(evHash[:])
	}

	var root Hash
	copy(root[:], hasher.Sum(nil))
	return root
}