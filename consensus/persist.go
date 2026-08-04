package consensus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

var (
	blocksBucket       = []byte("blocks")
	chainBucket        = []byte("chain")
	tipKey             = []byte("tip")
	proposalsBucket    = []byte("proposals")
	votesBucket        = []byte("votes")
	commitProofsBucket = []byte("commit_proofs")
	evidenceBucket     = []byte("evidence")
)

// StoreBlockHeader persists a block header to the database.
func StoreBlockHeader(ps *state.PersistentState, header *types.BlockHeader) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}
		data := encodeHeader(header)
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, header.Height)
		return b.Put(key, data)
	})
}

// LoadBlockHeader retrieves a block header by height.
func LoadBlockHeader(ps *state.PersistentState, height uint64) (*types.BlockHeader, error) {
	var header *types.BlockHeader
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(blocksBucket)
		if b == nil {
			return fmt.Errorf("blocks bucket not found")
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, height)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("block header not found at height %d", height)
		}
		decoded, err := decodeHeader(data)
		if err != nil {
			return fmt.Errorf("failed to decode header at height %d: %w", height, err)
		}
		header = decoded
		return nil
	})
	return header, err
}

// fullBlocksBucket is the BoltDB bucket for full block data.
var fullBlocksBucket = []byte("full_blocks")

// StoreBlock persists a full block (header + transactions) to the database.
func StoreBlock(ps *state.PersistentState, block *types.Block) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(fullBlocksBucket)
		if err != nil {
			return err
		}
		// Encode block using gossip format
		data, err := EncodeBlockMessage(block)
		if err != nil {
			return fmt.Errorf("failed to encode block: %w", err)
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, block.Header.Height)
		return b.Put(key, data)
	})
}

// LoadBlock retrieves a full block by height.
func LoadBlock(ps *state.PersistentState, height uint64) (*types.Block, error) {
	if ps == nil {
		return nil, fmt.Errorf("persistent state is nil")
	}
	var block *types.Block
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(fullBlocksBucket)
		if b == nil {
			return fmt.Errorf("full blocks bucket not found")
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, height)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("full block not found at height %d", height)
		}
		decoded, err := DecodeBlockMessage(data)
		if err != nil {
			return fmt.Errorf("failed to decode block: %w", err)
		}
		block = decoded
		return nil
	})
	return block, err
}

// StoreTip updates the chain tip (latest known block header).
func StoreTip(ps *state.PersistentState, header *types.BlockHeader) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(chainBucket)
		if err != nil {
			return err
		}
		return b.Put(tipKey, encodeHeader(header))
	})
}

// LoadTip retrieves the chain tip (latest known block header).
func LoadTip(ps *state.PersistentState) (*types.BlockHeader, error) {
	var header *types.BlockHeader
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(chainBucket)
		if b == nil {
			return fmt.Errorf("chain bucket not found")
		}
		data := b.Get(tipKey)
		if data == nil {
			return fmt.Errorf("tip not found")
		}
		decoded, err := decodeHeader(data)
		if err != nil {
			return fmt.Errorf("failed to decode tip header: %w", err)
		}
		header = decoded
		return nil
	})
	return header, err
}

// AtomicStoreBlockAndTip atomically stores the block header, full block body,
// and updates the chain tip in a single BoltDB transaction.
// If any write fails, the entire transaction rolls back - no partial state persists.
func AtomicStoreBlockAndTip(ps *state.PersistentState, block *types.Block) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		// 1. Store block header
		headerBucket, err := tx.CreateBucketIfNotExists(blocksBucket)
		if err != nil {
			return err
		}
		headerKey := make([]byte, 8)
		binary.BigEndian.PutUint64(headerKey, block.Header.Height)
		if err := headerBucket.Put(headerKey, encodeHeader(&block.Header)); err != nil {
			return err
		}

		// 2. Store full block body
		bodyBucket, err := tx.CreateBucketIfNotExists(fullBlocksBucket)
		if err != nil {
			return err
		}
		blockData, err := EncodeBlockMessage(block)
		if err != nil {
			return fmt.Errorf("failed to encode block: %w", err)
		}
		if err := bodyBucket.Put(headerKey, blockData); err != nil {
			return err
		}

		// 3. Update chain tip
		chainBucket, err := tx.CreateBucketIfNotExists(chainBucket)
		if err != nil {
			return err
		}
		return chainBucket.Put(tipKey, encodeHeader(&block.Header))
	})
}

