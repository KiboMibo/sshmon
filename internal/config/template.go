package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// tmplHead — начало конфига с закомментированным примером серверов.
// Серверы закомментированы: с примером-заглушкой sshmon бесконечно
// опрашивал бы мёртвый адрес вместо честного «добавьте серверы».
const tmplHead = `# Конфиг sshmon. Добавьте свои серверы и запустите sshmon снова.
interval: 5s

servers:
  # - name: web1
  #   host: 203.0.113.10
  #   port: 22
  #   user: root
  #   key: ~/.ssh/id_ed25519
  #   group: prod             # необязательная группа
  #   # password: secret        # альтернатива ключу (ещё есть ssh-agent)
  #   # insecure_host_key: true # не проверять host key
`

// tmplTail — пороги и настройка LLM.
const tmplTail = `
thresholds:
  cpu: 90
  mem: 90
  disk: 90

llm:
  provider: openai              # openai | anthropic | любой OpenAI-совместимый
  # base_url: http://localhost:11434/v1  # например, Ollama
  model: gpt-4o-mini
  api_key_env: OPENAI_API_KEY
  # api_key: sk-...

history:
  enabled: true
  path: ~/.local/share/sshmon/history.db
  raw_retention: 24h
  aggregate_retention: 720h
`

// Template — шаблон конфига без серверов (для headless / пустого ssh-конфига).
const Template = tmplHead + tmplTail

// WriteDefault создаёт файл конфига из шаблона. Не перезаписывает существующий.
func WriteDefault(path string) error {
	return writeNew(path, Template)
}

// WriteWithServers создаёт конфиг с выбранными серверами.
func WriteWithServers(path string, servers []Server) error {
	sb, err := yaml.Marshal(map[string][]Server{"servers": servers})
	if err != nil {
		return err
	}
	body := "# Конфиг sshmon (создан из ~/.ssh/config при первом запуске).\ninterval: 5s\n\n" +
		string(sb) + tmplTail
	return writeNew(path, body)
}

// PopulateServers атомарно добавляет серверы в существующий пустой конфиг.
func PopulateServers(path string, servers []Server) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if len(c.Servers) != 0 {
		return fmt.Errorf("%s: конфиг уже содержит серверы", path)
	}
	if len(servers) == 0 {
		return ErrNoServers
	}
	c.Servers = servers
	body, err := yaml.Marshal(&c)
	if err != nil {
		return err
	}
	return replaceFile(path, body)
}

func replaceFile(path string, body []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeNew(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(body)
	return err
}
