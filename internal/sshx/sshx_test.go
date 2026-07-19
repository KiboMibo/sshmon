package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestRunCommandCancellationDropsConnection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var dropped atomic.Bool

	// Given an SSH output operation that remains blocked.
	output := func() ([]byte, error) {
		close(started)
		<-release
		return nil, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := runCommand(ctx, output, func() { dropped.Store(true) })
		done <- err
	}()
	<-started

	// When its context is cancelled.
	cancel()
	// Then RunContext's shared execution path returns context.Canceled and drops the connection.
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if !dropped.Load() {
		t.Fatal("connection was not dropped")
	}
}

func TestAuthMethodsRequiresPassphraseForEncryptedKey(t *testing.T) {
	// Given an encrypted private key and no alternate authentication method.
	t.Setenv("SSH_AUTH_SOCK", "")
	keyPath := writeEncryptedPrivateKey(t, []byte("correct horse"))

	// When authentication methods are built without a passphrase.
	methods, needsPassphrase, err := authMethods(config.Server{Key: keyPath}, nil)

	// Then the key is classified without leaking a parser error.
	if err != nil || len(methods) != 0 || !needsPassphrase {
		t.Fatalf("methods=%d needsPassphrase=%v err=%v", len(methods), needsPassphrase, err)
	}
}

func TestAuthMethodsPreservesPasswordFallbackForEncryptedKey(t *testing.T) {
	// Given an encrypted key and a configured password fallback.
	t.Setenv("SSH_AUTH_SOCK", "")
	keyPath := writeEncryptedPrivateKey(t, []byte("correct horse"))

	// When authentication methods are built without a key passphrase.
	methods, needsPassphrase, err := authMethods(config.Server{Key: keyPath, Password: "fallback"}, nil)

	// Then password authentication remains available before prompting.
	if err != nil || len(methods) != 1 || !needsPassphrase {
		t.Fatalf("methods=%d needsPassphrase=%v err=%v", len(methods), needsPassphrase, err)
	}
}

func TestAuthMethodsUnlocksEncryptedKeyWithPassphrase(t *testing.T) {
	// Given an encrypted private key.
	t.Setenv("SSH_AUTH_SOCK", "")
	keyPath := writeEncryptedPrivateKey(t, []byte("correct horse"))

	// When its correct passphrase is supplied.
	methods, needsPassphrase, err := authMethods(config.Server{Key: keyPath}, []byte("correct horse"))

	// Then public-key authentication becomes available.
	if err != nil || len(methods) != 1 || needsPassphrase {
		t.Fatalf("methods=%d needsPassphrase=%v err=%v", len(methods), needsPassphrase, err)
	}
}

func TestAuthMethodsRejectsWrongPassphraseWithoutLeakingIt(t *testing.T) {
	// Given an encrypted private key and a wrong secret.
	t.Setenv("SSH_AUTH_SOCK", "")
	keyPath := writeEncryptedPrivateKey(t, []byte("correct horse"))
	wrong := "do-not-leak-me"

	// When authentication methods are built with that secret.
	_, _, err := authMethods(config.Server{Key: keyPath}, []byte(wrong))

	// Then callers receive a typed, secret-free error.
	if !errors.Is(err, ErrInvalidPassphrase) {
		t.Fatalf("got %v, want ErrInvalidPassphrase", err)
	}
	if strings.Contains(err.Error(), wrong) {
		t.Fatalf("error leaks passphrase: %v", err)
	}
}

func TestSetPassphraseCopiesInputAndResetIsSafeWithoutConnection(t *testing.T) {
	// Given a client and a caller-owned passphrase buffer.
	client := New(config.Server{})
	passphrase := []byte("memory-only")

	// When the passphrase is stored and the caller overwrites its buffer.
	client.SetPassphrase(passphrase)
	clear(passphrase)
	client.Reset()

	// Then the client retained an independent in-memory copy.
	if got := string(client.passphrase); got != "memory-only" {
		t.Fatal("client did not retain an independent passphrase copy")
	}
}

func writeEncryptedPrivateKey(t *testing.T, passphrase []byte) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/id_ed25519"
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
