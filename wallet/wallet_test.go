package wallet

import (
	"os"
	"testing"

	"github.com/dsn/dsn/types"
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
