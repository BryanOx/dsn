package network

import (
	"encoding/binary"
	"testing"
)

// TestMsgTypeSyncHeightRegistered verifies the 0x43 SyncHeight message type is
// declared, registered as known, and distinct from existing types.
func TestMsgTypeSyncHeightRegistered(t *testing.T) {
	if MsgTypeSyncHeight != 0x43 {
		t.Errorf("expected MsgTypeSyncHeight 0x43, got 0x%02X", MsgTypeSyncHeight)
	}

	if !IsKnownType(MsgTypeSyncHeight) {
		t.Error("expected MsgTypeSyncHeight to be a known message type")
	}

	// 0x43 must not collide with an existing type
	if MsgTypeSyncHeight == MsgTypeBlock || MsgTypeSyncHeight == MsgTypeVote ||
		MsgTypeSyncHeight == MsgTypeBlockRangeRequest || MsgTypeSyncHeight == MsgTypeBlockRangeResponse {
		t.Errorf("MsgTypeSyncHeight 0x%02X collides with an existing message type", MsgTypeSyncHeight)
	}
}

// TestMaxPayloadForTypeSyncHeight verifies the SyncHeight payload is capped at
// exactly 8 bytes (one uint64 big-endian height).
func TestMaxPayloadForTypeSyncHeight(t *testing.T) {
	if got := MaxPayloadForType(MsgTypeSyncHeight); got != 8 {
		t.Errorf("expected MaxPayloadForType(MsgTypeSyncHeight) = 8, got %d", got)
	}
}

// TestParseMessageSyncHeight verifies ParseMessage enforces the 8-byte payload
// cap for SyncHeight messages: oversized payloads are rejected, exact-size
// payloads pass through with the height intact.
func TestParseMessageSyncHeight(t *testing.T) {
	// A 9-byte payload exceeds the 8-byte cap and must be rejected.
	oversized := FrameMessage(MsgTypeSyncHeight, make([]byte, 9))
	if _, _, err := ParseMessage(oversized); err == nil {
		t.Error("expected error for SyncHeight payload exceeding 8-byte cap, got nil")
	}

	// An exact 8-byte payload (height 1_000_000) must parse cleanly.
	height := uint64(1_000_000)
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, height)

	framed := FrameMessage(MsgTypeSyncHeight, payload)
	msgType, gotPayload, err := ParseMessage(framed)
	if err != nil {
		t.Fatalf("unexpected error parsing valid SyncHeight frame: %v", err)
	}
	if msgType != MsgTypeSyncHeight {
		t.Errorf("expected parsed type 0x%02X, got 0x%02X", MsgTypeSyncHeight, msgType)
	}
	if len(gotPayload) != 8 {
		t.Errorf("expected parsed payload length 8, got %d", len(gotPayload))
	}
	if got := binary.BigEndian.Uint64(gotPayload); got != height {
		t.Errorf("expected decoded height %d, got %d", height, got)
	}
}
