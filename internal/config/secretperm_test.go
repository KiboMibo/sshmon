package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretPermWarning(t *testing.T) {
	write := func(t *testing.T, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(p, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
		// WriteFile режим урезается umask — выставим явно
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	withPass := &Config{Servers: []Server{{Password: "hunter2"}}}
	withKey := &Config{LLM: LLM{APIKey: "sk-x"}}
	noSecret := &Config{Servers: []Server{{Key: "~/.ssh/id_ed25519"}}}

	if w := SecretPermWarning(write(t, 0o644), withPass); w == "" {
		t.Errorf("0644 + пароль: ожидали предупреждение")
	}
	if w := SecretPermWarning(write(t, 0o644), withKey); w == "" {
		t.Errorf("0644 + api_key: ожидали предупреждение")
	}
	if w := SecretPermWarning(write(t, 0o600), withPass); w != "" {
		t.Errorf("0600 + пароль: не ожидали предупреждение, got %q", w)
	}
	if w := SecretPermWarning(write(t, 0o644), noSecret); w != "" {
		t.Errorf("0644 без секрета: не ожидали предупреждение, got %q", w)
	}
	if w := SecretPermWarning(filepath.Join(t.TempDir(), "missing.yaml"), withPass); w != "" {
		t.Errorf("нет файла: не ожидали предупреждение, got %q", w)
	}
}
