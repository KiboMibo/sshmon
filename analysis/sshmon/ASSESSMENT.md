# sshmon — Architecture Assessment

> Single-system modernization assessment. Tooling: `find`+`wc`+`codegraph` (scc/cloc/lizard unavailable on host). All file:line references verifiable via `.codegraph/`.

## Executive Summary

`sshmon` is a **modern, well-structured Go 1.26 TUI + MCP server for SSH server monitoring** — not a legacy system. ~6.8k SLOC across 52 source files with an unusually high 84.6% test-file ratio. Architecture is clean: `cmd/` → 8 focused `internal/` packages with clear seams between SSH transport (`sshx`), metrics collection (`collect`), history persistence (`history`), LLM chat (`llm`), MCP server (`mcpsrv`), and TUI (`tui`). Tech debt is **low-to-moderate and localized**, not systemic.

**Headline recommendation: Refactor in place** — not modernization. Highest-value actions: (1) split the 3.3k-LOC `internal/tui/` mega-package, (2) add tests for the two untested network-facing packages (`mcpsrv`, `llm`), (3) extract business logic out of `cmd/sshmon/main.go`, (4) stop committing the 18 MB binary, (5) harden error paths that currently call `os.Exit()` from inside helpers.

## System Inventory

| Metric | Value |
|---|---|
| Language | Go 1.26.5 (`go.mod`) |
| Source files (non-test) | 52 |
| Test files | 44 (84.6% file coverage) |
| Source LOC | 6,781 |
| Largest source file | `internal/sshx/sshx.go` — 295 LOC |
| Packages under `internal/` | 8 |
| Dependencies (direct) | 7 (`charmbracelet/*` TUI stack, `modernc.org/sqlite`, `golang.org/x/crypto`, `gopkg.in/yaml.v3`) |
| Binary committed to repo | ⚠️ yes, 18 MB `sshmon` at root |

### Package size heat-map

| Package | Files | Tests | LOC | Notes |
|---|---|---|---|---|
| `internal/tui` | 27 | 23 | 3,272 | ⚠️ **Mega-package** — half the codebase |
| `internal/collect` | 8 | 7 | 1,168 | Healthy |
| `internal/setup` | 3 | 1 | 455 | First-run wizard |
| `internal/history` | 6 | 5 | 425 | SQLite + async writer |
| `internal/config` | 3 | 3 | 507 | YAML config |
| `internal/sshx` | 2 | 4 | 391 | SSH client wrapper |
| `internal/mcpsrv` | 1 | **0** | 199 | ⚠️ **No tests** |
| `internal/llm` | 1 | **0** | 139 | ⚠️ **No tests** |
| `cmd/sshmon` | 1 | 1 | 225 | ⚠️ Logic that belongs in `internal/` |

### Technology fingerprint

- **Build**: `go build` / `go test` (no Makefile, no CI config in tree)
- **TUI**: `charmbracelet/bubbletea` v1.3.10 + `bubbles` + `lipgloss` v1.1.0
- **Storage**: `modernc.org/sqlite` v1.54.0 (pure-Go SQLite, WAL mode, single conn)
- **SSH**: `golang.org/x/crypto` v0.54.0 (`ssh`, `ssh/agent`)
- **Config**: YAML (`gopkg.in/yaml.v3`), default `~/.config/sshmon/config.yaml`
- **Transport (headless)**: stdio NDJSON JSON-RPC 2.0 (hand-rolled MCP, no SDK)
- **Tests**: stdlib `testing` only — no testify, no fixtures lib

## Architecture-at-a-Glance

```
cmd/sshmon/main.go
  ├─ config.Load (~/.config/sshmon/config.yaml)
  ├─ history.OpenService (~/.local/share/sshmon/history.db)  [SQLite, WAL]
  ├─ collect.New(cfg) ──► goroutine: poll servers every interval
  │     ├─ sshx.Client per server (agent → key → password)
  │     ├─ parse.go: /proc/meminfo, df, ss, netstat, iostat
  │     └─ events.go: publish Snapshot to subscribers (TUI) + HistorySink
  ├─ mode = headless?
  │    yes ► mcpsrv.Serve(ctx, col)             [stdio JSON-RPC]
  │    no  ► tui.New(col, llm.New(cfg), cfg)    [Bubble Tea AltScreen]
  │           └─ chat overlay: llm.Client.{openai|anthropic}
  └─ ctx from signal.NotifyContext (SIGINT, SIGTERM)
```

**Domain dependency direction is acyclic and clean**: `cmd → {collect, config, history, llm, mcpsrv, setup, tui}`; `collect → {config, history, sshx}`; `tui → {collect, config, history, llm}`; `mcpsrv → collect`. No cycles. No leaky upward deps.

## Production Runtime Profile

