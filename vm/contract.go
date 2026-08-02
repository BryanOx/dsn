package vm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/dsn/dsn/types"
)

// DeriveContractID computes a deterministic 32-byte contract identifier.
// contractID = SHA-256(deployer || nonce || codeHash)
func DeriveContractID(deployer types.Address, nonce uint64, codeHash types.Hash) types.Hash {
	buf := new(bytes.Buffer)
	buf.Write(deployer[:])
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, nonce)
	buf.Write(nonceBytes)
	buf.Write(codeHash[:])
	hash := sha256.Sum256(buf.Bytes())
	return types.Hash(hash)
}
