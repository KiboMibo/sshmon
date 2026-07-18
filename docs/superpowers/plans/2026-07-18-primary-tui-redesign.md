# Primary TUI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить глобальные вкладки sshmon на fleet-first TUI с проваливанием в сервер, постоянной историей метрик, read-only экранами процессов, портов и Docker, live-логами и глобальными оверлеями.

**Architecture:** Существующие agentless SSH-сборщик, headless MCP и LLM-клиенты сохраняются. Новый `internal/history` хранит raw-метрики и минутные rollup в SQLite через одного writer-а; `internal/tui` становится маршрутизатором явных экранов и оверлеев, а дорогие SSH-запросы живут только пока открыт соответствующий экран.

**Tech Stack:** Go 1.26.5, Bubble Tea v1.3.10, Bubbles v1.0.0, Lip Gloss v1.1.0, `database/sql`, `modernc.org/sqlite`, `golang.org/x/crypto/ssh`, YAML v3.

## Global Constraints

- Сохранить Bubble Tea/Bubbles/Lip Gloss v1 и существующий agentless SSH transport; не мигрировать на Bubble Tea v2.
- Сохранить контракты первого запуска, YAML servers/thresholds/LLM, `--headless`, stdio MCP и текущих LLM-провайдеров.
- Единственная новая зависимость — pure-Go `modernc.org/sqlite`; CGO запрещён.
- Экраны: Fleet, ServerDashboard, Processes, Ports, History, Logs, Containers. Оверлеи: Chat, Search, CommandPalette, Help.
- `server_key`: user без изменения регистра + `@` + hostname в lowercase + нормализованный порт (0→22); IPv6 форматировать эквивалентом `net.JoinHostPort`.
- История по умолчанию: `~/.local/share/sshmon/history.db`, raw 24h, минутные агрегаты 720h; значения настраиваются YAML.
- Offline samples хранят `online=false`, время и issues; числовые поля NULL, не входят в min/max/avg и создают разрывы графиков.
- Один history writer; UI читает отдельно; сбой SQLite не останавливает мониторинг.
- Core polling использует текущий interval; процессы — 2s только на Processes; порты/Docker — 5s только на своих экранах; live logs — отдельный cancellable stream.
- Processes и Docker read-only; без signals, start/stop/restart/exec.
- Live log buffer ограничен 10 000 строками; поддерживает pause, filter, source и reconnect.
- Минимум интерфейса 60×16; ниже показывать resize-state. Dashboard должен быть читаем на 80×24.
- Каждый production-файл целится в ≤250 чистых LOC; разделять по ответственности, без спекулятивных абстракций.
- Каждый production change проходит RED→GREEN, `gofmt`, `go vet`, `go test -race -shuffle=on -count=1`.

## File Map

- `internal/config/config.go`, `template.go`, tests — History YAML contract and defaults.
- `internal/history/{types,key,store,writer,query,retention}.go` and tests — SQLite ownership.
- `internal/collect/{collector,types,events,ondemand,parsers,logs}.go` and tests — snapshots, events and screen-scoped SSH data.
- `internal/sshx/sshx.go` and tests — context-aware exec/stream primitives.
- `internal/tui/{model,navigation,layout,fleet,dashboard,processes,ports,containers,history,logs,overlays,styles,sparkline}.go` and tests — explicit screens.
- `cmd/sshmon/main.go`, `README.md` — wiring and documentation.

---

### Task 1: History config, stable server keys, SQLite bootstrap

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `internal/config/config.go`, `internal/config/template.go`, `internal/config/config_test.go`
- Create: `internal/history/key.go`, `internal/history/key_test.go`
- Create: `internal/history/store.go`, `internal/history/store_test.go`

**Interfaces:**
- Produces: `config.History`, `history.ServerKey(config.Server) string`, `history.Open(path string) (*Store, error)`, `(*Store).Close() error`.

- [ ] **Step 1: Write RED config and key tests.** Add tests proving defaults and exact normalization:

```go
func TestServerKeyNormalizesHostAndPort(t *testing.T) {
    // Given
    server := config.Server{User: "Deploy", Host: "2001:DB8::1", Port: 0}
    // When
    got := ServerKey(server)
    // Then
    if got != "Deploy@[2001:db8::1]:22" { t.Fatalf("got %q", got) }
}

func TestLoadDefaultsHistoryRetention(t *testing.T) {
    // Given: minimal config with one server.
    // When: Load.
    // Then: enabled=true, raw=24h, aggregate=720h and default data path.
}
```

