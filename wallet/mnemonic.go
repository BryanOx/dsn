package wallet

import (
	"crypto/ed25519"

	"github.com/BryanOx/dsn/internal/bip39"
)

// KeyFromMnemonic derives an Ed25519 keypair from a valid BIP-39 English
// mnemonic. The phrase is validated before derivation; invalid phrases return
// an error without producing a key. Derivation never falls back to another
// algorithm.
func KeyFromMnemonic(phrase string) (*KeyPair, error) {
	if err := bip39.ValidateMnemonic(phrase); err != nil {
		return nil, err
	}
	seed, err := bip39.MnemonicToSeed(phrase)
	if err != nil {
		return nil, err
	}
	priv := ed25519.NewKeyFromSeed(seed[:32])
	kp := &KeyPair{}
	copy(kp.PublicKey[:], priv.Public().(ed25519.PublicKey))
	copy(kp.PrivateKey[:], priv)
	return kp, nil
}
