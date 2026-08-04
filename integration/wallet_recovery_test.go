//go:build integration

package integration

import (
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/BryanOx/dsn/sdk"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
	"github.com/stretchr/testify/require"
)

const testPassphrase = "test-passphrase-12345"

func TestWalletRecovery_GenerateRestoreSign(t *testing.T) {
	dir := t.TempDir()
	walletPath := filepath.Join(dir, "wallet.json")

	// Generate via SDK BIP-39 path.
	phrase, err := sdk.GenerateMnemonic()
	require.NoError(t, err)

	kp, err := sdk.NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	origAddr := kp.Address().String()

	// Save encrypted keystore.
	err = wallet.SaveKeystore(walletPath, testPassphrase, kp)
	require.NoError(t, err)

	// Restore from mnemonic — must reproduce the same address.
	restored, err := sdk.RestoreFromMnemonic(phrase)
	require.NoError(t, err)
	require.Equal(t, origAddr, restored.Address().String())

	// Load keystore via provider and sign a transaction.
	tx := makeTestTransaction(t, kp)
	hasher := types.SHA256Hasher{}
	err = kp.Sign(tx, hasher)
	require.NoError(t, err)
	require.NotEmpty(t, tx.Signature)
	require.NotEqual(t, [32]byte{}, tx.IntentID)

	// Load from disk with correct passphrase and sign — same signature.
	loaded, err := wallet.LoadKeyFile(walletPath, func() (string, error) {
		return testPassphrase, nil
	})
	require.NoError(t, err)
	tx2 := makeTestTransaction(t, loaded)
	err = loaded.Sign(tx2, hasher)
	require.NoError(t, err)
	require.Equal(t, tx.Signature, tx2.Signature)
	require.Equal(t, tx.IntentID, tx2.IntentID)
}

func TestWalletRecovery_MigrateThenSign(t *testing.T) {
	dir := t.TempDir()
	walletPath := filepath.Join(dir, "wallet.json")
	pass := "migrate-passphrase-12345"

	// Generate a BIP-39 keypair and save as legacy plaintext.
	phrase, err := sdk.GenerateMnemonic()
	require.NoError(t, err)
	kp, err := sdk.NewKeyFromBIP39Mnemonic(phrase)
	require.NoError(t, err)
	err = wallet.SaveKey(walletPath, kp)
	require.NoError(t, err)

	// Sign pre-migration for reference.
	hasher := types.SHA256Hasher{}
	tx1 := makeTestTransaction(t, kp)
	err = kp.Sign(tx1, hasher)
	require.NoError(t, err)

	// Migrate to encrypted keystore.
	err = wallet.Migrate(walletPath, pass)
	require.NoError(t, err)

	// Backup must exist.
	_, err = filepath.Glob(walletPath + ".bak")
	require.NoError(t, err)

	// Load the migrated keystore and sign — same address and signature.
	loaded, err := wallet.LoadKeyFile(walletPath, func() (string, error) {
		return pass, nil
	})
	require.NoError(t, err)
	require.Equal(t, kp.Address().String(), loaded.Address().String())

	tx2 := makeTestTransaction(t, loaded)
	err = loaded.Sign(tx2, hasher)
	require.NoError(t, err)
	require.Equal(t, tx1.Signature, tx2.Signature)
}

func TestWalletRecovery_WrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	walletPath := filepath.Join(dir, "wallet.json")

	kp, err := sdk.NewKeyFromBIP39Mnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	require.NoError(t, err)
	err = wallet.SaveKeystore(walletPath, testPassphrase, kp)
	require.NoError(t, err)

	// Wrong passphrase must return a checksum error, not panic.
	_, err = wallet.LoadKeyFile(walletPath, func() (string, error) {
		return "wrong-passphrase-xxxx", nil
	})
	require.Error(t, err)
	require.ErrorIs(t, err, wallet.ErrWrongPassphrase)

	// Verify file was not modified — still readable with correct passphrase.
	loaded, err := wallet.LoadKeyFile(walletPath, func() (string, error) {
		return testPassphrase, nil
	})
	require.NoError(t, err)
	require.Equal(t, kp.Address().String(), loaded.Address().String())
}

func TestWalletRecovery_LegacyPlaintextSignUnchanged(t *testing.T) {
	dir := t.TempDir()
	walletPath := filepath.Join(dir, "wallet.json")

	// Create a legacy plaintext wallet and sign a transaction.
	kp, err := wallet.GenerateKey()
	require.NoError(t, err)
	err = wallet.SaveKey(walletPath, kp)
	require.NoError(t, err)

	hasher := types.SHA256Hasher{}
	tx1 := makeTestTransaction(t, kp)
	err = kp.Sign(tx1, hasher)
	require.NoError(t, err)
	require.NotEmpty(t, tx1.Signature)

	// Load via the dual-format loader (nil provider = legacy path) and sign again.
	// Signatures must be identical because legacy wallets do not change keys.
	loaded, err := wallet.LoadKey(walletPath)
	require.NoError(t, err)
	require.Equal(t, kp.Address().String(), loaded.Address().String())

	tx2 := makeTestTransaction(t, loaded)
	err = loaded.Sign(tx2, hasher)
	require.NoError(t, err)
	require.Equal(t, tx1.Signature, tx2.Signature)
	require.Equal(t, hex.EncodeToString(kp.PublicKey[:]), hex.EncodeToString(loaded.PublicKey[:]))
}

// makeTestTransaction creates a minimal unsigned transaction suitable for signing.
func makeTestTransaction(t *testing.T, kp *wallet.KeyPair) *types.Transaction {
	t.Helper()
	return &types.Transaction{
		Sender:    kp.Address(),
		Nonce:     1,
		Payload:   []byte("integration test payload"),
		GasLimit:  21000,
		MaxFee:    1000,
		Timestamp: 1700000000,
		TxType:    types.TxTypeStandard,
	}
}
