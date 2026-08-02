package network

import (
	"encoding/binary"
	"errors"
)

// Message type constants for P2P protocol.
const (
	MsgTypeBlock              byte = 0x01 // Block message
	MsgTypeTransaction        byte = 0x02 // Transaction message
	MsgTypeSnapshotQuery      byte = 0x10 // Query: "what snapshots do you have?"
	MsgTypeSnapshotInfo       byte = 0x11 // Response: snapshot metadata
	MsgTypeSnapshotRequest    byte = 0x12 // Request: "send me chunk N of snapshot X"
	MsgTypeSnapshotChunk      byte = 0x13 // Response: chunk data
	MsgTypePeerExchange       byte = 0x20 // Peer exchange
	MsgTypeBlockRangeRequest  byte = 0x30 // Request a range of blocks
	MsgTypeBlockRangeResponse byte = 0x31 // Response with block range
	MsgTypePing               byte = 0x40 // Keepalive ping
	MsgTypePong               byte = 0x41 // Keepalive pong
)

// Max payload size limits.
const (
	MaxPayloadSize            uint32 = 1 * 1024 * 1024 // 1 MiB
	MaxPeersPerPEX            uint32 = 100
	MaxBlocksPerRangeResponse uint32 = 100
)

// Sentinel errors for message handling.
var (
	ErrUnknownMessageType = errors.New("unknown message type")
	ErrPayloadTooLarge    = errors.New("payload exceeds maximum size for message type")
)

// knownMessageTypes is a lookup table for valid message types.
var knownMessageTypes = map[byte]bool{
	MsgTypeBlock:              true,
	MsgTypeTransaction:        true,
	MsgTypeSnapshotQuery:      true,
	MsgTypeSnapshotInfo:       true,
	MsgTypeSnapshotRequest:    true,
	MsgTypeSnapshotChunk:      true,
	MsgTypePeerExchange:       true,
	MsgTypeBlockRangeRequest:  true,
	MsgTypeBlockRangeResponse: true,
	MsgTypePing:               true,
	MsgTypePong:               true,
}

// maxPayloadByType defines per-type payload limits (currently all 1 MiB).
var maxPayloadByType = map[byte]uint32{
	MsgTypeBlock:              MaxPayloadSize,
	MsgTypeTransaction:        MaxPayloadSize,
	MsgTypeSnapshotQuery:      MaxPayloadSize,
	MsgTypeSnapshotInfo:       MaxPayloadSize,
	MsgTypeSnapshotRequest:    MaxPayloadSize,
	MsgTypeSnapshotChunk:      MaxPayloadSize,
	MsgTypePeerExchange:       MaxPayloadSize,
	MsgTypeBlockRangeRequest:  MaxPayloadSize,
	MsgTypeBlockRangeResponse: MaxPayloadSize,
	MsgTypePing:               MaxPayloadSize,
	MsgTypePong:               MaxPayloadSize,
}

// FrameMessage creates a framed message with length prefix, message type, and payload.
// Format: [4-byte BE length][1-byte type][payload]
func FrameMessage(msgType byte, payload []byte) []byte {
	// Total message = 1 byte type + payload
	msgLen := 1 + len(payload)

	// Frame: 4-byte length + 1-byte type + payload
	frame := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(frame[0:4], uint32(msgLen))
	frame[4] = msgType
	copy(frame[5:], payload)

	return frame
}

// ParseMessage validates and extracts message type and payload from framed data.
// Returns the message type byte and payload, or an error if validation fails.
func ParseMessage(data []byte) (msgType byte, payload []byte, err error) {
	if len(data) < 5 {
		return 0, nil, errors.New("message too short: need at least 5 bytes for length prefix + type")
	}

	// Read length prefix (4 bytes big-endian)
	msgLen := binary.BigEndian.Uint32(data[0:4])
	if msgLen > MaxPayloadSize {
		return 0, nil, ErrPayloadTooLarge
	}

	// Verify we have enough data for the declared length
	if len(data) < 4+int(msgLen) {
		return 0, nil, errors.New("message truncated: declared length exceeds available data")
	}

	// Extract message type and payload
	msgType = data[4]
	if !IsKnownType(msgType) {
		return 0, nil, ErrUnknownMessageType
	}

	payload = data[5 : 4+msgLen]
	return msgType, payload, nil
}

// IsKnownType returns true if the given message type is recognized.
func IsKnownType(msgType byte) bool {
	return knownMessageTypes[msgType]
}

// MaxPayloadForType returns the maximum payload size for a given message type.
// Currently all types support 1 MiB, but this is structured for future per-type limits.
func MaxPayloadForType(msgType byte) uint32 {
	if limit, ok := maxPayloadByType[msgType]; ok {
		return limit
	}
	// Default to generic max if type not found (shouldn't happen for known types)
	return MaxPayloadSize
}
