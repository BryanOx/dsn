package types

import (
	"encoding/binary"
	"io"
)

// MaxVarBytesLength is the maximum allowed length for variable-length byte fields (1 MiB)
const MaxVarBytesLength = 1 * 1024 * 1024

// writeVarBytes writes a length-prefixed byte slice to the writer.
// It writes a uint32 length in big-endian format followed by the data.
func writeVarBytes(w io.Writer, data []byte) error {
	// Validate length doesn't exceed maximum
	if uint64(len(data)) > uint64(MaxVarBytesLength) {
		return ErrInvalidEncoding
	}

	// Write length as uint32 big-endian
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	if _, err := w.Write(lengthBuf); err != nil {
		return err
	}

	// Write data
	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

// readVarBytes reads a length-prefixed byte slice from the reader.
// It reads a uint32 length first, then reads that many bytes.
func readVarBytes(r io.Reader) ([]byte, error) {
	// Read length
	lengthBuf := make([]byte, 4)
	n, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, ErrInvalidEncoding
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	// Validate length doesn't exceed maximum
	if length > MaxVarBytesLength {
		return nil, ErrPayloadTooLarge
	}

	// Read data
	data := make([]byte, length)
	n, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}
	if uint32(n) != length {
		return nil, ErrInvalidEncoding
	}

	return data, nil
}
