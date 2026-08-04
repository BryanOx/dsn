package state

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

// createTestSnapshotData creates a snapshot with accounts and kvstore entries.
// It returns the serialized snapshot data and the InMemoryState used to create it.
func createTestSnapshotData(t *testing.T) ([]byte, *InMemoryState) {
	t.Helper()
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Create 3 accounts with different balances
	addr1 := types.Address{0: 0x01}
	pub1 := [32]byte{}
	acc1 := NewAccount(addr1, pub1)
	acc1.AddBalance(types.NewAmount(1000))
	s.SetAccount(addr1, acc1)

	addr2 := types.Address{0: 0x02}
	acc2 := NewAccount(addr2, [32]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	s.SetAccount(addr2, acc2)

	addr3 := types.Address{0: 0x10}
	acc3 := NewAccount(addr3, [32]byte{})
	acc3.AddBalance(types.NewAmount(3000))
	s.SetAccount(addr3, acc3)

	// Add kvstore entries in non-sorted order (to test sorting)
	s.SetBytes("z_last", []byte("should be last after sort"))
	s.SetBytes("a_first", []byte("should be first after sort"))
	s.SetBytes("m_middle", []byte("middle entry"))

	// Commit to get a state root
	root, err := s.Commit()
	require.NoError(t, err)
	_ = root

	// Create snapshot from state
	snap, err := s.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	require.NoError(t, err)
	require.NotNil(t, snap)

	// Serialize
	data, err := SerializeSnapshot(snap)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	return data, s
}

// Test 1: TestSnapshotSerializeDeserialize
func TestSnapshotSerializeDeserialize(t *testing.T) {
	// Create snapshot
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	s.SetAccount(addr1, acc1)

	addr2 := types.Address{0: 0x02}
	acc2 := NewAccount(addr2, [32]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	s.SetAccount(addr2, acc2)

	addr3 := types.Address{0: 0x10}
	acc3 := NewAccount(addr3, [32]byte{})
	acc3.AddBalance(types.NewAmount(3000))
	s.SetAccount(addr3, acc3)

	// Add kvstore entries in non-sorted order
	s.SetBytes("z_last", []byte("value_z"))
	s.SetBytes("a_first", []byte("value_a"))
	s.SetBytes("m_middle", []byte("value_m"))

	s.Commit()

	snap, err := s.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	require.NoError(t, err)

	// Serialize
	data, err := SerializeSnapshot(snap)
	require.NoError(t, err)

	// Deserialize
	deserialized, err := DeserializeSnapshot(data)
	require.NoError(t, err)

	// Verify all fields match
	require.Equal(t, uint64(100), deserialized.Height)
	require.Equal(t, uint64(1), deserialized.Epoch)
	require.Equal(t, types.Hash{0xAB}, deserialized.ValidatorSetHash)
	require.Equal(t, uint64(1234567890), deserialized.Timestamp)

	// Verify 3 accounts present with correct balances
	require.Len(t, deserialized.Accounts, 3)

	// Account data is encoded - need to decode to check balance
	acc0 := &Account{}
	err = acc0.Decode(bytes.NewReader(deserialized.Accounts[0].Data))
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(1000), acc0.Balance)

	acc1Dec := &Account{}
	err = acc1Dec.Decode(bytes.NewReader(deserialized.Accounts[1].Data))
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(2000), acc1Dec.Balance)

	acc2Dec := &Account{}
	err = acc2Dec.Decode(bytes.NewReader(deserialized.Accounts[2].Data))
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(3000), acc2Dec.Balance)

	// Verify 3 kvstore entries
	require.Len(t, deserialized.KVStore, 3)
	require.Equal(t, "a_first", deserialized.KVStore[0].Key)
	require.Equal(t, "value_a", string(deserialized.KVStore[0].Value))
	require.Equal(t, "m_middle", deserialized.KVStore[1].Key)
	require.Equal(t, "value_m", string(deserialized.KVStore[1].Value))
	require.Equal(t, "z_last", deserialized.KVStore[2].Key)
	require.Equal(t, "value_z", string(deserialized.KVStore[2].Value))

	// Verify accounts are sorted by address (0x01, 0x02, 0x10)
	require.True(t, bytes.Compare(deserialized.Accounts[0].Address[:], deserialized.Accounts[1].Address[:]) < 0)
	require.True(t, bytes.Compare(deserialized.Accounts[1].Address[:], deserialized.Accounts[2].Address[:]) < 0)

	// Verify KV entries are sorted by key
	require.Equal(t, "a_first", deserialized.KVStore[0].Key)
	require.Equal(t, "m_middle", deserialized.KVStore[1].Key)
	require.Equal(t, "z_last", deserialized.KVStore[2].Key)
}