- [ ] **Step 2: Run RED.** Run `go test ./internal/config ./internal/history -run 'TestServerKey|TestLoadDefaultsHistory' -count=1`. Expected: compile failure because History/ServerKey do not exist.
- [ ] **Step 3: Add config contract.** Add:

```go
type History struct {
    Enabled            *bool         `yaml:"enabled,omitempty"`
    Path               string        `yaml:"path,omitempty"`
    RawRetention       time.Duration `yaml:"-"`
    RawRetentionText   string        `yaml:"raw_retention,omitempty"`
    AggregateRetention time.Duration `yaml:"-"`
    AggregateText      string        `yaml:"aggregate_retention,omitempty"`
}
```

Add `func (h History) IsEnabled() bool { return h.Enabled == nil || *h.Enabled }`. Parse durations in `Load`, default 24h/720h and expand `~`; an omitted `enabled` means true while explicit `false` remains disabled. Add equivalent fields to the built-in template without breaking `PopulateServers`.
- [ ] **Step 4: Implement `ServerKey`.** Default user/port like `config.Load`, lowercase host only, and use `net.JoinHostPort`.
- [ ] **Step 5: Add modernc driver and bootstrap schema.** Run `go get modernc.org/sqlite`. `Open` must create parent directory, open one writer connection, set WAL/NORMAL/busy timeout pragmas, and create:

```sql
CREATE TABLE IF NOT EXISTS metric_samples (
 server_key TEXT NOT NULL, sampled_at_ms INTEGER NOT NULL, online INTEGER NOT NULL,
 cpu_pct REAL, mem_pct REAL, disk_pct REAL, net_rx_bps REAL, net_tx_bps REAL,
 load1 REAL, issues_json TEXT NOT NULL,
 PRIMARY KEY(server_key, sampled_at_ms)
);
CREATE TABLE IF NOT EXISTS metric_rollups_minute (
 server_key TEXT NOT NULL, bucket_at_ms INTEGER NOT NULL, online_count INTEGER NOT NULL,
 sample_count INTEGER NOT NULL, cpu_min REAL, cpu_max REAL, cpu_avg REAL,
 mem_min REAL, mem_max REAL, mem_avg REAL, disk_min REAL, disk_max REAL, disk_avg REAL,
 net_rx_avg REAL, net_tx_avg REAL, load1_avg REAL,
 PRIMARY KEY(server_key, bucket_at_ms)
);
```

- [ ] **Step 6: GREEN verification.** Run `gofmt -w internal/config internal/history && go test -race -shuffle=on -count=1 ./internal/config ./internal/history` and `go build ./...`. Expected: PASS.
- [ ] **Step 7: Commit.** `git add go.mod go.sum internal/config internal/history && git commit -m "feat: добавить хранилище истории метрик"`.

### Task 2: History writer, rollups, retention, fail-soft service

**Files:**
- Create: `internal/history/types.go`, `writer.go`, `query.go`, `retention.go`
- Create: `internal/history/writer_test.go`, `query_test.go`, `retention_test.go`

**Interfaces:**
- Consumes: `*history.Store` from Task 1.
- Produces: `Sample`, `Point`, `Range`, `Service`, `NewService(*Store, Options) *Service`, `Write(context.Context, Sample) error`, `Query(context.Context, string, Range) ([]Point,error)`, `Maintain(context.Context,time.Time) error`.

