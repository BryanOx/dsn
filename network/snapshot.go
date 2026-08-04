package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// SnapshotInfo contains metadata about an available snapshot.
type SnapshotInfo struct {
	Height       uint64
	SnapshotHash [32]byte
	StateRoot    [32]byte
	Epoch        uint64
	Timestamp    uint64
	ChunkCount   uint32
}

// SnapshotChunk represents a chunk of snapshot data.
type SnapshotChunk struct {
	Index        uint32
	TotalCount   uint32
	SnapshotHash [32]byte
	ChunkHash    [32]byte
	Data         []byte
}

// SnapshotQueryHandler is the callback type for handling snapshot query requests.
type SnapshotQueryHandler func() *SnapshotInfo

// SnapshotInfoHandler is the callback type for handling incoming snapshot info messages.
type SnapshotInfoHandler func(info *SnapshotInfo)

// SnapshotRequestHandler is the callback type for handling chunk requests.
// Returns the chunk if available, and a boolean indicating success.
type SnapshotRequestHandler func(snapshotHash [32]byte, chunkIndex uint32) (*SnapshotChunk, bool)

// SnapshotChunkHandler is the callback type for handling incoming snapshot chunks.
// The peer parameter is the peer's address string for failure reporting.
type SnapshotChunkHandler func(chunk *SnapshotChunk, peer string)

// SetSnapshotQueryHandler registers a handler for snapshot query messages.
func (n *P2PNode) SetSnapshotQueryHandler(handler SnapshotQueryHandler) {
	n.snapshotQueryHandler = handler
}

// SetSnapshotInfoHandler registers a handler for snapshot info messages.
func (n *P2PNode) SetSnapshotInfoHandler(handler SnapshotInfoHandler) {
	n.snapshotInfoHandler = handler
}

// SetSnapshotRequestHandler registers a handler for snapshot chunk requests.
func (n *P2PNode) SetSnapshotRequestHandler(handler SnapshotRequestHandler) {
	n.snapshotRequestHandler = handler
}

// SetSnapshotChunkHandler registers a handler for incoming snapshot chunks.
func (n *P2PNode) SetSnapshotChunkHandler(handler SnapshotChunkHandler) {
	n.snapshotChunkHandler = handler
}

// handleSnapshotMessage dispatches incoming snapshot messages to the appropriate handler.
func (n *P2PNode) handleSnapshotMessage(msgType byte, data []byte, peer string) {
	// Get PeerID from address for failure reporting
	var peerID PeerID
	if p := n.pm.GetPeerByAddr(peer); p != nil {
		peerID = p.ID
	}

	switch msgType {
	case MsgTypeSnapshotQuery:
		if n.snapshotQueryHandler != nil {
			info := n.snapshotQueryHandler()
			if info != nil {
				if err := n.SendSnapshotInfo(peer, info); err != nil {
					fmt.Printf("Failed to send snapshot info to %s: %v\n", peer, err)
				}
			}
		}

	case MsgTypeSnapshotInfo:
		info := parseSnapshotInfo(data)
		if info == nil {
			// Penalize for malformed snapshot info
			if peerID != [32]byte{} {
				n.pm.ReportFailure(peerID, 2)
			}
		} else if n.snapshotInfoHandler != nil {
			n.snapshotInfoHandler(info)
		}

	case MsgTypeSnapshotRequest:
		req := parseSnapshotRequest(data)
		if req == nil {
			// Penalize for malformed snapshot request
			if peerID != [32]byte{} {
				n.pm.ReportFailure(peerID, 2)
			}
		} else if n.snapshotRequestHandler != nil {
			chunk, ok := n.snapshotRequestHandler(req.SnapshotHash, req.ChunkIndex)
			if ok {
				if err := n.SendSnapshotChunk(peer, chunk); err != nil {
					fmt.Printf("Failed to send snapshot chunk to %s: %v\n", peer, err)
				}
			}
		}

	case MsgTypeSnapshotChunk:
		chunk := parseSnapshotChunk(data)
		if chunk == nil {
			// Penalize for malformed snapshot chunk
			if peerID != [32]byte{} {
				n.pm.ReportFailure(peerID, 2)
			}
		} else if n.snapshotChunkHandler != nil {
			n.snapshotChunkHandler(chunk, peer)
		}
	}
}

// sendToPeer sends a message to a specific peer.
func (n *P2PNode) sendToPeer(peer string, msgType byte, payload []byte) error {
	n.connMu.RLock()
	conn, ok := n.connections[peer]
	n.connMu.RUnlock()
	if !ok {
		return fmt.Errorf("peer %s not found", peer)
	}

	// Frame the type byte + payload with the 4-byte big-endian length prefix
	// in ONE buffer and write it with a single conn.Write (see Broadcast):
	// separate Write calls from concurrent senders on the same connection
	// would interleave length prefixes with payloads and corrupt the stream.
	msg := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(msg[:4], uint32(1+len(payload)))
	msg[4] = msgType
	copy(msg[5:], payload)

	if _, err := conn.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	return nil
}

