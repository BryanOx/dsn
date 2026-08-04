package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BryanOx/dsn/internal/bip39"
	"github.com/BryanOx/dsn/internal/passphrase"
	wallet2 "github.com/BryanOx/dsn/wallet"
	"github.com/spf13/cobra"
)

const validPhrase = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// badChecksumPhrase is a 12-word phrase whose words are all in the English
// wordlist but whose trailing checksum bits do not match (see bip39 tests).
const badChecksumPhrase = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon zebra"

func saveWalletState(t *testing.T) {
	t.Helper()
	oldFlags := walletFlags
	oldMnemonic := lastMnemonic
	oldProvider := passphraseProvider
	oldStdin := passphrase.Stdin
	oldTTY := passphrase.StdinIsTTY
	t.Cleanup(func() {
		walletFlags = oldFlags
		lastMnemonic = oldMnemonic
		passphraseProvider = oldProvider
		passphrase.Stdin = oldStdin
		passphrase.StdinIsTTY = oldTTY
	})
}

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "generate"}
	cmd.Flags().StringVar(&walletFlags.passphrase, "passphrase", "", "unsupported")
	cmd.Flags().StringVarP(&walletFlags.output, "output", "o", "", "output file path")
	cmd.Flags().BoolVar(&walletFlags.legacyPlaintext, "legacy-plaintext", false, "legacy plaintext format")
	return cmd
}

func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	fn()
	if err := outW.Close(); err != nil {
		t.Fatal(err)
	}
	if err := errW.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	errs, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), string(errs)
}

func extractMnemonic(t *testing.T, stdout string) string {
	t.Helper()
	const prefix = "Recovery mnemonic: "
	idx := strings.Index(stdout, prefix)
	if idx < 0 {
		t.Fatalf("stdout does not contain %q: %q", prefix, stdout)
	}
	rest := stdout[idx+len(prefix):]
	if end := strings.IndexByte(rest, '\n'); end >= 0 {
		rest = rest[:end]
	}
	phrase := strings.TrimSpace(rest)
	if phrase == "" {
		t.Fatal("empty mnemonic in stdout")
	}
	return phrase
}

func assertFileMode0600(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("file mode = %o, want 0600", got)
	}
}

func TestWalletGenerate_PassphraseFlagRejected(t *testing.T) {
	saveWalletState(t)
	out := filepath.Join(t.TempDir(), "wallet.json")
	cmd := newGenerateCmd()
	if err := cmd.Flags().Set("output", out); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("passphrase", "secret-value"); err != nil {
		t.Fatal(err)
	}
	passphraseProvider = func() (string, error) {
		t.Fatal("passphrase provider must never be invoked for a rejected --passphrase")
		return "", nil
	}

	err := runWalletGenerate(cmd, nil)
	if err == nil {
		t.Fatal("expected an explicit rejection error")
	}
	if !strings.Contains(err.Error(), "--passphrase") {
		t.Errorf("error should name the flag, got %q", err.Error())
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("no wallet file should be written when --passphrase is rejected")
	}
}

func TestWalletRestore_InvalidMnemonicBeforePromptOrFile(t *testing.T) {
	saveWalletState(t)
	walletFlags.mnemonic = badChecksumPhrase
	walletFlags.output = filepath.Join(t.TempDir(), "wallet.json")
	passphraseProvider = func() (string, error) {
		t.Fatal("must not prompt before validating the mnemonic")
		return "", nil
	}

	err := runWalletRestore(nil, nil)
	if err == nil {
		t.Fatal("expected an error for a bad-checksum mnemonic")
	}
	if _, statErr := os.Stat(walletFlags.output); !os.IsNotExist(statErr) {
		t.Error("no wallet file should be written for an invalid mnemonic")
	}
}

func TestWalletExportMnemonic_FreshProcessFails(t *testing.T) {
	saveWalletState(t)
	lastMnemonic = ""

	stdout, _ := captureOutput(t, func() {
		err := runWalletExportMnemonic(nil, nil)
		if err == nil {
			t.Fatal("expected an error when no mnemonic is held in process memory")
		}
		if !strings.Contains(err.Error(), "never stored") && !strings.Contains(err.Error(), "process") {
			t.Errorf("error should explain the mnemonic cannot be recovered, got %q", err.Error())
		}
	})
	if stdout != "" {
		t.Errorf("export must print nothing on failure, got %q", stdout)
	}
}