- [ ] **Step 1: RED writer/query tests.** Use `t.TempDir()` SQLite. Insert online and offline samples; query must return ordered points, NULL numeric values for offline, and explicit gaps.
- [ ] **Step 2: RED rollup/retention tests.** Seed 61 seconds of samples plus old rows. Assert one minute bucket min/max/avg excludes offline values; raw older than 24h and rollups older than 720h are deleted.
- [ ] **Step 3: Run RED.** `go test ./internal/history -run 'TestWrite|TestQuery|TestRollup|TestRetention' -count=1`. Expected: missing Service methods.
- [ ] **Step 4: Implement bounded single writer.** `Service` owns `chan writeRequest` and one goroutine; `Write` respects caller context and returns `ErrClosed` after shutdown. SQL uses parameterized statements and `INSERT ... ON CONFLICT DO UPDATE`.
- [ ] **Step 5: Implement rollup transaction.** Group raw data by minute; use SQL aggregate functions over non-NULL metric values; store online_count/sample_count; commit atomically.
- [ ] **Step 6: Implement range query.** Define constants `Range1H`, `Range6H`, `Range24H`, `Range7D`, `Range30D`; use raw for ≤24h and rollups beyond. Return typed `Point{At,Online,CPU,Memory,Disk,NetRX,NetTX,Load1}` with nullable metrics represented by pointers.
- [ ] **Step 7: Implement fail-soft wrapper.** `OpenService(config.History) (*Service,error)` may fail; caller is allowed to continue with nil. Service errors are surfaced separately from server online status.
- [ ] **Step 8: GREEN and commit.** Run `gofmt -w internal/history && go test -race -shuffle=on -count=1 ./internal/history`; commit `feat: сохранять и агрегировать историю метрик`.

### Task 3: Collector event subscription and history sink

**Files:**
- Modify: `internal/collect/collector.go`, `internal/collect/types.go`
- Create: `internal/collect/events.go`, `internal/collect/events_test.go`, `internal/collect/history_test.go`

**Interfaces:**
- Produces: `type Event struct { Snapshot Snapshot }`, `(*Collector).Subscribe(buffer int) (<-chan Event, func())`, `(*Collector).RunWithSink(ctx context.Context, sink func(context.Context, Snapshot) error)` while preserving `Run(ctx)` and `Snapshot()`.

- [ ] **Step 1: RED subscription test.** Start a test collector/fake polling seam, subscribe with buffer 1, produce two snapshots, assert latest event is delivered and slow subscribers never block polling.
- [ ] **Step 2: RED history mapping test.** Map each server Metrics to one `history.Sample`; verify stable key, offline NULL metrics, issue JSON, and snapshot timestamp.
- [ ] **Step 3: Run RED.** `go test ./internal/collect -run 'TestSubscribe|TestHistory' -count=1`. Expected: undefined API.
- [ ] **Step 4: Implement fan-out.** Under mutex copy subscriber channels; use non-blocking send with latest-value replacement. Unsubscribe closes only after removal.
- [ ] **Step 5: Implement optional sink.** `Run` delegates to `RunWithSink(ctx,nil)`. Sink errors are reported through a health field/event but do not mark SSH servers offline or stop polling.
- [ ] **Step 6: GREEN/full compatibility.** `go test -race -shuffle=on -count=1 ./internal/collect ./internal/mcpsrv && go build ./...`.
- [ ] **Step 7: Commit.** `git commit -am "feat: публиковать снимки коллектора"` plus new tests.

### Task 4: Context-aware SSH and read-only diagnostics parsers

**Files:**
- Modify: `internal/sshx/sshx.go`
- Create: `internal/sshx/sshx_test.go`
- Create: `internal/collect/diagnostics.go`, `parsers.go`, `parsers_test.go`

**Interfaces:**
- Produces: `(*sshx.Client).RunContext(context.Context,string) (string,error)`, `Process`, `Container`, extended `Port`, `ParseProcesses`, `ParseContainers`, and collector methods `Processes`, `Containers`, `Ports` accepting context/server.

