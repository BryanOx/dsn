package consensus

import (
	"bytes"
	"sort"

	"github.com/BryanOx/dsn/types"
)

// SortedValidators returns a copy of validators sorted by ConsensusID.
func SortedValidators(validators []types.Address) []types.Address {
	sorted := make([]types.Address, len(validators))
	copy(sorted, validators)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i][:], sorted[j][:]) < 0
	})
	return sorted
}

// ValidatorRoot returns the SHA256 hash of the sorted validator set.
func ValidatorRoot(validators []types.Address) types.Hash {
	if len(validators) == 0 {
		return types.Hash{}
	}
	hasher := types.SHA256Hasher{}
	sorted := SortedValidators(validators)
	var buf bytes.Buffer
	for _, v := range sorted {
		buf.Write(v[:])
	}
	hash, _ := hasher.Hash(buf.Bytes()) // SHA256 never fails
	return hash
}