// Test 2: TestSnapshotDeterminism
func TestSnapshotDeterminism(t *testing.T) {
	hasher := types.SHA256Hasher{}

	// Create first state
	s1 := NewInMemoryState(hasher)
	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	s1.SetAccount(addr1, acc1)
	s1.SetBytes("key1", []byte("value1"))
	s1.Commit()
	snap1, _ := s1.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	data1, _ := SerializeSnapshot(snap1)

	// Create second state with identical content
	s2 := NewInMemoryState(hasher)
	acc2 := NewAccount(addr1, [32]byte{})
	acc2.AddBalance(types.NewAmount(1000))
	s2.SetAccount(addr1, acc2)
	s2.SetBytes("key1", []byte("value1"))
	s2.Commit()
	snap2, _ := s2.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	data2, _ := SerializeSnapshot(snap2)

	// Verify serialized bytes are identical
	require.Equal(t, data1, data2, "identical states should produce identical snapshots")
}

// Test 3: TestSnapshotHash
func TestSnapshotHash(t *testing.T) {
	// Get test data
	originalData, _ := createTestSnapshotData(t)

	// Same data = same hash
	hash1 := SnapshotHash(originalData)
	hash2 := SnapshotHash(originalData)
	require.Equal(t, hash1, hash2, "same data should produce same hash")

	// Different data = different hash
	modifiedData := make([]byte, len(originalData))
	copy(modifiedData, originalData)
	modifiedData[0] ^= 0xFF // Flip all bits in first byte
	hash3 := SnapshotHash(modifiedData)
	require.NotEqual(t, hash1, hash3, "modified data should produce different hash")

	// Different length = different hash
	shortData := originalData[:len(originalData)-1]
	hash4 := SnapshotHash(shortData)
	require.NotEqual(t, hash1, hash4, "shorter data should produce different hash")
}

// Test 4: TestChunkSnapshotDefaultSize
func TestChunkSnapshotDefaultSize(t *testing.T) {
	data, _ := createTestSnapshotData(t)

	// Chunk with default size
	chunks, err := ChunkSnapshot(data, DefaultChunkSize)
	require.NoError(t, err)

	// For small test data, should get exactly 1 chunk
	require.Len(t, chunks, 1, "small snapshot should produce 1 chunk with DefaultChunkSize")

	chunk := chunks[0]
	require.Equal(t, uint32(0), chunk.Index)
	require.Equal(t, uint32(1), chunk.TotalCount)
	require.Equal(t, SnapshotHash(data), chunk.SnapshotHash)
	require.NotEqual(t, types.Hash{}, chunk.ChunkHash, "ChunkHash should be non-zero")

	// Verify chunk data matches original
	require.Equal(t, data, chunk.Data, "chunk data should match original")
}

// Test 5: TestChunkSnapshotMultipleChunks
func TestChunkSnapshotMultipleChunks(t *testing.T) {
	// Create a large snapshot by adding many accounts
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Add 100 accounts to create a larger snapshot
	for i := 0; i < 100; i++ {
		addr := types.Address{byte(i >> 8), byte(i)}
		acc := NewAccount(addr, [32]byte{})
		acc.AddBalance(types.NewAmount(uint64(i + 1)))
		s.SetAccount(addr, acc)
	}

	// Add many kvstore entries
	for i := 0; i < 50; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		s.SetBytes(key, []byte("value_"+key))
	}

	s.Commit()
	snap, _ := s.CreateSnapshot(100, 1, types.Hash{}, 1234567890)
	data, _ := SerializeSnapshot(snap)

	// Use small chunk size
	chunkSize := uint64(1024)
	chunks, err := ChunkSnapshot(data, chunkSize)
	require.NoError(t, err)

	// Should have multiple chunks
	require.Greater(t, len(chunks), 1, "should have more than 1 chunk with small chunk size")

	// Verify total count
	totalCount := chunks[0].TotalCount
	require.Equal(t, uint32(len(chunks)), totalCount, "TotalCount should match number of chunks")

	// Verify each chunk has correct index
	for i, chunk := range chunks {
		require.Equal(t, uint32(i), chunk.Index, "chunk index should be sequential")
		require.Equal(t, SnapshotHash(data), chunk.SnapshotHash, "all chunks should share same SnapshotHash")
	}

	// Verify each chunk's ChunkHash matches its data
	for _, chunk := range chunks {
		require.True(t, VerifyChunk(&chunk), "each chunk should verify against its ChunkHash")
	}
}

