// Package genesis provides canonical genesis file loading, validation, and state initialization.
package genesis

import (
	"crypto/sha256"
	"encoding/json"
	"os"

	"github.com/dsn/dsn/types"
)

// LoadGenesis reads and parses a genesis file from the given path.
func LoadGenesis(path string) (*GenesisDoc, error) {
	if path == "" {
		return nil, ErrGenesisPathRequired
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrGenesisFileNotFound
		}
		return nil, ErrGenesisReadFailed.Wrap(err)
	}

	doc := &GenesisDoc{}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, ErrGenesisParseFailed.Wrap(err)
	}

	// Validate required top-level fields
	if doc.ChainID == "" {
		return nil, ErrGenesisMissingChainID
	}
	if doc.GenesisTime.IsZero() {
		return nil, ErrGenesisMissingGenesisTime
	}

	return doc, nil
}

// HashGenesis computes a deterministic SHA-256 hash of the genesis document.
func HashGenesis(doc *GenesisDoc) (types.Hash, error) {
	data, err := CanonicalJSON(doc)
	if err != nil {
		return types.Hash{}, ErrGenesisHashFailed.Wrap(err)
	}

	hash := sha256.Sum256(data)
	var h types.Hash
	copy(h[:], hash[:])
	return h, nil
}