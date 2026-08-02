package txfile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

func TestParseFile_MissingSender(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tx-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"nonce": 1}`)
	f.Close()

	_, err = ParseFile(f.Name())
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
}

func TestWriteFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tx-001.json")

	tf := &TransactionFile{
		Version:   1,
		ChainID:   0,
		Sender:    "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr",
		Nonce:     1,
		Payload:   "0x",
		MaxFee:    10,
		GasLimit:  100000,
		Timestamp: 1700000000,
		TxType:    0,
	}

	if err := WriteFile(tf, path); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	loaded, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if loaded.Sender != tf.Sender {
		t.Errorf("sender = %q, want %q", loaded.Sender, tf.Sender)
	}
	if loaded.Nonce != tf.Nonce {
		t.Errorf("nonce = %d, want %d", loaded.Nonce, tf.Nonce)
	}
}

func TestToTransaction_Unsigned(t *testing.T) {
	tf := &TransactionFile{
		Version:   1,
		ChainID:   0,
		Sender:    "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr",
		Nonce:     1,
		Payload:   "0x",
		MaxFee:    10,
		GasLimit:  100000,
		Timestamp: 1700000000,
		TxType:    0,
	}

	hasher := types.SHA256Hasher{}
	tx, err := tf.ToTransaction(hasher)
	if err != nil {
		t.Fatalf("ToTransaction failed: %v", err)
	}

	if tx.Nonce != tf.Nonce {
		t.Errorf("nonce = %d, want %d", tx.Nonce, tf.Nonce)
	}
	if tx.IntentID == (types.Hash{}) {
		t.Error("expected non-zero IntentID")
	}
}

func TestToTransaction_TransferPayload(t *testing.T) {
	payload, err := BuildTransferPayload("16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr", 5000)
	if err != nil {
		t.Fatalf("BuildTransferPayload failed: %v", err)
	}

	tf := &TransactionFile{
		Version:   1,
		Sender:    "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr",
		Nonce:     1,
		Payload:   payload,
		MaxFee:    10,
		GasLimit:  100000,
		Timestamp: 1700000000,
	}

	hasher := types.SHA256Hasher{}
	tx, err := tf.ToTransaction(hasher)
	if err != nil {
		t.Fatalf("ToTransaction failed: %v", err)
	}

	if len(tx.Payload) != 28 {
		t.Errorf("payload length = %d, want 28", len(tx.Payload))
	}
}

func TestSignAndRoundTrip(t *testing.T) {
	kp, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	tf := &TransactionFile{
		Version:   1,
		ChainID:   0,
		Sender:    kp.Address().String(),
		Nonce:     1,
		Payload:   "0x",
		MaxFee:    10,
		GasLimit:  100000,
		Timestamp: 1700000000,
		TxType:    0,
	}

	hasher := types.SHA256Hasher{}

	tx, err := tf.ToTransaction(hasher)
	if err != nil {
		t.Fatalf("ToTransaction failed: %v", err)
	}

	if err := kp.Sign(tx, hasher); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	signed := FromTransaction(tx)
	if !signed.IsSigned() {
		t.Fatal("expected signed transaction")
	}
	if signed.Signature == "" {
		t.Fatal("expected non-empty signature")
	}
	if signed.IntentID == "" {
		t.Fatal("expected non-empty intentId")
	}

	tx2, err := signed.ToTransaction(hasher)
	if err != nil {
		t.Fatalf("ToTransaction (signed) failed: %v", err)
	}
	if tx2.IntentID != tx.IntentID {
		t.Error("IntentID mismatch after round-trip")
	}
}

func TestAutoNumber_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	path, num, err := AutoNumber(dir)
	if err != nil {
		t.Fatalf("AutoNumber failed: %v", err)
	}
	if num != 1 {
		t.Errorf("num = %d, want 1", num)
	}
	if filepath.Base(path) != "tx-001.json" {
		t.Errorf("path = %s, want tx-001.json", filepath.Base(path))
	}
}

func TestAutoNumber_Sequential(t *testing.T) {
	dir := t.TempDir()

	for i := 1; i <= 2; i++ {
		tf := &TransactionFile{
			Sender:  "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr",
			Nonce:   uint64(i),
			Payload: "0x",
		}
		name := filepath.Join(dir, fmt.Sprintf("tx-%03d.json", i))
		WriteFile(tf, name)
	}

	path, num, err := AutoNumber(dir)
	if err != nil {
		t.Fatalf("AutoNumber failed: %v", err)
	}
	if num != 3 {
		t.Errorf("num = %d, want 3", num)
	}
	if filepath.Base(path) != "tx-003.json" {
		t.Errorf("path = %s, want tx-003.json", filepath.Base(path))
	}
}

func TestAutoNumber_SkipsSigned(t *testing.T) {
	dir := t.TempDir()

	tf1 := &TransactionFile{Sender: "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr", Nonce: 1}
	WriteFile(tf1, filepath.Join(dir, "tx-001.json"))

	tf2 := &TransactionFile{Sender: "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr", Nonce: 1, Signature: "0xabcd"}
	WriteFile(tf2, filepath.Join(dir, "tx-001-signed.json"))

	path, num, err := AutoNumber(dir)
	if err != nil {
		t.Fatalf("AutoNumber failed: %v", err)
	}
	if num != 2 {
		t.Errorf("num = %d, want 2 (signed files skipped)", num)
	}
	if filepath.Base(path) != "tx-002.json" {
		t.Errorf("path = %s, want tx-002.json", filepath.Base(path))
	}
}

func TestIsSigned(t *testing.T) {
	unsigned := &TransactionFile{Sender: "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr", Nonce: 1}
	if unsigned.IsSigned() {
		t.Error("expected unsigned file to report not signed")
	}

	signed := &TransactionFile{Sender: "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr", Nonce: 1, Signature: "0xabcd"}
	if !signed.IsSigned() {
		t.Error("expected signed file to report signed")
	}
}
