package vm

import (
	"crypto/sha256"
	"testing"

	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestDeriveContractID_Deterministic(t *testing.T) {
	deployer := types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	code := []byte{0x00, 0x61, 0x73, 0x6d}
	codeHash := types.Hash(sha256.Sum256(code))

	id1 := DeriveContractID(deployer, 1, codeHash)
	id2 := DeriveContractID(deployer, 1, codeHash)
	require.Equal(t, id1, id2)
}

func TestDeriveContractID_DifferentInputs(t *testing.T) {
	deployer := types.Address{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	code := []byte{0x00, 0x61, 0x73, 0x6d}
	codeHash := types.Hash(sha256.Sum256(code))

	id1 := DeriveContractID(deployer, 1, codeHash)
	id2 := DeriveContractID(deployer, 2, codeHash)
	require.NotEqual(t, id1, id2)
}
