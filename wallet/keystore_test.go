package wallet

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestKeyPair(t *testing.T) *KeyPair {
	t.Helper()
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return kp
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKey("correct horse battery staple", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateKey != kp.PrivateKey {
		t.Error("private key not byte-identical after roundtrip")
	}
	if got.PublicKey != kp.PublicKey {
		t.Error("public key not byte-identical after roundtrip")
	}
	if got.Address() != kp.Address() {
		t.Error("address not identical after roundtrip")
	}
}

func TestEncryptKey_RandomizedCiphertext(t *testing.T) {
	kp := newTestKeyPair(t)
	first, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}

	var ks1, ks2 keystoreJSON
	if err := json.Unmarshal(first, &ks1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &ks2); err != nil {
		t.Fatal(err)
	}
	if ks1.Salt == ks2.Salt {
		t.Error("salt should differ across encryptions")
	}
	if ks1.IV == ks2.IV {
		t.Error("iv should differ across encryptions")
	}
	if ks1.Ciphertext == ks2.Ciphertext {
		t.Error("ciphertext should differ across encryptions")
	}
}

func TestDecryptKey_WrongPassphrase(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKey("wrong passphrase", data)
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("error = %v, want ErrWrongPassphrase (checksum, no GCM open)", err)
	}
	if got != nil {
		t.Error("no keypair should be released on wrong passphrase")
	}
}

func TestDecryptKey_TruncatedCiphertext(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	var ks keystoreJSON
	if err := json.Unmarshal(data, &ks); err != nil {
		t.Fatal(err)
	}
	ks.Ciphertext = ks.Ciphertext[:len(ks.Ciphertext)-8]
	truncated, err := json.Marshal(ks)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKey("correct horse battery staple", truncated)
	if !errors.Is(err, ErrCorruptKeystore) {
		t.Fatalf("error = %v, want ErrCorruptKeystore (GCM auth)", err)
	}
	if got != nil {
		t.Error("no keypair should be released from truncated ciphertext")
	}
}

func TestDecryptKey_MalformedJSON(t *testing.T) {
	if _, err := DecryptKey("x", []byte(`{"version": 1, broken`)); !errors.Is(err, ErrCorruptKeystore) {
		t.Fatalf("error = %v, want ErrCorruptKeystore wrapping a parse error", err)
	}
}

func TestKeystoreSchema_WellFormed(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	var ks keystoreJSON
	if err := json.Unmarshal(data, &ks); err != nil {
		t.Fatal(err)
	}
	if ks.Version != 1 {
		t.Errorf("version = %d, want 1", ks.Version)
	}
	if ks.Cipher != "aes-256-gcm" {
		t.Errorf("cipher = %q, want aes-256-gcm", ks.Cipher)
	}
	if ks.KDF != "scrypt" {
		t.Errorf("kdf = %q, want scrypt", ks.KDF)
	}
	if ks.KDFParams.N != 1<<17 || ks.KDFParams.R != 8 || ks.KDFParams.P != 1 || ks.KDFParams.DKLen != 32 {
		t.Errorf("kdfparams = %+v, want {N:131072 r:8 p:1 dkLen:32}", ks.KDFParams)
	}
	if salt, _ := hex.DecodeString(ks.Salt); len(salt) != 16 {
		t.Errorf("salt length = %d, want 16", len(salt))
	}
	if iv, _ := hex.DecodeString(ks.IV); len(iv) != 12 {
		t.Errorf("iv length = %d, want 12", len(iv))
	}
	if ct, _ := hex.DecodeString(ks.Ciphertext); len(ct) != 80 {
		t.Errorf("ciphertext length = %d, want 80 (64 plaintext + 16 GCM tag)", len(ct))
	}
	if sum, _ := hex.DecodeString(ks.Checksum); len(sum) != 4 {
		t.Errorf("checksum length = %d, want 4", len(sum))
	}
}

