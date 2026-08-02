# sshmon — Security Audit

**Дата:** 2026-07-21 · **Область:** весь модуль `github.com/kibomibo/sshmon` (Go 1.26.5)
**Метод:** ручной аудит потоков данных (SSH/MCP/LLM/config) + `govulncheck` + grep захардкоженных секретов.
**Вердикт:** ⚠️ уязвимостей, пригодных к эксплуатации, **не найдено**. Все замечания — это осознанные проектные компромиссы, уже смягчённые в раундах 1–2. Захардкоженных секретов в репозитории нет.

---

## Итоговая сводка

| # | CWE | Заголовок | Severity | Статус |
|---|-----|-----------|----------|--------|
| 1 | CWE-312 | Cleartext storage: SSH password + LLM api_key в config.yaml | Medium | Mitigated |
| 2 | CWE-295 | Host-key: явный `insecure_host_key` + TOFU без known_hosts | Medium | Mitigated |
| 3 | CWE-306 | MCP stdio JSON-RPC без аутентификации | Low | By design |
| 4 | CWE-77/78 | Command injection в SSH-командах | — | Не достижимо |
| 5 | — | LLM prompt-injection → RCE | — | Не достижимо (положительный дизайн) |
| 6 | CWE-1104 | Уязвимые зависимости | Low | 0 затрагивают код (govulncheck) |

---

## Findings

### 1. CWE-312 — Cleartext Storage of Sensitive Information · Medium · Mitigated
**Где:** `internal/config/config.go` — `Server.Password` (plaintext YAML), `LLM.APIKey`.
Оба секрета могут храниться открытым текстом в `~/.config/sshmon/config.yaml`.
**Смягчение (уже внедрено):**
- `config.SecretPermWarning(path, cfg)` предупреждает на stderr, если файл с секретом читается группой/миром (`mode&0o077 != 0`) — вызывается из `run()`.
- `LLM.APIKeyEnv` даёт indirection через переменную окружения (api_key можно не хранить в файле).
**Остаточный риск:** для SSH-пароля env-альтернативы нет. Рекомендация пользователю: `chmod 600`, предпочитать ssh-agent/ключ вместо пароля. Приемлемый компромисс для локального инструмента.
**Масок секретов не требуется — реальных значений в репозитории нет** (секреты появляются только в пользовательском конфиге в рантайме).

### 2. CWE-295 — Improper Host Key Validation · Medium · Mitigated
**Где:** `internal/sshx/sshx.go` — `hostKeyVerification(cfg, knownHostsPath)` / `hostKeyCallback`.
Два непроверяющих пути: (а) явный опт-ин `insecure_host_key: true`; (б) отсутствие `~/.ssh/known_hosts` → молчаливый TOFU.
**Смягчение (раунд-2 a):** ветка (б) теперь выдаёт одноразовое (`sync.Once`) предупреждение о риске MITM на stderr. Поведение сохранено (не блокирует старт). Ветка (а) — осознанный выбор пользователя. Несовпадение ключа по-прежнему жёстко ошибка (`knownhosts: key mismatch` → FriendlyErr).

### 3. CWE-306 — Missing Authentication (MCP stdio) · Low · By design
**Где:** `internal/mcpsrv/mcpsrv.go` — `Serve`/`loop`.
JSON-RPC 2.0 по stdio без аутентификации. Это **не сетевой listener** — сервер запускается родительским процессом и общается через stdin/stdout. Аутентификация — ответственность родителя. Остаточно: любой локальный процесс, унаследовавший тот же stdio, видит метрики. Приемлемо.
Дополнительно укреплено ранее: битый JSON → `-32700` (id=null) вместо тихого дропа; плохие аргументы инструмента → `isError` текст, а не `_ =`.

### 4. CWE-77/78 — Command Injection · Не достижимо
Все удалённые команды строятся из **констант, целых чисел и валидированных имён**:
- `internal/collect/collector.go` `TailLog` — `lines` это `int` в `fmt.Sprintf`; сервер ищется точным равенством `st.cfg.Name == server`, имя в команду не подставляется.
- `internal/collect/systemd.go` `systemdUnitsCommand` — каждое имя unit валидируется `safeLogName.MatchString` до склейки, иначе ошибка.
- `poll` использует константный `sampleCmd`.
Пользовательские строки (имена серверов/units) никогда не интерполируются в шелл без валидации. **Инъекция не проходит.**

### 5. LLM prompt-injection → RCE · Не достижимо (положительный дизайн)
**Где:** `internal/tui/chat.go`. Чат-оверлей строит системный контекст из снапшота метрик и отдаёт ответ LLM только на **отрисовку** (`renderChat`). Ответ модели **никогда не исполняется** как команда. Даже если удалённый хост вернёт вредоносный лог, «отравляющий» контекст, — исполняемого пути нет. Хороший дизайн, зафиксировать как есть.

### 6. CWE-1104 — Use of Unmaintained/Vulnerable Components · Low
`govulncheck ./...` (v.latest, 2026-07-21): **0 уязвимостей затрагивают код**; 1 уязвимость в требуемом модуле, но код её не вызывает. Все зависимости свежие (`golang.org/x/crypto v0.54.0`, `bubbletea v1.3.10`, `modernc.org/sqlite v1.54.0`). Рекомендация: добавить `govulncheck` в CI как регулярный gate.

---

## Транспорт и секреты (сводно)
- SSH-канал шифрован; порядок аутентификации `authMethods` = ssh-agent → ключ → пароль (как openssh); dial timeout 10s.
- LLM-вызовы (`internal/llm/llm.go`) по HTTPS, timeout 2 мин, ответ ограничен 1 MB.
- Grep по исходникам: **захардкоженных паролей/токенов/ключей нет** → `SECRETS.local.md` не требуется.

## Рекомендации (приоритет по убыванию)
1. **CI:** добавить `govulncheck ./...` как обязательный шаг.
2. **Док:** в README явно указать `chmod 600 ~/.config/sshmon/config.yaml` и рекомендовать ssh-agent/ключ вместо пароля.
3. **Опционально:** рассмотреть env-indirection и для SSH-пароля (симметрично `APIKeyEnv`).