// BroadcastSnapshotQuery sends a snapshot query to all connected peers.
func (n *P2PNode) BroadcastSnapshotQuery() {
	msg := []byte{MsgTypeSnapshotQuery}
	n.Broadcast(msg)
}

// BroadcastSnapshotInfo broadcasts snapshot info to all connected peers.
func (n *P2PNode) BroadcastSnapshotInfo(info *SnapshotInfo) error {
	payload, err := serializeSnapshotInfo(info)
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot info: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = MsgTypeSnapshotInfo
	copy(msg[1:], payload)

	n.Broadcast(msg)
	return nil
}

// SendSnapshotInfo sends snapshot info to a specific peer.
func (n *P2PNode) SendSnapshotInfo(peer string, info *SnapshotInfo) error {
	payload, err := serializeSnapshotInfo(info)
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot info: %w", err)
	}
	return n.sendToPeer(peer, MsgTypeSnapshotInfo, payload)
}

// RequestSnapshotChunk requests a specific chunk from a peer.
func (n *P2PNode) RequestSnapshotChunk(peer string, snapshotHash [32]byte, index uint32) error {
	payload := make([]byte, 36)
	copy(payload[0:32], snapshotHash[:])
	binary.BigEndian.PutUint32(payload[32:36], index)
	return n.sendToPeer(peer, MsgTypeSnapshotRequest, payload)
}

// SendSnapshotChunk sends a snapshot chunk to a specific peer.
func (n *P2PNode) SendSnapshotChunk(peer string, chunk *SnapshotChunk) error {
	payload, err := serializeSnapshotChunk(chunk)
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot chunk: %w", err)
	}
	return n.sendToPeer(peer, MsgTypeSnapshotChunk, payload)
}

// serializeSnapshotInfo serializes SnapshotInfo to wire format.
// Format: Height(8) + SnapshotHash(32) + StateRoot(32) + Epoch(8) + Timestamp(8) + ChunkCount(4) = 92 bytes
func serializeSnapshotInfo(info *SnapshotInfo) ([]byte, error) {
	buf := make([]byte, 92)
	binary.BigEndian.PutUint64(buf[0:8], info.Height)
	copy(buf[8:40], info.SnapshotHash[:])
	copy(buf[40:72], info.StateRoot[:])
	binary.BigEndian.PutUint64(buf[72:80], info.Epoch)
	binary.BigEndian.PutUint64(buf[80:88], info.Timestamp)
	binary.BigEndian.PutUint32(buf[88:92], info.ChunkCount)
	return buf, nil
}

// parseSnapshotInfo deserializes SnapshotInfo from wire format.
func parseSnapshotInfo(data []byte) *SnapshotInfo {
	if len(data) < 92 {
		return nil
	}
	info := &SnapshotInfo{}
	info.Height = binary.BigEndian.Uint64(data[0:8])
	copy(info.SnapshotHash[:], data[8:40])
	copy(info.StateRoot[:], data[40:72])
	info.Epoch = binary.BigEndian.Uint64(data[72:80])
	info.Timestamp = binary.BigEndian.Uint64(data[80:88])
	info.ChunkCount = binary.BigEndian.Uint32(data[88:92])
	return info
}

// snapshotRequest represents a parsed snapshot request.
type snapshotRequest struct {
	SnapshotHash [32]byte
	ChunkIndex   uint32
}

// parseSnapshotRequest deserializes a snapshot request from wire format.
func parseSnapshotRequest(data []byte) *snapshotRequest {
	if len(data) < 36 {
		return nil
	}
	req := &snapshotRequest{}
	copy(req.SnapshotHash[:], data[0:32])
	req.ChunkIndex = binary.BigEndian.Uint32(data[32:36])
	return req
}

// serializeSnapshotChunk serializes SnapshotChunk to wire format.
// Format: Index(4) + TotalCount(4) + SnapshotHash(32) + ChunkHash(32) + DataLength(4) + Data
func serializeSnapshotChunk(chunk *SnapshotChunk) ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, chunk.Index); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.BigEndian, chunk.TotalCount); err != nil {
		return nil, err
	}
	if _, err := buf.Write(chunk.SnapshotHash[:]); err != nil {
		return nil, err
	}
	if _, err := buf.Write(chunk.ChunkHash[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(chunk.Data))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(chunk.Data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parseSnapshotChunk deserializes SnapshotChunk from wire format.
func parseSnapshotChunk(data []byte) *SnapshotChunk {
	if len(data) < 76 {
		return nil
	}
	chunk := &SnapshotChunk{}
	chunk.Index = binary.BigEndian.Uint32(data[0:4])
	chunk.TotalCount = binary.BigEndian.Uint32(data[4:8])
	copy(chunk.SnapshotHash[:], data[8:40])
	copy(chunk.ChunkHash[:], data[40:72])
	dataLen := binary.BigEndian.Uint32(data[72:76])
	if len(data) < 76+int(dataLen) {
		return nil
	}
	chunk.Data = data[76 : 76+dataLen]
	return chunk
}
