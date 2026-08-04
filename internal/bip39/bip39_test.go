package bip39

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

const (
	phrase0000 = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	phrase7f7f = "legal winner thank year wave sausage worth useful legal winner thank yellow"
	phrase8080 = "letter advice cage absurd amount doctor acoustic avoid letter advice cage above"
	phraseffff = "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"
)

// Pinned verbatim from the official BIP-39 test vectors (never recomputed).
const (
	seed0000 = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"
	seed7f7f = "2e8905819b8723fe2c1d161860e5ee1830318dbf49a83bd451cfb8440c28bd6fa457fe1296106559a3c80937a1c1069be3a3a5bd381ee6260e8d9739fce1f607"
	seed8080 = "d71de856f81a8acc65e6fc851a38d4d7ec216fd0796d0a6827a3ad6ed5511a30fa280f12eb2e47ed2ac03b5c462a0358d18d69fe4f985ec81778c1b370b652a8"
	seedffff = "ac27495480225222079d7be181583751e86f571027b0497b5b5d11218e0a8a13332572917f0f8e5a589620c6f15b11c61dee327651a14c34e18231052e48c069"
)

func TestEntropyToMnemonic_OfficialVectors(t *testing.T) {
	cases := []struct {
		name    string
		entropy string
		want    string
	}{
		{"0000", "00000000000000000000000000000000", phrase0000},
		{"7f7f", "7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f7f", phrase7f7f},
		{"8080", "80808080808080808080808080808080", phrase8080},
		{"ffff", "ffffffffffffffffffffffffffffffff", phraseffff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entropy, err := hex.DecodeString(tc.entropy)
			if err != nil {
				t.Fatal(err)
			}
			got, err := EntropyToMnemonic(entropy)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("phrase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEntropyToMnemonic_WrongEntropyLength(t *testing.T) {
	if _, err := EntropyToMnemonic(make([]byte, 15)); err == nil {
		t.Fatal("expected error for 15-byte entropy")
	}
}

func TestMnemonicToSeed_OfficialVector(t *testing.T) {
	seed, err := MnemonicToSeed(phrase0000)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString(seed0000)
	if !bytesEqual(seed, want) {
		t.Errorf("seed = %x, want %s", seed, seed0000)
	}
	if len(seed) != 64 {
		t.Errorf("seed length = %d, want 64", len(seed))
	}
}

func TestMnemonicToSeed_MoreOfficialVectors(t *testing.T) {
	cases := []struct {
		name   string
		phrase string
		want   string
	}{
		{"7f7f", phrase7f7f, seed7f7f},
		{"8080", phrase8080, seed8080},
		{"ffff", phraseffff, seedffff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seed, err := MnemonicToSeed(tc.phrase)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := hex.DecodeString(tc.want)
			if !bytesEqual(seed, want) {
				t.Errorf("seed = %x, want %s", seed, tc.want)
			}
		})
	}
}

func TestMnemonicToSeed_Deterministic(t *testing.T) {
	first, err := MnemonicToSeed(phrase0000)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MnemonicToSeed(phrase0000)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(first, second) {
		t.Error("seeds differ for the same phrase")
	}
}

func TestValidateMnemonic(t *testing.T) {
	cases := []struct {
		name    string
		phrase  string
		wantErr error
		errWord string
	}{
		{"valid", phrase0000, nil, ""},
		{"uppercase", strings.ToUpper(phrase0000), nil, ""},
		{"word_not_in_wordlist", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon xyzzy", ErrWordNotInWordlist, "xyzzy"},
		{"zebra_is_valid_but_bad_checksum", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon zebra", ErrChecksum, ""},
		{"bad_checksum", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon", ErrChecksum, ""},
		{"eleven_words", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon", ErrWordCount, ""},
		{"thirteen_words", phrase0000 + " about", ErrWordCount, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMnemonic(tc.phrase)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want errors.Is %v", err, tc.wantErr)
			}
			if tc.errWord != "" && !strings.Contains(err.Error(), tc.errWord) {
				t.Errorf("error %q should name the invalid word %q", err, tc.errWord)
			}
		})
	}
}

func TestGenerateMnemonic_WellFormed(t *testing.T) {
	phrase, err := GenerateMnemonic()
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Fields(phrase)
	if len(words) != 12 {
		t.Fatalf("phrase has %d words, want 12", len(words))
	}
	for _, w := range words {
		if _, ok := englishIndex[w]; !ok {
			t.Errorf("word %q not in the English wordlist", w)
		}
	}
	if err := ValidateMnemonic(phrase); err != nil {
		t.Errorf("generated phrase fails validation: %v", err)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
