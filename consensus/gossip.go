package consensus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dsn/dsn/types"
)

const (
	BlockMessageType byte = 0x01
	MaxBlockSize          = 10 * 1024 * 1024 // 10MB
	MaxSigLen            = 8192               // 8KB max signature length
)

// EncodeBlockMessage serializes a block for gossip.
func EncodeBlockMessage(block *types.Block) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(BlockMessageType)

	var blockBuf bytes.Buffer
	if err := binary.Write(&blockBuf, binary.BigEndian, block.Header.Version); err != nil {
		return nil, err
	}
	if err := binary.Write(&blockBuf, binary.BigEndian, block.Header.Height); err != nil {
		return nil, err
	}
	blockBuf.Write(block.Header.PreviousHash[:])
	blockBuf.Write(block.Header.StateRoot[:])
	blockBuf.Write(block.Header.TxRoot[:])
	blockBuf.Write(block.Header.ReceiptRoot[:])
	blockBuf.Write(block.Header.ValidatorRoot[:])
	blockBuf.Write(block.Header.ValidatorSetHash[:]) // Add ValidatorSetHash(32) after ValidatorRoot
	if err := binary.Write(&blockBuf, binary.BigEndian, block.Header.Epoch); err != nil { // Add Epoch(8) after ValidatorSetHash
		return nil, err
	}
	if err := binary.Write(&blockBuf, binary.BigEndian, block.Header.Timestamp); err != nil {
		return nil, err
	}
	blockBuf.Write(block.Header.Proposer[:])

	// Num txs
	if err := binary.Write(&blockBuf, binary.BigEndian, uint32(len(block.Transactions))); err != nil {
		return nil, err
	}
	for _, tx := range block.Transactions {
		if err := tx.Encode(&blockBuf); err != nil {
			return nil, err
		}
	}

	// Fee summary
	if err := binary.Write(&blockBuf, binary.BigEndian, block.FeeSummary.TotalFees); err != nil {
		return nil, err
	}
	if err := binary.Write(&blockBuf, binary.BigEndian, block.FeeSummary.ValidatorShare); err != nil {
		return nil, err
	}
	if err := binary.Write(&blockBuf, binary.BigEndian, block.FeeSummary.BurnShare); err != nil {
		return nil, err
	}
	if err := binary.Write(&blockBuf, binary.BigEndian, block.FeeSummary.TreasuryShare); err != nil {
		return nil, err
	}

	// Signature length + bytes
	sigLen := uint32(len(block.Signature))
	if err := binary.Write(&blockBuf, binary.BigEndian, sigLen); err != nil {
		return nil, err
	}
	blockBuf.Write(block.Signature)

	// Events: length-prefixed list of events
	if err := binary.Write(&blockBuf, binary.BigEndian, uint32(len(block.Events))); err != nil {
		return nil, err
	}
	for _, ev := range block.Events {
		if err := ev.Encode(&blockBuf); err != nil {
			return nil, err
		}
	}

	// Length-prefixed envelope: [4-byte total len][1-byte type][payload]
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(blockBuf.Len()))
	buf.Write(lenBuf)
	buf.Write(blockBuf.Bytes())

	return buf.Bytes(), nil
}

// DecodeBlockMessage deserializes a block from gossip bytes.
func DecodeBlockMessage(data []byte) (*types.Block, error) {
	if len(data) < 6 { // 1 (type) + 4 (length) + minimum payload
		return nil, fmt.Errorf("block message too short")
	}
	if data[0] != BlockMessageType {
		return nil, fmt.Errorf("unknown message type: %d", data[0])
	}

	// Read the length prefix first
	r := bytes.NewReader(data[1:])
	var payloadLen uint32
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		return nil, err
	}

	block := &types.Block{}

	if err := binary.Read(r, binary.BigEndian, &block.Header.Version); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &block.Header.Height); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, block.Header.PreviousHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read previous hash: %w", err)
	}
	if _, err := io.ReadFull(r, block.Header.StateRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read state root: %w", err)
	}
	if _, err := io.ReadFull(r, block.Header.TxRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read tx root: %w", err)
	}
	if _, err := io.ReadFull(r, block.Header.ReceiptRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read receipt root: %w", err)
	}
	if _, err := io.ReadFull(r, block.Header.ValidatorRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read validator root: %w", err)
	}
	if _, err := io.ReadFull(r, block.Header.ValidatorSetHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read validator set hash: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &block.Header.Epoch); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &block.Header.Timestamp); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, block.Header.Proposer[:]); err != nil {
		return nil, fmt.Errorf("failed to read proposer: %w", err)
	}

	var numTxs uint32
	if err := binary.Read(r, binary.BigEndian, &numTxs); err != nil {
		return nil, err
	}
	for i := uint32(0); i < numTxs; i++ {
		var tx types.Transaction
		if err := tx.Decode(r); err != nil {
			return nil, fmt.Errorf("tx %d: %w", i, err)
		}
		block.Transactions = append(block.Transactions, tx)
	}

	if err := binary.Read(r, binary.BigEndian, &block.FeeSummary.TotalFees); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &block.FeeSummary.ValidatorShare); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &block.FeeSummary.BurnShare); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &block.FeeSummary.TreasuryShare); err != nil {
		return nil, err
	}

	var sigLen uint32
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return nil, err
	}
	if sigLen > MaxSigLen {
		return nil, fmt.Errorf("signature too long: %d > %d", sigLen, MaxSigLen)
	}
	block.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r, block.Signature); err != nil {
		return nil, fmt.Errorf("failed to read signature: %w", err)
	}

	// Try to read events (backward compatible - if EOF, no events)
	var numEvents uint32
	if err := binary.Read(r, binary.BigEndian, &numEvents); err != nil {
		// EOF means no events (backward compatibility)
		block.Events = nil
		return block, nil
	}
	for i := uint32(0); i < numEvents; i++ {
		var ev types.Event
		if err := ev.Decode(r); err != nil {
			return nil, fmt.Errorf("event %d: %w", i, err)
		}
		block.Events = append(block.Events, ev)
	}

	return block, nil
}

// SeenSet tracks seen block hashes to prevent duplicate gossip.
type SeenSet struct {
	hashes map[types.Hash]struct{}
	order  []types.Hash
	max    int
}

// NewSeenSet creates a new SeenSet with the specified maximum size.
func NewSeenSet(max int) *SeenSet {
	return &SeenSet{
		hashes: make(map[types.Hash]struct{}),
		order:  make([]types.Hash, 0, max),
		max:    max,
	}
}

// Seen checks if a hash has been seen.
func (s *SeenSet) Seen(h types.Hash) bool {
	_, ok := s.hashes[h]
	return ok
}

// Mark adds a hash to the seen set, evicting oldest if at capacity.
func (s *SeenSet) Mark(h types.Hash) {
	if s.Seen(h) {
		return
	}
	s.hashes[h] = struct{}{}
	s.order = append(s.order, h)
	if len(s.order) > s.max {
		oldest := s.order[0]
		delete(s.hashes, oldest)
		s.order = s.order[1:]
	}
}

// Size returns the number of seen hashes.
func (s *SeenSet) Size() int {
	return len(s.hashes)
}