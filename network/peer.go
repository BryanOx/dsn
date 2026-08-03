package network

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

// PeerID is a [32]byte SHA-256 hash identifying a peer uniquely.
type PeerID [32]byte

// String returns the hex representation of the PeerID.
func (p PeerID) String() string {
	return hex.EncodeToString(p[:])
}

// PeerIDFromBytes computes a PeerID from raw bytes (e.g. public key or address).
func PeerIDFromBytes(data []byte) PeerID {
	return sha256.Sum256(data)
}

// PeerState enum represents the connection state of a peer.
type PeerState int

const (
	PeerDisconnected PeerState = iota
	PeerConnecting
	PeerConnected
	PeerBanned
)

// String implements fmt.Stringer for PeerState.
func (s PeerState) String() string {
	switch s {
	case PeerDisconnected:
		return "disconnected"
	case PeerConnecting:
		return "connecting"
	case PeerConnected:
		return "connected"
	case PeerBanned:
		return "banned"
	default:
		return "unknown"
	}
}

// PeerRecord is the binary-serializable peer data for persistence.
// Version field enables future format changes.
type PeerRecord struct {
	Version   uint8
	ID        [32]byte // peer ID
	Address   string
	Score     int
	Latency   int64 // nanoseconds
	LastSeen  int64 // unix timestamp
	FirstSeen int64
	IsBanned  bool
	State     PeerState
	// Additional fields for compatibility
	SyncHeight     uint64
	MessageCount   uint64
	FailCount      uint64
	ConnectedSince int64 // unix timestamp
	BanExpiry      int64 // unix timestamp, 0 = not banned
}

// Ensure PeerRecord implements BinaryMarshaler/BinaryUnmarshaler for custom encoding
var (
	_ encoding.BinaryMarshaler   = (*PeerRecord)(nil)
	_ encoding.BinaryUnmarshaler = (*PeerRecord)(nil)
)

// Peer represents a P2P network peer with connection and scoring information.
type Peer struct {
	ID             PeerID
	Address        string
	Conn           net.Conn
	State          PeerState
	Score          int
	LastSeen       time.Time
	Latency        time.Duration // rolling average
	SyncHeight     uint64
	MessageCount   uint64
	FailCount      uint64
	ConnectedSince time.Time
	BanExpiry      time.Time // zero = not banned
	mu             sync.RWMutex

	// Internal tracking for scoring rules
	lastBlockSuccess time.Time
	blockSuccesses   int // count today
}

// MarshalBinary encodes PeerRecord to binary format using little-endian.
func (pr *PeerRecord) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Version byte
	if err := binary.Write(buf, binary.LittleEndian, pr.Version); err != nil {
		return nil, err
	}
	// Fixed-size fields
	if err := binary.Write(buf, binary.LittleEndian, pr.ID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.Latency); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.LastSeen); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.FirstSeen); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.SyncHeight); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.MessageCount); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.FailCount); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.ConnectedSince); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pr.BanExpiry); err != nil {
		return nil, err
	}
	// Variable-size fields: address (length-prefixed), score, isBanned
	// Address: length (2 bytes) + string bytes
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(pr.Address))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(pr.Address); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(pr.Score)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, bool(pr.IsBanned)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(pr.State)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary decodes PeerRecord from binary format.
func (pr *PeerRecord) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)
	// Version byte
	if err := binary.Read(buf, binary.LittleEndian, &pr.Version); err != nil {
		return err
	}
	// Fixed-size fields
	if err := binary.Read(buf, binary.LittleEndian, &pr.ID); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.Latency); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.LastSeen); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.FirstSeen); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.SyncHeight); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.MessageCount); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.FailCount); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.ConnectedSince); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pr.BanExpiry); err != nil {
		return err
	}
	// Address: length-prefixed
	var addrLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &addrLen); err != nil {
		return err
	}
	addrBytes := make([]byte, addrLen)
	if _, err := io.ReadFull(buf, addrBytes); err != nil {
		return err
	}
	pr.Address = string(addrBytes)
	// Score and IsBanned
	var score int32
	if err := binary.Read(buf, binary.LittleEndian, &score); err != nil {
		return err
	}
	pr.Score = int(score)
	if err := binary.Read(buf, binary.LittleEndian, &pr.IsBanned); err != nil {
		return err
	}
	var state int32
	if err := binary.Read(buf, binary.LittleEndian, &state); err != nil {
		return err
	}
	pr.State = PeerState(state)
	return nil
}

