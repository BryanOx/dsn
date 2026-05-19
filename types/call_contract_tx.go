package types

import (
	"bytes"
	"encoding/binary"
	"io"
)

// CallContractTx invokes an already-deployed contract by its ContractID.
type CallContractTx struct {
	Sender      Address
	Nonce       uint64
	ContractID  Hash
	Entrypoint  string
	Calldata    []byte
	MaxFee      uint64
	GasLimit    uint64
	Signature   []byte
}

// Validate validates the call transaction fields
func (tx *CallContractTx) Validate() error {
	// Validate sender address
	if len(tx.Sender.Bytes()) != 20 {
		return &ValidationError{Err: ErrInvalidAddress, Field: "Sender", Value: tx.Sender.String()}
	}

	// Validate nonce > 0
	if tx.Nonce == 0 {
		return &ValidationError{Err: ErrNonceMismatch, Field: "Nonce", Value: tx.Nonce}
	}

	// Validate contract ID is not zero
	if tx.ContractID.IsZero() {
		return &ValidationError{Err: ErrInvalidContractID, Field: "ContractID", Value: nil}
	}

	// Validate entrypoint is not empty
	if tx.Entrypoint == "" {
		return &ValidationError{Err: ErrInvalidEncoding, Field: "Entrypoint", Value: ""}
	}

	// Validate max fee > 0
	if tx.MaxFee == 0 {
		return &ValidationError{Err: ErrMaxFeeExceeded, Field: "MaxFee", Value: tx.MaxFee}
	}

	// Validate gas limit > 0
	if tx.GasLimit == 0 {
		return &ValidationError{Err: ErrGasLimitExceeded, Field: "GasLimit", Value: tx.GasLimit}
	}

	// Validate signature exists
	if len(tx.Signature) == 0 {
		return &ValidationError{Err: ErrInvalidSignature, Field: "Signature", Value: nil}
	}

	return nil
}

// ComputeIntentID computes the transaction intent ID.
// IntentID = Hash(sender || nonce || contract_id || entrypoint)
func (tx *CallContractTx) ComputeIntentID(hasher Hasher) (Hash, error) {
	buf := new(bytes.Buffer)
	buf.Write(tx.Sender.Bytes())
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, tx.Nonce)
	buf.Write(nonceBytes)
	buf.Write(tx.ContractID[:])
	buf.Write([]byte(tx.Entrypoint))
	return hasher.Hash(buf.Bytes())
}

// Encode writes the call transaction to the writer
func (tx *CallContractTx) Encode(w io.Writer) error {
	// Write sender (20 bytes)
	if _, err := w.Write(tx.Sender[:]); err != nil {
		return err
	}

	// Write nonce (8 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, tx.Nonce); err != nil {
		return err
	}

	// Write contract ID (32 bytes)
	if _, err := w.Write(tx.ContractID[:]); err != nil {
		return err
	}

	// Write entrypoint (variable length)
	if err := writeVarBytes(w, []byte(tx.Entrypoint)); err != nil {
		return err
	}

	// Write calldata (variable length)
	if err := writeVarBytes(w, tx.Calldata); err != nil {
		return err
	}

	// Write max fee (8 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, tx.MaxFee); err != nil {
		return err
	}

	// Write gas limit (8 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, tx.GasLimit); err != nil {
		return err
	}

	// Write signature length and data
	sigLen := len(tx.Signature)
	if sigLen > 255 {
		return ErrInvalidSignature
	}
	if err := binary.Write(w, binary.BigEndian, uint8(sigLen)); err != nil {
		return err
	}
	if _, err := w.Write(tx.Signature); err != nil {
		return err
	}

	return nil
}

// Decode reads a call transaction from the reader
func (tx *CallContractTx) Decode(r io.Reader) error {
	// Read sender (20 bytes)
	if _, err := io.ReadFull(r, tx.Sender[:]); err != nil {
		return err
	}

	// Read nonce (8 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &tx.Nonce); err != nil {
		return err
	}

	// Read contract ID (32 bytes)
	if _, err := io.ReadFull(r, tx.ContractID[:]); err != nil {
		return err
	}

	// Read entrypoint (variable length)
	entrypointBytes, err := readVarBytes(r)
	if err != nil {
		return err
	}
	tx.Entrypoint = string(entrypointBytes)

	// Read calldata (variable length)
	tx.Calldata, err = readVarBytes(r)
	if err != nil {
		return err
	}

	// Read max fee (8 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &tx.MaxFee); err != nil {
		return err
	}

	// Read gas limit (8 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &tx.GasLimit); err != nil {
		return err
	}

	// Read signature
	var sigLen uint8
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return err
	}
	tx.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r, tx.Signature); err != nil {
		return err
	}

	return nil
}