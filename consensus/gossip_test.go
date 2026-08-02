package consensus

import (
	"testing"

	"github.com/dsn/dsn/types"
)

func TestBlockMessage_EncodeDecode(t *testing.T) {
	block := &types.Block{
		Header: types.BlockHeader{
			Version:       1,
			Height:        42,
			PreviousHash:  types.Hash{1, 2, 3},
			Proposer:      types.Address{1},
			StateRoot:     types.Hash{4, 5, 6},
			TxRoot:        types.Hash{7, 8, 9},
			ReceiptRoot:   types.Hash{10, 11, 12},
			ValidatorRoot: types.Hash{13, 14, 15},
			Timestamp:     1234567890,
		},
		Transactions: []types.Transaction{
			{
				Version:   1,
				ChainID:   1,
				IntentID:  types.Hash{99},
				Sender:    types.Address{1},
				Nonce:     1,
				Payload:   []byte("test payload"),
				MaxFee:    100,
				Timestamp: 1234567890,
			},
		},
		FeeSummary: types.NewFeeSummary(100),
		Signature:  []byte{1, 2, 3, 4},
	}

	data, err := EncodeBlockMessage(block)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeBlockMessage(data)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Header.Height != 42 {
		t.Errorf("expected height 42, got %d", decoded.Header.Height)
	}
	if decoded.Header.Version != 1 {
		t.Errorf("expected version 1, got %d", decoded.Header.Version)
	}
	if decoded.Header.Proposer != block.Header.Proposer {
		t.Errorf("expected proposer %v, got %v", block.Header.Proposer, decoded.Header.Proposer)
	}
	if len(decoded.Transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(decoded.Transactions))
	}
	if decoded.FeeSummary.TotalFees != 100 {
		t.Errorf("expected total fees 100, got %d", decoded.FeeSummary.TotalFees)
	}
}

func TestBlockMessage_SeenSet(t *testing.T) {
	ss := NewSeenSet(10)

	h1 := types.Hash{1}
	h2 := types.Hash{2}

	if ss.Seen(h1) {
		t.Error("h1 should not be seen yet")
	}
	ss.Mark(h1)
	if !ss.Seen(h1) {
		t.Error("h1 should be seen after Mark")
	}
	if ss.Seen(h2) {
		t.Error("h2 should not be seen")
	}
}

func TestSeenSet_MaxSize(t *testing.T) {
	ss := NewSeenSet(3)

	ss.Mark(types.Hash{1})
	ss.Mark(types.Hash{2})
	ss.Mark(types.Hash{3})
	ss.Mark(types.Hash{4}) // should evict oldest

	if ss.Size() > 3 {
		t.Errorf("seen set exceeded max size: %d", ss.Size())
	}
}

func TestSeenSet_EvictionOrder(t *testing.T) {
	ss := NewSeenSet(3)

	h1 := types.Hash{1}
	h2 := types.Hash{2}
	h3 := types.Hash{3}

	ss.Mark(h1)
	ss.Mark(h2)
	ss.Mark(h3)
	ss.Mark(types.Hash{4}) // should evict h1

	if ss.Seen(h1) {
		t.Error("h1 should have been evicted")
	}
	if !ss.Seen(h2) {
		t.Error("h2 should still be present")
	}
	if !ss.Seen(h3) {
		t.Error("h3 should still be present")
	}
}

func TestEncodeBlockMessage_EmptyBlock(t *testing.T) {
	block := &types.Block{
		Header: types.BlockHeader{
			Version:       1,
			Height:        0,
			PreviousHash:  types.Hash{},
			Proposer:      types.Address{},
			StateRoot:     types.Hash{},
			TxRoot:        types.Hash{},
			ReceiptRoot:   types.Hash{},
			ValidatorRoot: types.Hash{},
			Timestamp:     0,
		},
		Transactions: []types.Transaction{},
		FeeSummary:   types.FeeSummary{},
		Signature:    []byte{},
	}

	data, err := EncodeBlockMessage(block)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("encoded data should not be empty")
	}

	decoded, err := DecodeBlockMessage(data)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Header.Height != 0 {
		t.Errorf("expected height 0, got %d", decoded.Header.Height)
	}
	if len(decoded.Transactions) != 0 {
		t.Error("expected no transactions")
	}
}

func TestDecodeBlockMessage_InvalidType(t *testing.T) {
	data := []byte{0x02} // wrong message type

	_, err := DecodeBlockMessage(data)
	if err == nil {
		t.Error("expected error for unknown message type")
	}
}

func TestDecodeBlockMessage_TooShort(t *testing.T) {
	data := []byte{0x01} // only type, no data

	_, err := DecodeBlockMessage(data)
	if err == nil {
		t.Error("expected error for too short data")
	}
}
