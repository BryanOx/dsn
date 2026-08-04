package bip39

import (
	"embed"
	"strings"
)

//go:embed english.txt
var wordlistFS embed.FS

var englishWords []string
var englishIndex map[string]int

func init() {
	data, err := wordlistFS.ReadFile("english.txt")
	if err != nil {
		panic("bip39: cannot read embedded wordlist: " + err.Error())
	}
	englishWords = strings.Split(strings.TrimSpace(string(data)), "\n")
	englishIndex = make(map[string]int, len(englishWords))
	for i, word := range englishWords {
		englishIndex[word] = i
	}
}