No telemetry available (no observability/APM MCP server connected, no batch logs supplied). **Gap noted.** Adding even stdout structured logging from `poll()` with p95 per-server would materially improve operational insight — currently the only signal that a server is slow is the 15 s hard timeout in `sshx.Client.Run` (`collector.go:99`).

## Technical Debt (ranked by remediation value)

| # | Finding | Evidence | Impact |
|---|---|---|---|
| 1 | **`internal/tui/` is a god package** — 27 files, 3.2k LOC, one `package tui` doing fleet, dashboard, processes, ports, logs, history, chat, palette, overlays, navigation, reconnect, sparkline. | `find internal/tui -name '*.go' \| wc -l` → 27 | High — every change touches an enormous compilation unit; test fixtures sprawl; import graph flat. Split by feature: `tui/fleet`, `tui/dashboard`, `tui/chat`, `tui/screens`, `tui/shared`. |
| 2 | **Committed 18 MB binary in repo root** | `ls -la sshmon` (18,676,770 B, Jul 21) | High — bloats every clone, breaks reproducible builds, almost certainly accidental. Add `/sshmon` to `.gitignore`, `git rm --cached sshmon`. |
| 3 | **`os.Exit()` called from inside helpers** defeats `defer` cleanup in `main()`. | `cmd/sshmon/main.go:46, 62, 157, 161, 171, 177, 197, 207, 213, 224` (10 sites) | High — when `firstRun()`/`importServers()` exit, `runtime.Wait()` and `historyService.Close()` (deferred at main:90-98) never run. History writes in flight are lost; SSH sessions may leak. Return errors up the stack instead. |
| 4 | **`mcpsrv` has zero tests** despite being network-facing (parses untrusted stdin). | `find internal/mcpsrv -name '*_test.go'` → empty; codegraph blast-radius: `Serve`, `handle`, `TailLog` all flagged `⚠️ no covering tests found` | High — a regression in JSON parsing or tool dispatch ships silently. `loop()` reads global `os.Stdin`/`os.Stdout` with no injection seam. Refactor to `func loop(in io.Reader, out io.Writer, col *collect.Collector) error` and the file becomes trivially testable. |
| 5 | **`llm` has zero tests** despite making real HTTP calls with money consequences. | `find internal/llm -name '*_test.go'` → empty | High — any change to request shape, header set, or response parsing ships unverified. Introduce `httptest.Server`-based tests for both `openai` and `anthropic` paths. |
| 6 | **Silent JSON unmarshal failures in MCP handler** — malformed requests vanish without a trace. | `internal/mcpsrv/mcpsrv.go:55-57` (continue on unmarshal err), `:75` (`_ =` on initialize params), `:142` (`_ =` on tool args) | Medium — clients debugging "why didn't sshmon respond" get no help. At minimum log to stderr; ideally return `-32602 Invalid params`. |
| 7 | **Hardcoded constants duplicated across packages** | `"0.3.0"` at `main.go:28` AND `mcpsrv.go:82`; `"2025-03-26"` MCP version `mcpsrv.go:77`; `2 * time.Minute` HTTP timeout `llm.go:28`; `1<<20` (1 MB) response cap `llm.go:61`; `2048` Anthropic max_tokens `llm.go:115` | Medium — version skew on release, no way to tune HTTP timeout for slow models, silent truncation of large completions. Centralize in `internal/config` or a `buildinfo` package. |
| 8 | **`publish()` drop policy is racy and undocumented** — when a subscriber's buffer is full, publisher tries to drop one old event and retry; two concurrent publishers can both succeed at dropping, then both fail to push. | `internal/collect/events.go:45-62` | Medium — TUI subscriber can silently lose the only snapshot where a server flipped offline. Either (a) block on push (acceptable — subscriber is fast), or (b) make the drop atomic under the lock (drop+push inside one `c.mu` critical section). |
| 9 | **`Snapshot()` shares inner slices** (`Disks`, `Ports`, `Net`, `IO`) by reference with internal state. | `internal/collect/collector.go:162-172` (comment admits "Слайсы внутри Metrics разделяются с внутренним состоянием: только чтение") | Medium — documented but unenforced. Any caller that appends/reuses one of these slices corrupts collector state. Either copy them in `Snapshot()` (cheap at these sizes) or return an interface that only exposes len/at-index. |
| 10 | **Mixed-language user surface with no i18n hook** — Russian user-facing strings and comments interleaved with English identifiers. | Throughout (`mcpsrv.go` descriptions, `config.go` errors, `tui/chat.go` prompts) | Low — fine for the current audience, but blocks international contributors and localizers. If ever intended for wider distribution, extract strings to a `messages.go` map now while the surface is small. |

## Security Findings

