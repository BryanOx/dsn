package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/dsn/dsn/types"
)

const (
	SnapshotMagic          = "DSNS"
	SnapshotVersion        = uint32(1)
	SnapshotHeaderSize     = 144
	HashSize               = 32
	DefaultChunkSize       = 256 * 1024
	MaxSnapshotEntrySize   = 1 * 1024 * 1024 // 1 MiB per account / KV entry
	MaxSnapshotKVKeyLength = 256              // bytes
	MaxSnapshotKVValLength = 1 * 1024 * 1024  // 1 MiB value limit
)

type Snapshot struct {
	Height           uint64
	Epoch            uint64
	StateRoot        types.Hash
	ValidatorSetHash types.Hash
	Timestamp        uint64
	Accounts         []AccountEntry
	KVStore          []KVEntry
}

type AccountEntry struct {
	Address types.Address
	Data    []byte
}

type KVEntry struct {
	Key   string
	Value []byte
}

type SnapshotChunk struct {
	Index        uint32
	TotalCount   uint32
	SnapshotHash types.Hash
	ChunkHash    types.Hash
	Data         []byte
}

func (s *InMemoryState) CreateSnapshot(height, epoch uint64, valSetHash types.Hash, timestamp uint64) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	accounts := make([]AccountEntry, 0, len(s.accounts))
	for addr, acc := range s.accounts {
		var buf bytes.Buffer
		if err := acc.Encode(&buf); err != nil {
			return nil, fmt.Errorf("failed to encode account: %w", err)
		}
		accounts = append(accounts, AccountEntry{
			Address: addr,
			Data:    buf.Bytes(),
		})
	}

	sort.Slice(accounts, func(i, j int) bool {
		return bytes.Compare(accounts[i].Address[:], accounts[j].Address[:]) < 0
	})

	kvs := make([]KVEntry, 0, len(s.kvstore))
	for k, v := range s.kvstore {
		valCopy := make([]byte, len(v))
		copy(valCopy, v)
		kvs = append(kvs, KVEntry{Key: k, Value: valCopy})
	}

	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })

	snap := &Snapshot{
		Height:           height,
		Epoch:            epoch,
		StateRoot:        s.smt.Root(),
		ValidatorSetHash: valSetHash,
		Timestamp:        timestamp,
		Accounts:         accounts,
		KVStore:          kvs,
	}

	return snap, nil
}

func (ps *PersistentState) CreateSnapshot(height, epoch uint64, valSetHash types.Hash, timestamp uint64) (*Snapshot, error) {
	accounts := make([]AccountEntry, 0, len(ps.cache))
	for addr, acc := range ps.cache {
		var buf bytes.Buffer
		if err := acc.Encode(&buf); err != nil {
			return nil, fmt.Errorf("failed to encode account: %w", err)
		}
		accounts = append(accounts, AccountEntry{
			Address: addr,
			Data:    buf.Bytes(),
		})
	}

	sort.Slice(accounts, func(i, j int) bool {
		return bytes.Compare(accounts[i].Address[:], accounts[j].Address[:]) < 0
	})

	kvs := make([]KVEntry, 0, len(ps.kvstore))
	for k, v := range ps.kvstore {
		valCopy := make([]byte, len(v))
		copy(valCopy, v)
		kvs = append(kvs, KVEntry{Key: k, Value: valCopy})
	}

	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })

	snap := &Snapshot{
		Height:           height,
		Epoch:            epoch,
		StateRoot:        ps.root,
		ValidatorSetHash: valSetHash,
		Timestamp:        timestamp,
		Accounts:         accounts,
		KVStore:          kvs,
	}

	return snap, nil
}

