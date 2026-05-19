package types

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io"
)

// Proposal is a block proposal from a proposer at a given height/round.
type Proposal struct {
	Height    uint64
	Round     uint32
	BlockHash Hash
	Proposer  Address
	Signature []byte
}

// ProposalHash computes the hash of the proposal content (everything except signature).
func (p *Proposal) ProposalHash(hasher Hasher) (Hash, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, p.Height); err != nil {
		return Hash{}, err
	}
	if err := binary.Write(&buf, binary.BigEndian, p.Round); err != nil {
		return Hash{}, err
	}
	buf.Write(p.BlockHash[:])
	buf.Write(p.Proposer[:])
	return hasher.Hash(buf.Bytes())
}

// Sign signs the proposal hash with the given private key.
func (p *Proposal) Sign(privKey ed25519.PrivateKey) error {
	hash, err := p.ProposalHash(SHA256Hasher{})
	if err != nil {
		return err
	}
	p.Signature = ed25519.Sign(privKey, hash[:])
	return nil
}

// Verify checks the proposal's signature against the given public key.
func (p *Proposal) Verify(pubKey [32]byte) bool {
	hash, err := p.ProposalHash(SHA256Hasher{})
	if err != nil {
		return false
	}
	return ed25519.Verify(pubKey[:], hash[:], p.Signature)
}

// Encode serializes the proposal to binary.
// Layout: Height(8) + Round(4) + BlockHash(32) + Proposer(20) + SigLen(4) + Signature(N)
func (p *Proposal) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, p.Height); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, p.Round); err != nil {
		return err
	}
	if _, err := w.Write(p.BlockHash[:]); err != nil {
		return err
	}
	if _, err := w.Write(p.Proposer[:]); err != nil {
		return err
	}
	sigLen := uint32(len(p.Signature))
	if err := binary.Write(w, binary.BigEndian, sigLen); err != nil {
		return err
	}
	if _, err := w.Write(p.Signature); err != nil {
		return err
	}
	return nil
}

// Decode deserializes a proposal from binary.
func (p *Proposal) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &p.Height); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &p.Round); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, p.BlockHash[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, p.Proposer[:]); err != nil {
		return err
	}
	var sigLen uint32
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return err
	}
	if sigLen > 1024 {
		return fmt.Errorf("%w: signature too long (%d)", ErrInvalidEncoding, sigLen)
	}
	p.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r, p.Signature); err != nil {
		return err
	}
	return nil
}

// Validate performs basic validation of proposal fields.
func (p *Proposal) Validate() error {
	if len(p.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}
	if len(p.Signature) > 512 {
		return fmt.Errorf("signature too long: %d", len(p.Signature))
	}
	return nil
}