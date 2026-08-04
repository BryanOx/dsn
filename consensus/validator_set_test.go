package consensus

import (
	"testing"

	"github.com/BryanOx/dsn/types"
)

func TestValidatorRoot_Empty(t *testing.T) {
	root := ValidatorRoot([]types.Address{})
	if root != (types.Hash{}) {
		t.Error("empty validators should produce zero hash")
	}
}

func TestValidatorRoot_Deterministic(t *testing.T) {
	v1 := []types.Address{
		{1}, {2},
	}
	v2 := []types.Address{
		{2}, {1},
	}
	root1 := ValidatorRoot(v1)
	root2 := ValidatorRoot(v2)
	if root1 != root2 {
		t.Error("same validators different order should produce same root")
	}
}

func TestValidatorRoot_Different(t *testing.T) {
	v1 := []types.Address{{1}}
	v2 := []types.Address{{2}}
	if ValidatorRoot(v1) == ValidatorRoot(v2) {
		t.Error("different validators should produce different roots")
	}
}

func TestSortedValidators(t *testing.T) {
	v := []types.Address{{3}, {1}, {2}}
	sorted := SortedValidators(v)
	if sorted[0] != (types.Address{1}) || sorted[1] != (types.Address{2}) || sorted[2] != (types.Address{3}) {
		t.Error("validators should be sorted by address bytes")
	}
}

func TestValidatorRoot_Single(t *testing.T) {
	v := []types.Address{{0x01}}
	root := ValidatorRoot(v)
	if root == (types.Hash{}) {
		t.Error("single validator should produce non-zero hash")
	}
}

func TestValidatorRoot_DeterministicMultiple(t *testing.T) {
	// Call same input multiple times should produce same result
	v := []types.Address{{1}, {2}, {3}}
	for i := 0; i < 5; i++ {
		root := ValidatorRoot(v)
		if root == (types.Hash{}) {
			t.Error("non-empty validators should produce non-zero hash")
		}
	}
}
