package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHConfig(t *testing.T) {
	dir := t.TempDir()
	confD := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confD, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "config")
	mainBody := `Include conf.d/*.conf

Host web1
    HostName 203.0.113.10
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519

Host db1 db2
    HostName 10.0.0.5

Host *
    ServerAliveInterval 30

Host bare
`
	if err := os.WriteFile(main, []byte(mainBody), 0o600); err != nil {
		t.Fatal(err)
	}
	prodBody := `Host prod-app
    HostName 198.51.100.1
    User root
`
	if err := os.WriteFile(filepath.Join(confD, "prod.conf"), []byte(prodBody), 0o600); err != nil {
		t.Fatal(err)
	}

	hosts, err := ParseSSHConfig(main)
	if err != nil {
		t.Fatal(err)
	}
	byAlias := map[string]SSHHost{}
	for _, h := range hosts {
		byAlias[h.Alias] = h
	}
	if _, ok := byAlias["*"]; ok {
		t.Error("wildcard-хост не должен попадать в список")
	}
	w := byAlias["web1"]
	if w.HostName != "203.0.113.10" || w.User != "deploy" || w.Port != 2222 || w.Group != "" {
		t.Errorf("web1 распарсен неверно: %+v", w)
	}
	if byAlias["db1"].HostName != "10.0.0.5" || byAlias["db2"].HostName != "10.0.0.5" {
		t.Error("мульти-алиас Host db1 db2 должен дать две записи с одним HostName")
	}
	if byAlias["bare"].HostName != "bare" {
		t.Errorf("без HostName алиас должен стать хостом, got %q", byAlias["bare"].HostName)
	}
	p := byAlias["prod-app"]
	if p.HostName != "198.51.100.1" || p.Group != "prod" {
		t.Errorf("prod-app: группа должна быть 'prod' (из имени файла), got %+v", p)
	}
}

func TestWriteWithServersRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := []Server{
		{Name: "web1", Host: "203.0.113.10", Port: 2222, User: "deploy", Group: "prod"},
		{Name: "db1", Host: "10.0.0.5"},
	}
	if err := WriteWithServers(path, in); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("ожидалось 2 сервера, got %d", len(cfg.Servers))
	}
	s0 := cfg.Servers[0]
	if s0.Name != "web1" || s0.Host != "203.0.113.10" || s0.Port != 2222 || s0.User != "deploy" || s0.Group != "prod" {
		t.Errorf("web1 после round-trip: %+v", s0)
	}
	s1 := cfg.Servers[1]
	if s1.Port != 22 || s1.User != "root" || s1.Name != "db1" {
		t.Errorf("db1 должен получить дефолты Port=22/User=root: %+v", s1)
	}
}
