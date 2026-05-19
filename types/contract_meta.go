package types

import (
	"io"
)

// ContractMetadata holds information about a deployed contract.
type ContractMetadata struct {
	ContractID  Hash
	CodeHash    Hash
	Deployer    Address
	BlockHeight uint64
}

// Encode writes the contract metadata to the writer
func (m *ContractMetadata) Encode(w io.Writer) error {
	// Write contract ID (32 bytes)
	if _, err := w.Write(m.ContractID[:]); err != nil {
		return err
	}

	// Write code hash (32 bytes)
	if _, err := w.Write(m.CodeHash[:]); err != nil {
		return err
	}

	// Write deployer (20 bytes)
	if _, err := w.Write(m.Deployer[:]); err != nil {
		return err
	}

	// Write block height (8 bytes big-endian)
	if err := writeVarUint64(w, m.BlockHeight); err != nil {
		return err
	}

	return nil
}

// Decode reads contract metadata from the reader
func (m *ContractMetadata) Decode(r io.Reader) error {
	// Read contract ID (32 bytes)
	if _, err := io.ReadFull(r, m.ContractID[:]); err != nil {
		return err
	}

	// Read code hash (32 bytes)
	if _, err := io.ReadFull(r, m.CodeHash[:]); err != nil {
		return err
	}

	// Read deployer (20 bytes)
	if _, err := io.ReadFull(r, m.Deployer[:]); err != nil {
		return err
	}

	// Read block height (varint)
	var err error
	m.BlockHeight, err = readVarUint64(r)
	if err != nil {
		return err
	}

	return nil
}

// writeVarUint64 writes a uint64 in variable-length encoding
func writeVarUint64(w io.Writer, v uint64) error {
	if v < 0xFD {
		return writeVarBytes(w, []byte{byte(v)})
	} else if v <= 0xFFFF {
		buf := []byte{0xFD}
		buf = append(buf, byte(v>>8), byte(v))
		return writeVarBytes(w, buf)
	} else if v <= 0xFFFFFFFF {
		buf := []byte{0xFE}
		buf = append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
		return writeVarBytes(w, buf)
	}
	buf := []byte{0xFF}
	buf = append(buf, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	return writeVarBytes(w, buf)
}

// readVarUint64 reads a uint64 in variable-length encoding
func readVarUint64(r io.Reader) (uint64, error) {
	// Read first byte to determine encoding
	first := make([]byte, 1)
	if _, err := io.ReadFull(r, first); err != nil {
		return 0, err
	}

	switch first[0] {
	case 0xFD:
		buf := make([]byte, 2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		return uint64(buf[0])<<8 | uint64(buf[1]), nil
	case 0xFE:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		return uint64(buf[0])<<24 | uint64(buf[1])<<16 | uint64(buf[2])<<8 | uint64(buf[3]), nil
	case 0xFF:
		buf := make([]byte, 8)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		return uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 | uint64(buf[3])<<32 |
			uint64(buf[4])<<24 | uint64(buf[5])<<16 | uint64(buf[6])<<8 | uint64(buf[7]), nil
	default:
		return uint64(first[0]), nil
	}
}