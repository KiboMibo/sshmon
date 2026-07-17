package config

import (
	"os"
	"path/filepath"
)

// Template — шаблон конфига, который создаётся при первом запуске.
// Серверы закомментированы: с примером-заглушкой sshmon бесконечно
// опрашивал бы мёртвый адрес вместо честного «добавьте серверы».
const Template = `# Конфиг sshmon. Добавьте свои серверы и запустите sshmon снова.
interval: 5s

servers:
  # - name: web1
  #   host: 203.0.113.10
  #   port: 22
  #   user: root
  #   key: ~/.ssh/id_ed25519
  #   # password: secret        # альтернатива ключу (ещё есть ssh-agent)
  #   # insecure_host_key: true # не проверять host key

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
`

// WriteDefault создаёт файл конфига из шаблона. Не перезаписывает существующий.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(Template)
	return err
}