// Test 6: TestVerifyChunk
func TestVerifyChunk(t *testing.T) {
	data, _ := createTestSnapshotData(t)
	chunks, _ := ChunkSnapshot(data, DefaultChunkSize)
	chunk := chunks[0]

	// Valid chunk returns true
	require.True(t, VerifyChunk(&chunk), "valid chunk should return true")

	// Nil chunk returns false
	require.False(t, VerifyChunk(nil), "nil chunk should return false")

	// Tampered data returns false
	tamperedChunk := chunk
	tamperedChunk.Data[0] ^= 0xFF
	require.False(t, VerifyChunk(&tamperedChunk), "tampered chunk should return false")
}

// Test 7: TestReassembleSnapshot
func TestReassembleSnapshot(t *testing.T) {
	data, _ := createTestSnapshotData(t)

	// Use small chunk size to ensure multiple chunks
	chunks, err := ChunkSnapshot(data, 512)
	require.NoError(t, err)

	// Need at least 2 chunks for missing chunk test
	if len(chunks) < 2 {
		// Create more data to force multiple chunks
		hasher := types.SHA256Hasher{}
		s := NewInMemoryState(hasher)
		for i := 0; i < 50; i++ {
			addr := types.Address{byte(i >> 8), byte(i)}
			acc := NewAccount(addr, [32]byte{})
			acc.AddBalance(types.NewAmount(uint64(i + 1)))
			s.SetAccount(addr, acc)
		}
		s.Commit()
		snap, _ := s.CreateSnapshot(100, 1, types.Hash{}, 1234567890)
		data, _ = SerializeSnapshot(snap)
		chunks, _ = ChunkSnapshot(data, 512)
	}

	// Reassemble and verify
	reassembled, err := ReassembleSnapshot(chunks)
	require.NoError(t, err)
	require.Equal(t, data, reassembled)
	require.Equal(t, SnapshotHash(data), SnapshotHash(reassembled))

	// Error on missing chunks - create gaps (only pass first chunk when there are multiple)
	if len(chunks) > 1 {
		gappedChunks := []SnapshotChunk{chunks[0]}
		_, err = ReassembleSnapshot(gappedChunks)
		require.Error(t, err, "should error on missing chunks")
	}

	// Error on duplicate chunks (only if we have chunks)
	if len(chunks) > 0 {
		dupeChunks := []SnapshotChunk{chunks[0], chunks[0]}
		_, err = ReassembleSnapshot(dupeChunks)
		require.Error(t, err, "should error on duplicate chunks")
	}

	// Error on corrupt chunk (tamper data before reassembly)
	corruptChunks := make([]SnapshotChunk, len(chunks))
	copy(corruptChunks, chunks)
	corruptChunks[0].Data[0] ^= 0xFF
	_, err = ReassembleSnapshot(corruptChunks)
	require.Error(t, err, "should error on corrupt chunk")
}

