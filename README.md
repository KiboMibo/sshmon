# sshmon

[![CI](https://github.com/KiboMibo/sshmon/actions/workflows/ci.yml/badge.svg)](https://github.com/KiboMibo/sshmon/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/KiboMibo/sshmon)](https://github.com/KiboMibo/sshmon/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/KiboMibo/sshmon)](go.mod)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos-lightgrey)](https://github.com/KiboMibo/sshmon/releases)

**TUI-мониторинг Linux-серверов по SSH без агентов** — плюс чат с LLM и встроенный
MCP-сервер. Раз в несколько секунд одним `exec`'ом читает `/proc` (CPU, память,
load, диски, IO, сеть), `df` и `ss`. Работает и на обычных дистрибутивах, и на
BusyBox-роутерах (OpenWrt/Keenetic: `logread` вместо journalctl). Ничего не ставит
на сервер и ничего на нём не меняет — только чтение.

![Демо sshmon](docs/demo.gif)

## Возможности

- **Fleet** — список серверов с группами, фильтром по проблемам, трендами и адаптивным превью.
- **Dashboard** — CPU, RAM, диски и IO, Docker, сеть, systemd-юниты и хвост последних 50 строк логов на одном экране.
- **Processes / Ports / Docker** — диагностические read-only экраны: sshmon не шлёт сигналы процессам и не трогает контейнеры.
- **History** — локальные графики (SQLite) с offline-разрывами, сырыми точками и минутными агрегатами.
- **Logs** — живой journalctl / tail syslog / logread с паузой, фильтром, уровнями, переподключением и буфером до 10 000 строк.
- **Chat + MCP** — чат с LLM, которому виден актуальный Fleet и состояние диагностики; тот же движок доступен агентам по MCP (stdio).

## Быстрый старт

```sh
# 1. Установить (Linux/macOS, amd64/arm64)
curl -fsSL https://raw.githubusercontent.com/KiboMibo/sshmon/main/install.sh | sh

# 2. Запустить — при первом старте выберите хосты из ~/.ssh/config
sshmon

# 3. Дальше
sshmon --import                  # добавить новые хосты из ~/.ssh/config
sshmon --config ./my.yaml        # свой конфиг
sshmon --headless                # фон: сбор метрик + MCP-сервер на stdio
```

Скрипт ставит бинарник в `/usr/local/bin` (sudo только при необходимости) и
сверяет SHA-256, если в релизе есть `checksums.txt`. Своя папка —
`... | BINDIR="$HOME/.local/bin" sh`; конкретная версия — `... | VERSION=v0.5.0 sh`.
Сборка из исходников — в разделе [Установка](#установка).

Конфиг сохраняется в `~/.config/sshmon/config.yaml`. Если `~/.ssh/config` пуст —
sshmon создаст шаблон, впишите серверы вручную (см. ниже).

Глобальные клавиши: `c` — Chat, `/` — Search, `:` — Command Palette, `?` — Help,
`esc` — назад, `q` — выход из Fleet.

## Установка

<details>
<summary>Скрипт установки (curl | sh)</summary>

```sh
curl -fsSL https://raw.githubusercontent.com/KiboMibo/sshmon/main/install.sh | sh
```

Определяет ОС/архитектуру (Linux/macOS, amd64/arm64), берёт последний релиз и
ставит бинарник в `/usr/local/bin` (`sudo` только если нет прав на запись). Если в
релизе опубликован `checksums.txt` — проверяет SHA-256. Переменные окружения:

- `BINDIR` — папка установки, например `$HOME/.local/bin` (без sudo);
- `VERSION` — конкретная версия, например `v0.5.0`.

Хотите сперва прочитать скрипт — скачайте и запустите локально:

```sh
curl -fsSL https://raw.githubusercontent.com/KiboMibo/sshmon/main/install.sh -o install.sh
less install.sh
sh install.sh
```
</details>

<details>
<summary>Из исходников</summary>

Нужен Go (версия — см. [go.mod](go.mod)).

```sh
git clone https://github.com/KiboMibo/sshmon.git
cd sshmon
go build -o sshmon ./cmd/sshmon
# при желании — в $PATH:
install -m755 sshmon ~/.local/bin/sshmon
```
</details>

<details>
<summary>Готовый бинарник</summary>

На странице [Releases](https://github.com/KiboMibo/sshmon/releases) публикуются
архивы под Linux и macOS (amd64/arm64):

```sh
tar xzf sshmon_<версия>_linux_amd64.tar.gz
install -m755 sshmon ~/.local/bin/sshmon
sshmon --version
```
</details>

## Конфигурация

<details>
<summary>Выбор хостов из ~/.ssh/config</summary>

При первом запуске (и по `sshmon --import`) sshmon читает `~/.ssh/config`
(включая `Include`, например `~/.ssh/conf.d/*.conf`) и показывает конфиги
свёрнутым деревом. Хосты из `~/.ssh/config` попадают в группу `main`, хосты
Include-файла — в группу по имени файла (`prod.conf` → `prod`).

- `enter` / `→` / `l` — раскрыть файл; `←` / `h` — свернуть или вернуться к файлу;
- `space` — выбрать хост или весь файл; `a` — выбрать/снять всё;
- `s` — сохранить выбранные серверы; `q` / `esc` — отменить.

Результат сохраняется в `~/.config/sshmon/config.yaml`.
</details>

<details>
<summary>Файл config.yaml</summary>

```yaml
interval: 5s                 # период опроса

servers:
  - name: web1               # имя в интерфейсе (по умолчанию = host)
    host: 203.0.113.10
    port: 22                 # по умолчанию 22
    user: root               # по умолчанию root
    key: ~/.ssh/id_ed25519   # ключ; ещё есть ssh-agent и password
    group: prod              # необязательная группа
    # password: secret            # альтернатива ключу
    # password_env: WEB1_PASS     # взять пароль из переменной окружения
    # insecure_host_key: true     # не проверять host key (осознанно)

thresholds:                  # пороги подсветки, %
  cpu: 90
  mem: 90
  disk: 90

llm:
  provider: openai           # openai | anthropic | любой OpenAI-совместимый
  # base_url: http://localhost:11434/v1   # например, Ollama
  model: gpt-4o-mini
  api_key_env: OPENAI_API_KEY
  # api_key: sk-...

history:
  enabled: true
  path: ~/.local/share/sshmon/history.db
  raw_retention: 24h         # сырые точки
  aggregate_retention: 720h  # минутные агрегаты (30 дней)

dashboard:
  systemd_units:             # точные имена; пустой список — показать запущенные сервисы
    - nginx.service
    - docker.service
```

- **Аутентификация**: ключ (`key`), ssh-agent или пароль (`password` / `password_env`).
- **LLM**: OpenAI, Anthropic или любой OpenAI-совместимый API.
- **История** включена по умолчанию. Если SQLite открыть не удаётся — sshmon
  предупреждает и работает без истории.
</details>

## Флаги

| Флаг | Описание |
| --- | --- |
| `--config PATH` | путь к `config.yaml` (по умолчанию `~/.config/sshmon/config.yaml`) |
| `--headless` | без TUI: сбор метрик + MCP-сервер на stdio |
| `--import` | добавить серверы из `~/.ssh/config` в существующий конфиг |
| `--version` | показать версию и выйти |

## Управление

<details>
<summary>Клавиши по экранам</summary>

Глобальных вкладок нет: навигация идёт от Fleet через Dashboard к диагностическому
экрану. На узких терминалах панели складываются вертикально.

**Глобально:** `c` — Chat, `/` — Search, `:` — Command Palette, `?` — Help,
`esc` — закрыть оверлей или назад, `q` — выход из Fleet.

**Fleet:** `j/k` или стрелки — выбор, `enter` — Dashboard, `tab` — группа,
`f` — только проблемные, `v` — превью, `pgup/pgdown` — страница.

**Dashboard:** `p` — Processes, `o` — Ports, `d` — Docker, `ctrl+l` — Logs на весь
экран, `ctrl+h` — History; `f` — фильтр systemd-юнитов, `j/k` — выбор юнита,
`enter` — его journal, `x` — вернуться к системному логу, `r` — переподключить.
Нижняя панель — статичный снимок последних 50 строк, не обновляется автоматически.

**History:** `1-5` — диапазон, `j/k` — метрика, `h/l` — курсор, `r` — обновить.

**Logs:** `space` — пауза хвоста, `/` — фильтр, `w` — уровень, `←/→` — источник,
`r` — переподключить, стрелки и Page Up/Down — прокрутка.
</details>

## Безопасность

<details>
<summary>Права на конфиг, секреты, host-key, MCP</summary>

- **Права на конфиг.** `~/.config/sshmon/config.yaml` может содержать SSH-пароль и
  API-ключ LLM в открытом виде. Держите файл доступным только владельцу:
  `chmod 600 ~/.config/sshmon/config.yaml`. Если файл читается группой или
  остальными — sshmon предупреждает при старте.
- **Секреты через окружение.** Чтобы не хранить секреты в файле, задайте
  `password_env` у сервера и `api_key_env` у LLM. Предпочитайте ssh-agent или
  ключ (`key`) вместо пароля.
- **Host-key.** Хост-ключи проверяются по `~/.ssh/known_hosts`. Если файла нет,
  sshmon один раз предупреждает о риске MITM и продолжает; `insecure_host_key: true`
  осознанно отключает проверку для конкретного сервера.
- **MCP.** MCP-сервер работает только на stdio и без аутентификации — запускайте
  его как локальный дочерний процесс доверенного агента и не публикуйте наружу.
</details>

## MCP

В headless-режиме sshmon отвечает по MCP (stdio): `list_servers`, `get_metrics`,
`get_issues`, `tail_log`. Регистрация для агента:

```json
{"mcpServers": {"sshmon": {"command": "sshmon", "args": ["--headless"]}}}
```
