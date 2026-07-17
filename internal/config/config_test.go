package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDefaultAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")

	if err := WriteDefault(path); err != nil {
		t.Fatalf("WriteDefault: %v", err)
	}
	if err := WriteDefault(path); err == nil {
		t.Fatal("WriteDefault перезаписал существующий конфиг")
	}
	// Шаблон — валидный YAML без серверов: Load обязан отказать с понятной ошибкой.
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "ни одного сервера") {
		t.Fatalf("Load шаблона: ожидали ошибку про серверы, получили: %v", err)
	}
}

func TestLoadMinimal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - host: 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := c.Servers[0]
	if s.Port != 22 || s.User != "root" || s.Name != "10.0.0.1" {
		t.Fatalf("дефолты не применились: %+v", s)
	}
	if c.Interval.Seconds() != 5 {
		t.Fatalf("interval по умолчанию: %v", c.Interval)
	}
}