func TestDecryptKey_UnsupportedVersion(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	var ks keystoreJSON
	if err := json.Unmarshal(data, &ks); err != nil {
		t.Fatal(err)
	}
	ks.Version = 2
	bumped, err := json.Marshal(ks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptKey("correct horse battery staple", bumped); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestDecryptKey_NonDefaultParams(t *testing.T) {
	kp := newTestKeyPair(t)
	data, err := encryptKeyWithParams("correct horse battery staple", kp, kdfParams{N: 1 << 15, R: 8, P: 1, DKLen: 32})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKey("correct horse battery staple", data)
	if err != nil {
		t.Fatalf("params from file should be honored: %v", err)
	}
	if got.PrivateKey != kp.PrivateKey {
		t.Error("private key not recovered with non-default params")
	}
}

func TestDecryptKey_AbsurdParamsRejected(t *testing.T) {
	kp := newTestKeyPair(t)
	base, err := EncryptKey("correct horse battery staple", kp)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		n    int
	}{
		{"n_2^30", 1 << 30},
		{"n_non_pow2", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ks keystoreJSON
			if err := json.Unmarshal(base, &ks); err != nil {
				t.Fatal(err)
			}
			ks.KDFParams.N = tc.n
			bumped, err := json.Marshal(ks)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecryptKey("correct horse battery staple", bumped); !errors.Is(err, ErrKDFParams) {
				t.Fatalf("error = %v, want ErrKDFParams before any derivation", err)
			}
		})
	}
}

func TestSaveKeystore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.json")
	kp := newTestKeyPair(t)
	if err := SaveKeystore(path, "correct horse battery staple", kp); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("target missing after save: %v", err)
	}
	if _, err := DecryptKey("correct horse battery staple", data); err != nil {
		t.Fatalf("target is not a complete valid keystore: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".keystore-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("temp files remain after save: %v", matches)
	}
}

func TestSaveKeystore_Permissions(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "wallets", "wallet.json")
	kp := newTestKeyPair(t)
	if err := SaveKeystore(path, "correct horse battery staple", kp); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file mode = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm > 0700 {
		t.Errorf("parent dir mode = %o, want at most 0700", perm)
	}
}

func TestMigrate_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	kp := newTestKeyPair(t)
	if err := SaveKey(path, kp); err != nil {
		t.Fatal(err)
	}
	plain, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := Migrate(path, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	enc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKey("correct horse battery staple", enc)
	if err != nil {
		t.Fatalf("target is not a valid keystore: %v", err)
	}
	if got.PrivateKey != kp.PrivateKey {
		t.Error("migrated keystore holds the wrong key")
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf(".bak missing: %v", err)
	}
	if string(backup) != string(plain) {
		t.Error(".bak does not hold the original plaintext")
	}
}

func TestMigrate_InterruptedAfterBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	kp := newTestKeyPair(t)
	if err := SaveKey(path, kp); err != nil {
		t.Fatal(err)
	}
	plain, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	old := migrateInterrupt
	migrateInterrupt = func() error { return errors.New("simulated crash between renames") }
	defer func() { migrateInterrupt = old }()

	err = Migrate(path, "correct horse battery staple")
	if err == nil {
		t.Fatal("expected error when interrupted between renames")
	}
	if !strings.Contains(err.Error(), ".bak") {
		t.Errorf("error should name the .bak path so the original stays recoverable, got: %v", err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf(".bak missing after interruption: %v", err)
	}
	if string(backup) != string(plain) {
		t.Error(".bak does not hold the original plaintext")
	}
}

func TestMigrate_BackupExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	kp := newTestKeyPair(t)
	if err := SaveKey(path, kp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, "correct horse battery staple"); !errors.Is(err, ErrBackupExists) {
		t.Fatalf("error = %v, want ErrBackupExists", err)
	}
}

func TestMigrate_AlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.json")
	kp := newTestKeyPair(t)
	if err := SaveKeystore(path, "correct horse battery staple", kp); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(path, "correct horse battery staple"); !errors.Is(err, ErrAlreadyEncrypted) {
		t.Fatalf("error = %v, want ErrAlreadyEncrypted", err)
	}
}
