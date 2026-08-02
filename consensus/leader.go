package consensus

import "github.com/dsn/dsn/types"

// ProposerAtHeight returns the validator address that should propose a block at the given height.
// Panics if validators is empty (programming error — should never happen at runtime).
func ProposerAtHeight(height uint64, validators []types.Address) types.Address {
	if len(validators) == 0 {
		panic("consensus: empty validator set")
	}
	return validators[height%uint64(len(validators))]
}