- [ ] **Step 1: RED parser table tests.** Fixtures for GNU/BusyBox `ps`, `docker ps --format`, `docker stats --no-stream --format`, and `ss -tulpn`; malformed lines are skipped, unavailable command returns typed `ErrUnsupported`.
- [ ] **Step 2: RED cancellation test.** Inject a session runner that blocks; cancel context and assert `RunContext` returns `context.Canceled` and drops the broken connection.
- [ ] **Step 3: Implement RunContext.** Keep existing `Run(cmd,timeout)` by wrapping `context.WithTimeout`; share one private execution path.
- [ ] **Step 4: Implement read-only commands.** Processes use PID/command/%CPU/%MEM; Containers combine `docker ps` and one-shot stats; Ports preserve protocol/local/process and PID where available. No mutating commands.
- [ ] **Step 5: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/sshx ./internal/collect && go build ./...`; commit `feat: собирать процессы и контейнеры по SSH`.

### Task 5: Live SSH log stream and bounded log buffer

**Files:**
- Create: `internal/sshx/stream.go`, `stream_test.go`
- Create: `internal/collect/logs.go`, `logs_test.go`

**Interfaces:**
- Produces: `Stream{Lines <-chan string, Errors <-chan error, Close func() error}`, `(*sshx.Client).StreamContext(ctx,cmd)`, `LogSource`, `LogRequest`, `Collector.StreamLogs`, `LogBuffer` with max 10_000 lines and filter/pause.

- [ ] **Step 1: RED buffer tests.** Append 10_005 lines, assert oldest five evicted; pause prevents viewport advance but retains bounded input; substring filter is case-insensitive and reversible.
- [ ] **Step 2: RED stream cancellation/reconnect tests.** Fake scanner emits lines then error; context cancellation closes channels once; new request gets a distinct request ID.
- [ ] **Step 3: Implement SSH stream.** Use `StdoutPipe`, `StderrPipe`, `Session.Start`, scanner goroutine, `Wait`, context-driven session close; never leak goroutines.
- [ ] **Step 4: Implement source commands.** journalctl unit, syslog/messages, Docker container logs; validate source/container names against discovered values, never interpolate arbitrary shell text.
- [ ] **Step 5: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/sshx ./internal/collect`; commit `feat: добавить потоковый просмотр логов`.

### Task 6: Root TUI navigation and adaptive layout contract

**Files:**
- Replace: `internal/tui/tui.go`
- Create: `internal/tui/model.go`, `navigation.go`, `layout.go`, `messages.go`, `styles.go`
- Create: `internal/tui/navigation_test.go`, `layout_test.go`

**Interfaces:**
- Produces: `screenKind` values Fleet/Dashboard/Processes/Ports/History/Logs/Containers; `overlayKind`; `Model` preserving public `New(*collect.Collector,*llm.Client,*config.Config) Model`.

- [ ] **Step 1: RED navigation tests.** Enter Fleet→Dashboard; Esc deep→Dashboard→Fleet; `p/o/h/l/d` only from Dashboard; `c`, `/`, `:`, `?` open overlays; Esc closes overlay first; q exits only Fleet without overlay.
- [ ] **Step 2: RED layout tests.** Window 59×15 returns resize view; 60×16 Fleet list-only; wide Fleet split; 80×24 Dashboard readable; resize preserves selected server/screen.
- [ ] **Step 3: Implement screen stack and overlay precedence.** One root model owns selected server key, screen, overlay, request generation and dimensions. Child renderers are pure functions/components, not independent programs.
- [ ] **Step 4: Subscribe to collector.** Bubble Tea command waits for next collector event then re-arms. Keep one-second fallback tick only for age labels.
- [ ] **Step 5: GREEN, LOC check, commit.** `go test -race -shuffle=on -count=1 ./internal/tui`; measure each production file ≤250 pure LOC; commit `refactor: заменить вкладки маршрутизацией экранов`.

### Task 7: Fleet list, preview, search and filters

**Files:**
- Create: `internal/tui/fleet.go`, `fleet_test.go`, `filter.go`, `filter_test.go`, `sparkline.go`, `sparkline_test.go`

**Interfaces:**
- Produces: `fleetModel`, `fleetFilter{Query,Group,ProblemsOnly}`, pure `filterServers`, `sparkline([]float64,width int) string`.

- [ ] **Step 1: RED filter/navigation tests.** Preserve config order; j/k/Pg clamp; search matches name/host/group; group cycles deterministically; problems-only uses Issues; selection moves to nearest surviving row.
- [ ] **Step 2: RED render tests.** Wide layout contains dense list and selected preview; narrow layout hides preview; status uses color plus distinct glyph; stale age visible; no tabs/numeric navigation.
- [ ] **Step 3: Implement Fleet.** Columns: state/name/group/CPU/MEM/disk/load/age. Preview: hostname, uptime, issue summary, CPU/memory/network sparklines from history or current fallback.
- [ ] **Step 4: Implement Fleet keys.** `/` search overlay, `g` group, `!` problems, `v` preview, Enter dashboard.
- [ ] **Step 5: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui`; commit `feat: добавить экран списка серверов`.

### Task 8: Server dashboard panels and sparklines

**Files:**
- Create: `internal/tui/dashboard.go`, `dashboard_test.go`, `panels.go`, `panels_test.go`

**Interfaces:**
- Consumes current Metrics, Issues, short history.
- Produces pure render panels for CPU/load, memory/swap, disks/IO, network, identity/uptime/issues.

- [ ] **Step 1: RED dashboard tests.** At 80×24 all mandatory sections and deep-screen hints fit; at wide width panels expand; offline/stale retains last success and age; unsupported subfeature does not show server offline.
- [ ] **Step 2: RED sparkline/gauge tests.** Clamp percentages, preserve gaps, Unicode width exact, empty series renders placeholder.
- [ ] **Step 3: Implement bounded panels.** Use socktop-style density but project styles; panels receive explicit rectangles and never write outside width/height.
- [ ] **Step 4: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui`; commit `feat: добавить дашборд выбранного сервера`.