func SerializeSnapshot(snap *Snapshot) ([]byte, error) {
	var buf bytes.Buffer

	if _, err := buf.Write([]byte(SnapshotMagic)); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, SnapshotVersion); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, snap.Height); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, snap.Epoch); err != nil {
		return nil, err
	}

	if _, err := buf.Write(snap.StateRoot[:]); err != nil {
		return nil, err
	}

	if _, err := buf.Write(snap.ValidatorSetHash[:]); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, snap.Timestamp); err != nil {
		return nil, err
	}

	accountCount := uint64(len(snap.Accounts))
	if err := binary.Write(&buf, binary.BigEndian, accountCount); err != nil {
		return nil, err
	}

	kvCount := uint64(len(snap.KVStore))
	if err := binary.Write(&buf, binary.BigEndian, kvCount); err != nil {
		return nil, err
	}

	reserved := make([]byte, 32)
	if _, err := buf.Write(reserved); err != nil {
		return nil, err
	}

	sortedAccounts := make([]AccountEntry, len(snap.Accounts))
	copy(sortedAccounts, snap.Accounts)
	sort.Slice(sortedAccounts, func(i, j int) bool {
		return bytes.Compare(sortedAccounts[i].Address[:], sortedAccounts[j].Address[:]) < 0
	})

	for _, acc := range sortedAccounts {
		if _, err := buf.Write(acc.Address[:]); err != nil {
			return nil, err
		}

		dataLen := uint32(len(acc.Data))
		if err := binary.Write(&buf, binary.BigEndian, dataLen); err != nil {
			return nil, err
		}

		if _, err := buf.Write(acc.Data); err != nil {
			return nil, err
		}
	}

	sortedKVs := make([]KVEntry, len(snap.KVStore))
	copy(sortedKVs, snap.KVStore)
	sort.Slice(sortedKVs, func(i, j int) bool { return sortedKVs[i].Key < sortedKVs[j].Key })

	for _, kv := range sortedKVs {
		keyLen := uint32(len(kv.Key))
		if err := binary.Write(&buf, binary.BigEndian, keyLen); err != nil {
			return nil, err
		}

		if _, err := buf.WriteString(kv.Key); err != nil {
			return nil, err
		}

		valLen := uint32(len(kv.Value))
		if err := binary.Write(&buf, binary.BigEndian, valLen); err != nil {
			return nil, err
		}

		if _, err := buf.Write(kv.Value); err != nil {
			return nil, err
		}
	}

	hasher := sha256.New()
	hasher.Write(buf.Bytes())
	checksum := hasher.Sum(nil)

	if _, err := buf.Write(checksum); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DeserializeSnapshot(data []byte) (*Snapshot, error) {
	minLen := SnapshotHeaderSize + HashSize
	if len(data) < minLen {
		return nil, fmt.Errorf("snapshot data too short: got %d, expected at least %d", len(data), minLen)
	}

	reader := bytes.NewReader(data[:len(data)-HashSize])

	magic := make([]byte, 4)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return nil, fmt.Errorf("failed to read magic: %w", err)
	}
	if string(magic) != SnapshotMagic {
		return nil, fmt.Errorf("invalid magic: got %s, expected %s", string(magic), SnapshotMagic)
	}

	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if version != SnapshotVersion {
		return nil, fmt.Errorf("unsupported version: got %d, expected %d", version, SnapshotVersion)
	}

	var height, epoch, timestamp uint64
	var stateRoot, validatorSetHash types.Hash
	var accountCount, kvCount uint64

	if err := binary.Read(reader, binary.BigEndian, &height); err != nil {
		return nil, fmt.Errorf("failed to read height: %w", err)
	}
	if err := binary.Read(reader, binary.BigEndian, &epoch); err != nil {
		return nil, fmt.Errorf("failed to read epoch: %w", err)
	}
	if _, err := io.ReadFull(reader, stateRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read state root: %w", err)
	}
	if _, err := io.ReadFull(reader, validatorSetHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read validator set hash: %w", err)
	}
	if err := binary.Read(reader, binary.BigEndian, &timestamp); err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}
	if err := binary.Read(reader, binary.BigEndian, &accountCount); err != nil {
		return nil, fmt.Errorf("failed to read account count: %w", err)
	}
	if err := binary.Read(reader, binary.BigEndian, &kvCount); err != nil {
		return nil, fmt.Errorf("failed to read kv count: %w", err)
	}

	if _, err := reader.Seek(32, io.SeekCurrent); err != nil {
		return nil, fmt.Errorf("failed to skip reserved bytes: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(data[:len(data)-HashSize])
	computedChecksum := hasher.Sum(nil)
	storedChecksum := data[len(data)-HashSize:]
	if !bytes.Equal(computedChecksum, storedChecksum) {
		return nil, fmt.Errorf("checksum mismatch")
	}

	accounts := make([]AccountEntry, 0, accountCount)
	var prevAddr types.Address
	for i := uint64(0); i < accountCount; i++ {
		addrBytes := make([]byte, 20)
		if _, err := io.ReadFull(reader, addrBytes); err != nil {
			return nil, fmt.Errorf("failed to read account address: %w", err)
		}
		addr, err := types.AddressFromBytes(addrBytes)
		if err != nil {
			return nil, fmt.Errorf("invalid account address: %w", err)
		}

		if i > 0 && bytes.Compare(addr[:], prevAddr[:]) <= 0 {
			return nil, fmt.Errorf("accounts not in ascending order at index %d", i)
		}
		prevAddr = addr

		var dataLen uint32
		if err := binary.Read(reader, binary.BigEndian, &dataLen); err != nil {
			return nil, fmt.Errorf("failed to read account data length: %w", err)
		}
		if dataLen > MaxSnapshotEntrySize {
			return nil, fmt.Errorf("account data too large: %d > %d", dataLen, MaxSnapshotEntrySize)
		}

		accData := make([]byte, dataLen)
		if _, err := io.ReadFull(reader, accData); err != nil {
			return nil, fmt.Errorf("failed to read account data: %w", err)
		}

		accounts = append(accounts, AccountEntry{
			Address: addr,
			Data:    accData,
		})
	}

	kvs := make([]KVEntry, 0, kvCount)
	var prevKey string
	for i := uint64(0); i < kvCount; i++ {
		var keyLen uint32
		if err := binary.Read(reader, binary.BigEndian, &keyLen); err != nil {
			return nil, fmt.Errorf("failed to read kv key length: %w", err)
		}
		if keyLen > MaxSnapshotKVKeyLength {
			return nil, fmt.Errorf("kv key too long: %d > %d", keyLen, MaxSnapshotKVKeyLength)
		}

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, keyBytes); err != nil {
			return nil, fmt.Errorf("failed to read kv key: %w", err)
		}
		key := string(keyBytes)

		if i > 0 && key <= prevKey {
			return nil, fmt.Errorf("kv entries not in ascending order at index %d", i)
		}
		prevKey = key

		var valLen uint32
		if err := binary.Read(reader, binary.BigEndian, &valLen); err != nil {
			return nil, fmt.Errorf("failed to read kv value length: %w", err)
		}
		if valLen > MaxSnapshotKVValLength {
			return nil, fmt.Errorf("kv value too large: %d > %d", valLen, MaxSnapshotKVValLength)
		}

		val := make([]byte, valLen)
		if _, err := io.ReadFull(reader, val); err != nil {
			return nil, fmt.Errorf("failed to read kv value: %w", err)
		}

		kvs = append(kvs, KVEntry{
			Key:   key,
			Value: val,
		})
	}

	return &Snapshot{
		Height:           height,
		Epoch:            epoch,
		StateRoot:        stateRoot,
		ValidatorSetHash: validatorSetHash,
		Timestamp:        timestamp,
		Accounts:         accounts,
		KVStore:          kvs,
	}, nil
}

