package types

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"testing"
)

// Helper to create a valid vote for testing
func createTestVote(voteType VoteType, height uint64, round uint32, blockHash Hash, validator Address) Vote {
	return Vote{
		VoteType:  voteType,
		Height:    height,
		Round:     round,
		BlockHash: blockHash,
		Validator: validator,
		Signature: make([]byte, 64), // valid Ed25519 signature length
	}
}

// Helper to create a valid commit proof for testing
func createTestCommitProof(height uint64, blockHash Hash, numPrecommits int) CommitProof {
	precommits := make([]Vote, numPrecommits)
	for i := 0; i < numPrecommits; i++ {
		var addr Address
		addr[0] = byte(i)
		precommits[i] = Vote{
			VoteType:  VotePrecommit,
			Height:    height,
			Round:     0,
			BlockHash: blockHash,
			Validator: addr,
			Signature: make([]byte, 64),
		}
	}
	return CommitProof{
		Height:      height,
		BlockHash:   blockHash,
		Precommits:  precommits,
		TotalPower:  uint64(numPrecommits * 10),
		SignedPower: uint64(numPrecommits * 10),
		SetHash:     Hash{1, 2, 3},
	}
}

// ============ DoubleSignEvidence Tests ============

func TestDoubleSignEvidence_Validate_Valid(t *testing.T) {
	validator := Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	blockHashA := Hash{1, 2, 3, 4}
	blockHashB := Hash{5, 6, 7, 8}

	voteA := createTestVote(VotePrevote, 100, 5, blockHashA, validator)
	voteB := createTestVote(VotePrevote, 100, 5, blockHashB, validator)

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Valid double sign evidence should pass validation: %v", err)
	}
}

func TestDoubleSignEvidence_Validate_SameBlockHash(t *testing.T) {
	validator := Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	blockHash := Hash{1, 2, 3, 4}

	voteA := createTestVote(VotePrevote, 100, 5, blockHash, validator)
	voteB := createTestVote(VotePrevote, 100, 5, blockHash, validator)

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Double sign with same block hash should fail validation")
	}
}

func TestDoubleSignEvidence_Validate_VoteTypeMismatch(t *testing.T) {
	validator := Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	voteA := createTestVote(VotePrevote, 100, 5, Hash{1}, validator)
	voteB := createTestVote(VotePrecommit, 100, 5, Hash{2}, validator)

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Vote type mismatch should fail validation")
	}
}

func TestDoubleSignEvidence_Validate_ValidatorMismatch(t *testing.T) {
	voteA := createTestVote(VotePrevote, 100, 5, Hash{1}, Address{1})
	voteB := createTestVote(VotePrevote, 100, 5, Hash{2}, Address{2})

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Validator mismatch should fail validation")
	}
}

func TestDoubleSignEvidence_Validate_HeightMismatch(t *testing.T) {
	validator := Address{1}

	voteA := createTestVote(VotePrevote, 100, 5, Hash{1}, validator)
	voteB := createTestVote(VotePrevote, 200, 5, Hash{2}, validator)

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Height mismatch should fail validation")
	}
}

func TestDoubleSignEvidence_Validate_RoundMismatch(t *testing.T) {
	validator := Address{1}

	voteA := createTestVote(VotePrevote, 100, 5, Hash{1}, validator)
	voteB := createTestVote(VotePrevote, 100, 10, Hash{2}, validator)

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Round mismatch should fail validation")
	}
}

func TestDoubleSignEvidence_Validate_EmptySignature(t *testing.T) {
	validator := Address{1}

	voteA := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1},
		Validator: validator,
		Signature: []byte{}, // empty signature
	}
	voteB := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{2},
		Validator: validator,
		Signature: []byte{}, // empty signature
	}

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err == nil {
		t.Error("Empty signature should fail validation")
	}
}

// ============ InvalidCommitEvidence Tests ============

func TestInvalidCommitEvidence_Validate_Valid(t *testing.T) {
	blockHash := Hash{1, 2, 3}
	proof := createTestCommitProof(100, blockHash, 3)

	ev := InvalidCommitEvidence{
		Proof:  proof,
		Reason: "insufficient voting power",
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Valid invalid commit evidence should pass validation: %v", err)
	}
}

func TestInvalidCommitEvidence_Validate_EmptyReason(t *testing.T) {
	blockHash := Hash{1, 2, 3}
	proof := createTestCommitProof(100, blockHash, 3)

	ev := InvalidCommitEvidence{
		Proof:  proof,
		Reason: "",
	}

	if err := ev.Validate(); err == nil {
		t.Error("Empty reason should fail validation")
	}
}