func TestWalletGenerate_EncryptedKeystore(t *testing.T) {
	saveWalletState(t)
	cmd := newGenerateCmd()
	out := filepath.Join(t.TempDir(), "wallet.json")
	if err := cmd.Flags().Set("output", out); err != nil {
		t.Fatal(err)
	}
	passphrase.Stdin = strings.NewReader("correcthorsebatterystaple\ncorrecthorsebatterystaple\n")
	passphrase.StdinIsTTY = func() bool { return true }

	stdout, stderr := captureOutput(t, func() {
		if err := runWalletGenerate(cmd, nil); err != nil {
			t.Fatalf("generate failed: %v", err)
		}
	})

	phrase := extractMnemonic(t, stdout)
	if strings.Count(stdout, phrase) != 1 {
		t.Errorf("mnemonic must print exactly once, stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "mnemonic") {
		t.Errorf("stderr should warn about backing up the mnemonic, got %q", stderr)
	}
	if err := bip39.ValidateMnemonic(phrase); err != nil {
		t.Errorf("generated mnemonic is invalid: %v", err)
	}
	if lastMnemonic != phrase {
		t.Errorf("lastMnemonic = %q, want %q", lastMnemonic, phrase)
	}

	assertFileMode0600(t, out)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version") {
		t.Error("generate must write an encrypted keystore by default")
	}
	wantKp, err := wallet2.KeyFromMnemonic(phrase)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := wallet2.LoadKeyFile(out, func() (string, error) {
		return "correcthorsebatterystaple", nil
	})
	if err != nil {
		t.Fatalf("loading generated keystore: %v", err)
	}
	if loaded.Address() != wantKp.Address() {
		t.Errorf("keystore address = %s, want %s", loaded.Address(), wantKp.Address())
	}

	exportOut, exportErr := captureOutput(t, func() {
		if err := runWalletExportMnemonic(nil, nil); err != nil {
			t.Fatalf("export after generate failed: %v", err)
		}
	})
	if strings.Count(exportOut, phrase) != 1 {
		t.Errorf("export must print the mnemonic exactly once, stdout = %q", exportOut)
	}
	if !strings.Contains(exportErr, "warning") {
		t.Errorf("export should warn, got %q", exportErr)
	}
}

func TestWalletGenerate_PassphraseMismatchNoFile(t *testing.T) {
	saveWalletState(t)
	cmd := newGenerateCmd()
	out := filepath.Join(t.TempDir(), "wallet.json")
	if err := cmd.Flags().Set("output", out); err != nil {
		t.Fatal(err)
	}
	passphrase.Stdin = strings.NewReader("correcthorsebatterystaple\nwrongwrongwrongwrong\n")
	passphrase.StdinIsTTY = func() bool { return true }

	err := runWalletGenerate(cmd, nil)
	if !errors.Is(err, passphrase.ErrMismatch) {
		t.Fatalf("error = %v, want passphrase.ErrMismatch", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("no wallet file should be written on a passphrase mismatch")
	}
}

func TestWalletGenerate_LegacyPlaintextNoPrompt(t *testing.T) {
	saveWalletState(t)
	cmd := newGenerateCmd()
	out := filepath.Join(t.TempDir(), "wallet.json")
	if err := cmd.Flags().Set("output", out); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("legacy-plaintext", "true"); err != nil {
		t.Fatal(err)
	}
	passphraseProvider = func() (string, error) {
		t.Fatal("legacy generate must not prompt for a passphrase")
		return "", nil
	}

	stdout, _ := captureOutput(t, func() {
		if err := runWalletGenerate(cmd, nil); err != nil {
			t.Fatalf("legacy generate failed: %v", err)
		}
	})
	if !strings.Contains(stdout, "Address:") {
		t.Errorf("legacy generate should print the address, got %q", stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version") {
		t.Error("legacy generate must write the legacy plaintext format, not a keystore")
	}
	if lastMnemonic != "" {
		t.Error("legacy generate should not retain a mnemonic")
	}
}

func TestWalletRestore_ValidMnemonicWritesKeystore(t *testing.T) {
	saveWalletState(t)
	walletFlags.mnemonic = validPhrase
	walletFlags.output = filepath.Join(t.TempDir(), "wallet.json")
	passphrase.Stdin = strings.NewReader("correcthorsebatterystaple\ncorrecthorsebatterystaple\n")
	passphrase.StdinIsTTY = func() bool { return true }

	wantKp, err := wallet2.KeyFromMnemonic(validPhrase)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureOutput(t, func() {
		if err := runWalletRestore(nil, nil); err != nil {
			t.Fatalf("restore failed: %v", err)
		}
	})
	if !strings.Contains(stdout, wantKp.Address().String()) {
		t.Errorf("restore should print the derived address %s, got %q", wantKp.Address(), stdout)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("restore should warn about the mnemonic, got %q", stderr)
	}

	assertFileMode0600(t, walletFlags.output)
	data, err := os.ReadFile(walletFlags.output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version") {
		t.Error("restore must write an encrypted keystore")
	}
	if lastMnemonic != validPhrase {
		t.Errorf("lastMnemonic = %q, want %q", lastMnemonic, validPhrase)
	}
}
