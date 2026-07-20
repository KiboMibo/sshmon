package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestFilterAgentSignersByKey keeps only the signer whose public key matches.
func TestFilterAgentSignersByKey(t *testing.T) {
	// Given: три signer'а, один из которых целевой.
	signers := generateThreeSigners(t)
	expected := signers[1].PublicKey()

	// When: фильтруем по ожидаемому публичному ключу.
	got := filterAgentSigners(signers, expected)

	// Then: возвращается ровно один matching signer.
	if len(got) != 1 {
		t.Fatalf("expected 1 matching signer, got %d", len(got))
	}
	if string(got[0].PublicKey().Marshal()) != string(expected.Marshal()) {
		t.Fatal("returned signer pubkey does not match expected")
	}
}

// TestFilterAgentSignersNilExpectedReturnsAll preserves legacy behaviour.
func TestFilterAgentSignersNilExpectedReturnsAll(t *testing.T) {
	// Given: три signer'а и nil-ожидаемый ключ (cfg.Key пуст или непрочитаем).
	signers := generateThreeSigners(t)

	// When: фильтруем с nil.
	got := filterAgentSigners(signers, nil)

	// Then: возвращаются все signers без фильтрации.
	if len(got) != len(signers) {
		t.Fatalf("expected all %d signers, got %d", len(signers), len(got))
	}
}

// TestFilterAgentSignersNoMatchReturnsAll prevents silently dropping auth.
func TestFilterAgentSignersNoMatchReturnsAll(t *testing.T) {
	// Given: три signer'а и ожидаемый ключ, которого нет в агенте.
	signers := generateThreeSigners(t)
	outside, _ := generateOneSignerAndKey(t)

	// When: фильтруем по отсутствующему ключу.
	got := filterAgentSigners(signers, outside.PublicKey())

	// Then: fallback — возвращаются все, чтобы не обрезать аутентификацию.
	if len(got) != len(signers) {
		t.Fatalf("expected fallback to all %d signers, got %d", len(signers), len(got))
	}
}

// TestPublicKeyFromKeyFileReadsPubSidecar derives pubkey from .pub file.
func TestPublicKeyFromKeyFileReadsPubSidecar(t *testing.T) {
	// Given: приватный ключ + sidecar .pub файл.
	signer, priv := generateOneSignerAndKey(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_test")
	writePrivateKeyFile(t, keyPath, priv)
	writePubFile(t, keyPath, signer)

	// When: читаем публичный ключ без passphrase.
	got := publicKeyFromKeyFile(keyPath, nil)

	// Then: возвращается публичный ключ из sidecar-файла.
	if got == nil {
		t.Fatal("expected pubkey from .pub sidecar, got nil")
	}
	if string(got.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatal("pubkey from .pub does not match signer's pubkey")
	}
}

// TestPublicKeyFromKeyFileParsesUnencrypted falls back to private key.
func TestPublicKeyFromKeyFileParsesUnencrypted(t *testing.T) {
	// Given: приватный ключ БЕЗ .pub sidecar.
	signer, priv := generateOneSignerAndKey(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_nopub")
	writePrivateKeyFile(t, keyPath, priv)

	// When: читаем публичный ключ.
	got := publicKeyFromKeyFile(keyPath, nil)

	// Then: ключ выводится из самого приватного файла.
	if got == nil {
		t.Fatal("expected pubkey from unencrypted private, got nil")
	}
	if string(got.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatal("pubkey from private does not match signer's pubkey")
	}
}

// TestPublicKeyFromKeyFileReturnsNilForMissing signals no key derivable.
func TestPublicKeyFromKeyFileReturnsNilForMissing(t *testing.T) {
	// Given: несуществующий путь к ключу.
	// When: пытаемся прочитать публичный ключ.
	got := publicKeyFromKeyFile("/nonexistent/path/key", nil)

	// Then: возвращается nil — вызывающая сторона сохранит старое поведение.
	if got != nil {
		t.Fatalf("expected nil for missing key, got %T", got)
	}
}

// generateOneSignerAndKey возвращает signer и исходный ed25519 приватный ключ.
func generateOneSignerAndKey(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 generate: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("ssh.NewSignerFromKey: %v", err)
	}
	return signer, priv
}

// generateThreeSigners создаёт три независимых signer'а.
func generateThreeSigners(t *testing.T) []ssh.Signer {
	t.Helper()
	out := make([]ssh.Signer, 3)
	for i := range out {
		out[i], _ = generateOneSignerAndKey(t)
	}
	return out
}

func writePrivateKeyFile(t *testing.T, path string, priv ed25519.PrivateKey) {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private: %v", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write private: %v", err)
	}
}

// writePubFile writes an authorized_keys-format public key sidecar.
func writePubFile(t *testing.T, path string, signer ssh.Signer) {
	t.Helper()
	pub := signer.PublicKey()
	if err := os.WriteFile(path+".pub", ssh.MarshalAuthorizedKey(pub), 0o644); err != nil {
		t.Fatalf("write pub: %v", err)
	}
}
