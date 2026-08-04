package sdk

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// Frozen legacy SHA-256 golden pins — use verbatim, never recompute.
const (
	legacyPrivAbandon = "c557eec878dfd852ba3f88087c4f350f09c55537ab5e549c3cd14320ec3cef3893a5f261984931e0df5c7434b16d468efb1953098d3cad4fa1506b9e052e7fc7"
	legacyPrivLegal  = "220a434e7ba2cd85416a23e8fecbc0d638da46e53e6e92713cfe374eb3ade9484ea4a64a0244ac06201d26d4eec93c04ffabb3d3330381ce8f37a8122f0a7eff"
	legacyPrivDSN    = "4b171ff52d8947f838ffb71ab59100fcc24b74568f168e0cf75cec8f507713e70a298b2ef0b103d4b303321dbc0045d014309c8f5ab22b830afedbee91f6bf3c"

	bip39PrivAbandon = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e534955351425909c1e61287d378cf7af24fed87fa767e19a3462f7a01c93f95d73c465b"
	bip39PubAbandon  = "51425909c1e61287d378cf7af24fed87fa767e19a3462f7a01c93f95d73c465b"

	bip39PrivLegal = "2e8905819b8723fe2c1d161860e5ee1830318dbf49a83bd451cfb8440c28bd6f451bde832454ba73e6e0de313fcf5d1565ec51080edc73bb19287b8e0ab2122b"
	bip39PrivLetter = "d71de856f81a8acc65e6fc851a38d4d7ec216fd0796d0a6827a3ad6ed5511a305cd870f091a239f45c960fa79fd375ba8d225b351bb9bee0db6d26c01e055b0e"
	bip39PrivZoo   = "ac27495480225222079d7be181583751e86f571027b0497b5b5d11218e0a8a133d7a9648678b740cc418d56d0bb2e2074b0d807cec7e231e58a4853e16d18423"

	bip39SeedAbandon = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"
)

func TestNewKeyFromMnemonic_LegacyPins(t *testing.T) {
	cases := []struct {
		phrase   string
		wantPriv string
	}{
		{"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", legacyPrivAbandon},
		{"legal winner winner winner winner winner winner winner winner winner winner yellow", legacyPrivLegal},
		{"dsn demo mnemonic for testing only not for production use words eight", legacyPrivDSN},
	}
	for _, tc := range cases {
		t.Run(tc.phrase[:20], func(t *testing.T) {
			kp, err := NewKeyFromMnemonic(tc.phrase)
			require.NoError(t, err)
			require.Equal(t, tc.wantPriv, KeyToHex(kp))
		})
	}
}

func TestNewKeyFromMnemonic_Determinism(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kp1, err := NewKeyFromMnemonic(phrase)
	require.NoError(t, err)
	kp2, err := NewKeyFromMnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, kp1.PublicKey, kp2.PublicKey)
	require.Equal(t, kp1.PrivateKey, kp2.PrivateKey)
}

func TestNewKeyFromMnemonic_SHA256(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	hash := sha256.Sum256([]byte(phrase))
	kp, err := NewKeyFromMnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, hash[:], kp.PrivateKey[:32])
}

func TestNewKeyFromBIP39Mnemonic_AbandonVector(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kp, err := NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, bip39PrivAbandon, KeyToHex(kp))
	require.Equal(t, bip39PubAbandon, PublicKeyHex(kp))
}

func TestNewKeyFromBIP39Mnemonic_AdditionalVectors(t *testing.T) {
	cases := []struct {
		entropy  string
		phrase   string
		wantPriv string
	}{
		{"7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f", "legal winner thank year wave sausage worth useful legal winner thank yellow", bip39PrivLegal},
		{"80808080808080808080808080808080", "letter advice cage absurd amount doctor acoustic avoid letter advice cage above", bip39PrivLetter},
		{"ffffffffffffffffffffffffffffffff", "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong", bip39PrivZoo},
	}
	for _, tc := range cases {
		t.Run(tc.entropy[:4], func(t *testing.T) {
			kp, err := NewKeyFromBIP39Mnemonic(tc.phrase)
			require.NoError(t, err)
			require.Equal(t, tc.wantPriv, KeyToHex(kp))
		})
	}
}