func SnapshotHash(data []byte) types.Hash {
	hasher := sha256.New()
	hasher.Write(data)
	hashBytes := hasher.Sum(nil)

	var hash types.Hash
	copy(hash[:], hashBytes)
	return hash
}

func ChunkSnapshot(data []byte, chunkSize uint64) ([]SnapshotChunk, error) {
	if chunkSize == 0 {
		return nil, fmt.Errorf("chunk size cannot be zero")
	}

	dataLen := uint64(len(data))
	if dataLen == 0 {
		return nil, fmt.Errorf("snapshot data is empty")
	}

	totalChunks := (dataLen + chunkSize - 1) / chunkSize
	if totalChunks > 0xFFFFFFFF {
		return nil, fmt.Errorf("too many chunks: %d", totalChunks)
	}

	snapHash := SnapshotHash(data)

	chunks := make([]SnapshotChunk, 0, totalChunks)
	var offset uint64
	var index uint32

	for offset < dataLen {
		remain := dataLen - offset
		chunkDataSize := chunkSize
		if remain < chunkSize {
			chunkDataSize = remain
		}

		chunkData := make([]byte, chunkDataSize)
		copy(chunkData, data[offset:offset+chunkDataSize])

		chunkHasher := sha256.New()
		chunkHasher.Write(chunkData)
		chunkHashBytes := chunkHasher.Sum(nil)

		var chunkHash types.Hash
		copy(chunkHash[:], chunkHashBytes)

		chunks = append(chunks, SnapshotChunk{
			Index:        index,
			TotalCount:   uint32(totalChunks),
			SnapshotHash: snapHash,
			ChunkHash:    chunkHash,
			Data:         chunkData,
		})

		offset += chunkDataSize
		index++
	}

	return chunks, nil
}

func VerifyChunk(chunk *SnapshotChunk) bool {
	if chunk == nil {
		return false
	}

	hasher := sha256.New()
	hasher.Write(chunk.Data)
	computedHash := hasher.Sum(nil)

	return bytes.Equal(computedHash, chunk.ChunkHash[:])
}

func ReassembleSnapshot(chunks []SnapshotChunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks provided")
	}

	sortedChunks := make([]SnapshotChunk, len(chunks))
	copy(sortedChunks, chunks)
	sort.Slice(sortedChunks, func(i, j int) bool {
		return sortedChunks[i].Index < sortedChunks[j].Index
	})

	totalCount := sortedChunks[0].TotalCount
	if len(sortedChunks) != int(totalCount) {
		return nil, fmt.Errorf("chunk count mismatch: got %d, expected %d", len(sortedChunks), totalCount)
	}

	data := make([]byte, 0, len(chunks)*int(DefaultChunkSize))
	expectedSnapshotHash := sortedChunks[0].SnapshotHash

	for i, chunk := range sortedChunks {
		if chunk.Index != uint32(i) {
			return nil, fmt.Errorf("missing chunk at index %d", i)
		}

		if !VerifyChunk(&chunk) {
			return nil, fmt.Errorf("chunk %d hash verification failed", i)
		}

		if chunk.SnapshotHash != expectedSnapshotHash {
			return nil, fmt.Errorf("chunk %d has different snapshot hash", i)
		}

		data = append(data, chunk.Data...)
	}

	computedSnapHash := SnapshotHash(data)
	if computedSnapHash != expectedSnapshotHash {
		return nil, fmt.Errorf("reassembled data hash mismatch")
	}

	return data, nil
}