package state

import (
	"github.com/BryanOx/dsn/types"
)

const smtDepth = 256

// siblingsMap is a flat map storing non-zero sibling hashes.
// Key: path with bits cleared from depth onwards (the sibling's position)
// Value: sibling hash at that position
type siblingsMap map[[32]byte]types.Hash

// SMT implements a Sparse Merkle Tree with 256-bit path.
type SMT struct {
	root     types.Hash
	hasher   types.Hasher
	siblings siblingsMap         // flat map: path → sibling hash (only non-zero)
	values   map[[32]byte][]byte // pathPrefix → raw value
}

func NewSMT(hasher types.Hasher) *SMT {
	return &SMT{
		root:     types.Hash{},
		hasher:   hasher,
		siblings: make(siblingsMap),
		values:   make(map[[32]byte][]byte),
	}
}

// pathForKey derives the 256-bit path from a key
func (s *SMT) pathForKey(key []byte) ([32]byte, error) {
	h, err := s.hasher.Hash(key)
	if err != nil {
		return [32]byte{}, err
	}
	return h, nil
}

// leafHash computes Hash(0x00 || path || valueHash)
func (s *SMT) leafHash(path [32]byte, value []byte) (types.Hash, error) {
	valueHash, err := s.hasher.Hash(value)
	if err != nil {
		return types.Hash{}, err
	}
	data := append([]byte{0x00}, path[:]...)
	data = append(data, valueHash[:]...)
	return s.hasher.Hash(data)
}

// internalHash computes Hash(0x01 || left || right)
func (s *SMT) internalHash(left, right types.Hash) (types.Hash, error) {
	data := append([]byte{0x01}, left[:]...)
	data = append(data, right[:]...)
	return s.hasher.Hash(data)
}

// getNode gets the sibling hash for a path at a given depth.
// The key is the original path (with lower bits cleared to the depth).
func (s *SMT) getNode(path [32]byte, depth int) types.Hash {
	// Create a prefix by clearing bits from depth onwards
	var prefix [32]byte
	copy(prefix[:], path[:])
	clearBitsFrom(&prefix, depth)
	return s.siblings[prefix]
}

// setNode stores a sibling hash for a path at a given depth.
// The key is the original path with lower bits cleared.
func (s *SMT) setNode(path [32]byte, depth int, hash types.Hash) {
	// Create a prefix by clearing bits from depth onwards
	var prefix [32]byte
	copy(prefix[:], path[:])
	clearBitsFrom(&prefix, depth)
	if hash != (types.Hash{}) {
		s.siblings[prefix] = hash
	} else {
		// Remove zero-value entries
		delete(s.siblings, prefix)
	}
}

// bitAt returns the bit at position depth (0-indexed from MSB)
func bitAt(path [32]byte, depth int) byte {
	return (path[depth/8] >> (7 - uint(depth%8))) & 1
}

// clearBitsFrom clears bits from depth onwards
func clearBitsFrom(path *[32]byte, depth int) {
	for d := depth; d < smtDepth; d++ {
		path[d/8] &^= 1 << (7 - uint(d%8))
	}
}

// Insert adds or updates a key-value pair
func (s *SMT) Insert(key []byte, value []byte) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}

	s.values[path] = value

	// Compute and store leaf node
	lh, err := s.leafHash(path, value)
	if err != nil {
		return err
	}
	s.setNode(path, smtDepth, lh)

	// Recompute up the tree
	s.recomputeUp(path)
	return nil
}

// Get retrieves a value by key
func (s *SMT) Get(key []byte) ([]byte, bool) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, false
	}
	val, ok := s.values[path]
	return val, ok
}

// Delete removes a key
func (s *SMT) Delete(key []byte) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}

	delete(s.values, path)

	// Remove the leaf node (stored at depth smtDepth)
	var leafPrefix [32]byte
	copy(leafPrefix[:], path[:])
	// Clear nothing - we're deleting the exact leaf entry
	delete(s.siblings, path)

	// Recompute up with default empty hash at leaf
	s.recomputeUp(path)
	return nil
}

// Root returns the current Merkle root
func (s *SMT) Root() types.Hash {
	return s.root
}

// recomputeUp recalculates hashes from leaf to root along a path
func (s *SMT) recomputeUp(path [32]byte) {
	currentHash := s.getNode(path, smtDepth)

	for depth := smtDepth - 1; depth >= 0; depth-- {
		bit := bitAt(path, depth)

		var left, right types.Hash
		if bit == 0 {
			left = currentHash
			right = s.siblingAt(path, depth)
		} else {
			left = s.siblingAt(path, depth)
			right = currentHash
		}

		ih, _ := s.internalHash(left, right)

		var prefix [32]byte = path
		clearBitsFrom(&prefix, depth)
		s.setNode(prefix, depth, ih)

		currentHash = ih
	}

	s.root = currentHash
}

// siblingAt gets the hash of the sibling at a given depth.
// It flips the bit at 'depth' to get the sibling's path, then getNode
// handles clearing lower bits to find the stored sibling position.
func (s *SMT) siblingAt(path [32]byte, depth int) types.Hash {
	var sibling [32]byte = path
	sibling[depth/8] ^= 1 << (7 - uint(depth%8))
	// Don't clear bits here - getNode will clear from depth onwards
	return s.getNode(sibling, depth)
}
