package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if !errors.Is(err, ErrNoServers) {
		t.Fatalf("Load шаблона: errors.Is(err, ErrNoServers) = false: %v", err)
	}
}

func TestPopulateServersPreservesExistingSettings(t *testing.T) {
	// Given: существующий конфиг без серверов с пользовательскими настройками.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := `interval: 17s
servers:
thresholds:
  cpu: 81
  mem: 82
  disk: 83
llm:
  provider: openai
  base_url: http://127.0.0.1:11434/v1
  model: debug-model
  api_key_env: SSHMON_DEBUG_API_KEY
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: интерактивная настройка добавляет выбранный сервер.
	servers := []Server{{Name: "prod-web", Host: "127.0.0.2", Port: 2222, User: "deploy", Group: "prod"}}
	if err := PopulateServers(path, servers); err != nil {
		t.Fatalf("PopulateServers: %v", err)
	}

	// Then: сервер добавлен, а пользовательские настройки сохранены.
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Interval != 17*time.Second || c.Thresholds != (Thresholds{CPU: 81, Mem: 82, Disk: 83}) {
		t.Fatalf("настройки изменились: interval=%v thresholds=%+v", c.Interval, c.Thresholds)
	}
	if c.LLM.BaseURL != "http://127.0.0.1:11434/v1" || c.LLM.Model != "debug-model" || c.LLM.APIKeyEnv != "SSHMON_DEBUG_API_KEY" {
		t.Fatalf("LLM-настройки изменились: %+v", c.LLM)
	}
	if len(c.Servers) != 1 || c.Servers[0].Group != "prod" {
		t.Fatalf("серверы не сохранены: %+v", c.Servers)
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

func TestLoadDefaultsHistoryRetention(t *testing.T) {
	// Given: a minimal configuration with one server and no history block.
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - host: 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: the configuration is loaded.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Then: history is enabled with the documented path and retention defaults.
	if !cfg.History.IsEnabled() {
		t.Fatal("history disabled by default")
	}
	if cfg.History.Path != filepath.Join(os.Getenv("HOME"), ".local", "share", "sshmon", "history.db") {
		t.Fatalf("History.Path=%q", cfg.History.Path)
	}
	if cfg.History.RawRetention != 24*time.Hour || cfg.History.AggregateRetention != 720*time.Hour {
		t.Fatalf("history retention=%v/%v", cfg.History.RawRetention, cfg.History.AggregateRetention)
	}
}

func TestLoadPreservesExplicitlyDisabledHistory(t *testing.T) {
	// Given: history is explicitly disabled.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "servers:\n  - host: 10.0.0.1\nhistory:\n  enabled: false\n  raw_retention: 2h\n  aggregate_retention: 48h\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: the configuration is loaded.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Then: false and custom durations remain authoritative.
	if cfg.History.IsEnabled() {
		t.Fatal("explicit history.enabled=false was ignored")
	}
	if cfg.History.RawRetention != 2*time.Hour || cfg.History.AggregateRetention != 48*time.Hour {
		t.Fatalf("history retention=%v/%v", cfg.History.RawRetention, cfg.History.AggregateRetention)
	}
}

func TestWriteWithServersOmitsZeroPortAndLoadDefaultsToSSH(t *testing.T) {
	// Given: an imported SSH host without an explicit Port.
	path := filepath.Join(t.TempDir(), "config.yaml")
	servers := []Server{{Name: "web", Host: "10.0.0.1", User: "deploy"}}

	// When: the generated configuration is written.
	if err := WriteWithServers(path, servers); err != nil {
		t.Fatalf("WriteWithServers: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Then: zero is not serialized and Load still applies SSH port 22.
	if strings.Contains(string(body), "port: 0") {
		t.Fatalf("generated YAML contains zero port:\n%s", body)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Servers[0].Port; got != 22 {
		t.Fatalf("Port=%d, want 22", got)
	}
}