| Severity | CWE | Finding | Evidence |
|---|---|---|---|
| ⚠️ Medium | CWE-256 | **Plaintext SSH password in config YAML** | `internal/config/config.go:21` (`Password string yaml:"password,omitempty"`); consumed at `sshx.go:209-211` (`ssh.Password(cfg.Password)`) |
| ⚠️ Medium | CWE-256 | **Plaintext LLM API key in config YAML** (mitigated by `api_key_env` alternative) | `internal/config/config.go:32-33` (`APIKey`/`APIKeyEnv`); resolved via `LLM.Key()` at `:36-44` |
| ⚠️ Low | CWE-295 | **`InsecureHostKey` opt-in disables host-key verification** | `internal/config/config.go:22`; behavior depends on `hostKeyCallback` (not fully traced in this pass — recommend follow-up audit) |
| ℹ️ Info | CWE-311 | **MCP stdio transport has no auth** — assumes trusted parent process. Acceptable per MCP spec but should be documented in `--help`. | `internal/mcpsrv/mcpsrv.go:34-66` |
| ✅ Good | — | **Passphrase handled correctly**: `[]byte` (not string), zeroed before reassignment. | `internal/sshx/sshx.go:70-76` (`clear(c.passphrase)` then append) |
| ✅ Good | — | **Response body capped** at 1 MiB in LLM client (limits unbounded memory on hostile upstream). | `internal/llm/llm.go:61` (`io.LimitReader(resp.Body, 1<<20)`) — though cap is undocumented; see Debt #7 |
| ✅ Good | — | **SQLite configured correctly**: WAL + `synchronous=NORMAL` + single conn (avoids the classic `database is locked`). | `internal/history/store.go:24, 38-39` |

**No hardcoded credentials observed** in scanned source. SECRETS.local.md not written (nothing to inventory).

## Documentation Gaps

1. **No operational runbook** — what to do when a server stays offline, how to rotate an API key, how to migrate the SQLite schema. `README.md` (6.2 KB) covers config + usage but not operations.
2. **`Run()` non-zero-exit-with-output-is-success is undocumented externally** (`sshx.go:99-100, 124-130`). Intentional for `journalctl || tail syslog || ...` chains, but a maintainer reading just the code will call it a bug.
3. **The subscriber drop contract in `publish()` is undocumented** (Debt #8) — callers of `Subscribe()` have no way to know they may have missed events.
4. **`Snapshot()` aliasing contract is a comment inside the implementation** (Debt #9), not in the public `Snapshot` docstring visible to consumers in `tui/` and `mcpsrv/`.
5. **No ADRs** — the choice of hand-rolled MCP over the Go SDK, pure-Go SQLite over cgo, sync-over-async history writer — all worth recording. `docs/superpowers/` has design specs for features but not architecture decisions.

## Relative Scale (COCOMO-II index — **not a timeline**)

- KSLOC = 6.781
- COCOMO-II basic (nominal scale factors): `2.94 × (KSLOC)^1.10` ≈ **2.94 × 8.40** ≈ **24.7 person-months (traditional team)**
- **This is a relative size index for ranking against other systems, NOT an estimate of how long modernization will take or cost.** It assumes traditional human-team productivity, which agentic transformation does not follow. Do not attach a date, duration, or budget to this number.

For context: at ~6.8 k SLOC this sits in the "small system" band — an order of magnitude smaller than the typical "legacy modernization" target. Most of the value here is in **preventing** tech debt accumulation, not in excavating existing debt.

## Recommended Modernization Pattern

### **Refactor in place** (route to: incremental refactoring, no `/modernize-*` stage needed)

**Rationale.** This is a clean modern Go codebase. There is no legacy stack to migrate off, no architectural decay to reverse, no schema to extract. The recommended actions are ordinary refactoring tickets, not a modernization program:

1. **Split `internal/tui/`** into feature sub-packages (1–2 days). Biggest single win for maintainability.
2. **Add tests for `mcpsrv` + `llm`** by injecting `io.Reader/Writer` and `*http.Client` (1 day). Closes the two coverage holes that matter most.
3. **Move logic out of `cmd/sshmon/main.go`** into `internal/app/` — `firstRun`, `importServers`, `maintainHistory` become testable functions returning `error` instead of calling `os.Exit` (half day).
4. **Untrack the committed binary** and add a `.gitignore` entry (10 minutes, but do it first — it's polluting every clone).
5. **Centralize constants** — version, timeouts, response caps — into `internal/config` or a new `internal/buildinfo` (half day).
6. **Tighten `publish()` and copy slices in `Snapshot()`** per Debt #8/#9 (half day).
7. **Consider adopting a Go MCP SDK** (e.g. `github.com/mark3labs/mcp-go` or `github.com/modelcontextprotocol/go-sdk`) instead of the 199-LOC hand-rolled protocol — only if the maintenance burden of the hand-rolled version starts to grow.

None of these block shipping. Sequence them opportunistically alongside feature work.
