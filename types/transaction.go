package types

import (
	"bytes"
	"encoding/binary"
	"io"
)

// TxType indicates the type of transaction payload
type TxType uint8

const (
	TxTypeStandard            TxType = 0 // Standard transfer transaction
	TxTypeDeployContract      TxType = 1 // Contract deployment
	TxTypeCallContract        TxType = 2 // Contract call
	TxTypeValidatorRegistration TxType = 3 // Validator registration
)

type Transaction struct {
	Version     uint16
	ChainID     uint32
	IntentID    Hash
	Sender      Address
	Nonce       uint64
	Payload     []byte
	Constraints []byte
	MaxFee      uint64
	GasLimit    uint64
	Timestamp   uint64
	Signature   []byte
	TxType      TxType // NEW: indicates if payload contains a contract tx
}

func NewTransaction(
	version uint16,
	chainID uint32,
	sender Address,
	nonce uint64,
	payload []byte,
	constraints []byte,
	maxFee uint64,
	gasLimit uint64,
	timestamp uint64,
) *Transaction {
	return &Transaction{
		Version:     version,
		ChainID:     chainID,
		Sender:      sender,
		Nonce:       nonce,
		Payload:     payload,
		Constraints: constraints,
		MaxFee:      maxFee,
		GasLimit:    gasLimit,
		Timestamp:   timestamp,
		TxType:      TxTypeStandard,
	}
}

// IntentID = Hash(sender || nonce || payload || constraints)
func (tx *Transaction) ComputeIntentID(h Hasher) (Hash, error) {
	buf := new(bytes.Buffer)
	buf.Write(tx.Sender.Bytes())
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, tx.Nonce)
	buf.Write(nonceBytes)
	buf.Write(tx.Payload)
	buf.Write(tx.Constraints)
	return h.Hash(buf.Bytes())
}

func (tx *Transaction) Validate() error {
	// Validate sender address
	if len(tx.Sender.Bytes()) != 20 {
		return &ValidationError{Err: ErrInvalidAddress, Field: "Sender", Value: tx.Sender.String()}
	}

	// Validate nonce > 0
	if tx.Nonce == 0 {
		return &ValidationError{Err: ErrNonceMismatch, Field: "Nonce", Value: tx.Nonce}
	}

	// Validate signature exists
	if len(tx.Signature) == 0 {
		return &ValidationError{Err: ErrInvalidSignature, Field: "Signature", Value: nil}
	}

	// Validate timestamp
	if err := ValidateTimestamp(tx.Timestamp); err != nil {
		return &ValidationError{Err: err, Field: "Timestamp", Value: tx.Timestamp}
	}

	// Validate max_fee > 0
	if tx.MaxFee == 0 {
		return &ValidationError{Err: ErrMaxFeeExceeded, Field: "MaxFee", Value: tx.MaxFee}
	}

	return nil
}

func (tx *Transaction) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, tx.Version); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, tx.ChainID); err != nil {
		return err
	}
	if _, err := w.Write(tx.IntentID[:]); err != nil {
		return err
	}
	if _, err := w.Write(tx.Sender[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, tx.Nonce); err != nil {
		return err
	}
	if err := writeVarBytes(w, tx.Payload); err != nil {
		return err
	}
	if err := writeVarBytes(w, tx.Constraints); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, tx.MaxFee); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, tx.GasLimit); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, tx.Timestamp); err != nil {
		return err
	}

	// Signature length as uint8 (max 255)
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

	// Write TxType (default to 0 for standard)
	if err := binary.Write(w, binary.BigEndian, tx.TxType); err != nil {
		return err
	}

	return nil
}

func (tx *Transaction) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &tx.Version); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &tx.ChainID); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, tx.IntentID[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, tx.Sender[:]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &tx.Nonce); err != nil {
		return err
	}

	payload, err := readVarBytes(r)
	if err != nil {
		return err
	}
	tx.Payload = payload

	constraints, err := readVarBytes(r)
	if err != nil {
		return err
	}
	tx.Constraints = constraints

	if err := binary.Read(r, binary.BigEndian, &tx.MaxFee); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &tx.GasLimit); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &tx.Timestamp); err != nil {
		return err
	}

	var sigLen uint8
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return err
	}
	tx.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r, tx.Signature); err != nil {
		return err
	}

	// Read TxType (default to standard if not present in old transactions)
	if err := binary.Read(r, binary.BigEndian, &tx.TxType); err != nil {
		tx.TxType = TxTypeStandard // backward compatibility
	}

	return nil
}

