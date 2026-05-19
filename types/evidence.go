package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// EvidenceType distinguishes between different evidence types.
type EvidenceType uint8

const (
	EvidenceTypeDoubleSign    EvidenceType = 1
	EvidenceTypeInvalidCommit EvidenceType = 2
	EvidenceTypeMalformedVote EvidenceType = 3
)

// Evidence is the interface that all evidence types must implement.
// Evidence objects are replayable, serializable structures used for
// deterministic Byzantine fault detection in the DSN blockchain.
type Evidence interface {
	Type() EvidenceType
	Height() uint64
	Validate() error
	Encode(w io.Writer) error
	Decode(r io.Reader) error
	EvidenceHash(hasher Hasher) (Hash, error)
}

// DoubleSignEvidence represents a validator signing two different votes
// at the same height/round/vote type (a classic Byzantine fault).
type DoubleSignEvidence struct {
	VoteA Vote // first vote
	VoteB Vote // second vote (different block hash)
}

// Type returns the evidence type.
func (e *DoubleSignEvidence) Type() EvidenceType {
	return EvidenceTypeDoubleSign
}

// Height returns the height of the evidence.
func (e *DoubleSignEvidence) Height() uint64 {
	return e.VoteA.Height
}

// Validate checks that both votes form valid double sign evidence.
func (e *DoubleSignEvidence) Validate() error {
	// Both votes must be the same type
	if e.VoteA.VoteType != e.VoteB.VoteType {
		return fmt.Errorf("vote type mismatch: %v != %v", e.VoteA.VoteType, e.VoteB.VoteType)
	}

	// Same validator address
	if e.VoteA.Validator != e.VoteB.Validator {
		return fmt.Errorf("validator address mismatch: %v != %v", e.VoteA.Validator, e.VoteB.Validator)
	}

	// Same height
	if e.VoteA.Height != e.VoteB.Height {
		return fmt.Errorf("height mismatch: %d != %d", e.VoteA.Height, e.VoteB.Height)
	}

	// Same round
	if e.VoteA.Round != e.VoteB.Round {
		return fmt.Errorf("round mismatch: %d != %d", e.VoteA.Round, e.VoteB.Round)
	}

	// Different block hashes (must differ!)
	if e.VoteA.BlockHash == e.VoteB.BlockHash {
		return fmt.Errorf("block hash must be different for double sign")
	}

	// Validate individual votes
	if err := e.VoteA.Validate(); err != nil {
		return fmt.Errorf("VoteA validation failed: %w", err)
	}
	if err := e.VoteB.Validate(); err != nil {
		return fmt.Errorf("VoteB validation failed: %w", err)
	}

	return nil
}

// Encode serializes the double sign evidence to binary.
// Layout: VoteType(1) + VoteA.Encode + VoteB.Encode
func (e *DoubleSignEvidence) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, uint8(e.VoteA.VoteType)); err != nil {
		return err
	}
	if err := e.VoteA.Encode(w); err != nil {
		return err
	}
	if err := e.VoteB.Encode(w); err != nil {
		return err
	}
	return nil
}

// Decode deserializes a double sign evidence from binary.
func (e *DoubleSignEvidence) Decode(r io.Reader) error {
	// Read VoteType to know how to decode both votes
	var voteType uint8
	if err := binary.Read(r, binary.BigEndian, &voteType); err != nil {
		return err
	}

	// Decode VoteA - it will read the vote type from the stream
	// but we've already consumed it, so we need to re-read
	// Actually, let's reset and decode properly
	// The encoding includes VoteType, so Decode should handle it
	if err := e.VoteA.Decode(r); err != nil {
		return err
	}
	if err := e.VoteB.Decode(r); err != nil {
		return err
	}
	return nil
}

// EvidenceHash computes the hash of the encoded evidence.
func (e *DoubleSignEvidence) EvidenceHash(hasher Hasher) (Hash, error) {
	var buf bytes.Buffer
	if err := e.Encode(&buf); err != nil {
		return Hash{}, err
	}
	return hasher.Hash(buf.Bytes())
}

