package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrNoServers = errors.New("не задано ни одного сервера")

// ErrBadHost — адрес, который внешние программы разберут не как адрес.
var ErrBadHost = errors.New("недопустимый адрес хоста")

// checkHost отсеивает значения, которые ssh поймёт иначе, чем задумано. Хост
// уходит в argv команды `ssh`, и адрес вида «-oProxyCommand=…» (такой легко
// приезжает из чужого ~/.ssh/config) разбирался бы как опция, а не как адрес.
// Ошибка называет сервер и причину: молча выкинутый сервер выглядит как
// пропажа хоста из мониторинга.
func checkHost(server Server) error {
	name := server.Name
	if name == "" {
		name = server.Host
	}
	switch {
	case strings.TrimSpace(server.Host) == "":
		return fmt.Errorf("сервер %q: %w: адрес пустой", name, ErrBadHost)
	case strings.HasPrefix(server.Host, "-"):
		return fmt.Errorf("сервер %q: %w: %q начинается с «-» — ssh примет его за опцию, а не за адрес", name, ErrBadHost, server.Host)
	case strings.ContainsAny(server.Host, " \t\r\n"):
		return fmt.Errorf("сервер %q: %w: %q содержит пробел или перевод строки", name, ErrBadHost, server.Host)
	}
	return nil
}

// checkHosts проверяет пачку серверов перед записью в конфиг: импорт из
// ~/.ssh/config идёт именно так, и негодный адрес лучше отклонить на входе.
func checkHosts(servers []Server) error {
	for _, server := range servers {
		if err := checkHost(server); err != nil {
			return err
		}
	}
	return nil
}

type Server struct {
	Name            string `yaml:"name"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port,omitempty"`
	User            string `yaml:"user"`
	Key             string `yaml:"key,omitempty"`
	Password        string `yaml:"password,omitempty"`
	PasswordEnv     string `yaml:"password_env,omitempty"`
	InsecureHostKey bool   `yaml:"insecure_host_key,omitempty"`
	Group           string `yaml:"group,omitempty"`
}

func (s Server) Addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func (s Server) Pass() string {
	if s.Password != "" {
		return s.Password
	}
	if s.PasswordEnv != "" {
		return os.Getenv(s.PasswordEnv)
	}
	return ""
}

type LLM struct {
	Provider  string `yaml:"provider"` // openai | anthropic | любой OpenAI-совместимый
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`
}

func (l LLM) Key() string {
	if l.APIKey != "" {
		return l.APIKey
	}
	if l.APIKeyEnv != "" {
		return os.Getenv(l.APIKeyEnv)
	}
	return ""
}

type Thresholds struct {
	CPU  float64 `yaml:"cpu"`
	Mem  float64 `yaml:"mem"`
	Disk float64 `yaml:"disk"`
}

type History struct {
	Enabled            *bool         `yaml:"enabled,omitempty"`
	Path               string        `yaml:"path,omitempty"`
	RawRetention       time.Duration `yaml:"-"`
	RawRetentionText   string        `yaml:"raw_retention,omitempty"`
	AggregateRetention time.Duration `yaml:"-"`
	AggregateText      string        `yaml:"aggregate_retention,omitempty"`
}

func (h History) IsEnabled() bool { return h.Enabled == nil || *h.Enabled }

type Dashboard struct {
	SystemdUnits []string `yaml:"systemd_units,omitempty"`
}

type Config struct {
	Interval   time.Duration `yaml:"-"`
	IntervalS  string        `yaml:"interval"`
	Servers    []Server      `yaml:"servers"`
	LLM        LLM           `yaml:"llm"`
	Thresholds Thresholds    `yaml:"thresholds"`
	History    History       `yaml:"history"`
	Dashboard  Dashboard     `yaml:"dashboard,omitempty"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sshmon", "config.yaml")
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.IntervalS == "" {
		c.Interval = 5 * time.Second
	} else {
		d, err := time.ParseDuration(c.IntervalS)
		if err != nil {
			return nil, fmt.Errorf("interval: %w", err)
		}
		c.Interval = d
	}
	if c.Thresholds.CPU == 0 {
		c.Thresholds.CPU = 90
	}
	if c.Thresholds.Mem == 0 {
		c.Thresholds.Mem = 90
	}
	if c.Thresholds.Disk == 0 {
		c.Thresholds.Disk = 90
	}
	if c.History.Path == "" {
		c.History.Path = expandHome("~/.local/share/sshmon/history.db")
	} else {
		c.History.Path = expandHome(c.History.Path)
	}
	if c.History.RawRetentionText == "" {
		c.History.RawRetention = 24 * time.Hour
	} else {
		d, err := time.ParseDuration(c.History.RawRetentionText)
		if err != nil {
			return nil, fmt.Errorf("history.raw_retention: %w", err)
		}
		c.History.RawRetention = d
	}
	if c.History.AggregateText == "" {
		c.History.AggregateRetention = 720 * time.Hour
	} else {
		d, err := time.ParseDuration(c.History.AggregateText)
		if err != nil {
			return nil, fmt.Errorf("history.aggregate_retention: %w", err)
		}
		c.History.AggregateRetention = d
	}
	if len(c.Servers) == 0 {
		return nil, fmt.Errorf("%s: %w (servers)", path, ErrNoServers)
	}
	for i := range c.Servers {
		s := &c.Servers[i]
		if s.Port == 0 {
			s.Port = 22
		}
		if s.User == "" {
			s.User = "root"
		}
		if s.Name == "" {
			s.Name = s.Host
		}
		if s.Key != "" {
			s.Key = expandHome(s.Key)
		}
		if err := checkHost(*s); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return &c, nil
}

// SecretPermWarning возвращает предупреждение, если файл конфига доступен
// группе/остальным (mode & 0o077 != 0) И содержит секрет в открытом виде
// (пароль сервера или api_key LLM). Иначе — "". Чистая функция, тестируема.
func SecretPermWarning(path string, cfg *Config) string {
	if cfg == nil || !hasPlaintextSecret(cfg) {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	perm := info.Mode().Perm()
	if perm&0o077 == 0 {
		return ""
	}
	return fmt.Sprintf("sshmon: %s содержит секрет и доступен для чтения другим (%#o) — chmod 600 %s", path, perm, path)
}

func hasPlaintextSecret(cfg *Config) bool {
	if cfg.LLM.APIKey != "" {
		return true
	}
	for _, s := range cfg.Servers {
		if s.Password != "" {
			return true
		}
	}
	return false
}

func expandHome(p string) string {
	if len(p) > 1 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