func TestNewKeyFromBIP39Mnemonic_SeedConsistency(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kp, err := NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)

	seed, err := bip39MnemonicToSeed(phrase)
	require.NoError(t, err)
	kpFromSeed, err := NewKeyFromSeed(seed[:32])
	require.NoError(t, err)

	require.Equal(t, kpFromSeed.PublicKey, kp.PublicKey)
}

func TestNewKeyFromBIP39Mnemonic_BadChecksum(t *testing.T) {
	_, err := NewKeyFromBIP39Mnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon zebra")
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")
}

func TestNewKeyFromBIP39Mnemonic_Determinism(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kp1, err := NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	kp2, err := NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, kp1.PublicKey, kp2.PublicKey)
	require.Equal(t, kp1.PrivateKey, kp2.PrivateKey)
}

func TestGenerateMnemonic_12Words(t *testing.T) {
	phrase, err := GenerateMnemonic()
	require.NoError(t, err)
	words := strings.Fields(phrase)
	require.Len(t, words, 12)

	err = bip39ValidateMnemonic(phrase)
	require.NoError(t, err)
}

func TestGenerateMnemonic_DifferentEachTime(t *testing.T) {
	phrase1, err := GenerateMnemonic()
	require.NoError(t, err)
	phrase2, err := GenerateMnemonic()
	require.NoError(t, err)
	require.NotEqual(t, phrase1, phrase2)
}

func TestRestoreFromMnemonic(t *testing.T) {
	phrase := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kp1, err := NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	kp2, err := RestoreFromMnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, kp1.PublicKey, kp2.PublicKey)
}

func TestEncryptKey_Roundtrip(t *testing.T) {
	kp, err := NewKeyFromBIP39Mnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	require.NoError(t, err)

	data, err := EncryptKey("test-passphrase-12345", kp)
	require.NoError(t, err)
	require.Contains(t, string(data), `"version": 1`)

	got, err := DecryptKey("test-passphrase-12345", data)
	require.NoError(t, err)
	require.Equal(t, kp.PublicKey, got.PublicKey)
	require.Equal(t, kp.PrivateKey, got.PrivateKey)
}

func TestDecryptKey_WrongPassphrase(t *testing.T) {
	kp, err := NewKeyFromBIP39Mnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	require.NoError(t, err)

	data, err := EncryptKey("correct-passphrase-12345", kp)
	require.NoError(t, err)

	_, err = DecryptKey("wrong-passphrase-12345", data)
	require.ErrorIs(t, err, wallet.ErrWrongPassphrase)
}

func TestLegacyWalletLoadedPostChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.json")

	kp, err := NewKeyFromMnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	require.NoError(t, err)
	err = SaveKey(path, kp)
	require.NoError(t, err)

	got, err := LoadKey(path)
	require.NoError(t, err)
	require.Equal(t, kp.PublicKey, got.PublicKey)
	require.Equal(t, kp.PrivateKey, got.PrivateKey)
}

func TestDeprecatedDocExists(t *testing.T) {
	doc, err := os.ReadFile("wallet.go")
	require.NoError(t, err)
	require.Contains(t, string(doc), "Deprecated")
}

// bip39MnemonicToSeed re-exports bip39.MnemonicToSeed for test use.
// This avoids importing internal/bip39 directly in test (Go convention).
func bip39MnemonicToSeed(phrase string) ([]byte, error) {
	return mnemonicToSeed(phrase)
}

// bip39ValidateMnemonic re-exports bip39.ValidateMnemonic for test use.
func bip39ValidateMnemonic(phrase string) error {
	return validateMnemonic(phrase)
}
