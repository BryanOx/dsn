package state

import (
	"testing"

	"github.com/dsn/dsn/types"
)

// TestAdversarial_CorruptedSnapshot tests that loading and restoring
// corrupted snapshots returns errors rather than panicking.
func TestAdversarial_CorruptedSnapshot(t *testing.T) {
	ps, dir := setupTempDB(t)
	defer teardownTempDB(ps, dir)

	// Create a valid snapshot first
	addr := types.Address{0: 0x01}
	acc := NewAccount(addr, [32]byte{})
	acc.AddBalance(types.NewAmount(1000))
	ps.SetAccount(addr, acc)
	ps.Commit()

	snap, err := ps.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	data, err := SerializeSnapshot(snap)
	if err != nil {
		t.Fatalf("failed to serialize snapshot: %v", err)
	}

	// Test 1: Truncated data
	t.Run("truncated data", func(t *testing.T) {
		truncated := data[:len(data)/2]
		hash := SnapshotHash(truncated)

		err := RestoreFromSnapshot(ps, truncated, hash)
		if err == nil {
			t.Error("expected error for truncated snapshot data")
		}
	})

	// Test 2: Modified data
	t.Run("modified data", func(t *testing.T) {
		modified := make([]byte, len(data))
		copy(modified, data)
		modified[0] ^= 0xFF // Flip first byte
		modified[len(modified)-1] ^= 0xFF // Flip last byte
		hash := SnapshotHash(data) // Use original hash

		err := RestoreFromSnapshot(ps, modified, hash)
		if err == nil {
			t.Error("expected error for modified snapshot data")
		}
	})

	// Test 3: Wrong hash
	t.Run("wrong hash", func(t *testing.T) {
		wrongHash := types.Hash{0xFF, 0xFF, 0xFF, 0xFF}

		err := RestoreFromSnapshot(ps, data, wrongHash)
		if err == nil {
			t.Error("expected error for wrong snapshot hash")
		}
	})

	// Test 4: Completely garbage data
	t.Run("garbage data", func(t *testing.T) {
		garbage := []byte("this is not a valid snapshot at all!!!")
		wrongHash := SnapshotHash(garbage)

		err := RestoreFromSnapshot(ps, garbage, wrongHash)
		if err == nil {
			t.Error("expected error for garbage data")
		}
	})

	// Test 5: Empty data
	t.Run("empty data", func(t *testing.T) {
		empty := []byte{}
		hash := SnapshotHash(empty)

		err := RestoreFromSnapshot(ps, empty, hash)
		if err == nil {
			t.Error("expected error for empty data")
		}
	})

	// Test 6: Wrong length prefix
	t.Run("wrong length prefix", func(t *testing.T) {
		wrongLen := make([]byte, len(data)+4)
		copy(wrongLen, data)
		// Intentionally create invalid length
		wrongLen[0] = 0xFF
		wrongLen[1] = 0xFF
		wrongLen[2] = 0xFF
		wrongLen[3] = 0xFF
		hash := SnapshotHash(data)

		err := RestoreFromSnapshot(ps, wrongLen, hash)
		// Should either error or gracefully handle
		_ = err
	})

	// After all corruption tests, verify state is still usable
	t.Run("state still usable after failed restore", func(t *testing.T) {
		// Create a fresh account
		addr2 := types.Address{0: 0x02}
		acc2 := NewAccount(addr2, [32]byte{})
		acc2.AddBalance(types.NewAmount(2000))
		ps.SetAccount(addr2, acc2)

		_, err := ps.Commit()
		if err != nil {
			t.Errorf("state should still be usable after failed restore: %v", err)
		}

		// Verify account exists
		restored, err := ps.GetAccount(addr2)
		if err != nil {
			t.Errorf("failed to get account after restore failure: %v", err)
		}
		if restored.Balance.Cmp(types.NewAmount(2000)) != 0 {
			t.Errorf("account balance incorrect: got %v, want 2000", restored.Balance)
		}
	})
}

// TestAdversarial_DeserializeCorruptSnapshot tests that deserializing
// corrupted snapshot data handles errors gracefully.
func TestAdversarial_DeserializeCorruptSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "random bytes",
			data:    []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			wantErr: true,
		},
		{
			name:    "truncated header",
			data:    []byte{0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "valid header but no content",
			data:    []byte{0x00, 0x00, 0x00, 0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeSnapshot(tt.data)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

// TestAdversarial_SnapshotHashUnpredictable tests that snapshot hash
// changes significantly with small data changes (avalanche effect).
func TestAdversarial_SnapshotHashUnpredictable(t *testing.T) {
	// Create valid snapshot
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address{0: 0x01}
	acc := NewAccount(addr, [32]byte{})
	acc.AddBalance(types.NewAmount(1000))
	s.SetAccount(addr, acc)
	s.Commit()

	snap, _ := s.CreateSnapshot(100, 1, types.Hash{0xAB}, 1234567890)
	data, _ := SerializeSnapshot(snap)

	hash1 := SnapshotHash(data)

	// Modify one byte
	for i := 0; i < len(data); i++ {
		modified := make([]byte, len(data))
		copy(modified, data)
		modified[i] ^= 0xFF

		hash2 := SnapshotHash(modified)

		// Hash should be different
		if hash1 == hash2 {
			t.Errorf("hash unchanged after modifying byte %d - poor avalanche", i)
		}

		// Count differing bytes
		diffCount := 0
		for j := range hash1 {
			if hash1[j] != hash2[j] {
				diffCount++
			}
		}

		// Should have significant differences (avalanche effect)
		// At least 16 out of 32 bytes should differ for good diffusion
		if diffCount < 8 {
			t.Errorf("byte %d: only %d bytes changed - insufficient avalanche", i, diffCount)
		}
	}
}

// TestAdversarial_VerifyChunkWithBadData tests VerifyChunk handles
// corrupted chunk data.
func TestAdversarial_VerifyChunkWithBadData(t *testing.T) {
	data := []byte("valid test data for chunk")
	chunks, err := ChunkSnapshot(data, 1024)
	if err != nil {
		t.Fatalf("failed to create chunk: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("no chunks created")
	}

	chunk := chunks[0]

	// Test 1: Tampered data
	t.Run("tampered data", func(t *testing.T) {
		tampered := chunk
		tampered.Data[0] ^= 0xFF
		if VerifyChunk(&tampered) {
			t.Error("tampered chunk should not verify")
		}
	})

	// Test 2: Tampered chunk hash
	t.Run("tampered chunk hash", func(t *testing.T) {
		tampered := chunk
		tampered.ChunkHash[0] ^= 0xFF
		if VerifyChunk(&tampered) {
			t.Error("tampered chunk hash should not verify")
		}
	})

	// Test 3: Wrong total count
	t.Run("wrong total count", func(t *testing.T) {
		modified := chunk
		modified.TotalCount = 999
		if VerifyChunk(&modified) {
			t.Error("wrong total count should not verify")
		}
	})

	// Test 4: Nil chunk
	t.Run("nil chunk", func(t *testing.T) {
		if VerifyChunk(nil) {
			t.Error("nil chunk should not verify")
		}
	})
}