// encodeHeader serializes a BlockHeader to bytes.
// Layout: Version(8) + Height(8) + PreviousHash(32) + StateRoot(32) + TxRoot(32) +
//
//	ReceiptRoot(32) + ValidatorRoot(32) + ValidatorSetHash(32) + Epoch(8) + Round(4) + Timestamp(8) + Proposer(20) = 248 bytes
const expectedHeaderLen = 248

// encodeHeader serializes a BlockHeader to bytes.
func encodeHeader(h *types.BlockHeader) []byte {
	data := make([]byte, expectedHeaderLen)
	offset := 0

	binary.BigEndian.PutUint64(data[offset:], h.Version)
	offset += 8
	binary.BigEndian.PutUint64(data[offset:], h.Height)
	offset += 8
	copy(data[offset:], h.PreviousHash[:])
	offset += 32
	copy(data[offset:], h.StateRoot[:])
	offset += 32
	copy(data[offset:], h.TxRoot[:])
	offset += 32
	copy(data[offset:], h.ReceiptRoot[:])
	offset += 32
	copy(data[offset:], h.ValidatorRoot[:])
	offset += 32
	copy(data[offset:], h.ValidatorSetHash[:])
	offset += 32
	binary.BigEndian.PutUint64(data[offset:], h.Epoch)
	offset += 8
	binary.BigEndian.PutUint32(data[offset:], h.Round)
	offset += 4
	binary.BigEndian.PutUint64(data[offset:], h.Timestamp)
	offset += 8
	copy(data[offset:], h.Proposer[:])
	// offset += 20

	return data
}

// decodeHeader deserializes a BlockHeader from bytes.
func decodeHeader(data []byte) (*types.BlockHeader, error) {
	if len(data) < expectedHeaderLen {
		return nil, fmt.Errorf("header too short: got %d bytes, need at least %d", len(data), expectedHeaderLen)
	}

	h := &types.BlockHeader{}
	offset := 0

	h.Version = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	h.Height = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	copy(h.PreviousHash[:], data[offset:])
	offset += 32
	copy(h.StateRoot[:], data[offset:])
	offset += 32
	copy(h.TxRoot[:], data[offset:])
	offset += 32
	copy(h.ReceiptRoot[:], data[offset:])
	offset += 32
	copy(h.ValidatorRoot[:], data[offset:])
	offset += 32
	copy(h.ValidatorSetHash[:], data[offset:])
	offset += 32
	h.Epoch = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	h.Round = binary.BigEndian.Uint32(data[offset:])
	offset += 4
	h.Timestamp = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	copy(h.Proposer[:], data[offset:])

	return h, nil
}

// StoreProposal persists a proposal for the given height.
func StoreProposal(ps *state.PersistentState, proposal *types.Proposal) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(proposalsBucket)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := proposal.Encode(&buf); err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, proposal.Height)
		return b.Put(key, buf.Bytes())
	})
}

// LoadProposal retrieves a proposal by height.
func LoadProposal(ps *state.PersistentState, height uint64) (*types.Proposal, error) {
	var proposal *types.Proposal
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(proposalsBucket)
		if b == nil {
			return fmt.Errorf("proposals bucket not found")
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, height)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("proposal not found at height %d", height)
		}
		proposal = &types.Proposal{}
		return proposal.Decode(bytes.NewReader(data))
	})
	return proposal, err
}

// StoreVote persists a vote for the given height and vote type.
// Key format: height(8) + voteType(1) + validator(20)
func StoreVote(ps *state.PersistentState, vote *types.Vote) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(votesBucket)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := vote.Encode(&buf); err != nil {
			return err
		}
		// Key: height(8) + voteType(1) + validator(20)
		key := make([]byte, 8+1+20)
		binary.BigEndian.PutUint64(key, vote.Height)
		key[8] = byte(vote.VoteType)
		copy(key[9:], vote.Validator[:])
		return b.Put(key, buf.Bytes())
	})
}

// LoadVotes loads all votes for a given height.
// If voteType is 0, loads all vote types; otherwise filters by type.
func LoadVotes(ps *state.PersistentState, height uint64, voteType types.VoteType) ([]*types.Vote, error) {
	var votes []*types.Vote
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(votesBucket)
		if b == nil {
			return nil // no votes stored yet
		}

		c := b.Cursor()
		heightBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(heightBytes, height)

		// Prefix for this height
		prefix := heightBytes

		k, v := c.Seek(prefix)
		for k != nil {
			// Stop if we've moved past our height's votes
			if len(k) < 8 || binary.BigEndian.Uint64(k[:8]) != height {
				break
			}

			// Check vote type filter
			if voteType != 0 && len(k) >= 9 && types.VoteType(k[8]) != voteType {
				// Move to next
				k, v = c.Next()
				continue
			}

			vote := &types.Vote{}
			if err := vote.Decode(bytes.NewReader(v)); err != nil {
				return err
			}
			votes = append(votes, vote)

			k, v = c.Next()
		}
		return nil
	})
	return votes, err
}