// EncodePeerRecord writes a PeerRecord to an io.Writer using binary encoding.
func EncodePeerRecord(w io.Writer, pr PeerRecord) error {
	data, err := pr.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// DecodePeerRecord reads a PeerRecord from an io.Reader.
func DecodePeerRecord(r io.Reader) (PeerRecord, error) {
	var pr PeerRecord
	// We need to read all data since we don't know the size ahead of time
	data, err := io.ReadAll(r)
	if err != nil {
		return pr, err
	}
	err = pr.UnmarshalBinary(data)
	return pr, err
}

// peerToRecord converts a Peer to a PeerRecord for persistence.
func peerToRecord(p *Peer) PeerRecord {
	return PeerRecord{
		Version:        2,
		ID:             p.ID,
		Address:        p.Address,
		Score:          p.Score,
		Latency:        p.Latency.Nanoseconds(),
		LastSeen:       p.LastSeen.Unix(),
		FirstSeen:      0, // not tracked in Peer currently
		IsBanned:       p.State == PeerBanned,
		State:          p.State,
		SyncHeight:     p.SyncHeight,
		MessageCount:   p.MessageCount,
		FailCount:      p.FailCount,
		ConnectedSince: p.ConnectedSince.Unix(),
		BanExpiry:      p.BanExpiry.Unix(),
	}
}

// recordToPeer converts a PeerRecord to a Peer.
func recordToPeer(pr PeerRecord) *Peer {
	peer := &Peer{
		ID:             pr.ID,
		Address:        pr.Address,
		State:          pr.State,
		Score:          pr.Score,
		Latency:        time.Duration(pr.Latency),
		LastSeen:       time.Unix(pr.LastSeen, 0),
		SyncHeight:     pr.SyncHeight,
		MessageCount:   pr.MessageCount,
		FailCount:      pr.FailCount,
		ConnectedSince: time.Unix(pr.ConnectedSince, 0),
		BanExpiry:      time.Unix(pr.BanExpiry, 0),
	}
	if pr.IsBanned {
		peer.State = PeerBanned
	}
	return peer
}

// PeerManager manages peer registry, scoring, and persistence.
type PeerManager struct {
	peers       map[PeerID]*Peer
	peersByAddr map[string]*Peer
	mu          sync.RWMutex
	db          *bbolt.DB // optional, nil if no persistence
	stopCh      chan struct{}
	running     bool
	// onConnected is invoked after ConnectToPeer dials successfully. It is
	// set once at construction by the P2P layer (see SetOnConnectedHandler).
	// It runs after the peer lock is released, so it may safely take
	// PeerManager and peer locks (the P2P registration logic does both).
	onConnected func(net.Conn, string)
}

// peersBucket is the BoltDB bucket name for peer persistence.
var peersBucket = []byte("peers")

// NewPeerManager creates a new PeerManager. If db is nil, persistence methods are no-ops.
func NewPeerManager(db *bbolt.DB) *PeerManager {
	pm := &PeerManager{
		peers:       make(map[PeerID]*Peer),
		peersByAddr: make(map[string]*Peer),
		db:          db,
		stopCh:      make(chan struct{}),
	}
	return pm
}

// SetOnConnectedHandler registers a callback invoked after ConnectToPeer
// successfully establishes a connection. The callback runs after the peer
// lock is released and receives the raw connection and the dialed address.
func (pm *PeerManager) SetOnConnectedHandler(h func(net.Conn, string)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onConnected = h
}

// AddPeer adds a peer with score 0 and state Disconnected.
// If the peer already exists, returns the existing peer.
func (pm *PeerManager) AddPeer(id PeerID, addr string) *Peer {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if peer already exists by ID
	if p, ok := pm.peers[id]; ok {
		return p
	}

	// Check if peer already exists by address
	if p, ok := pm.peersByAddr[addr]; ok {
		return p
	}

	peer := &Peer{
		ID:         id,
		Address:    addr,
		State:      PeerDisconnected,
		Score:      0,
		LastSeen:   time.Now(),
		Latency:    0,
		SyncHeight: 0,
	}

	pm.peers[id] = peer
	pm.peersByAddr[addr] = peer

	return peer
}

// RemovePeer removes a peer from both maps.
func (pm *PeerManager) RemovePeer(id PeerID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, ok := pm.peers[id]; ok {
		delete(pm.peersByAddr, peer.Address)
		delete(pm.peers, id)
	}
}

// GetPeer returns a peer by ID, or nil if not found.
func (pm *PeerManager) GetPeer(id PeerID) *Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.peers[id]
}