// InvalidCommitEvidence represents a commit proof that was submitted
// but is invalid (e.g., incorrect signatures, insufficient voting power, etc.).
type InvalidCommitEvidence struct {
	Proof  CommitProof // the invalid proof
	Reason string      // why it was rejected (NOT part of hash)
}

// Type returns the evidence type.
func (e *InvalidCommitEvidence) Type() EvidenceType {
	return EvidenceTypeInvalidCommit
}

// Height returns the height of the evidence.
func (e *InvalidCommitEvidence) Height() uint64 {
	return e.Proof.Height
}

// Validate checks that the evidence has a valid proof and non-empty reason.
func (e *InvalidCommitEvidence) Validate() error {
	// Proof.Validate() is called - it may fail but that's fine
	// The proof CAN BE INVALID - this evidence documents invalid proofs
	if err := e.Proof.Validate(); err != nil {
		// We accept invalid proofs, but we still call validate
		// This is expected for InvalidCommitEvidence
		_ = err
	}

	// Reason must not be empty
	if len(e.Reason) == 0 {
		return fmt.Errorf("empty reason")
	}

	return nil
}

// Encode serializes the invalid commit evidence to binary.
// Layout: CommitProof.Encode + Reason(length-prefixed)
func (e *InvalidCommitEvidence) Encode(w io.Writer) error {
	if err := e.Proof.Encode(w); err != nil {
		return err
	}
	// Reason: length-prefixed string
	reasonLen := uint32(len(e.Reason))
	if err := binary.Write(w, binary.BigEndian, reasonLen); err != nil {
		return err
	}
	if _, err := w.Write([]byte(e.Reason)); err != nil {
		return err
	}
	return nil
}

// Decode deserializes an invalid commit evidence from binary.
func (e *InvalidCommitEvidence) Decode(r io.Reader) error {
	if err := e.Proof.Decode(r); err != nil {
		return err
	}
	var reasonLen uint32
	if err := binary.Read(r, binary.BigEndian, &reasonLen); err != nil {
		return err
	}
	if reasonLen > 1024 { // sanity check
		return fmt.Errorf("%w: reason too long (%d)", ErrInvalidEncoding, reasonLen)
	}
	reason := make([]byte, reasonLen)
	if _, err := io.ReadFull(r, reason); err != nil {
		return err
	}
	e.Reason = string(reason)
	return nil
}

// EvidenceHash computes the hash of the encoded evidence (WITHOUT reason).
func (e *InvalidCommitEvidence) EvidenceHash(hasher Hasher) (Hash, error) {
	// Hash without reason - reason is not consensus-critical
	var buf bytes.Buffer
	if err := e.Proof.Encode(&buf); err != nil {
		return Hash{}, err
	}
	return hasher.Hash(buf.Bytes())
}

// MalformedVoteEvidence represents a vote that fails basic structural validation.
type MalformedVoteEvidence struct {
	BadVote Vote   // the malformed vote
	Reason  string // why it was rejected (NOT part of hash)
}

// Type returns the evidence type.
func (e *MalformedVoteEvidence) Type() EvidenceType {
	return EvidenceTypeMalformedVote
}

// Height returns the height of the evidence.
func (e *MalformedVoteEvidence) Height() uint64 {
	return e.BadVote.Height
}

// Validate checks that the evidence has a valid vote type, height, and non-empty reason.
func (e *MalformedVoteEvidence) Validate() error {
	// VoteType must be valid (Prevote or Precommit)
	if e.BadVote.VoteType != VotePrevote && e.BadVote.VoteType != VotePrecommit {
		return fmt.Errorf("invalid vote type: %d", e.BadVote.VoteType)
	}

	// Height must be > 0
	if e.BadVote.Height == 0 {
		return fmt.Errorf("height must be > 0")
	}

	// Reason must not be empty
	if len(e.Reason) == 0 {
		return fmt.Errorf("empty reason")
	}

	return nil
}

