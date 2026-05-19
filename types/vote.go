package types

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io"
)

// VoteType distinguishes between prevotes and precommits.
type VoteType uint8

const (
	VotePrevote   VoteType = 1
	VotePrecommit VoteType = 2
)

func (vt VoteType) String() string {
	switch vt {
	case VotePrevote:
		return "prevote"
	case VotePrecommit:
		return "precommit"
	default:
		return fmt.Sprintf("unknown(%d)", vt)
	}
}

// Vote represents a validator's vote on a block at a given height/round.
// Votes are signed by the validator and validated against the epoch's
// validator snapshot (not the latest validator state).
type Vote struct {
	VoteType  VoteType
	Height    uint64
	Round     uint32
	BlockHash Hash
	Validator Address
	Signature []byte
}

// VoteHash computes the hash of the vote's content (everything except signature).
// This is what gets signed.
func (v *Vote) VoteHash(hasher Hasher) (Hash, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, uint8(v.VoteType)); err != nil {
		return Hash{}, err
	}
	if err := binary.Write(&buf, binary.BigEndian, v.Height); err != nil {
		return Hash{}, err
	}
	if err := binary.Write(&buf, binary.BigEndian, v.Round); err != nil {
		return Hash{}, err
	}
	buf.Write(v.BlockHash[:])
	buf.Write(v.Validator[:])
	return hasher.Hash(buf.Bytes())
}

// Sign signs the vote hash with the given private key.
func (v *Vote) Sign(privKey ed25519.PrivateKey) error {
	hash, err := v.VoteHash(SHA256Hasher{})
	if err != nil {
		return err
	}
	v.Signature = ed25519.Sign(privKey, hash[:])
	return nil
}

// Verify checks the vote's signature against the given public key.
func (v *Vote) Verify(pubKey [32]byte) bool {
	hash, err := v.VoteHash(SHA256Hasher{})
	if err != nil {
		return false
	}
	return ed25519.Verify(pubKey[:], hash[:], v.Signature)
}

// Encode serializes the vote to binary (deterministic BigEndian).
// Layout: VoteType(1) + Height(8) + Round(4) + BlockHash(32) + Validator(20) + SigLen(4) + Signature(N)
func (v *Vote) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, uint8(v.VoteType)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.Height); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, v.Round); err != nil {
		return err
	}
	if _, err := w.Write(v.BlockHash[:]); err != nil {
		return err
	}
	if _, err := w.Write(v.Validator[:]); err != nil {
		return err
	}
	// Signature: length-prefixed
	sigLen := uint32(len(v.Signature))
	if err := binary.Write(w, binary.BigEndian, sigLen); err != nil {
		return err
	}
	if _, err := w.Write(v.Signature); err != nil {
		return err
	}
	return nil
}

// Decode deserializes a vote from binary.
func (v *Vote) Decode(r io.Reader) error {
	var voteType uint8
	if err := binary.Read(r, binary.BigEndian, &voteType); err != nil {
		return err
	}
	v.VoteType = VoteType(voteType)

	if err := binary.Read(r, binary.BigEndian, &v.Height); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &v.Round); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, v.BlockHash[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, v.Validator[:]); err != nil {
		return err
	}
	var sigLen uint32
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return err
	}
	if sigLen > 1024 { // sanity check
		return fmt.Errorf("%w: signature too long (%d)", ErrInvalidEncoding, sigLen)
	}
	v.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r, v.Signature); err != nil {
		return err
	}
	return nil
}

// Validate performs basic validation of vote fields (does NOT verify signature).
func (v *Vote) Validate() error {
	if v.VoteType != VotePrevote && v.VoteType != VotePrecommit {
		return fmt.Errorf("invalid vote type: %d", v.VoteType)
	}
	if len(v.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}
	if len(v.Signature) > 512 {
		return fmt.Errorf("signature too long: %d", len(v.Signature))
	}
	return nil
}