# sshmon

TUI-мониторинг Linux-серверов по SSH без агентов + чат с LLM + встроенный MCP-сервер.

Раз в несколько секунд по SSH одним exec'ом читает `/proc` (CPU, память, load,
диски, IO, сеть), `df` и `ss` — работает на обычных дистрибутивах и на
BusyBox-роутерах (OpenWrt/Keenetic: `logread` вместо journalctl).

## Возможности

- **Overview** — таблица всех серверов: CPU, RAM, диск, load, статус; детекция проблем по порогам.
- **Detail** — подробности по серверу: диски, IO, скорость сети.
- **Ports** — открытые порты с процессами (`ss -tulpn`).
- **Logs** — journalctl / tail syslog / logread.
- **Chat** — чат с LLM; в system prompt автоматически подставляется живой снапшот всех серверов и найденные проблемы.

## Установка

```sh
go build -o sshmon ./cmd/sshmon
```

## Конфигурация

```sh
mkdir -p ~/.config/sshmon
cp config.example.yaml ~/.config/sshmon/config.yaml
```

Аутентификация: ключ (`key`), ssh-agent или пароль (`password`).
LLM: OpenAI, Anthropic или любой OpenAI-совместимый API (Ollama: `base_url: http://localhost:11434/v1`).

## Запуск

```sh
sshmon                  # TUI
sshmon --headless       # фон: сбор метрик + MCP-сервер на stdio
sshmon --config /path/to/config.yaml
```

Клавиши: `tab`/`1-5` — вкладки, `j/k` — выбор сервера, `r` — обновить логи,
`i`/`enter` — фокус ввода в чате, `q` — выход.

## MCP

В headless-режиме sshmon отвечает по MCP (stdio): `list_servers`,
`get_metrics`, `get_issues`, `tail_log`. Регистрация для агента:

```json
{"mcpServers": {"sshmon": {"command": "sshmon", "args": ["--headless"]}}}
```