// GetPeerByAddr returns a peer by address, or nil if not found.
func (pm *PeerManager) GetPeerByAddr(addr string) *Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.peersByAddr[addr]
}

// GetPeersByState returns all peers in the given state.
func (pm *PeerManager) GetPeersByState(state PeerState) []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*Peer, 0)
	for _, peer := range pm.peers {
		if peer.State == state {
			result = append(result, peer)
		}
	}
	return result
}

// GetBestPeers returns the top-n connected peers by score.
func (pm *PeerManager) GetBestPeers(n int) []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Collect connected peers with positive score
	var candidates []*Peer
	for _, peer := range pm.peers {
		if peer.State == PeerConnected && peer.Score > 0 {
			candidates = append(candidates, peer)
		}
	}

	// Sort by score descending, latency as tiebreaker (lower is better)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Latency < candidates[j].Latency
		}
		return candidates[i].Score > candidates[j].Score
	})

	if n > len(candidates) {
		n = len(candidates)
	}
	return candidates[:n]
}

// ReportSuccess records a successful interaction with a peer.
// Scoring: +1 valid block (capped +10/day), +2 valid snapshot chunk, +1 valid PEX response.
func (pm *PeerManager) ReportSuccess(id PeerID, msgType byte) {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	peer.MessageCount++

	now := time.Now()
	// Reset daily block success counter if it's a new day
	if now.Sub(peer.lastBlockSuccess) > 24*time.Hour {
		peer.blockSuccesses = 0
		peer.lastBlockSuccess = now
	}

	switch msgType {
	case MsgTypeBlock:
		// Cap at +10 per day for block success
		if peer.blockSuccesses < 10 {
			peer.Score++
			peer.blockSuccesses++
		}
	case MsgTypeSnapshotChunk:
		peer.Score += 2
	case MsgTypePeerExchange:
		peer.Score++
	}

	// Clamp score to [-100, 100]
	if peer.Score > 100 {
		peer.Score = 100
	}

	peer.LastSeen = now
}

// ReportFailure records a failure with a peer.
// Scoring: -2 malformed message, -5 invalid block/snapshot, -10 spam, -1 connection timeout.
func (pm *PeerManager) ReportFailure(id PeerID, severity int) {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	peer.FailCount++
	peer.Score -= severity

	// Clamp to [-100, 100]
	if peer.Score < -100 {
		peer.Score = -100
	}

	// Auto-ban if score drops below -20
	if peer.Score < -20 {
		peer.State = PeerBanned
		peer.BanExpiry = time.Now().Add(5 * time.Minute)
		// Disconnect the peer
		if peer.Conn != nil {
			peer.Conn.Close()
			peer.Conn = nil
		}
	}

	peer.LastSeen = time.Now()
}

// UpdateScore applies a delta to the peer's score, clamps to [-100, 100],
// and auto-bans if score drops below -20.
func (pm *PeerManager) UpdateScore(id PeerID, delta int) {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	peer.Score += delta

	// Clamp to [-100, 100]
	if peer.Score > 100 {
		peer.Score = 100
	}
	if peer.Score < -100 {
		peer.Score = -100
	}

	// Auto-ban if score drops below -20
	if peer.Score < -20 && peer.State != PeerBanned {
		peer.State = PeerBanned
		peer.BanExpiry = time.Now().Add(5 * time.Minute)
		if peer.Conn != nil {
			peer.Conn.Close()
			peer.Conn = nil
		}
	}
}

// Ban sets a peer's state to Banned for the specified duration.
func (pm *PeerManager) Ban(id PeerID, duration time.Duration) {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	peer.State = PeerBanned
	peer.BanExpiry = time.Now().Add(duration)

	// Disconnect the peer
	if peer.Conn != nil {
		peer.Conn.Close()
		peer.Conn = nil
	}
}

// ConnectToPeer establishes a TCP connection to the peer and updates its state.
func (pm *PeerManager) ConnectToPeer(id PeerID, addr string) error {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		// Add peer if it doesn't exist
		peer = pm.AddPeer(id, addr)
	}

	peer.mu.Lock()

	peer.State = PeerConnecting

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		peer.State = PeerDisconnected
		peer.mu.Unlock()
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	peer.Conn = conn
	peer.State = PeerConnected
	peer.ConnectedSince = time.Now()
	peer.LastSeen = time.Now()
	peer.mu.Unlock()

	// Hand the connection to the P2P layer so it is registered for broadcast
	// and read (set via SetOnConnectedHandler). The hook runs outside the
	// peer lock so the registration logic can safely reconcile the peer's
	// connection, e.g. keep a live existing connection when this dial turns
	// out to duplicate it.
	if pm.onConnected != nil {
		pm.onConnected(conn, addr)
	}

	return nil
}

