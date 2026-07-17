package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Name            string `yaml:"name"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Key             string `yaml:"key,omitempty"`
	Password        string `yaml:"password,omitempty"`
	InsecureHostKey bool   `yaml:"insecure_host_key,omitempty"`
	Group           string `yaml:"group,omitempty"`
}

func (s Server) Addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

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

type Config struct {
	Interval   time.Duration `yaml:"-"`
	IntervalS  string        `yaml:"interval"`
	Servers    []Server      `yaml:"servers"`
	LLM        LLM           `yaml:"llm"`
	Thresholds Thresholds    `yaml:"thresholds"`
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
	if len(c.Servers) == 0 {
		return nil, fmt.Errorf("%s: не задано ни одного сервера (servers)", path)
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
	}
	return &c, nil
}

func expandHome(p string) string {
	if len(p) > 1 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
