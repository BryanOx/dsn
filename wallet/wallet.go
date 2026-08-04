package wallet

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/BryanOx/dsn/types"
)

// KeyPair holds an Ed25519 key pair for signing transactions.
type KeyPair struct {
	PublicKey  [32]byte `json:"public_key"`
	PrivateKey [64]byte `json:"private_key"`
}

type keyFile struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// GenerateKey creates a new Ed25519 key pair.
func GenerateKey() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	kp := &KeyPair{}
	copy(kp.PublicKey[:], pub)
	copy(kp.PrivateKey[:], priv)
	return kp, nil
}

// Address derives the DSN address from the public key (first 20 bytes).
func (kp *KeyPair) Address() types.Address {
	addr, _ := types.AddressFromBytes(kp.PublicKey[:20])
	return addr
}

// Sign computes the IntentID and signs the transaction with the private key.
func (kp *KeyPair) Sign(tx *types.Transaction, hasher types.Hasher) error {
	intentID, err := tx.ComputeIntentID(hasher)
	if err != nil {
		return err
	}
	tx.IntentID = intentID
	tx.Signature = ed25519.Sign(kp.PrivateKey[:], intentID[:])
	return nil
}

// SignHash signs an arbitrary hash with the private key (for block signing).
func (kp *KeyPair) SignHash(hash types.Hash) ([]byte, error) {
	return ed25519.Sign(kp.PrivateKey[:], hash[:]), nil
}

// SaveKey persists a key pair to a JSON file, creating the parent directory if needed.
func SaveKey(path string, kp *KeyPair) error {
	kf := keyFile{
		PublicKey:  hex.EncodeToString(kp.PublicKey[:]),
		PrivateKey: hex.EncodeToString(kp.PrivateKey[:]),
	}
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0600)
}

// LoadKey reads a key pair from a JSON file. Encrypted keystores require a
// passphrase provider, which LoadKey does not have, so loading one returns
// ErrKeystorePassphraseRequired; legacy plaintext files load directly.
func LoadKey(path string) (*KeyPair, error) {
	return LoadKeyFile(path, nil)
}

// loadLegacyKey parses a legacy plaintext key file.
func loadLegacyKey(data []byte) (*KeyPair, error) {
	var kf keyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, err
	}

	pubKey, err := hex.DecodeString(kf.PublicKey)
	if err != nil {
		return nil, err
	}
	privKey, err := hex.DecodeString(kf.PrivateKey)
	if err != nil {
		return nil, err
	}

	kp := &KeyPair{}
	copy(kp.PublicKey[:], pubKey)
	copy(kp.PrivateKey[:], privKey)
	return kp, nil
}
