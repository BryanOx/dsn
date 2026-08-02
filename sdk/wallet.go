package sdk

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

// GenerateKey creates a new Ed25519 keypair.
func GenerateKey() (*wallet.KeyPair, error) {
	return wallet.GenerateKey()
}

// LoadKey loads a keypair from a JSON file.
func LoadKey(path string) (*wallet.KeyPair, error) {
	return wallet.LoadKey(path)
}

// SaveKey saves a keypair to a JSON file.
func SaveKey(path string, kp *wallet.KeyPair) error {
	return wallet.SaveKey(path, kp)
}

// NewKeyFromSeed creates a keypair from a 32-byte seed.
func NewKeyFromSeed(seed []byte) (*wallet.KeyPair, error) {
	if len(seed) != 32 {
		return nil, ErrInvalidParams
	}

	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	kp := &wallet.KeyPair{}
	copy(kp.PublicKey[:], pub)
	copy(kp.PrivateKey[:], priv)

	return kp, nil
}

// NewKeyFromMnemonic creates a keypair from a mnemonic phrase.
// Note: This is a simple implementation. For production, use BIP-39.
func NewKeyFromMnemonic(mnemonic string) (*wallet.KeyPair, error) {
	seed := deriveSeedFromMnemonic(mnemonic)
	return NewKeyFromSeed(seed)
}

// deriveSeedFromMnemonic converts mnemonic to a 32-byte seed.
func deriveSeedFromMnemonic(mnemonic string) []byte {
	hash := sha256.Sum256([]byte(mnemonic))
	return hash[:]
}

// KeyToHex exports private key as hex string.
func KeyToHex(kp *wallet.KeyPair) string {
	return hex.EncodeToString(kp.PrivateKey[:])
}

// KeyFromHex imports private key from hex string.
func KeyFromHex(hexStr string) (*wallet.KeyPair, error) {
	privBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}

	if len(privBytes) != 64 {
		return nil, ErrInvalidParams
	}

	kp := &wallet.KeyPair{}
	copy(kp.PrivateKey[:], privBytes)

	// Derive public key from private key
	pub := ed25519.PrivateKey(kp.PrivateKey[:]).Public().(ed25519.PublicKey)
	copy(kp.PublicKey[:], pub)

	return kp, nil
}

// PublicKeyHex exports public key as hex string.
func PublicKeyHex(kp *wallet.KeyPair) string {
	return hex.EncodeToString(kp.PublicKey[:])
}

// Address returns the DSN address from a keypair.
func Address(kp *wallet.KeyPair) string {
	return kp.Address().String()
}

// ValidateAddress checks if a string is a valid DSN address.
func ValidateAddress(addr string) bool {
	_, err := types.ParseAddress(addr)
	return err == nil
}

// MustGetAddress returns the address from a keypair, panics on error.
func MustGetAddress(kp *wallet.KeyPair) string {
	return kp.Address().String()
}
