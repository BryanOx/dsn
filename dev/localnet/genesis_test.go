package localnet

import (
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/dsn/dsn/genesis"
)

// TestGenerateLocalnet_Validates tests that GenerateLocalnet produces a genesis
// document that loads and passes genesis validation, with hex-encoded
// validator addresses and public keys.
func TestGenerateLocalnet_Validates(t *testing.T) {
	outDir := t.TempDir()

	nodes, err := GenerateLocalnet(Config{
		NumValidators: 3,
		OutputDir:     outDir,
		ChainID:       "dsn-localnet-1",
	})
	if err != nil {
		t.Fatalf("GenerateLocalnet() error = %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("GenerateLocalnet() returned %d nodes, want 3", len(nodes))
	}

	doc, err := genesis.LoadGenesis(filepath.Join(outDir, "genesis.json"))
	if err != nil {
		t.Fatalf("LoadGenesis() error = %v", err)
	}

	if err := genesis.ValidateGenesis(doc); err != nil {
		t.Fatalf("ValidateGenesis() error = %v", err)
	}

	if doc.GenesisVersion != 1 {
		t.Errorf("GenesisVersion = %d, want 1", doc.GenesisVersion)
	}

	if len(doc.InitialValidators) != 3 {
		t.Fatalf("initial validators = %d, want 3", len(doc.InitialValidators))
	}

	for i, v := range doc.InitialValidators {
		if len(v.Address) != 40 {
			t.Errorf("validator %d: address length = %d, want 40 hex chars", i, len(v.Address))
		}
		if _, err := hex.DecodeString(v.Address); err != nil {
			t.Errorf("validator %d: address is not valid hex: %v", i, err)
		}
		if len(v.PubKey) != 64 {
			t.Errorf("validator %d: pubkey length = %d, want 64 hex chars", i, len(v.PubKey))
		}
		if _, err := hex.DecodeString(v.PubKey); err != nil {
			t.Errorf("validator %d: pubkey is not valid hex: %v", i, err)
		}
	}
}