// ============ MalformedVoteEvidence Tests ============

func TestMalformedVoteEvidence_Validate_Valid(t *testing.T) {
	vote := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64),
	}

	ev := MalformedVoteEvidence{
		BadVote: vote,
		Reason:  "invalid signature",
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Valid malformed vote evidence should pass validation: %v", err)
	}
}

func TestMalformedVoteEvidence_Validate_InvalidVoteType(t *testing.T) {
	vote := Vote{
		VoteType:  VoteType(255), // invalid
		Height:    100,
		Round:     5,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64),
	}

	ev := MalformedVoteEvidence{
		BadVote: vote,
		Reason:  "invalid vote type",
	}

	if err := ev.Validate(); err == nil {
		t.Error("Invalid vote type should fail validation")
	}
}

func TestMalformedVoteEvidence_Validate_ZeroHeight(t *testing.T) {
	vote := Vote{
		VoteType:  VotePrevote,
		Height:    0, // invalid
		Round:     5,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64),
	}

	ev := MalformedVoteEvidence{
		BadVote: vote,
		Reason:  "zero height",
	}

	if err := ev.Validate(); err == nil {
		t.Error("Zero height should fail validation")
	}
}

func TestMalformedVoteEvidence_Validate_EmptyReason(t *testing.T) {
	vote := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: Hash{1},
		Validator: Address{1},
		Signature: make([]byte, 64),
	}

	ev := MalformedVoteEvidence{
		BadVote: vote,
		Reason:  "",
	}

	if err := ev.Validate(); err == nil {
		t.Error("Empty reason should fail validation")
	}
}

// ============ Encode/Decode Round-Trip Tests ============

func TestDoubleSignEvidence_EncodeDecode_RoundTrip(t *testing.T) {
	validator := Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	blockHashA := Hash{1, 2, 3, 4}
	blockHashB := Hash{5, 6, 7, 8}

	original := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, blockHashA, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, blockHashB, validator),
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded := &DoubleSignEvidence{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.VoteA.VoteType != original.VoteA.VoteType {
		t.Errorf("VoteType mismatch: got %v, want %v", decoded.VoteA.VoteType, original.VoteA.VoteType)
	}
	if decoded.VoteA.Height != original.VoteA.Height {
		t.Errorf("Height mismatch: got %d, want %d", decoded.VoteA.Height, original.VoteA.Height)
	}
	if decoded.VoteA.Round != original.VoteA.Round {
		t.Errorf("Round mismatch: got %d, want %d", decoded.VoteA.Round, original.VoteA.Round)
	}
	if decoded.VoteA.BlockHash != original.VoteA.BlockHash {
		t.Errorf("VoteA BlockHash mismatch")
	}
	if decoded.VoteA.Validator != original.VoteA.Validator {
		t.Errorf("VoteA Validator mismatch")
	}
	if decoded.VoteB.BlockHash != original.VoteB.BlockHash {
		t.Errorf("VoteB BlockHash mismatch")
	}
}

func TestInvalidCommitEvidence_EncodeDecode_RoundTrip(t *testing.T) {
	blockHash := Hash{1, 2, 3}
	original := InvalidCommitEvidence{
		Proof:  createTestCommitProof(100, blockHash, 3),
		Reason: "insufficient voting power",
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded := &InvalidCommitEvidence{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Height() != original.Height() {
		t.Errorf("Height mismatch: got %d, want %d", decoded.Height(), original.Height())
	}
	if decoded.Reason != original.Reason {
		t.Errorf("Reason mismatch: got %q, want %q", decoded.Reason, original.Reason)
	}
}

func TestMalformedVoteEvidence_EncodeDecode_RoundTrip(t *testing.T) {
	original := MalformedVoteEvidence{
		BadVote: createTestVote(VotePrecommit, 50, 3, Hash{9, 8, 7}, Address{5}),
		Reason:  "signature verification failed",
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded := &MalformedVoteEvidence{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.BadVote.VoteType != original.BadVote.VoteType {
		t.Errorf("VoteType mismatch: got %v, want %v", decoded.BadVote.VoteType, original.BadVote.VoteType)
	}
	if decoded.BadVote.Height != original.BadVote.Height {
		t.Errorf("Height mismatch: got %d, want %d", decoded.BadVote.Height, original.BadVote.Height)
	}
	if decoded.Reason != original.Reason {
		t.Errorf("Reason mismatch: got %q, want %q", decoded.Reason, original.Reason)
	}
}

// ============ EvidenceHash Tests ============

func TestDoubleSignEvidence_EvidenceHash_Deterministic(t *testing.T) {
	validator := Address{1}
	blockHashA := Hash{1}
	blockHashB := Hash{2}

	ev := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, blockHashA, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, blockHashB, validator),
	}

	hash1, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	hash2, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("EvidenceHash should be deterministic")
	}
}

func TestDoubleSignEvidence_EvidenceHash_Different(t *testing.T) {
	validator := Address{1}

	ev1 := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, Hash{1}, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, Hash{2}, validator),
	}

	ev2 := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, Hash{3}, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, Hash{4}, validator),
	}

	hash1, _ := ev1.EvidenceHash(SHA256Hasher{})
	hash2, _ := ev2.EvidenceHash(SHA256Hasher{})

	if hash1 == hash2 {
		t.Error("Different evidence should produce different hashes")
	}
}