// StoreCommitProof persists a commit proof for the given height.
func StoreCommitProof(ps *state.PersistentState, proof *types.CommitProof) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(commitProofsBucket)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := proof.Encode(&buf); err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, proof.Height)
		return b.Put(key, buf.Bytes())
	})
}

// LoadCommitProof retrieves a commit proof by height.
func LoadCommitProof(ps *state.PersistentState, height uint64) (*types.CommitProof, error) {
	var proof *types.CommitProof
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(commitProofsBucket)
		if b == nil {
			return fmt.Errorf("commit proofs bucket not found")
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, height)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("commit proof not found at height %d", height)
		}
		proof = &types.CommitProof{}
		return proof.Decode(bytes.NewReader(data))
	})
	return proof, err
}

// StoreEvidence persists an evidence object.
// key format: "evidence/{height}/{hash_prefix(8)}"
// bucket: "evidence"
// Also stores an index key "evidence/all/{hash}" for listing all evidence.
func StoreEvidence(ps *state.PersistentState, ev types.Evidence) error {
	return ps.DB().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(evidenceBucket)
		if err != nil {
			return err
		}

		// Compute evidence hash
		hash, err := ev.EvidenceHash(types.SHA256Hasher{})
		if err != nil {
			return fmt.Errorf("failed to compute evidence hash: %w", err)
		}

		// Check for duplicate evidence using the all-index key
		indexKey := fmt.Sprintf("evidence/all/%x", hash[:])
		if b.Get([]byte(indexKey)) != nil {
			return types.ErrDuplicateEvidence
		}

		// Store evidence with key: "evidence/{height}/{hash_prefix(8)}"
		evidenceKey := fmt.Sprintf("evidence/%d/%x", ev.Height(), hash[:8])
		var buf bytes.Buffer
		if err := types.EncodeEvidence(&buf, ev); err != nil {
			return fmt.Errorf("failed to encode evidence: %w", err)
		}
		if err := b.Put([]byte(evidenceKey), buf.Bytes()); err != nil {
			return err
		}

		// Store index key for listing all evidence
		return b.Put([]byte(indexKey), buf.Bytes())
	})
}

// LoadEvidence loads a single evidence object by height and hash prefix.
func LoadEvidence(ps *state.PersistentState, height uint64, hashPrefix string) (types.Evidence, error) {
	var ev types.Evidence
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(evidenceBucket)
		if b == nil {
			return nil // no evidence stored yet
		}

		// Key: "evidence/{height}/{hashPrefix}"
		evidenceKey := fmt.Sprintf("evidence/%d/%s", height, hashPrefix)
		data := b.Get([]byte(evidenceKey))
		if data == nil {
			return nil // evidence not found
		}

		decoded, err := types.DecodeEvidence(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to decode evidence: %w", err)
		}
		ev = decoded
		return nil
	})
	return ev, err
}

// LoadAllEvidence loads all evidence objects, sorted by height descending (newest first).
func LoadAllEvidence(ps *state.PersistentState) ([]types.Evidence, error) {
	var evidence []types.Evidence
	err := ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(evidenceBucket)
		if b == nil {
			return nil // no evidence stored yet
		}

		// Collect all evidence from the index keys
		c := b.Cursor()
		prefix := []byte("evidence/all/")
		for k, v := c.Seek(prefix); k != nil; k, v = c.Next() {
			if len(k) <= len(prefix) {
				continue
			}
			decoded, err := types.DecodeEvidence(bytes.NewReader(v))
			if err != nil {
				return fmt.Errorf("failed to decode evidence: %w", err)
			}
			evidence = append(evidence, decoded)
		}

		// Sort by height descending (newest first)
		sort.Slice(evidence, func(i, j int) bool {
			return evidence[i].Height() > evidence[j].Height()
		})

		return nil
	})
	return evidence, err
}

// EvidenceCount returns the total number of stored evidence objects.
func EvidenceCount(ps *state.PersistentState) int {
	count := 0
	ps.DB().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(evidenceBucket)
		if b == nil {
			return nil
		}

		prefix := []byte("evidence/all/")
		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil; k, _ = c.Next() {
			if len(k) <= len(prefix) {
				continue
			}
			count++
		}
		return nil
	})
	return count
}
