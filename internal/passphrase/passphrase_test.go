package passphrase

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func withStdin(t *testing.T, input string, tty bool) {
	t.Helper()
	oldStdin := Stdin
	oldTTY := StdinIsTTY
	Stdin = strings.NewReader(input)
	StdinIsTTY = func() bool { return tty }
	t.Cleanup(func() {
		Stdin = oldStdin
		StdinIsTTY = oldTTY
	})
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPromptTwice_NonTTYFailsWithoutPrompts(t *testing.T) {
	withStdin(t, "correcthorsebatterystaple\ncorrecthorsebatterystaple\n", false)

	var got string
	var err error
	stderr := captureStderr(t, func() {
		got, err = PromptTwice("wallet passphrase")
	})
	if !errors.Is(err, ErrTTYRequired) {
		t.Fatalf("error = %v, want ErrTTYRequired", err)
	}
	if got != "" {
		t.Errorf("passphrase = %q, want empty", got)
	}
	if stderr != "" {
		t.Errorf("prompts must not be written when there is no TTY, got %q", stderr)
	}
}

func TestPromptTwice_InjectedReader(t *testing.T) {
	const pass = "correcthorsebatterystaple"
	withStdin(t, pass+"\n"+pass+"\n", true)

	stderr := captureStderr(t, func() {
		got, err := PromptTwice("wallet passphrase")
		if err != nil {
			t.Fatalf("PromptTwice failed: %v", err)
		}
		if got != pass {
			t.Fatalf("passphrase = %q, want %q", got, pass)
		}
	})
	if !strings.Contains(stderr, "wallet passphrase") {
		t.Errorf("prompt should name the label on stderr, got %q", stderr)
	}
}

func TestPromptTwice_TooShortStops(t *testing.T) {
	withStdin(t, "short\n", true)

	stderr := captureStderr(t, func() {
		got, err := PromptTwice("wallet passphrase")
		if !errors.Is(err, ErrTooShort) {
			t.Fatalf("error = %v, want ErrTooShort", err)
		}
		if got != "" {
			t.Errorf("passphrase = %q, want empty", got)
		}
	})
	if strings.Contains(stderr, "Confirm") {
		t.Errorf("prompting must stop after a too-short entry, got %q", stderr)
	}
}

func TestPromptTwice_MismatchAborts(t *testing.T) {
	withStdin(t, "aaaaaaaaaaaa\nbbbbbbbbbbbb\n", true)

	got, err := PromptTwice("wallet passphrase")
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("error = %v, want ErrMismatch", err)
	}
	if got != "" {
		t.Errorf("passphrase = %q, want empty", got)
	}
}

func TestIsTTY(t *testing.T) {
	withStdin(t, "injected\n", true)
	if !IsTTY() {
		t.Error("IsTTY() should report true for an injected TTY reader")
	}
	withStdin(t, "injected\n", false)
	if IsTTY() {
		t.Error("IsTTY() should report false for an injected non-TTY reader")
	}
}