// DisconnectPeer closes the connection and sets state to Disconnected.
func (pm *PeerManager) DisconnectPeer(id PeerID) {
	pm.mu.RLock()
	peer, ok := pm.peers[id]
	pm.mu.RUnlock()

	if !ok {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.Conn != nil {
		peer.Conn.Close()
		peer.Conn = nil
	}
	peer.State = PeerDisconnected
}

// NumPeers returns the total number of peers in the registry.
func (pm *PeerManager) NumPeers() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return len(pm.peers)
}

// NumConnected returns the number of connected peers.
func (pm *PeerManager) NumConnected() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	for _, peer := range pm.peers {
		if peer.State == PeerConnected {
			count++
		}
	}
	return count
}

// Persist writes all peers to BoltDB bucket "peers".
// Skips Conn and mu fields (they are nil after restart anyway).
func (pm *PeerManager) Persist() error {
	if pm.db == nil {
		return nil // no persistence configured
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(peersBucket)
		if err != nil {
			return fmt.Errorf("failed to create peers bucket: %w", err)
		}

		for id, peer := range pm.peers {
			pr := peerToRecord(peer)
			data, err := pr.MarshalBinary()
			if err != nil {
				return fmt.Errorf("failed to marshal peer: %w", err)
			}
			if err := bucket.Put(id[:], data); err != nil {
				return fmt.Errorf("failed to put peer: %w", err)
			}
		}

		return nil
	})
}

// LoadPeers reads peers from BoltDB bucket "peers".
// Tries binary format first, falls back to gob for backward compatibility.
func (pm *PeerManager) LoadPeers() error {
	if pm.db == nil {
		return nil // no persistence configured
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(peersBucket)
		if bucket == nil {
			return nil // bucket doesn't exist yet
		}

		return bucket.ForEach(func(k, v []byte) error {
			var id PeerID
			if len(k) != 32 {
				return nil // skip invalid keys
			}
			copy(id[:], k)

			var peer *Peer

			// Try binary format first (PeerRecord)
			var pr PeerRecord
			if err := pr.UnmarshalBinary(v); err == nil {
				peer = recordToPeer(pr)
			} else {
				// Fall back to gob for backward compatibility with old data
				var persistPeer struct {
					ID             PeerID
					Address        string
					State          PeerState
					Score          int
					LastSeen       time.Time
					Latency        time.Duration
					SyncHeight     uint64
					MessageCount   uint64
					FailCount      uint64
					ConnectedSince time.Time
					BanExpiry      time.Time
				}

				dec := gob.NewDecoder(bytes.NewReader(v))
				if err := dec.Decode(&persistPeer); err != nil {
					return nil // skip corrupt entries
				}

				peer = &Peer{
					ID:             persistPeer.ID,
					Address:        persistPeer.Address,
					State:          persistPeer.State,
					Score:          persistPeer.Score,
					LastSeen:       persistPeer.LastSeen,
					Latency:        persistPeer.Latency,
					SyncHeight:     persistPeer.SyncHeight,
					MessageCount:   persistPeer.MessageCount,
					FailCount:      persistPeer.FailCount,
					ConnectedSince: persistPeer.ConnectedSince,
					BanExpiry:      persistPeer.BanExpiry,
				}
			}

			pm.peers[id] = peer
			pm.peersByAddr[peer.Address] = peer

			return nil
		})
	})
}

// Start begins a goroutine that checks for expired bans every 30 seconds.
func (pm *PeerManager) Start() {
	pm.mu.Lock()
	if pm.running {
		pm.mu.Unlock()
		return
	}
	pm.running = true
	pm.mu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-pm.stopCh:
				return
			case <-ticker.C:
				pm.cleanupExpiredBans()
			}
		}
	}()
}

// Stop closes the stop channel to terminate background goroutines.
func (pm *PeerManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running {
		return
	}

	close(pm.stopCh)
	pm.stopCh = make(chan struct{})
	pm.running = false
}

// cleanupExpiredBans unbans peers whose ban has expired.
func (pm *PeerManager) cleanupExpiredBans() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	now := time.Now()
	for _, peer := range pm.peers {
		if peer.State == PeerBanned && !peer.BanExpiry.IsZero() && now.After(peer.BanExpiry) {
			peer.mu.Lock()
			peer.State = PeerDisconnected
			peer.BanExpiry = time.Time{}
			peer.mu.Unlock()
		}
	}
}
