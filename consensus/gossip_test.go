package consensus

import (
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
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
			ValidatorSetHash: types.Hash{20, 21},
			EventsRoot:       types.Hash{16, 17, 18},
			Epoch:            2,
			Timestamp:        1234567890,
		},
		Transactions: []types.Transaction{
			{
				Version:   1,
				ChainID:   1,
				IntentID:  types.Hash{99},
				Sender:    types.Address{1},
				Nonce:     1,
				Payload:   types.EncodeTransferPayload(types.Address{1}, 100),
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
	if decoded.Header.EventsRoot != block.Header.EventsRoot {
		t.Errorf("expected events root %x, got %x", block.Header.EventsRoot, decoded.Header.EventsRoot)
	}
	if decoded.CommitProof != nil {
		t.Error("expected nil commit proof, got non-nil")
	}
}

// TestBlockMessage_EncodeDecode_WithCommitProof verifies that a block carrying
// a CommitProof survives the gossip codec round-trip unchanged.
func TestBlockMessage_EncodeDecode_WithCommitProof(t *testing.T) {
	hasher, s, mp, _, _, _, validatorPrivKey, validatorConsensusID := setupTest(t)

	block, err := BuildBlock(s, nil, mp, 1, types.Hash{}, validatorConsensusID, &mockSigner{}, hasher, 100, nil, 1)
	require.NoError(t, err)
	attachCommitProof(t, block, s, validatorConsensusID, validatorPrivKey)
	require.NotNil(t, block.CommitProof)

	data, err := EncodeBlockMessage(block)
	require.NoError(t, err)

	decoded, err := DecodeBlockMessage(data)
	require.NoError(t, err)

	require.NotNil(t, decoded.CommitProof, "commit proof must round-trip")
	require.Equal(t, block.CommitProof.Height, decoded.CommitProof.Height)
	require.Equal(t, block.CommitProof.BlockHash, decoded.CommitProof.BlockHash)
	require.Equal(t, block.CommitProof.TotalPower, decoded.CommitProof.TotalPower)
	require.Equal(t, block.CommitProof.SignedPower, decoded.CommitProof.SignedPower)
	require.Equal(t, block.CommitProof.SetHash, decoded.CommitProof.SetHash)
	require.Equal(t, len(block.CommitProof.Precommits), len(decoded.CommitProof.Precommits))
	for i := range block.CommitProof.Precommits {
		require.Equal(t, block.CommitProof.Precommits[i], decoded.CommitProof.Precommits[i])
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
