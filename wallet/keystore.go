package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// Typed errors returned by the keystore package.
var (
	ErrKeystorePassphraseRequired = errors.New("wallet: encrypted keystore requires a passphrase provider")
	ErrWrongPassphrase            = errors.New("wallet: wrong passphrase")
	ErrCorruptKeystore            = errors.New("wallet: keystore is corrupt")
	ErrUnsupportedVersion         = errors.New("wallet: unsupported keystore version")
	ErrUnsupportedCipher          = errors.New("wallet: unsupported cipher")
	ErrUnsupportedKDF             = errors.New("wallet: unsupported KDF")
	ErrKDFParams                  = errors.New("wallet: invalid KDF parameters")
	ErrAlreadyEncrypted           = errors.New("wallet: keystore is already encrypted")
	ErrBackupExists               = errors.New("wallet: backup file already exists")
)

// PassphraseFunc supplies the keystore passphrase lazily, invoked only when
// the loaded file is detected to be encrypted.
type PassphraseFunc func() (string, error)

// kdfParams holds the scrypt parameters stored in the keystore.
type kdfParams struct {
	N     int `json:"n"`
	R     int `json:"r"`
	P     int `json:"p"`
	DKLen int `json:"dkLen"`
}

// keystoreJSON is the versioned Ethereum-style keystore schema.
type keystoreJSON struct {
	Version    int       `json:"version"`
	Cipher     string    `json:"cipher"`
	KDF        string    `json:"kdf"`
	KDFParams  kdfParams `json:"kdfparams"`
	Salt       string    `json:"salt"`
	IV         string    `json:"iv"`
	Ciphertext string    `json:"ciphertext"`
	Checksum   string    `json:"checksum"`
}

const (
	defaultN      = 1 << 17
	defaultR      = 8
	defaultP      = 1
	defaultDKLen  = 32
	saltBytes     = 16
	nonceBytes    = 12
	keyBytes      = 64 // Ed25519 private key
	gcmTagBytes   = 16
	checksumBytes = 4
	maxMemBytes   = int64(1) << 30 // 1 GiB KDF memory cap
)

// migrateInterrupt is a test seam for simulating a failure between the backup
// rename and the target rename (see Migrate). It is nil in production.
var migrateInterrupt func() error

// EncryptKey seals a keypair into a versioned JSON keystore using scrypt
// (N=2^17, r=8, p=1) and AES-256-GCM. The checksum is the first 4 bytes of
// SHA-256 of the derived key.
func EncryptKey(passphrase string, kp *KeyPair) ([]byte, error) {
	return encryptKeyWithParams(passphrase, kp, kdfParams{N: defaultN, R: defaultR, P: defaultP, DKLen: defaultDKLen})
}

// encryptKeyWithParams is EncryptKey with explicit scrypt parameters, used by
// tests to exercise non-default KDF params.
func encryptKeyWithParams(passphrase string, kp *KeyPair, params kdfParams) ([]byte, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	dk, err := scrypt.Key([]byte(passphrase), salt, params.N, params.R, params.P, params.DKLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(dk)
	ks := keystoreJSON{
		Version:    1,
		Cipher:     "aes-256-gcm",
		KDF:        "scrypt",
		KDFParams:  params,
		Salt:       hex.EncodeToString(salt),
		IV:         hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(gcm.Seal(nil, nonce, kp.PrivateKey[:], nil)),
		Checksum:   hex.EncodeToString(sum[:checksumBytes]),
	}
	return json.MarshalIndent(ks, "", "  ")
}

// DecryptKey recovers a keypair from an encrypted keystore. It rejects absurd
// KDF parameters before any derivation, verifies the checksum before opening
// the GCM box, and returns ErrWrongPassphrase on a checksum mismatch.
func DecryptKey(passphrase string, data []byte) (*KeyPair, error) {
	var ks keystoreJSON
	if err := json.Unmarshal(data, &ks); err != nil {
		return nil, fmt.Errorf("%w: invalid keystore JSON: %v", ErrCorruptKeystore, err)
	}
	if ks.Version != 1 {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, ks.Version)
	}
	if ks.Cipher != "aes-256-gcm" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedCipher, ks.Cipher)
	}
	if ks.KDF != "scrypt" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedKDF, ks.KDF)
	}
	if err := validateKDFParams(ks.KDFParams); err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(ks.Salt)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid salt: %v", ErrCorruptKeystore, err)
	}
	nonce, err := hex.DecodeString(ks.IV)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid iv: %v", ErrCorruptKeystore, err)
	}
	if len(nonce) != nonceBytes {
		return nil, fmt.Errorf("%w: iv must be %d bytes, got %d", ErrCorruptKeystore, nonceBytes, len(nonce))
	}
	ct, err := hex.DecodeString(ks.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid ciphertext: %v", ErrCorruptKeystore, err)
	}
	checksum, err := hex.DecodeString(ks.Checksum)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid checksum: %v", ErrCorruptKeystore, err)
	}

	dk, err := scrypt.Key([]byte(passphrase), salt, ks.KDFParams.N, ks.KDFParams.R, ks.KDFParams.P, ks.KDFParams.DKLen)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(dk)
	if !bytes.Equal(sum[:checksumBytes], checksum) {
		return nil, ErrWrongPassphrase
	}

	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptKeystore, err)
	}
	if len(pt) != keyBytes {
		return nil, fmt.Errorf("%w: plaintext must be %d bytes, got %d", ErrCorruptKeystore, keyBytes, len(pt))
	}
	kp := &KeyPair{}
	copy(kp.PrivateKey[:], pt)
	pub := ed25519.PrivateKey(pt).Public().(ed25519.PublicKey)
	copy(kp.PublicKey[:], pub)
	return kp, nil
}

