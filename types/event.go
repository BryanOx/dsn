package types

import (
	"encoding/binary"
	"io"
)

// Event represents a smart contract event emitted during execution.
type Event struct {
	ContractID  Hash
	Topic       string
	Data        []byte
	BlockHeight uint64
	TxIndex     uint32
}

// Encode writes the event to the writer
func (e *Event) Encode(w io.Writer) error {
	// Write contract ID (32 bytes)
	if _, err := w.Write(e.ContractID[:]); err != nil {
		return err
	}

	// Write topic (variable length)
	if err := writeVarBytes(w, []byte(e.Topic)); err != nil {
		return err
	}

	// Write data (variable length)
	if err := writeVarBytes(w, e.Data); err != nil {
		return err
	}

	// Write block height (8 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, e.BlockHeight); err != nil {
		return err
	}

	// Write tx index (4 bytes big-endian)
	if err := binary.Write(w, binary.BigEndian, e.TxIndex); err != nil {
		return err
	}

	return nil
}

// Decode reads an event from the reader
func (e *Event) Decode(r io.Reader) error {
	// Read contract ID (32 bytes)
	if _, err := io.ReadFull(r, e.ContractID[:]); err != nil {
		return err
	}

	// Read topic (variable length)
	topicBytes, err := readVarBytes(r)
	if err != nil {
		return err
	}
	e.Topic = string(topicBytes)

	// Read data (variable length)
	e.Data, err = readVarBytes(r)
	if err != nil {
		return err
	}

	// Read block height (8 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &e.BlockHeight); err != nil {
		return err
	}

	// Read tx index (4 bytes big-endian)
	if err := binary.Read(r, binary.BigEndian, &e.TxIndex); err != nil {
		return err
	}

	return nil
}