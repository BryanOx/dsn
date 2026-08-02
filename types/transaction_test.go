package types

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestTransaction_New(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	tx := NewTransaction(
		1,                  // version
		1,                  // chainID
		sender,             // sender
		42,                 // nonce
		[]byte{0x01, 0x02}, // payload
		[]byte{0x03},       // constraints
		1000,               // maxFee
		21000,              // gasLimit
		1700000000,         // timestamp
	)

	if tx.Version != 1 {
		t.Errorf("Version = %d, want 1", tx.Version)
	}
	if tx.ChainID != 1 {
		t.Errorf("ChainID = %d, want 1", tx.ChainID)
	}
	if tx.Nonce != 42 {
		t.Errorf("Nonce = %d, want 42", tx.Nonce)
	}
	if tx.MaxFee != 1000 {
		t.Errorf("MaxFee = %d, want 1000", tx.MaxFee)
	}
}

func TestTransaction_ComputeIntentID_Deterministic(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	tx := NewTransaction(
		1, 1, sender, 42,
		[]byte{0x01, 0x02},
		[]byte{0x03},
		1000, 21000, 1700000000,
	)

	hasher := SHA256Hasher{}

	// Compute twice - should be deterministic
	id1, err := tx.ComputeIntentID(hasher)
	if err != nil {
		t.Fatalf("ComputeIntentID failed: %v", err)
	}

	id2, err := tx.ComputeIntentID(hasher)
	if err != nil {
		t.Fatalf("ComputeIntentID failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("IntentID not deterministic: %x != %x", id1, id2)
	}
}

func TestTransaction_ComputeIntentID_Different(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	hasher := SHA256Hasher{}

	tx1 := NewTransaction(1, 1, sender, 42, []byte{0x01}, []byte{0x02}, 1000, 21000, 1700000000)
	tx2 := NewTransaction(1, 1, sender, 43, []byte{0x01}, []byte{0x02}, 1000, 21000, 1700000000)

	id1, _ := tx1.ComputeIntentID(hasher)
	id2, _ := tx2.ComputeIntentID(hasher)

	if id1 == id2 {
		t.Errorf("Different transactions should have different IntentIDs")
	}
}

func TestTransaction_EncodeDecode_RoundTrip(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	original := &Transaction{
		Version:     1,
		ChainID:     1,
		IntentID:    Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}),
		Sender:      sender,
		Nonce:       42,
		Payload:     []byte{0x01, 0x02, 0x03},
		Constraints: []byte{0x04, 0x05},
		MaxFee:      1000,
		GasLimit:    21000,
		Timestamp:   1700000000,
		Signature:   []byte{0xAA, 0xBB, 0xCC},
	}

	// Encode
	buf := new(bytes.Buffer)
	if err := original.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded := &Transaction{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Compare
	if decoded.Version != original.Version {
		t.Errorf("Version = %d, want %d", decoded.Version, original.Version)
	}
	if decoded.ChainID != original.ChainID {
		t.Errorf("ChainID = %d, want %d", decoded.ChainID, original.ChainID)
	}
	if decoded.Nonce != original.Nonce {
		t.Errorf("Nonce = %d, want %d", decoded.Nonce, original.Nonce)
	}
	if decoded.MaxFee != original.MaxFee {
		t.Errorf("MaxFee = %d, want %d", decoded.MaxFee, original.MaxFee)
	}
	if !bytes.Equal(decoded.Payload, original.Payload) {
		t.Errorf("Payload = %x, want %x", decoded.Payload, original.Payload)
	}
	if !bytes.Equal(decoded.Constraints, original.Constraints) {
		t.Errorf("Constraints = %x, want %x", decoded.Constraints, original.Constraints)
	}
	if !bytes.Equal(decoded.Signature, original.Signature) {
		t.Errorf("Signature = %x, want %x", decoded.Signature, original.Signature)
	}
}

func TestTransaction_Validate_OK(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	tx := &Transaction{
		Version:   1,
		ChainID:   1,
		Sender:    sender,
		Nonce:     1,
		Payload:   EncodeTransferPayload(sender, 0),
		MaxFee:    1,
		Timestamp: uint64(time.Now().Unix()), // Current timestamp is valid
		Signature: []byte{0x01, 0x02, 0x03},
	}

	err := tx.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestTransaction_Validate_InvalidAddress(t *testing.T) {
	// Empty address (0 bytes) is invalid - should be 20 bytes
	invalidAddr := Address{}
	tx := &Transaction{
		Version:   1,
		ChainID:   1,
		Sender:    invalidAddr,
		Nonce:     1,
		MaxFee:    1,
		Timestamp: 1700000000,
		Signature: []byte{0x01},
	}

	err := tx.Validate()
	if err == nil {
		t.Error("Validate() should return error for invalid address")
	}
}

func TestTransaction_Validate_InvalidSignature(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	tx := &Transaction{
		Version:   1,
		ChainID:   1,
		Sender:    sender,
		Nonce:     1,
		Payload:   EncodeTransferPayload(sender, 0),
		MaxFee:    1,
		Timestamp: 1700000000,
		Signature: []byte{}, // Empty signature
	}

	err := tx.Validate()
	if err == nil {
		t.Error("Validate() should return error for empty signature")
	}
}

func TestTransaction_Validate_TimestampOutOfRange(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	tx := &Transaction{
		Version:   1,
		ChainID:   1,
		Sender:    sender,
		Nonce:     1,
		Payload:   EncodeTransferPayload(sender, 0),
		MaxFee:    1,
		Timestamp: 1000000000, // Way in the past (2001) - definitely outside ±5s window
		Signature: []byte{0x01},
	}

	err := tx.Validate()
	if err == nil {
		t.Error("Validate() should return error for invalid timestamp")
	}
}

func TestTransaction_Validate_MalformedTransferPayload(t *testing.T) {
	sender := Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})

	base := func(payload []byte) *Transaction {
		return &Transaction{
			Version:   1,
			ChainID:   1,
			Sender:    sender,
			Nonce:     1,
			Payload:   payload,
			MaxFee:    1,
			Timestamp: uint64(time.Now().Unix()),
			Signature: []byte{0x01},
		}
	}

	t.Run("short payload rejected for standard transfer", func(t *testing.T) {
		for _, short := range [][]byte{nil, {}, []byte{0x01}, make([]byte, TransferPayloadLength-1)} {
			err := base(short).Validate()
			if !errors.Is(err, ErrMalformedTransferPayload) {
				t.Errorf("Validate() error = %v, want ErrMalformedTransferPayload", err)
			}
		}
	})

	t.Run("full payload accepted for standard transfer", func(t *testing.T) {
		err := base(EncodeTransferPayload(sender, 1000)).Validate()
		if err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("short payload allowed for non-standard transactions", func(t *testing.T) {
		tx := base([]byte{0x01})
		tx.TxType = TxTypeCallContract
		if err := tx.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})
}