// Encode serializes the malformed vote evidence to binary.
// Layout: VoteType(1) + BadVote.Encode + Reason(length-prefixed)
func (e *MalformedVoteEvidence) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, uint8(e.BadVote.VoteType)); err != nil {
		return err
	}
	if err := e.BadVote.Encode(w); err != nil {
		return err
	}
	// Reason: length-prefixed string
	reasonLen := uint32(len(e.Reason))
	if err := binary.Write(w, binary.BigEndian, reasonLen); err != nil {
		return err
	}
	if _, err := w.Write([]byte(e.Reason)); err != nil {
		return err
	}
	return nil
}

// Decode deserializes a malformed vote evidence from binary.
func (e *MalformedVoteEvidence) Decode(r io.Reader) error {
	var voteType uint8
	if err := binary.Read(r, binary.BigEndian, &voteType); err != nil {
		return err
	}
	e.BadVote.VoteType = VoteType(voteType)
	if err := e.BadVote.Decode(r); err != nil {
		return err
	}
	var reasonLen uint32
	if err := binary.Read(r, binary.BigEndian, &reasonLen); err != nil {
		return err
	}
	if reasonLen > 1024 { // sanity check
		return fmt.Errorf("%w: reason too long (%d)", ErrInvalidEncoding, reasonLen)
	}
	reason := make([]byte, reasonLen)
	if _, err := io.ReadFull(r, reason); err != nil {
		return err
	}
	e.Reason = string(reason)
	return nil
}

// EvidenceHash computes the hash of the encoded evidence (WITHOUT reason).
func (e *MalformedVoteEvidence) EvidenceHash(hasher Hasher) (Hash, error) {
	// Hash without reason - reason is not consensus-critical
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, uint8(e.BadVote.VoteType)); err != nil {
		return Hash{}, err
	}
	if err := e.BadVote.Encode(&buf); err != nil {
		return Hash{}, err
	}
	return hasher.Hash(buf.Bytes())
}

// EncodeEvidence encodes any evidence type with its type byte prefix.
func EncodeEvidence(w io.Writer, ev Evidence) error {
	if err := binary.Write(w, binary.BigEndian, uint8(ev.Type())); err != nil {
		return err
	}
	return ev.Encode(w)
}

// DecodeEvidence decodes any evidence type based on the type byte.
func DecodeEvidence(r io.Reader) (Evidence, error) {
	var evidenceType uint8
	if err := binary.Read(r, binary.BigEndian, &evidenceType); err != nil {
		return nil, err
	}

	switch EvidenceType(evidenceType) {
	case EvidenceTypeDoubleSign:
		ev := &DoubleSignEvidence{}
		if err := ev.Decode(r); err != nil {
			return nil, err
		}
		return ev, nil
	case EvidenceTypeInvalidCommit:
		ev := &InvalidCommitEvidence{}
		if err := ev.Decode(r); err != nil {
			return nil, err
		}
		return ev, nil
	case EvidenceTypeMalformedVote:
		ev := &MalformedVoteEvidence{}
		if err := ev.Decode(r); err != nil {
			return nil, err
		}
		return ev, nil
	default:
		return nil, fmt.Errorf("%w: unknown type %d", ErrInvalidEvidenceType, evidenceType)
	}
}

// EncodeEvidenceList encodes a slice of Evidence objects to a writer.
func EncodeEvidenceList(w io.Writer, evidence []Evidence) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(evidence))); err != nil {
		return err
	}
	for _, ev := range evidence {
		if err := EncodeEvidence(w, ev); err != nil {
			return err
		}
	}
	return nil
}

// DecodeEvidenceList decodes a slice of Evidence objects from a reader.
func DecodeEvidenceList(r io.Reader) ([]Evidence, error) {
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	result := make([]Evidence, count)
	for i := uint32(0); i < count; i++ {
		ev, err := DecodeEvidence(r)
		if err != nil {
			return nil, err
		}
		result[i] = ev
	}
	return result, nil
}