package wallet

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/BryanOx/dsn/internal/bip39"
)

const testPhrase0000 = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func TestKeyFromMnemonic_OfficialVector(t *testing.T) {
	seed, err := bip39.MnemonicToSeed(testPhrase0000)
	if err != nil {
		t.Fatal(err)
	}
	wantPriv := ed25519.NewKeyFromSeed(seed[:32])
	wantPub := wantPriv.Public().(ed25519.PublicKey)

	kp, err := KeyFromMnemonic(testPhrase0000)
	if err != nil {
		t.Fatal(err)
	}
	if kp.PrivateKey != [64]byte(wantPriv) {
		t.Errorf("private key = %x, want %x", kp.PrivateKey, wantPriv)
	}
	if kp.PublicKey != [32]byte(wantPub) {
		t.Errorf("public key = %x, want %x", kp.PublicKey, wantPub)
	}
}

func TestKeyFromMnemonic_InvalidChecksumRejected(t *testing.T) {
	phrase := strings.Repeat("abandon ", 11) + "abandon"
	kp, err := KeyFromMnemonic(phrase)
	if err == nil {
		t.Fatal("expected error for invalid checksum phrase")
	}
	if kp != nil {
		t.Error("no keypair should be produced for an invalid phrase")
	}
}

func TestKeyFromMnemonic_RestoreReproducesAddress(t *testing.T) {
	first, err := KeyFromMnemonic(testPhrase0000)
	if err != nil {
		t.Fatal(err)
	}
	second, err := KeyFromMnemonic(testPhrase0000)
	if err != nil {
		t.Fatal(err)
	}
	if first.Address() != second.Address() {
		t.Errorf("addresses differ across restores: %s vs %s", first.Address(), second.Address())
	}
	if first.PrivateKey != second.PrivateKey {
		t.Error("private keys differ across restores")
	}
}
