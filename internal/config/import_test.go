package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAddServersAppendsNewAndSkipsExistingNames(t *testing.T) {
	// Given: инициализированный конфиг с сервером web и пользовательскими настройками.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := `interval: 17s
servers:
  - name: web
    host: 10.0.0.1
    user: deploy
thresholds:
  cpu: 81
  mem: 82
  disk: 83
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: импорт добавляет дубль web и новый сервер db.
	added, err := AddServers(path, []Server{
		{Name: "web", Host: "192.168.0.1", User: "other"},
		{Name: "db", Host: "10.0.0.2", User: "deploy", Group: "prod"},
	})
	if err != nil {
		t.Fatalf("AddServers: %v", err)
	}

	// Then: добавлен только db, настройки и существующий web сохранены.
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Interval != 17*time.Second || c.Thresholds != (Thresholds{CPU: 81, Mem: 82, Disk: 83}) {
		t.Fatalf("настройки изменились: interval=%v thresholds=%+v", c.Interval, c.Thresholds)
	}
	if len(c.Servers) != 2 || c.Servers[0].Host != "10.0.0.1" || c.Servers[1].Name != "db" || c.Servers[1].Group != "prod" {
		t.Fatalf("серверы: %+v", c.Servers)
	}
}

func TestAddServersWithoutNewServersLeavesFileUntouched(t *testing.T) {
	// Given: конфиг, уже содержащий все импортируемые серверы.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "servers:\n  - name: web\n    host: 10.0.0.1\n    user: deploy\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: импорт не приносит новых имён.
	added, err := AddServers(path, []Server{{Name: "web", Host: "10.9.9.9"}})
	if err != nil {
		t.Fatalf("AddServers: %v", err)
	}

	// Then: ничего не добавлено и файл не переписан.
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("файл изменился:\n%s", got)
	}
}

func TestRemainingHostsFiltersConfiguredAliases(t *testing.T) {
	// Given: хосты из ssh-конфига и уже настроенные серверы.
	hosts := []SSHHost{{Alias: "web"}, {Alias: "db"}, {Alias: "cache"}}
	servers := []Server{{Name: "web"}, {Name: "cache"}}

	// When: вычисляются ещё не импортированные хосты.
	got := RemainingHosts(hosts, servers)

	// Then: остаётся только db.
	if len(got) != 1 || got[0].Alias != "db" {
		t.Fatalf("RemainingHosts = %+v", got)
	}
}