### Task 9: Processes, Ports and Containers screens

**Files:**
- Create: `internal/tui/processes.go`, `processes_test.go`, `ports.go`, `ports_test.go`, `containers.go`, `containers_test.go`, `requests.go`, `requests_test.go`

**Interfaces:**
- Consumes Task 4 collector methods.
- Produces screen models with request IDs, cancellation, 2s/5s cadence and read-only sortable tables.

- [ ] **Step 1: RED sorting/render tests.** Processes sort CPU/MEM/PID/name; ports proto/local/process/PID; containers name/image/state/CPU/MEM. Stable tie-breakers required.
- [ ] **Step 2: RED request lifecycle tests.** Enter screen starts request; leaving cancels; stale response ID is ignored; processes re-arm at 2s; ports/containers at 5s only while active.
- [ ] **Step 3: Implement reusable request generation.** Each screen stores `generation uint64`, cancel func, loading/ready/stale/unsupported/error state and last success.
- [ ] **Step 4: Implement screens.** Read-only; Docker detail/log action may navigate to Logs with validated container source; no control keys.
- [ ] **Step 5: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui ./internal/collect`; commit `feat: добавить экраны процессов портов и Docker`.

### Task 10: History screen

**Files:**
- Create: `internal/tui/history.go`, `history_test.go`

**Interfaces:**
- Consumes `history.Service.Query`.
- Produces ranges 1h/6h/24h/7d/30d, metric selector, graph cursor and gap-aware renderer.

- [ ] **Step 1: RED range/query tests.** Entry queries selected server/range; changing range increments request ID; stale query ignored; new minute bucket refreshes only active screen.
- [ ] **Step 2: RED graph tests.** Offline points create gaps, cursor returns exact timestamp/value, min/max labels fit, 30d rollup points render.
- [ ] **Step 3: Implement History.** Keys `1..5` ranges, j/k metric, h/l cursor, r refresh, Esc dashboard. SQLite errors show fail-soft state without affecting server online status.
- [ ] **Step 4: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui ./internal/history`; commit `feat: добавить экран истории метрик`.

### Task 11: Live Logs screen

**Files:**
- Create: `internal/tui/logs.go`, `logs_test.go`

**Interfaces:**
- Consumes `Collector.StreamLogs`, `LogBuffer`.
- Produces source selector, viewport, pause/filter/reconnect controls.