// DeployContract returns the embedded DeployContractTx if TxType is TxTypeDeployContract.
// Returns nil if the transaction is not a contract deployment.
func (tx *Transaction) DeployContract() (*DeployContractTx, error) {
	if tx.TxType != TxTypeDeployContract {
		return nil, nil
	}
	contractTx := &DeployContractTx{}
	if err := contractTx.Decode(bytes.NewReader(tx.Payload)); err != nil {
		return nil, err
	}
	return contractTx, nil
}

// CallContract returns the embedded CallContractTx if TxType is TxTypeCallContract.
// Returns nil if the transaction is not a contract call.
func (tx *Transaction) CallContract() (*CallContractTx, error) {
	if tx.TxType != TxTypeCallContract {
		return nil, nil
	}
	contractTx := &CallContractTx{}
	if err := contractTx.Decode(bytes.NewReader(tx.Payload)); err != nil {
		return nil, err
	}
	return contractTx, nil
}

// ValidatorRegistrationMsg contains the data for validator registration.
type ValidatorRegistrationMsg struct {
	Address    Address
	PubKey     []byte
	Stake      uint64
	Commission uint64 // basis points, e.g. 1000 = 10%
}

// Encode encodes the validator registration message to bytes.
func (m *ValidatorRegistrationMsg) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Address (20 bytes)
	if _, err := buf.Write(m.Address[:]); err != nil {
		return nil, err
	}
	// PubKey (32 bytes)
	if len(m.PubKey) != 32 {
		return nil, ErrInvalidEncoding
	}
	if _, err := buf.Write(m.PubKey); err != nil {
		return nil, err
	}
	// Stake (8 bytes)
	if err := binary.Write(buf, binary.BigEndian, m.Stake); err != nil {
		return nil, err
	}
	// Commission (8 bytes)
	if err := binary.Write(buf, binary.BigEndian, m.Commission); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode decodes the validator registration message from bytes.
func (m *ValidatorRegistrationMsg) Decode(data []byte) error {
	r := bytes.NewReader(data)
	// Address (20 bytes)
	if _, err := io.ReadFull(r, m.Address[:]); err != nil {
		return err
	}
	// PubKey (32 bytes)
	m.PubKey = make([]byte, 32)
	if _, err := io.ReadFull(r, m.PubKey); err != nil {
		return err
	}
	// Stake (8 bytes)
	if err := binary.Read(r, binary.BigEndian, &m.Stake); err != nil {
		return err
	}
	// Commission (8 bytes)
	return binary.Read(r, binary.BigEndian, &m.Commission)
}

// NewValidatorRegistrationTx creates a new transaction with validator registration payload.
func NewValidatorRegistrationTx(sender Address, nonce uint64, msg *ValidatorRegistrationMsg, maxFee uint64, timestamp uint64, chainID uint32) (*Transaction, error) {
	payload, err := msg.Encode()
	if err != nil {
		return nil, err
	}

	return &Transaction{
		Version:     1,
		ChainID:     chainID,
		Sender:      sender,
		Nonce:       nonce,
		Payload:     payload,
		MaxFee:      maxFee,
		GasLimit:    50000, // sufficient for registration
		Timestamp:   timestamp,
		TxType:      TxTypeValidatorRegistration,
	}, nil
}

// ValidatorRegistration returns the embedded ValidatorRegistrationMsg if TxType is TxTypeValidatorRegistration.
// Returns nil if the transaction is not a validator registration.
func (tx *Transaction) ValidatorRegistration() (*ValidatorRegistrationMsg, error) {
	if tx.TxType != TxTypeValidatorRegistration {
		return nil, nil
	}
	msg := &ValidatorRegistrationMsg{}
	if err := msg.Decode(tx.Payload); err != nil {
		return nil, err
	}
	return msg, nil
}