package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
)

// MaxContractCodeSize is the maximum allowed WASM bytecode size (1 MiB)
const MaxContractCodeSize = 1 * 1024 * 1024

// DeployContractTx carries the WASM bytecode for contract deployment.
// The contract is assigned a deterministic ContractID derived from
// deployer, nonce, and code hash.
type DeployContractTx struct {
	Sender    Address
	Nonce     uint64
	WasmCode  []byte
	CodeHash  Hash
	MaxFee    uint64
	GasLimit  uint64
	Signature []byte
}

// Validate validates the deploy transaction fields
func (tx *DeployContractTx) Validate() error {
	// Validate sender address
	if len(tx.Sender.Bytes()) != 20 {
		return &ValidationError{Err: ErrInvalidAddress, Field: "Sender", Value: tx.Sender.String()}
	}

	// Validate nonce > 0
	if tx.Nonce == 0 {
		return &ValidationError{Err: ErrNonceMismatch, Field: "Nonce", Value: tx.Nonce}
	}

	// Validate code size
	if len(tx.WasmCode) == 0 {
		return &ValidationError{Err: ErrInvalidEncoding, Field: "WasmCode", Value: nil}
	}
	if uint64(len(tx.WasmCode)) > MaxContractCodeSize {
		return &ValidationError{Err: ErrContractSizeExceeded, Field: "WasmCode", Value: len(tx.WasmCode)}
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
// IntentID = Hash(sender || nonce || code_hash)
func (tx *DeployContractTx) ComputeIntentID(hasher Hasher) (Hash, error) {
	buf := new(bytes.Buffer)
	buf.Write(tx.Sender.Bytes())
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, tx.Nonce)
	buf.Write(nonceBytes)
	buf.Write(tx.CodeHash[:])
	return hasher.Hash(buf.Bytes())
}

// Encode writes the deploy transaction to the writer
func (tx *DeployContractTx) Encode(w io.Writer) error {
	// Write sender (20 bytes)
	if _, err := w.Write(tx.Sender[:]); err != nil {
		return err
	}

	// Write nonce (8 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, tx.Nonce); err != nil {
		return err
	}

	// Write WASM code (variable length)
	if err := writeVarBytes(w, tx.WasmCode); err != nil {
		return err
	}

	// Write code hash (32 bytes)
	if _, err := w.Write(tx.CodeHash[:]); err != nil {
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

// Decode reads a deploy transaction from the reader
func (tx *DeployContractTx) Decode(r io.Reader) error {
	// Read sender (20 bytes)
	if _, err := io.ReadFull(r, tx.Sender[:]); err != nil {
		return err
	}

	// Read nonce (8 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &tx.Nonce); err != nil {
		return err
	}

	// Read WASM code (variable length)
	code, err := readVarBytes(r)
	if err != nil {
		return err
	}
	tx.WasmCode = code

	// Compute code hash from WASM code
	if len(tx.WasmCode) > 0 {
		tx.CodeHash = Hash(sha256.Sum256(tx.WasmCode))
	}

	// Read code hash (32 bytes)
	if _, err := io.ReadFull(r, tx.CodeHash[:]); err != nil {
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