// Test 8: TestCreateSnapshot
func TestCreateSnapshot(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Add accounts in non-sorted order
	addr3 := types.Address{0: 0x10}
	acc3 := NewAccount(addr3, [32]byte{})
	acc3.AddBalance(types.NewAmount(3000))
	s.SetAccount(addr3, acc3)

	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	s.SetAccount(addr1, acc1)

	addr2 := types.Address{0: 0x02}
	acc2 := NewAccount(addr2, [32]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	s.SetAccount(addr2, acc2)

	// Add kvstore entries in non-sorted order
	s.SetBytes("z_key", []byte("value_z"))
	s.SetBytes("a_key", []byte("value_a"))
	s.SetBytes("m_key", []byte("value_m"))

	// Commit to get state root
	root, err := s.Commit()
	require.NoError(t, err)

	// Create snapshot
	valSetHash := types.Hash{0xAB, 0xCD}
	timestamp := uint64(1234567890)
	snap, err := s.CreateSnapshot(100, 5, valSetHash, timestamp)
	require.NoError(t, err)

	// Verify accounts are sorted by address
	require.Len(t, snap.Accounts, 3)
	require.Equal(t, addr1, snap.Accounts[0].Address)
	require.Equal(t, addr2, snap.Accounts[1].Address)
	require.Equal(t, addr3, snap.Accounts[2].Address)

	// Verify kvstore entries are sorted by key
	require.Len(t, snap.KVStore, 3)
	require.Equal(t, "a_key", snap.KVStore[0].Key)
	require.Equal(t, "m_key", snap.KVStore[1].Key)
	require.Equal(t, "z_key", snap.KVStore[2].Key)

	// Verify state root matches SMT root
	require.Equal(t, root, snap.StateRoot)

	// Verify other fields
	require.Equal(t, uint64(100), snap.Height)
	require.Equal(t, uint64(5), snap.Epoch)
	require.Equal(t, valSetHash, snap.ValidatorSetHash)
	require.Equal(t, timestamp, snap.Timestamp)
}

// Test 9: TestCreateSnapshotPersistent
func TestCreateSnapshotPersistent(t *testing.T) {
	ps, dir := setupTempDB(t)
	defer teardownTempDB(ps, dir)

	// Create accounts and kvstore
	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	ps.SetAccount(addr1, acc1)

	addr2 := types.Address{0: 0x02}
	acc2 := NewAccount(addr2, [32]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	ps.SetAccount(addr2, acc2)

	ps.SetBytes("key1", []byte("value1"))
	ps.SetBytes("key2", []byte("value2"))

	// Commit
	_, err := ps.Commit()
	require.NoError(t, err)

	// Create snapshot
	snap, err := ps.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	require.NoError(t, err)
	require.NotNil(t, snap)

	// Verify accounts are captured
	require.Len(t, snap.Accounts, 2)

	// Verify kvstore entries are captured
	require.Len(t, snap.KVStore, 2)

	// Verify state root
	require.NotEqual(t, types.Hash{}, snap.StateRoot)
}

// Test 10: TestRestoreFromSnapshot
func TestRestoreFromSnapshot(t *testing.T) {
	// Create original PersistentState with accounts and kvstore
	ps1, dir := setupTempDB(t)
	defer teardownTempDB(ps1, dir)

	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	ps1.SetAccount(addr1, acc1)

	addr2 := types.Address{0: 0x02}
	acc2 := NewAccount(addr2, [32]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	ps1.SetAccount(addr2, acc2)

	ps1.SetBytes("key1", []byte("value1"))
	ps1.SetBytes("key2", []byte("value2"))

	// Commit to get state root
	originalRoot, err := ps1.Commit()
	require.NoError(t, err)

	// Create snapshot
	snap, err := ps1.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	require.NoError(t, err)

	// Serialize and compute hash
	data, err := SerializeSnapshot(snap)
	require.NoError(t, err)
	expectedHash := SnapshotHash(data)

	// Close original
	ps1.Close()

	// Create fresh PersistentState (simulating new node)
	hasher := types.SHA256Hasher{}
	ps2, err := NewPersistentState(filepath.Join(dir, "dsn.db"), hasher, false)
	require.NoError(t, err)
	defer ps2.Close()

	// Clear any existing state (start fresh)
	ps2.cache = make(map[types.Address]*Account)
	ps2.kvstore = make(map[string][]byte)

	// Restore from snapshot
	err = RestoreFromSnapshot(ps2, data, expectedHash)
	require.NoError(t, err)

	// Verify accounts exist with correct balances
	restoredAcc1, err := ps2.GetAccount(addr1)
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(1000), restoredAcc1.Balance)

	restoredAcc2, err := ps2.GetAccount(addr2)
	require.NoError(t, err)
	require.Equal(t, types.NewAmount(2000), restoredAcc2.Balance)

	// Verify KVStore entries exist with correct values
	val1, ok := ps2.GetBytes("key1")
	require.True(t, ok)
	require.Equal(t, "value1", string(val1))

	val2, ok := ps2.GetBytes("key2")
	require.True(t, ok)
	require.Equal(t, "value2", string(val2))

	// Verify state root matches original
	require.Equal(t, originalRoot, ps2.GetStateRoot())
}

// Test 11: TestRestoreCorruptData
func TestRestoreCorruptData(t *testing.T) {
	ps, dir := setupTempDB(t)
	defer teardownTempDB(ps, dir)

	// Pass garbage bytes to RestoreFromSnapshot
	garbageData := []byte("this is not a valid snapshot")
	wrongHash := SnapshotHash([]byte("some other data"))

	err := RestoreFromSnapshot(ps, garbageData, wrongHash)
	require.Error(t, err, "should return error for corrupt data")

	// Verify state is not corrupted - should still be usable
	addr := types.Address{0: 0x01}
	acc := NewAccount(addr, [32]byte{})
	ps.SetAccount(addr, acc)
	_, err = ps.Commit()
	require.NoError(t, err, "state should still be usable after failed restore")
}

// Test 12: TestRestoreSnapshotHashMismatch
func TestRestoreSnapshotHashMismatch(t *testing.T) {
	ps, dir := setupTempDB(t)
	defer teardownTempDB(ps, dir)

	// Create valid snapshot data
	addr1 := types.Address{0: 0x01}
	acc1 := NewAccount(addr1, [32]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	ps.SetAccount(addr1, acc1)
	ps.Commit()

	snap, _ := ps.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	data, _ := SerializeSnapshot(snap)

	// Create wrong expected hash
	wrongHash := types.Hash{0xFF, 0xFF}

	// Try to restore with wrong hash
	err := RestoreFromSnapshot(ps, data, wrongHash)
	require.Error(t, err, "should return error for hash mismatch")
}

// Test 13: TestSnapshotPersistence
func TestSnapshotPersistence(t *testing.T) {
	ps, dir := setupTempDB(t)

	// Create accounts and kvstore
	addr := types.Address{0: 0x01}
	acc := NewAccount(addr, [32]byte{})
	acc.AddBalance(types.NewAmount(5000))
	ps.SetAccount(addr, acc)
	ps.SetBytes("testkey", []byte("testvalue"))

	// Commit and create snapshot
	ps.Commit()
	snap, _ := ps.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	data, _ := SerializeSnapshot(snap)

	// Store snapshot
	err := StoreSnapshot(ps, 100, data)
	require.NoError(t, err)

	// Close the DB (dir is kept for cleanup later)
	ps.Close()

	// Reopen the DB (simulating restart)
	hasher := types.SHA256Hasher{}
	ps2, err := NewPersistentState(filepath.Join(dir, "dsn.db"), hasher, false)
	require.NoError(t, err)
	defer func() {
		ps2.Close()
		teardownTempDB(ps2, dir)
	}()

	// Verify snapshot exists
	exists := SnapshotExists(ps2, 100)
	require.True(t, exists, "snapshot should exist after restart")

	// Load snapshot
	loadedData, err := LoadSnapshot(ps2, 100)
	require.NoError(t, err)
	require.Equal(t, data, loadedData, "loaded data should match stored data")

	// Verify deserialize gives correct data
	restoredSnap, err := DeserializeSnapshot(loadedData)
	require.NoError(t, err)
	require.Equal(t, uint64(100), restoredSnap.Height)
	require.Equal(t, uint64(1), restoredSnap.Epoch)
}

// Test 14: TestChunkEdgeCases
func TestChunkEdgeCases(t *testing.T) {
	// Empty data -> error
	_, err := ChunkSnapshot([]byte{}, 1024)
	require.Error(t, err, "empty data should return error")

	// Zero chunk size -> error
	_, err = ChunkSnapshot([]byte{1, 2, 3}, 0)
	require.Error(t, err, "zero chunk size should return error")

	// Single byte chunk size (minimum)
	data := []byte{1, 2, 3, 4, 5}
	chunks, err := ChunkSnapshot(data, 1)
	require.NoError(t, err)
	require.Len(t, chunks, 5, "each byte should be its own chunk")

	// Verify each chunk verifies
	for _, chunk := range chunks {
		require.True(t, VerifyChunk(&chunk), "each chunk should verify")
	}
}
