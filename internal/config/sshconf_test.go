package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSSHConfig(t *testing.T) {
	// Given: root config and two included files contain literal hosts,
	// including duplicate aliases in different source files.
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
	stagingBody := `Host db1
    HostName 10.99.0.5

Host stage-app
    HostName 198.51.100.7
`
	if err := os.WriteFile(filepath.Join(confD, "staging.conf"), []byte(stagingBody), 0o600); err != nil {
		t.Fatal(err)
	}
	rootPath, err := filepath.Abs(main)
	if err != nil {
		t.Fatal(err)
	}
	prodPath, err := filepath.Abs(filepath.Join(confD, "prod.conf"))
	if err != nil {
		t.Fatal(err)
	}
	stagingPath, err := filepath.Abs(filepath.Join(confD, "staging.conf"))
	if err != nil {
		t.Fatal(err)
	}

	// When: the root config is parsed.
	hosts, err := ParseSSHConfig(main)
	if err != nil {
		t.Fatal(err)
	}

	// Then: wildcard aliases are skipped and fields are parsed.
	byAlias := map[string]SSHHost{}
	for _, h := range hosts {
		byAlias[h.Alias] = h
	}
	if _, ok := byAlias["*"]; ok {
		t.Error("wildcard-хост не должен попадать в список")
	}
	w := byAlias["web1"]
	if w.HostName != "203.0.113.10" || w.User != "deploy" || w.Port != 2222 {
		t.Errorf("web1 распарсен неверно: %+v", w)
	}
	if byAlias["db2"].HostName != "10.0.0.5" {
		t.Error("мульти-алиас Host db1 db2 должен дать записи с одним HostName")
	}
	if byAlias["bare"].HostName != "bare" {
		t.Errorf("без HostName алиас должен стать хостом, got %q", byAlias["bare"].HostName)
	}

	// Then: source identity, output groups, and declaration order are stable.
	if w.Group != "main" {
		t.Errorf("корневая группа = %q, want main", w.Group)
	}
	if w.SourcePath != rootPath {
		t.Errorf("корневой источник = %q, want %q", w.SourcePath, rootPath)
	}
	if w.Position != 0 || byAlias["db2"].Position != 2 {
		t.Errorf("позиции в корне должны идти по порядку объявления: web1=%d, db2=%d",
			w.Position, byAlias["db2"].Position)
	}
	p := byAlias["prod-app"]
	if p.HostName != "198.51.100.1" || p.Group != "prod" || p.SourcePath != prodPath {
		t.Errorf("prod-app: группа 'prod' и источник prod.conf, got %+v", p)
	}
	s := byAlias["stage-app"]
	if s.Group != "staging" || s.SourcePath != stagingPath {
		t.Errorf("stage-app: группа 'staging' и источник staging.conf, got %+v", s)
	}
	for i, h := range hosts {
		if h.Position < 0 {
			t.Fatalf("host %d имеет неверную позицию %d", i, h.Position)
		}
	}
	var dups []SSHHost
	for _, h := range hosts {
		if h.Alias == "db1" {
			dups = append(dups, h)
		}
	}
	if len(dups) != 2 {
		t.Fatalf("db1 объявлен в двух источниках и должен дать 2 записи, got %d", len(dups))
	}
	if dups[0].SourcePath == dups[1].SourcePath && dups[0].Position == dups[1].Position {
		t.Error("дубликаты алиаса должны различаться источником или позицией")
	}
}

func TestParseSSHConfigReturnsMatchedIncludeReadError(t *testing.T) {
	// Given: an Include glob matches a directory, which cannot be read as a file.
	dir := t.TempDir()
	includedDir := filepath.Join(dir, "conf.d", "broken.conf")
	if err := os.MkdirAll(includedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "config")
	if err := os.WriteFile(main, []byte("Include conf.d/*.conf\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: parsing follows the matched Include.
	_, err := ParseSSHConfig(main)

	// Then: the matched source error is returned instead of being swallowed.
	if err == nil || !strings.Contains(err.Error(), "broken.conf") {
		t.Fatalf("error = %v, want included path", err)
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
