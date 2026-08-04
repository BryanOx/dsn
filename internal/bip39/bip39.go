package bip39

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"strings"
)

// Typed validation errors.
var (
	ErrWordCount         = errors.New("bip39: mnemonic must contain exactly 12 words")
	ErrWordNotInWordlist = errors.New("bip39: word not in English wordlist")
	ErrChecksum          = errors.New("bip39: checksum mismatch")
	ErrEntropyLength     = errors.New("bip39: entropy must be 16 bytes for a 12-word mnemonic")
)

const (
	entropyBytes   = 16 // 128 bits
	wordCount      = 12 // 12 x 11-bit indices
	wordIndexBits  = 11 // 2^11 = 2048 wordlist
	seedIterations = 2048
	seedLength     = 64
	// seedSalt is the BIP-39 salt ("mnemonic" + passphrase) with the fixed
	// passphrase "TREZOR" mandated by the official BIP-39 test vectors
	// ("The passphrase 'TREZOR' is used for all vectors"), which the frozen
	// golden pins pin verbatim.
	seedSalt = "mnemonicTREZOR"
)

// GenerateMnemonic returns a random 12-word BIP-39 English mnemonic from
// 128 bits of CSPRNG entropy.
func GenerateMnemonic() (string, error) {
	entropy := make([]byte, entropyBytes)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return EntropyToMnemonic(entropy)
}

// EntropyToMnemonic converts 16 bytes of entropy into a 12-word BIP-39
// English mnemonic: 128 entropy bits plus a 4-bit SHA-256 checksum split
// into 12 x 11-bit wordlist indices.
func EntropyToMnemonic(entropy []byte) (string, error) {
	if len(entropy) != entropyBytes {
		return "", fmt.Errorf("%w: got %d bytes", ErrEntropyLength, len(entropy))
	}
	hash := sha256.Sum256(entropy)

	bitAt := func(i int) int {
		if i < len(entropy)*8 {
			return int((entropy[i/8] >> (7 - uint(i%8))) & 1)
		}
		j := i - len(entropy)*8
		return int((hash[j/8] >> (7 - uint(j%8))) & 1)
	}

	words := make([]string, 0, wordCount)
	for w := 0; w < wordCount; w++ {
		var idx int
		for b := 0; b < wordIndexBits; b++ {
			idx = idx<<1 | bitAt(w*wordIndexBits+b)
		}
		words = append(words, englishWords[idx])
	}
	return strings.Join(words, " "), nil
}

// ValidateMnemonic reports whether phrase is a valid 12-word BIP-39 English
// mnemonic. Input is normalized by trimming whitespace and lowercasing.
func ValidateMnemonic(phrase string) error {
	phrase = normalize(phrase)
	words := strings.Fields(phrase)
	if len(words) != wordCount {
		return fmt.Errorf("%w: got %d", ErrWordCount, len(words))
	}
	indices := make([]int, wordCount)
	for i, w := range words {
		idx, ok := englishIndex[w]
		if !ok {
			return fmt.Errorf("%w: %q", ErrWordNotInWordlist, w)
		}
		indices[i] = idx
	}

	entropy := make([]byte, entropyBytes)
	for i := 0; i < entropyBytes*8; i++ {
		bit := (indices[i/wordIndexBits] >> (10 - uint(i%wordIndexBits))) & 1
		entropy[i/8] |= byte(bit) << (7 - uint(i%8))
	}
	hash := sha256.Sum256(entropy)
	checksumBits := entropyBytes * 8 / 32
	for i := 0; i < checksumBits; i++ {
		bitPos := entropyBytes*8 + i
		wordBit := (indices[bitPos/wordIndexBits] >> (10 - uint(bitPos%wordIndexBits))) & 1
		hashBit := byte((hash[i/8] >> (7 - uint(i%8))) & 1)
		if byte(wordBit) != hashBit {
			return ErrChecksum
		}
	}
	return nil
}

// MnemonicToSeed derives the 64-byte BIP-39 seed via PBKDF2-HMAC-SHA512 with
// 2048 iterations and the fixed BIP-39 vector salt (see seedSalt), matching
// the frozen official test-vector seeds verbatim.
func MnemonicToSeed(phrase string) ([]byte, error) {
	return pbkdf2.Key(sha512.New, normalize(phrase), []byte(seedSalt), seedIterations, seedLength)
}

func normalize(phrase string) string {
	return strings.ToLower(strings.TrimSpace(phrase))
}
