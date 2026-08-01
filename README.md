# sshmon

[![CI](https://github.com/KiboMibo/sshmon/actions/workflows/ci.yml/badge.svg)](https://github.com/KiboMibo/sshmon/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/KiboMibo/sshmon)](https://github.com/KiboMibo/sshmon/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/KiboMibo/sshmon)](go.mod)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey)](https://github.com/KiboMibo/sshmon/releases)

**TUI-мониторинг Linux-серверов по SSH без агентов** — плюс чат с LLM и встроенный
MCP-сервер. Раз в несколько секунд одним `exec`'ом читает `/proc` (CPU, память,
load, диски, IO, сеть), `df` и `ss`. Работает и на обычных дистрибутивах, и на
BusyBox-роутерах (OpenWrt/Keenetic: `logread` вместо journalctl). Ничего не ставит
на сервер и ничего на нём не меняет — только чтение.

![Демо sshmon](demo/demo.gif)

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
архивы под Linux и macOS (amd64/arm64, `.tar.gz`) и Windows (amd64, `.zip`):

```sh
tar xzf sshmon_<версия>_linux_amd64.tar.gz
install -m755 sshmon ~/.local/bin/sshmon
sshmon --version
```

Windows: скачайте `sshmon_<версия>_windows_amd64.zip`, распакуйте `sshmon.exe`
и положите в папку из `PATH`.
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

**Глобально:** `c` — Chat (кроме экрана логов, там `c` — контекст строки),
`/` — Search, `:` — Command Palette, `?` — Help,
`x` — ssh в терминале, `ctrl+r` — переподключить сервер, `esc` — закрыть оверлей
или назад, `q` и `ctrl+c` — выход.

**Fleet:** `j/k` или стрелки — выбор, `enter` — Dashboard, `→/←` — раскрыть и
свернуть детали, `tab` (или `g`) — следующая группа, `a` — все хосты,
`/` — поиск, `f` — только проблемные, `esc` — сбросить фильтры, `v` — боковая
панель, `pgup/pgdown` — страница, `p` — Processes, `o` — Ports, `d` — Docker,
`l` — ящик логов над списком, повторное `l` его закрывает (в нём `↑↓` — хост,
`s` — источник, `enter` — на весь экран); из раскрытой карточки и по `ctrl+l`
логи открываются сразу на весь экран.

**Dashboard:** `tab` / `shift+tab` — фокус панели (СЕРВИСЫ → DOCKER → ЛОГИ),
`j/k` — прокрутка внутри сфокусированной панели; на панели СЕРВИСЫ те же `j/k`
двигают курсор юнита, `enter` открывает его journal, а `f` — фильтр юнитов.
`s` — вернуться к системному логу, `r` — обновить данные. Переходы:
`p` — Processes, `o` — Ports, `d` — Docker, `l` (или `ctrl+l`) — Logs на весь
экран, `h` (или `ctrl+h`) — History.
Панель логов — статичный снимок последних 50 строк, не обновляется автоматически.

**History:** `1-5` — диапазон, `j/k` — метрика, `h/l` — курсор, `r` — обновить.

**Logs:** `space` — пауза хвоста, `/` — фильтр, `n/N` — совпадения, `w` — только
warn и выше (`W` — перебор уровней), `s` или `←/→` — источник, `↑↓` — выделенная
строка, `y` — скопировать её, `c` — контекст ±5 строк без фильтра, `t` — скрыть
время, `r` — перезапустить поток, Page Up/Down и `home/end` — прокрутка.
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
- **Демо-конфиг — не шаблон.** `demo/config.yaml` нужен только для записи
  `demo.gif` на одноразовых контейнерах и намеренно небезопасен: пароль в
  открытом виде и `insecure_host_key: true`. Не копируйте его для реальных
  серверов — рабочий конфиг sshmon создаёт сам при первом запуске.
</details>

## Операции

<details>
<summary>Runbook: офлайн-сервер, файлы, ротация ключа, диагностика</summary>

**Где что лежит**

| Что | Путь |
| --- | --- |
| Конфиг | `~/.config/sshmon/config.yaml` (переопределяется `--config PATH`) |
| База истории | `~/.local/share/sshmon/history.db` (`history.path` в конфиге) |
| Хост-ключи | `~/.ssh/known_hosts` — свой файл sshmon не заводит |

База — обычный SQLite в режиме WAL: рядом с `history.db` живут `-wal` и
`-shm`. Удалить историю можно, остановив sshmon и стерев все три файла; при
следующем запуске база создастся заново. Отключить сбор — `history.enabled: false`.

**Сервер ушёл в офлайн**

1. В списке хостов сервер помечается как недоступный (`× недоступен` в шапке
   Dashboard). Человекочитаемая причина — отказ аутентификации, таймаут,
   неизвестный host-key — видна в раскрытой карточке (`→`) и в боковой панели
   «ЧТО НЕ ТАК».
2. `ctrl+r` — переподключить выбранный сервер. Если ключ зашифрован и
   ssh-agent недоступен, sshmon спросит passphrase; она хранится только в
   памяти процесса и запрашивается заново после перезапуска.
3. `x` — открыть обычную сессию `ssh` в том же терминале и посмотреть,
   воспроизводится ли проблема вне sshmon. По выходу из ssh вернётесь в TUI.
4. Ошибка про host-key означает, что ключ сервера не совпал с `known_hosts`
   (переустановка хоста или MITM). Проверьте отпечаток и, если смена
   ожидаемая, удалите старую запись: `ssh-keygen -R <host>`.
5. Сбор метрик по остальным серверам продолжается: один офлайн-хост не
   останавливает опрос и не роняет sshmon.

**Ротация ключа LLM**

Ключ берётся при каждом запросе, но конфиг читается один раз при старте.

- Если ключ задан через `api_key_env` (рекомендуется) — обновите переменную
  окружения и перезапустите sshmon.
- Если ключ лежит в `api_key` — впишите новый в конфиг, убедитесь в
  `chmod 600 ~/.config/sshmon/config.yaml` и перезапустите sshmon.
- Старый ключ после этого отзывайте на стороне провайдера — sshmon его нигде
  не кеширует и в историю не пишет.

**Снять диагностический вывод**

TUI занимает альтернативный экран, поэтому сообщения об ошибках старта
(права на конфиг, недоступная история) уходят в stderr и на экране не видны.

```sh
sshmon 2>/tmp/sshmon.err            # TUI на экране, диагностика в файл
sshmon --headless 2>/tmp/sshmon.log # без TUI: сбор + MCP на stdio, лог в файл
sshmon --version                    # версия для баг-репорта
```

Диагностические экраны (`p` / `o` / `d`) и логи (`l`) — read-only и
показывают то же, что видит сборщик. Строку логов можно скопировать в буфер
клавишей `y`.
</details>

## MCP

В headless-режиме sshmon отвечает по MCP (stdio): `list_servers`, `get_metrics`,
`get_issues`, `tail_log`. Регистрация для агента:

```json
{"mcpServers": {"sshmon": {"command": "sshmon", "args": ["--headless"]}}}
```