- [ ] **Step 1: RED state tests.** Open starts stream; space pauses display; `/` filters; `s` cycles source; `r` cancels old and reconnects; Esc cancels; old request lines ignored.
- [ ] **Step 2: RED bounded rendering test.** Feed >10k lines, assert buffer bound and cursor/viewport remain valid while paused/resized.
- [ ] **Step 3: Implement Logs.** Bubble Tea command reads one stream event and re-arms. Footer shows source/pause/filter/line count/reconnect. Preserve last lines on transient error with age/state.
- [ ] **Step 4: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui ./internal/collect`; commit `feat: добавить экран потоковых логов`.

### Task 12: Global Chat, Search, Palette and Help overlays

**Files:**
- Create: `internal/tui/overlays.go`, `overlays_test.go`, `chat.go`, `chat_test.go`, `palette.go`, `palette_test.go`

**Interfaces:**
- Consumes existing LLM Client and collector snapshot.
- Produces modal overlay rendering/input with focus ownership and command actions.

- [ ] **Step 1: RED overlay precedence tests.** Overlay captures keys before screen; Esc closes it; resize preserves content; only one overlay active.
- [ ] **Step 2: RED chat context test.** System context includes fleet snapshot, selected server (if any), active screen and current subfeature state/error, but no persisted chat history.
- [ ] **Step 3: RED palette/help tests.** Palette searches available actions and servers; unavailable actions excluded; help derives key hints from active screen.
- [ ] **Step 4: Implement overlays.** `c` chat, `/` search, `:` palette, `?` help; asynchronous LLM reply carries request ID and honors cancellation.
- [ ] **Step 5: GREEN and commit.** `go test -race -shuffle=on -count=1 ./internal/tui`; commit `feat: добавить глобальные оверлеи TUI`.

### Task 13: Wiring, compatibility, documentation and PTY E2E

**Files:**
- Modify: `cmd/sshmon/main.go`, `README.md`
- Modify: `internal/config/template.go`
- Test: `cmd/sshmon/main_test.go` or PTY script under approved temporary directory only.

**Interfaces:**
- Wires optional history service into collector/TUI without changing headless MCP behavior.

- [ ] **Step 1: RED wiring tests.** History disabled or SQLite open failure still launches collector/TUI; enabled history persists snapshots; headless initialize/tools responses unchanged; first-run template round-trip retains history defaults.
- [ ] **Step 2: Implement main wiring.** Open history after config load, log one user-facing warning on failure, defer close, pass sink/query dependency to collector/TUI. Preserve current public `tui.New` only if practical; otherwise introduce an `Options` struct and update one caller.
- [ ] **Step 3: Update README.** Document Fleet/drill-down keys, screens, overlays, history path/retention, Docker/process read-only behavior and fail-soft history.
- [ ] **Step 4: Run full automated gates.** Run:

```bash
gofmt -l cmd internal
go build ./...
go vet ./...
go test -race -shuffle=on -count=1 ./...
go build -o sshmon ./cmd/sshmon
git diff --check
```

Expected: every command exit 0, gofmt/diff output empty.
- [ ] **Step 5: PTY E2E at three sizes.** Under `/var/folders/q8/ql4_4q7d6yng80qqr1smz_m00000gp/T/opencode/sshmon-primary-tui-e2e`, use fake SSH fixtures and `/usr/bin/expect` to verify:
  - 80×24 Dashboard readable and Esc returns Fleet;
  - 120×30 Fleet split preview and deep screens;
  - 160×40 overlays and panels;
  - 59×15 resize-state;
  - no global tabs;
  - processes/ports/Docker unsupported states do not mark server offline;
  - log pause/filter/reconnect and bounded buffer;
  - history survives restart and offline gap renders;
  - `--headless` MCP initialize/tools remains compatible.
- [ ] **Step 6: Remove all PTY fixtures.** `rm -rf` the approved temporary directory; verify git status has no artifacts and `.opencode/` is not staged.
- [ ] **Step 7: Review file sizes and branch diff.** Measure pure LOC for every new production file, inspect `git diff --stat`, `git diff --check`, and scan for secrets/debug prints.
- [ ] **Step 8: Commit docs/wiring.** `git add cmd/sshmon/main.go internal/config/template.go README.md` plus direct wiring tests; commit `docs: описать новый основной TUI` (use `feat:` instead if wiring is substantial and not already committed with its task).
- [ ] **Step 9: Committed-tree gates.** Repeat Step 4 and run final broad review against the branch base.

## Plan Self-Review

- Spec coverage: Fleet, Dashboard, all five deep screens, four overlays, adaptive sizes, SQLite raw/rollup retention, fail-soft behavior, on-demand cadences, cancellation/request IDs, read-only constraints, live logs, headless/MCP and first-run compatibility each map to an explicit task.
- File boundaries: history persistence, SSH collection, TUI navigation, each screen and overlays are independently testable; no planned production file needs mixed responsibilities.
- Type consistency: `history.ServerKey`, `history.Service`, collector subscription/sink, context-aware diagnostics, `screenKind`/`overlayKind`, request generations and public main wiring are defined before consumers.
- Scope: no Bubble Tea v2, remote agent, ProxyJump, process signals, Docker mutations, chat persistence or mandatory MCP history extension.
- Verification: every task contains RED, expected failure, minimal GREEN, race/shuffle tests and an atomic commit; Task 13 supplies multi-size PTY and committed-tree gates.
- Placeholder scan: completed; the plan contains no deferred work markers or vague implementation instructions.

## Execution Handoff

Create an isolated worktree from the commit containing this plan. Execute sequentially with `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`; do not run history, collector and TUI implementation workers concurrently because later tasks consume exact interfaces from earlier tasks.
