package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math/big"
)

// Base58 alphabet (Bitcoin style)
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Address is a 20-byte identifier
type Address [20]byte

// AddressFromBytes creates an Address from raw bytes, validating length == 20
func AddressFromBytes(b []byte) (Address, error) {
	if len(b) != 20 {
		return Address{}, ErrInvalidAddress
	}
	var addr Address
	copy(addr[:], b)
	return addr, nil
}

// Bytes returns the raw 20-byte representation
func (a Address) Bytes() []byte {
	return a[:]
}

// String returns the Base58Check encoded representation
func (a Address) String() string {
	return base58CheckEncode(a[:], 0x00) // 0x00 version byte for mainnet
}

// Encode writes the raw 20 bytes to the writer
func (a Address) Encode(w io.Writer) error {
	_, err := w.Write(a[:])
	return err
}

// Decode reads exactly 20 bytes from the reader
func (a *Address) Decode(r io.Reader) error {
	_, err := io.ReadFull(r, a[:])
	return err
}

// ParseAddress decodes a Base58Check string
func ParseAddress(s string) (Address, error) {
	if s == "" {
		return Address{}, ErrInvalidAddress
	}

	payload, err := base58CheckDecode(s)
	if err != nil {
		return Address{}, err
	}

	// Remove version byte and verify we have 20 bytes left
	if len(payload) != 21 {
		return Address{}, ErrInvalidAddress
	}

	return AddressFromBytes(payload[1:])
}

// base58CheckEncode encodes data with a version byte and Base58Check checksum
func base58CheckEncode(payload []byte, version byte) string {
	// Prepend version byte
	data := append([]byte{version}, payload...)

	// Double SHA256 checksum
	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])

	// Append first 4 bytes of checksum
	data = append(data, hash2[:4]...)

	// Encode using Base58
	return base58Encode(data)
}

// base58CheckDecode decodes a Base58Check string and verifies checksum
func base58CheckDecode(s string) ([]byte, error) {
	// Decode Base58
	data := base58Decode(s)
	if len(data) < 5 {
		return nil, ErrInvalidAddress
	}

	// Split payload and checksum
	payload := data[:len(data)-4]
	checksum := data[len(data)-4:]

	// Verify checksum
	hash1 := sha256.Sum256(payload)
	hash2 := sha256.Sum256(hash1[:])

	if !bytes.Equal(checksum, hash2[:4]) {
		return nil, ErrInvalidAddress
	}

	return payload, nil
}

// base58Encode encodes bytes to Base58
func base58Encode(data []byte) string {
	// Count leading zeros
	var zeros int
	for _, b := range data {
		if b == 0 {
			zeros++
		} else {
			break
		}
	}

	// Convert to big integer
	num := new(big.Int).SetBytes(data)

	// Build result
	var result []byte
	for num.Sign() > 0 {
		mod := new(big.Int)
		num.DivMod(num, big.NewInt(58), mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// Reverse result
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	// Add leading '1's for each leading zero byte
	for i := 0; i < zeros; i++ {
		result = append([]byte{'1'}, result...)
	}

	if len(result) == 0 {
		return "1"
	}

	return string(result)
}

// base58Decode decodes a Base58 string to bytes
func base58Decode(s string) []byte {
	// Count leading '1's (zeros)
	var zeros int
	for _, c := range s {
		if c == '1' {
			zeros++
		} else {
			break
		}
	}

	// Process each character
	num := new(big.Int)
	for _, c := range s {
		idx := bytes.IndexByte([]byte(base58Alphabet), byte(c))
		if idx == -1 {
			continue // Skip invalid chars
		}
		num.Mul(num, big.NewInt(58))
		num.Add(num, big.NewInt(int64(idx)))
	}

	// Convert to bytes
	result := num.Bytes()

	// Add leading zero bytes
	for i := 0; i < zeros; i++ {
		result = append([]byte{0}, result...)
	}

	return result
}

// ReadUint64 reads a uint64 from reader in big-endian
func ReadUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}

// WriteUint64 writes a uint64 to writer in big-endian
func WriteUint64(w io.Writer, v uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

// ReadUint32 reads a uint32 from reader in big-endian
func ReadUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

// WriteUint32 writes a uint32 to writer in big-endian
func WriteUint32(w io.Writer, v uint32) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

// ReadUint16 reads a uint16 from reader in big-endian
func ReadUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

// WriteUint16 writes a uint16 to writer in big-endian
func WriteUint16(w io.Writer, v uint16) error {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

// ReadBytesFixed reads exactly n bytes from reader
func ReadBytesFixed(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// WriteBytesFixed writes exactly n bytes to writer
func WriteBytesFixed(w io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := w.Write(data)
	return err
}