func TestInvalidCommitEvidence_EvidenceHash_Deterministic(t *testing.T) {
	proof := createTestCommitProof(100, Hash{1, 2, 3}, 3)

	ev := InvalidCommitEvidence{
		Proof:  proof,
		Reason: "reason not included in hash",
	}

	hash1, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	hash2, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("EvidenceHash should be deterministic")
	}
}

func TestMalformedVoteEvidence_EvidenceHash_Deterministic(t *testing.T) {
	vote := createTestVote(VotePrevote, 100, 5, Hash{1}, Address{1})

	ev := MalformedVoteEvidence{
		BadVote: vote,
		Reason:  "reason not included in hash",
	}

	hash1, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	hash2, err := ev.EvidenceHash(SHA256Hasher{})
	if err != nil {
		t.Fatalf("EvidenceHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("EvidenceHash should be deterministic")
	}
}

// ============ EncodeEvidence/DecodeEvidence Dispatcher Tests ============

func TestEncodeEvidence_DoubleSign(t *testing.T) {
	validator := Address{1}
	ev := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, Hash{1}, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, Hash{2}, validator),
	}

	var buf bytes.Buffer
	if err := EncodeEvidence(&buf, &ev); err != nil {
		t.Fatalf("EncodeEvidence failed: %v", err)
	}

	// First byte should be EvidenceTypeDoubleSign
	firstByte := buf.Bytes()[0]
	if EvidenceType(firstByte) != EvidenceTypeDoubleSign {
		t.Errorf("Expected EvidenceTypeDoubleSign (%d), got %d", EvidenceTypeDoubleSign, firstByte)
	}
}

func TestEncodeEvidence_InvalidCommit(t *testing.T) {
	ev := InvalidCommitEvidence{
		Proof:  createTestCommitProof(100, Hash{1}, 3),
		Reason: "test reason",
	}

	var buf bytes.Buffer
	if err := EncodeEvidence(&buf, &ev); err != nil {
		t.Fatalf("EncodeEvidence failed: %v", err)
	}

	firstByte := buf.Bytes()[0]
	if EvidenceType(firstByte) != EvidenceTypeInvalidCommit {
		t.Errorf("Expected EvidenceTypeInvalidCommit (%d), got %d", EvidenceTypeInvalidCommit, firstByte)
	}
}

func TestEncodeEvidence_MalformedVote(t *testing.T) {
	ev := MalformedVoteEvidence{
		BadVote: createTestVote(VotePrevote, 100, 5, Hash{1}, Address{1}),
		Reason:  "test reason",
	}

	var buf bytes.Buffer
	if err := EncodeEvidence(&buf, &ev); err != nil {
		t.Fatalf("EncodeEvidence failed: %v", err)
	}

	firstByte := buf.Bytes()[0]
	if EvidenceType(firstByte) != EvidenceTypeMalformedVote {
		t.Errorf("Expected EvidenceTypeMalformedVote (%d), got %d", EvidenceTypeMalformedVote, firstByte)
	}
}