// validateKDFParams rejects non-power-of-two N and memory demands above 1 GiB
// before any KDF work happens.
func validateKDFParams(p kdfParams) error {
	if p.N <= 0 || p.N&(p.N-1) != 0 {
		return fmt.Errorf("%w: n must be a positive power of two, got %d", ErrKDFParams, p.N)
	}
	if p.R <= 0 || p.P <= 0 || p.DKLen <= 0 {
		return fmt.Errorf("%w: r, p and dkLen must be positive", ErrKDFParams)
	}
	if mem := int64(p.N) * int64(p.R) * 128; mem > maxMemBytes {
		return fmt.Errorf("%w: memory demand %d bytes exceeds 1 GiB", ErrKDFParams, mem)
	}
	return nil
}

// SaveKeystore encrypts the keypair and writes it atomically with mode 0600.
func SaveKeystore(path, passphrase string, kp *KeyPair) error {
	data, err := EncryptKey(passphrase, kp)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return writeAtomic(path, data, 0600)
}

// LoadKeyFile loads a wallet file in either format: encrypted keystores are
// sniffed by their "version" field and decrypted via the passphrase provider
// (nil provider yields ErrKeystorePassphraseRequired); legacy plaintext files
// load directly with a stderr warning and are never modified.
func LoadKeyFile(path string, getPassphrase PassphraseFunc) (*KeyPair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isKeystore(data) {
		if getPassphrase == nil {
			return nil, ErrKeystorePassphraseRequired
		}
		passphrase, err := getPassphrase()
		if err != nil {
			return nil, err
		}
		return DecryptKey(passphrase, data)
	}
	kp, err := loadLegacyKey(data)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "warning: %s is a legacy plaintext wallet; consider `dsn wallet migrate`\n", path)
	return kp, nil
}

// isKeystore reports whether data is encrypted keystore JSON by sniffing for
// a "version" field.
func isKeystore(data []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	_, ok := m["version"]
	return ok
}

// Migrate encrypts a legacy plaintext wallet in place. It writes the new
// keystore to a synced temp file, renames the original to <path>.bak, renames
// the temp file into place, and verifies the result decrypts before reporting
// success. Failures after the backup rename name the .bak path so the
// original stays recoverable.
func Migrate(path, passphrase string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if isKeystore(data) {
		return ErrAlreadyEncrypted
	}
	kp, err := loadLegacyKey(data)
	if err != nil {
		return err
	}
	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); err == nil {
		return ErrBackupExists
	}
	enc, err := EncryptKey(passphrase, kp)
	if err != nil {
		return err
	}
	tmp, err := createTempAndSync(filepath.Dir(path), enc, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if tmp != "" {
			os.Remove(tmp)
		}
	}()
	if err := os.Rename(path, backupPath); err != nil {
		return err
	}
	if migrateInterrupt != nil {
		if err := migrateInterrupt(); err != nil {
			return fmt.Errorf("wallet: migration failed after backup; original preserved at %s: %w", backupPath, err)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("wallet: migration failed after backup; original preserved at %s: %w", backupPath, err)
	}
	tmp = ""
	written, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("wallet: migration failed after backup; original preserved at %s: %w", backupPath, err)
	}
	if _, err := DecryptKey(passphrase, written); err != nil {
		return fmt.Errorf("wallet: migration failed after backup; original preserved at %s: %w", backupPath, err)
	}
	return nil
}

// writeAtomic writes data to path via a temp file, fsync, and rename so a
// failed write never leaves a partial file.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := createTempAndSync(dir, data, perm)
	if err != nil {
		return err
	}
	defer func() {
		if tmp != "" {
			os.Remove(tmp)
		}
	}()
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	tmp = ""
	syncDir(dir)
	return nil
}

// createTempAndSync writes data to a new temp file in dir, chmods it, fsyncs
// it, and leaves the file in place (caller renames it).
func createTempAndSync(dir string, data []byte, perm os.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, ".keystore-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// syncDir is a best-effort directory fsync after a rename.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}
