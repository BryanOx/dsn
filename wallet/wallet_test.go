package wallet

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/BryanOx/dsn/types"
)

func TestGenerateKey(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if kp == nil {
		t.Fatal("keypair is nil")
	}
	if kp.PublicKey == [32]byte{} {
		t.Error("public key should not be zero")
	}
	if kp.PrivateKey == [64]byte{} {
		t.Error("private key should not be zero")
	}
}

func TestAddress(t *testing.T) {
	kp, _ := GenerateKey()
	addr := kp.Address()
	if addr == (types.Address{}) {
		t.Error("address should not be zero")
	}
	if len(addr.Bytes()) != 20 {
		t.Errorf("address length = %d, want 20", len(addr.Bytes()))
	}
}

func TestAddressConsistency(t *testing.T) {
	kp, _ := GenerateKey()
	addr1 := kp.Address()
	addr2 := kp.Address()
	if addr1 != addr2 {
		t.Error("address should be deterministic for same key")
	}
}

func TestSignAndVerify(t *testing.T) {
	kp, _ := GenerateKey()
	hasher := types.SHA256Hasher{}

	// Create a transaction
	addr := kp.Address()
	tx := types.NewTransaction(1, 0, addr, 1, types.EncodeTransferPayload(addr, 0), nil, 100, 50000, 1234567890)
	intentID, _ := tx.ComputeIntentID(hasher)
	tx.IntentID = intentID

	// Sign
	err := kp.Sign(tx, hasher)
	if err != nil {
		t.Fatal(err)
	}

	// Verify signature
	if len(tx.Signature) == 0 {
		t.Fatal("signature is empty")
	}
	if string(tx.Signature) == string(intentID[:]) {
		t.Error("signature should not equal intentID")
	}
}

func TestSaveAndLoadKey(t *testing.T) {
	kp1, _ := GenerateKey()
	tmpFile := "test_key.json"
	defer os.Remove(tmpFile)

	err := SaveKey(tmpFile, kp1)
	if err != nil {
		t.Fatal(err)
	}

	kp2, err := LoadKey(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	if kp1.PublicKey != kp2.PublicKey {
		t.Error("public keys should match after load")
	}
	if kp1.PrivateKey != kp2.PrivateKey {
		t.Error("private keys should match after load")
	}
	if kp1.Address() != kp2.Address() {
		t.Error("addresses should match after load")
	}
}

func TestLoadKeyFileNotFound(t *testing.T) {
	_, err := LoadKey("nonexistent.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestLoadKeyFile_LegacyWarningAndUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/legacy.json"
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveKey(path, kp); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		got, err := LoadKeyFile(path, nil)
		if err != nil {
			t.Fatalf("LoadKeyFile(legacy) failed: %v", err)
		}
		if got.PrivateKey != kp.PrivateKey {
			t.Error("legacy keypair not recovered")
		}
	})
	if !strings.Contains(stderr, "legacy") {
		t.Errorf("stderr should warn about a legacy plaintext wallet, got: %q", stderr)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("legacy file should be unchanged by loading")
	}
}

func TestLoadKeyFile_KeystoreWithProvider(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wallet.json"
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveKeystore(path, "correct horse battery staple", kp); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKeyFile(path, func() (string, error) { return "correct horse battery staple", nil })
	if err != nil {
		t.Fatalf("LoadKeyFile(keystore) with provider failed: %v", err)
	}
	if got.PrivateKey != kp.PrivateKey {
		t.Error("keystore keypair not recovered via provider")
	}
}

func TestLoadKey_KeystoreNilProvider(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/wallet.json"
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveKeystore(path, "correct horse battery staple", kp); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(path)
	if !errors.Is(err, ErrKeystorePassphraseRequired) {
		t.Fatalf("error = %v, want ErrKeystorePassphraseRequired", err)
	}
	if got != nil {
		t.Error("no keypair should be returned without a passphrase provider")
	}
}
