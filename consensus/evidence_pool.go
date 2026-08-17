package consensus

import (
	"fmt"
	"sync"

	"github.com/BryanOx/dsn/types"
)

// EvidencePool is a thread-safe pool for pending evidence awaiting inclusion
// in a block. It deduplicates by evidence hash and enforces a maximum capacity.
type EvidencePool struct {
	mu       sync.Mutex
	pending  []types.Evidence
	seen     map[types.Hash]bool
	capacity int
}

// NewEvidencePool creates a new evidence pool with the given capacity.
func NewEvidencePool(capacity int) *EvidencePool {
	return &EvidencePool{
		pending:  make([]types.Evidence, 0),
		seen:     make(map[types.Hash]bool),
		capacity: capacity,
	}
}

// Add inserts evidence into the pool. Returns an error if the evidence is
// a duplicate (already seen) or the pool is at capacity.
func (ep *EvidencePool) Add(e types.Evidence) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	hash, err := e.EvidenceHash(types.SHA256Hasher{})
	if err != nil {
		return fmt.Errorf("compute evidence hash: %w", err)
	}
	if ep.seen[hash] {
		return fmt.Errorf("duplicate evidence")
	}
	if len(ep.pending) >= ep.capacity {
		return fmt.Errorf("evidence pool full")
	}
	ep.seen[hash] = true
	ep.pending = append(ep.pending, e)
	return nil
}

// Has reports whether evidence with the given hash has been seen.
func (ep *EvidencePool) Has(hash types.Hash) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.seen[hash]
}

// Drain removes and returns up to max evidence items from the pool.
// The seen map is NOT cleared so duplicates remain rejected.
func (ep *EvidencePool) Drain(max int) []types.Evidence {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if max > len(ep.pending) {
		max = len(ep.pending)
	}
	result := make([]types.Evidence, max)
	copy(result, ep.pending[:max])
	ep.pending = ep.pending[max:]
	return result
}

// Len returns the number of pending evidence items.
func (ep *EvidencePool) Len() int {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return len(ep.pending)
}