func TestDecodeEvidence_Dispatch(t *testing.T) {
	// Test DoubleSign
	validator := Address{1}
	doubleSign := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, Hash{1}, validator),
		VoteB: createTestVote(VotePrevote, 100, 5, Hash{2}, validator),
	}
	var buf1 bytes.Buffer
	EncodeEvidence(&buf1, &doubleSign)

	decoded1, err := DecodeEvidence(bytes.NewReader(buf1.Bytes()))
	if err != nil {
		t.Fatalf("DecodeEvidence failed: %v", err)
	}
	if _, ok := decoded1.(*DoubleSignEvidence); !ok {
		t.Error("Expected DoubleSignEvidence")
	}

	// Test InvalidCommit
	invalidCommit := InvalidCommitEvidence{
		Proof:  createTestCommitProof(100, Hash{1}, 3),
		Reason: "test",
	}
	var buf2 bytes.Buffer
	EncodeEvidence(&buf2, &invalidCommit)

	decoded2, err := DecodeEvidence(bytes.NewReader(buf2.Bytes()))
	if err != nil {
		t.Fatalf("DecodeEvidence failed: %v", err)
	}
	if _, ok := decoded2.(*InvalidCommitEvidence); !ok {
		t.Error("Expected InvalidCommitEvidence")
	}

	// Test MalformedVote
	malformedVote := MalformedVoteEvidence{
		BadVote: createTestVote(VotePrevote, 100, 5, Hash{1}, Address{1}),
		Reason:  "test",
	}
	var buf3 bytes.Buffer
	EncodeEvidence(&buf3, &malformedVote)

	decoded3, err := DecodeEvidence(bytes.NewReader(buf3.Bytes()))
	if err != nil {
		t.Fatalf("DecodeEvidence failed: %v", err)
	}
	if _, ok := decoded3.(*MalformedVoteEvidence); !ok {
		t.Error("Expected MalformedVoteEvidence")
	}
}

func TestDecodeEvidence_UnknownType(t *testing.T) {
	// Write unknown evidence type
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint8(255)) // unknown type
	buf.Write([]byte{1, 2, 3})                      // dummy data

	_, err := DecodeEvidence(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Error("Unknown evidence type should return error")
	}
}

// ============ Height() Method Tests ============

func TestDoubleSignEvidence_Height(t *testing.T) {
	ev := DoubleSignEvidence{
		VoteA: createTestVote(VotePrevote, 100, 5, Hash{1}, Address{1}),
		VoteB: createTestVote(VotePrevote, 100, 5, Hash{2}, Address{1}),
	}

	if ev.Height() != 100 {
		t.Errorf("Expected height 100, got %d", ev.Height())
	}
}

func TestInvalidCommitEvidence_Height(t *testing.T) {
	ev := InvalidCommitEvidence{
		Proof:  createTestCommitProof(200, Hash{1}, 3),
		Reason: "test",
	}

	if ev.Height() != 200 {
		t.Errorf("Expected height 200, got %d", ev.Height())
	}
}

func TestMalformedVoteEvidence_Height(t *testing.T) {
	ev := MalformedVoteEvidence{
		BadVote: createTestVote(VotePrevote, 300, 5, Hash{1}, Address{1}),
		Reason:  "test",
	}

	if ev.Height() != 300 {
		t.Errorf("Expected height 300, got %d", ev.Height())
	}
}

// ============ Type() Method Tests ============

func TestEvidenceType_Methods(t *testing.T) {
	ev1 := &DoubleSignEvidence{}
	if ev1.Type() != EvidenceTypeDoubleSign {
		t.Errorf("Expected EvidenceTypeDoubleSign, got %v", ev1.Type())
	}

	ev2 := &InvalidCommitEvidence{}
	if ev2.Type() != EvidenceTypeInvalidCommit {
		t.Errorf("Expected EvidenceTypeInvalidCommit, got %v", ev2.Type())
	}

	ev3 := &MalformedVoteEvidence{}
	if ev3.Type() != EvidenceTypeMalformedVote {
		t.Errorf("Expected EvidenceTypeMalformedVote, got %v", ev3.Type())
	}
}

// ============ Sign and Verify Integration Test ============

func TestDoubleSignEvidence_WithRealKeys(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	var validator [20]byte
	copy(validator[:], pubKey[:20])

	blockHashA := Hash{1, 2, 3, 4}
	blockHashB := Hash{5, 6, 7, 8}

	voteA := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: blockHashA,
		Validator: validator,
	}
	if err := voteA.Sign(privKey); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	voteB := Vote{
		VoteType:  VotePrevote,
		Height:    100,
		Round:     5,
		BlockHash: blockHashB,
		Validator: validator,
	}
	if err := voteB.Sign(privKey); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	ev := DoubleSignEvidence{
		VoteA: voteA,
		VoteB: voteB,
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Valid double sign with real keys should pass: %v", err)
	}

	// Test encoding/decoding round-trip with signed votes
	var buf bytes.Buffer
	if err := ev.Encode(&buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded := &DoubleSignEvidence{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify the decoded votes
	var pk [32]byte
	copy(pk[:], pubKey)
	if !decoded.VoteA.Verify(pk) {
		t.Error("Decoded VoteA should have valid signature")
	}
	if !decoded.VoteB.Verify(pk) {
		t.Error("Decoded VoteB should have valid signature")
	}
}