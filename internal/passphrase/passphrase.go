// Package passphrase provides TTY-only passphrase entry for the wallet CLI
// and TUI. Passphrases are prompted on stderr with echo disabled, never read
// from argv, environment variables, or piped stdin, and must be at least 12
// characters.
package passphrase

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Typed errors returned by the passphrase package.
var (
	ErrTTYRequired = errors.New("passphrase: a TTY is required for passphrase entry; passphrases are never read from pipes, environment variables, or argv")
	ErrTooShort    = errors.New("passphrase: passphrase is too short")
	ErrMismatch    = errors.New("passphrase: passphrase entries do not match")
)

// minLength is the minimum passphrase length enforced at the CLI layer (the
// keystore library itself does not enforce a length).
const minLength = 12

var (
	// Stdin is the passphrase input source. Production reads os.Stdin; tests
	// replace it with an in-memory reader to drive prompting.
	Stdin io.Reader = os.Stdin

	// StdinIsTTY reports whether Stdin is a terminal. Production probes the
	// os.Stdin file descriptor; tests replace it to simulate a TTY or a pipe.
	StdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

// IsTTY reports whether the passphrase input source is a terminal.
func IsTTY() bool {
	return StdinIsTTY()
}

// PromptTwice reads a passphrase with double entry from the TTY. Prompts go to
// stderr; if the first entry is shorter than minLength the flow stops without
// a confirmation prompt; if the two entries differ the flow aborts. On a
// non-TTY input source it returns ErrTTYRequired without reading or prompting.
func PromptTwice(label string) (string, error) {
	if !StdinIsTTY() {
		return "", ErrTTYRequired
	}
	if f, ok := Stdin.(*os.File); ok {
		return promptTwice(ttyReader{fd: int(f.Fd())}, label)
	}
	return promptTwice(lineReader{r: bufio.NewReader(Stdin)}, label)
}

// passphraseReader abstracts reading a single passphrase entry. The ttyReader
// disables echo via x/term; lineReader reads from an injected reader in tests.
type passphraseReader interface {
	ReadLine() (string, error)
}

// ttyReader reads a single no-echo line from a terminal file descriptor.
type ttyReader struct {
	fd int
}

func (r ttyReader) ReadLine() (string, error) {
	b, err := term.ReadPassword(r.fd)
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

// lineReader reads a single line from an arbitrary reader (test seam).
type lineReader struct {
	r *bufio.Reader
}

func (r lineReader) ReadLine() (string, error) {
	line, err := r.r.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

// promptTwice is the pure double-entry flow shared by the TTY and injected
// readers.
func promptTwice(r passphraseReader, label string) (string, error) {
	fmt.Fprintf(os.Stderr, "Enter %s: ", label)
	first, err := r.ReadLine()
	if err != nil {
		return "", fmt.Errorf("passphrase: reading entry: %w", err)
	}
	if len(first) < minLength {
		return "", fmt.Errorf("%w: need at least %d characters, got %d", ErrTooShort, minLength, len(first))
	}
	fmt.Fprintf(os.Stderr, "Confirm %s: ", label)
	second, err := r.ReadLine()
	if err != nil {
		return "", fmt.Errorf("passphrase: reading confirmation: %w", err)
	}
	if first != second {
		return "", ErrMismatch
	}
	return first, nil
}
