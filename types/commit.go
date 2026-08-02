package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// CommitProof proves that a block was committed by >=2/3 of voting power.
// This is the finality proof for a block.
type CommitProof struct {
	Height      uint64
	BlockHash   Hash
	Precommits  []Vote // precommit votes that committed this block
	TotalPower  uint64 // total voting power of the epoch's validator set
	SignedPower uint64 // sum of voting power of validators that precommitted
	SetHash     Hash   // hash of the validator snapshot this proof is against
}

// HasTwoThirdsMajority returns true if signed power >= 2/3 of total power.
func (c *CommitProof) HasTwoThirdsMajority() bool {
	if c.TotalPower == 0 {
		return false
	}
	// Check: signed_power * 3 >= total_power * 2
	// Using multiplication to avoid floating point
	return c.SignedPower*3 >= c.TotalPower*2
}

// Encode serializes the commit proof to binary.
// Layout: Height(8) + BlockHash(32) + PrecommitCount(8) + [Vote entries] + TotalPower(8) + SignedPower(8) + SetHash(32)
func (c *CommitProof) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, c.Height); err != nil {
		return err
	}
	if _, err := w.Write(c.BlockHash[:]); err != nil {
		return err
	}
	// Precommits count
	if err := binary.Write(w, binary.BigEndian, uint64(len(c.Precommits))); err != nil {
		return err
	}
	// Each precommit vote
	for _, v := range c.Precommits {
		if err := v.Encode(w); err != nil {
			return err
		}
	}
	if err := binary.Write(w, binary.BigEndian, c.TotalPower); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.SignedPower); err != nil {
		return err
	}
	if _, err := w.Write(c.SetHash[:]); err != nil {
		return err
	}
	return nil
}

// Decode deserializes a commit proof from binary.
func (c *CommitProof) Decode(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &c.Height); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, c.BlockHash[:]); err != nil {
		return err
	}
	var count uint64
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return err
	}
	if count > 10000 { // sanity check
		return fmt.Errorf("%w: too many precommits (%d)", ErrInvalidEncoding, count)
	}
	c.Precommits = make([]Vote, count)
	for i := uint64(0); i < count; i++ {
		if err := c.Precommits[i].Decode(r); err != nil {
			return fmt.Errorf("precommit %d: %w", i, err)
		}
	}
	if err := binary.Read(r, binary.BigEndian, &c.TotalPower); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &c.SignedPower); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, c.SetHash[:]); err != nil {
		return err
	}
	return nil
}

// Validate checks the commit proof for internal consistency.
func (c *CommitProof) Validate() error {
	if len(c.Precommits) == 0 {
		return fmt.Errorf("empty precommits")
	}
	if c.SignedPower > c.TotalPower {
		return fmt.Errorf("signed power (%d) exceeds total power (%d)", c.SignedPower, c.TotalPower)
	}
	if !c.HasTwoThirdsMajority() {
		return fmt.Errorf("insufficient voting power: %d/%d", c.SignedPower, c.TotalPower)
	}
	return nil
}

// CommitProofHash computes the hash of the commit proof content.
func (c *CommitProof) CommitProofHash(hasher Hasher) (Hash, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, c.Height); err != nil {
		return Hash{}, err
	}
	buf.Write(c.BlockHash[:])
	if err := binary.Write(&buf, binary.BigEndian, c.TotalPower); err != nil {
		return Hash{}, err
	}
	if err := binary.Write(&buf, binary.BigEndian, c.SignedPower); err != nil {
		return Hash{}, err
	}
	buf.Write(c.SetHash[:])
	return hasher.Hash(buf.Bytes())
}
