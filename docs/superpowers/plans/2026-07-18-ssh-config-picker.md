# Иерархический выбор SSH-серверов — план реализации

> **Для агентных исполнителей:** ОБЯЗАТЕЛЬНЫЙ ПОДНАВЫК: используйте `superpowers:subagent-driven-development` (рекомендуется) или `superpowers:executing-plans`, выполняя задачи по одной. Шаги используют чекбоксы `- [ ]` для отслеживания.

**Цель:** заменить переполняющий терминал плоский список на прокручиваемое дерево SSH-конфигов с выбором целого файла или отдельных хостов.

**Архитектура:** парсер добавит каждому `SSHHost` устойчивую идентичность источника (`SourcePath`, `Position`) и вычисленную группу. Setup-TUI будет разделён на запуск, модель дерева и представление; модель хранит выбор независимо от раскрытия, а `bubbles/viewport` показывает только доступную высоту терминала.

**Технологии:** Go 1.26.5, Bubble Tea v1.3.10, Bubbles viewport v1.0.0, Lip Gloss v1.1.0, стандартный `testing`, `/usr/bin/expect` для PTY-проверки.

## Глобальные ограничения

- Сохранять Bubble Tea/Bubbles/Lip Gloss на v1; миграция на v2 запрещена.
- Не добавлять зависимости: `github.com/charmbracelet/bubbles/viewport` уже есть в модуле.
- Корневой `~/.ssh/config` всегда даёт группу `main`.
- Include-файл даёт группу из basename без завершающего `.conf`: `prod.conf` → `prod`.
- Путь источника и позиция хоста остаются внутренними; YAML и `config.Server` не получают служебных полей.
- Порядок источников и хостов сохраняется; одинаковые алиасы из разных источников не объединяются.
- Совпавший Include-файл, который нельзя прочитать, должен вернуть явную ошибку; отсутствие совпадений не является ошибкой.
- Отмена не пишет YAML; сохранение разрешено только при непустом выборе.
- Не добавлять `ProxyJump`, `ProxyCommand` и полную семантику OpenSSH.
- Не менять основной мониторинговый TUI.
- Каждый production-шаг проходит RED → GREEN → refactor, `gofmt`, race/shuffle-тесты и отдельный коммит.

---

## Карта файлов

- `internal/config/sshconf.go` — разбор SSH-конфигов, идентичность источника, порядок и ошибки Include.
- `internal/config/sshconf_test.go` — контракт парсера и преобразования в `config.Server`.
- `internal/setup/setup.go` — только публичный `Run` и запуск Bubble Tea.
- `internal/setup/model.go` — дерево, видимые строки, выбор, навигация и resize.
- `internal/setup/view.go` — терминальная псевдографика, обрезка строк и viewport-контент.
- `internal/setup/model_test.go` — чистые тесты состояний и клавиатурных переходов модели.
- `README.md` — новые клавиши и поведение свёрнутого дерева.
- `docs/superpowers/specs/2026-07-18-ssh-config-picker-design.md` — утверждённый контракт; не изменять при реализации без нового согласования.

### Задача 1: идентичность источника и строгие ошибки Include

**Файлы:**
- Изменить: `internal/config/sshconf.go:10-147`
- Изменить: `internal/config/sshconf_test.go:1-94`

**Интерфейсы:**
- Производит: `SSHHost{SourcePath string, Position int, Group string}`.
- Сохраняет: `ParseSSHConfig(path string) ([]SSHHost, error)` и `HostsToServers([]SSHHost) []Server`.
- Используется далее: `setup.newModel` группирует строго по `SourcePath`, а `Group` переносит в итоговый `Server`.

- [ ] **Шаг 1: расширить fixture и написать падающие тесты идентичности**

  В `TestParseSSHConfig` добавить второй Include-файл с тем же алиасом, а после существующих проверок — проверки корневой группы, абсолютного источника, позиции и порядка:

  ```go
  // Given: root config and two included files contain literal hosts,
  // including duplicate aliases in different source files.
  rootPath, err := filepath.Abs(main)
  if err != nil {
      t.Fatal(err)
  }
  prodPath, err := filepath.Abs(filepath.Join(dir, "conf.d", "prod.conf"))
  if err != nil {
      t.Fatal(err)
  }

  // Then: source identity, output groups, and declaration order are stable.
  if byAlias["web1"].Group != "main" {
      t.Fatalf("root group = %q, want main", byAlias["web1"].Group)
  }
  if byAlias["web1"].SourcePath != rootPath {
      t.Fatalf("root source = %q, want %q", byAlias["web1"].SourcePath, rootPath)
  }
  if byAlias["prod-app"].SourcePath != prodPath || byAlias["prod-app"].Group != "prod" {
      t.Fatalf("included host identity = %#v", byAlias["prod-app"])
  }
  for i, host := range hosts {
      if host.Position < 0 {
          t.Fatalf("host %d has invalid position %d", i, host.Position)
      }
  }
  ```

  Не строить единственную `map[alias]SSHHost` для проверки дубликатов: найти их проходом по `hosts` и доказать, что у двух элементов разные `SourcePath` или `Position`.

- [ ] **Шаг 2: написать падающий тест ошибки совпавшего Include**

  Добавить отдельный тест:

  ```go
  func TestParseSSHConfigReturnsMatchedIncludeReadError(t *testing.T) {
      // Given: an Include glob matches a directory, which cannot be read as a file.
      dir := t.TempDir()
      includedDir := filepath.Join(dir, "conf.d", "broken.conf")
      if err := os.MkdirAll(includedDir, 0o755); err != nil {
          t.Fatal(err)
      }
      main := filepath.Join(dir, "config")
      if err := os.WriteFile(main, []byte("Include conf.d/*.conf\n"), 0o600); err != nil {
          t.Fatal(err)
      }

      // When: parsing follows the matched Include.
      _, err := ParseSSHConfig(main)

      // Then: the matched source error is returned instead of being swallowed.
      if err == nil || !strings.Contains(err.Error(), "broken.conf") {
          t.Fatalf("error = %v, want included path", err)
      }
  }
  ```

- [ ] **Шаг 3: запустить RED**

  Выполнить:

  ```bash
  go test ./internal/config -run 'TestParseSSHConfig' -count=1
  ```

  Ожидается FAIL: `SSHHost` не имеет `SourcePath`/`Position`, корневая группа пуста, а ошибка Include сейчас проглатывается.

- [ ] **Шаг 4: минимально изменить парсер**

  Привести тип к точному контракту:

  ```go
  type SSHHost struct {
      Alias      string
      HostName   string
      User       string
      Port       int
      Key        string
      Group      string
      SourcePath string
      Position   int
  }
  ```

  В `ParseSSHConfig` сначала нормализовать корень и передать группу `main`:

  ```go
  func ParseSSHConfig(path string) ([]SSHHost, error) {
      sourcePath, err := filepath.Abs(path)
      if err != nil {
          return nil, fmt.Errorf("normalize SSH config %q: %w", path, err)
      }
      return parseSSHFile(filepath.Clean(sourcePath), "main", map[string]bool{})
  }
  ```

  В `parseSSHFile` использовать нормализованный `path` как ключ `seen`, завести `nextPosition := 0`, а при создании каждого буквального алиаса присваивать `SourcePath: path`, `Position: nextPosition`, затем увеличивать `nextPosition`. Для Include:

  ```go
  files, err := filepath.Glob(pat)
  if err != nil {
      return nil, fmt.Errorf("expand SSH Include %q: %w", pat, err)
  }
  for _, file := range files {
      sourcePath, err := filepath.Abs(file)
      if err != nil {
          return nil, fmt.Errorf("normalize SSH Include %q: %w", file, err)
      }
      sourcePath = filepath.Clean(sourcePath)
      group := strings.TrimSuffix(filepath.Base(sourcePath), ".conf")
      hosts, err := parseSSHFile(sourcePath, group, seen)
      if err != nil {
          return nil, fmt.Errorf("parse SSH Include %q: %w", sourcePath, err)
      }
      out = append(out, hosts...)
  }
  ```

  Не менять `HostsToServers`: поля `SourcePath` и `Position` намеренно отбрасываются, `Group` уже переносится.

- [ ] **Шаг 5: получить GREEN и проверить формат**

  Выполнить:

  ```bash
  gofmt -w internal/config/sshconf.go internal/config/sshconf_test.go
  go test -race -shuffle=on -count=1 ./internal/config
  ```

  Ожидается PASS всех тестов `internal/config`.

- [ ] **Шаг 6: закоммитить парсер**

  ```bash
  git add internal/config/sshconf.go internal/config/sshconf_test.go
  git diff --staged --check
  git commit -m "fix: сохранять источник хостов SSH-конфига"
  ```

### Задача 2: чистая модель свёрнутого дерева и tri-state выбора

**Файлы:**
- Создать: `internal/setup/model.go`
- Создать: `internal/setup/model_test.go`
- Изменить: `internal/setup/setup.go:22-184`

**Интерфейсы:**
- Потребляет: `config.SSHHost.SourcePath`, `Position`, `Group` из задачи 1.
- Производит: `newModel(hosts []config.SSHHost) model`, `model.selectedHosts() []config.SSHHost`.
- Внутренние типы: `sourceNode`, `hostNode`, `visibleRow`, `rowKind`, `checkState`.
- Следующая задача добавит viewport-поля и rendering, не меняя семантику выбора.

- [ ] **Шаг 1: написать тест свёрнутого состояния и порядка**

  Создать `internal/setup/model_test.go` в пакете `setup` с helper-fixture, где два файла имеют одинаковый basename, а хосты отличаются `SourcePath`/`Position`:

  ```go
  func setupHosts() []config.SSHHost {
      return []config.SSHHost{
          {Alias: "root-a", HostName: "10.0.0.1", Group: "main", SourcePath: "/home/u/.ssh/config", Position: 0},
          {Alias: "prod-a", HostName: "10.0.1.1", Group: "prod", SourcePath: "/home/u/.ssh/a/prod.conf", Position: 0},
          {Alias: "prod-b", HostName: "10.0.2.1", Group: "prod", SourcePath: "/home/u/.ssh/b/prod.conf", Position: 0},
      }
  }

  func TestModelStartsWithCollapsedSourcesInInputOrder(t *testing.T) {
      // Given: hosts from three ordered source files.
      hosts := setupHosts()

      // When: the setup model is created.
      m := newModel(hosts)

      // Then: only three distinct source rows are visible and all are collapsed.
      if len(m.sources) != 3 || len(m.visible) != 3 {
          t.Fatalf("sources=%d visible=%d, want 3/3", len(m.sources), len(m.visible))
      }
      for _, source := range m.sources {
          if source.expanded {
              t.Fatalf("source %q starts expanded", source.path)
          }
      }
  }
  ```

- [ ] **Шаг 2: написать тесты tri-state и сохранения выбора**

  Добавить отдельные Given/When/Then тесты:

  ```go
  func TestSourceSelectionCyclesThroughCheckedAndPartialStates(t *testing.T) {
      // Given: one source with two hosts.
      m := newModel([]config.SSHHost{
          {Alias: "a", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 0},
          {Alias: "b", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 1},
      })

      // When: the source is toggled, expanded, then one child is toggled off.
      m.toggleCurrent()
      m.toggleExpanded(0)
      m.cursor = 1
      m.toggleCurrent()

      // Then: the source is partial and exactly one host remains selected.
      if got := m.sourceState(0); got != statePartial {
          t.Fatalf("state=%v, want partial", got)
      }
      if got := len(m.selectedHosts()); got != 1 {
          t.Fatalf("selected=%d, want 1", got)
      }
  }
  ```

  Отдельный тест должен выбрать один хост, свернуть и снова раскрыть источник и доказать, что выбор не потерян.

- [ ] **Шаг 3: написать тесты клавиш и безопасного завершения**

  Проверить через `m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})`:

  - `s` при пустом выборе не ставит `done` и ставит `saveBlocked`;
  - `s` после выбора ставит `done` и возвращает `tea.Quit`;
  - q/Esc/Ctrl+C ставят `abort`;
  - Enter раскрывает source, но на host переключает только host;
  - `a` выбирает все хосты, второе `a` снимает все.

- [ ] **Шаг 4: запустить RED модели**

  ```bash
  go test ./internal/setup -run 'TestModel|TestSource|TestSave|TestToggle' -count=1
  ```

  Ожидается FAIL: новых типов/методов нет, старая модель плоская.

- [ ] **Шаг 5: реализовать типы и построение дерева**

  В `model.go` определить:

  ```go
  type rowKind uint8

  const (
      rowSource rowKind = iota
      rowHost
  )

  type checkState uint8

  const (
      stateEmpty checkState = iota
      statePartial
      stateChecked
  )

  type hostNode struct {
      host     config.SSHHost
      selected bool
  }

  type sourceNode struct {
      path     string
      group    string
      hosts    []hostNode
      expanded bool
  }

  type visibleRow struct {
      kind   rowKind
      source int
      host   int
  }

  type model struct {
      sources     []sourceNode
      visible     []visibleRow
      cursor      int
      done        bool
      abort       bool
      saveBlocked bool
  }
  ```

  `newModel` должен собирать source только по изменению/первому появлению `SourcePath`, используя `map[string]int` для индекса, но append — в порядке входа. Источники без хостов не создаются. После сборки вызвать `rebuildVisible()`; этот метод добавляет source row и дочерние host rows только для `expanded=true`.

- [ ] **Шаг 6: реализовать выбор и переходы**

  Добавить методы с точной семантикой:

  ```go
  func (m *model) sourceState(sourceIndex int) checkState
  func (m *model) toggleCurrent()
  func (m *model) toggleSource(sourceIndex int)
  func (m *model) toggleExpanded(sourceIndex int)
  func (m *model) toggleAll()
  func (m *model) selectedHosts() []config.SSHHost
  func (m *model) selectedCount() int
  func (m *model) rebuildVisible()
  func (m *model) move(delta int)
  func (m *model) moveToParent()
  ```

  Source-toggle ставит все hosts в `true`, если хотя бы один выключен, иначе снимает все. `selectedHosts` проходит sources/hosts по порядку, не по visible rows. После collapse курсор остаётся на source row; left/h на host сначала переводит курсор на родительский source без изменения expanded.

- [ ] **Шаг 7: переписать Update и упростить Run**

  Оставить в `setup.go` стили, `Run` и `Init`. `Run` после завершения должен использовать только:

  ```go
  finalModel, ok := result.(model)
  if !ok {
      return nil, fmt.Errorf("unexpected setup model %T", result)
  }
  if finalModel.abort {
      return nil, nil
  }
  return config.HostsToServers(finalModel.selectedHosts()), nil
  ```

  В `Update` реализовать утверждённые клавиши. После действия сбрасывать `saveBlocked=false`, кроме заблокированного `s`. Возвращать `tea.Quit` только при cancel или успешном `s`.

- [ ] **Шаг 8: получить GREEN модели**

  ```bash
  gofmt -w internal/setup/setup.go internal/setup/model.go internal/setup/model_test.go
  go test -race -shuffle=on -count=1 ./internal/setup
  ```

  Ожидается PASS всех setup-тестов.

- [ ] **Шаг 9: закоммитить модель**

  ```bash
  git add internal/setup/setup.go internal/setup/model.go internal/setup/model_test.go
  git diff --staged --check
  git commit -m "feat: добавить дерево выбора SSH-хостов"
  ```

### Задача 3: viewport, resize и однострочная псевдографика

**Файлы:**
- Изменить: `internal/setup/model.go`
- Создать: `internal/setup/view.go`
- Изменить: `internal/setup/model_test.go`

**Интерфейсы:**
- Добавляет в `model`: `viewport viewport.Model`, `ready bool`, `width int`, `height int`.
- Производит методы: `resize(width, height int)`, `refreshViewport()`, `ensureCursorVisible()` и `renderRows(width int) string`.
- Не меняет публичный `Run` и семантику выбора из задачи 2.

- [ ] **Шаг 1: написать RED-тест resize и прокрутки**

  Создать модель с 30 hosts в одном source, раскрыть её, послать `tea.WindowSizeMsg{Width: 50, Height: 8}`, затем 20 раз `down` и проверить:

  ```go
  if m.viewport.Height != 5 {
      t.Fatalf("viewport height=%d, want 5", m.viewport.Height)
  }
  if m.viewport.YOffset == 0 {
      t.Fatal("viewport did not scroll with cursor")
  }
  if m.cursor < m.viewport.YOffset || m.cursor >= m.viewport.YOffset+m.viewport.Height {
      t.Fatalf("cursor %d outside viewport [%d,%d)", m.cursor, m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height)
  }
  ```

  Константа `chromeHeight = 3`: заголовок, viewport, footer; поэтому при высоте 8 viewport получает 5 строк.

- [ ] **Шаг 2: написать RED-тест collapse/resize и однострочного рендера**

  - Поставить курсор на host, выполнить left/h: курсор должен перейти к source.
  - Выполнить left/h на source: source сворачивается, `visible` сокращается, cursor остаётся допустимым.
  - Послать resize высотой 1: `viewport.Height` остаётся минимум 1 и q всё ещё возвращает `tea.Quit`.
  - Создать очень длинный Unicode alias и IPv6 HostName, вызвать `renderRows(24)`, проверить отсутствие переносов внутри одной visible row и `lipgloss.Width(line) <= 24`.

- [ ] **Шаг 3: запустить RED viewport**

  ```bash
  go test ./internal/setup -run 'TestViewport|TestResize|TestRender' -count=1
  ```

  Ожидается FAIL: viewport-поля и rendering helpers отсутствуют.

- [ ] **Шаг 4: добавить viewport в модель**

  Импортировать `github.com/charmbracelet/bubbles/viewport`. Реализовать:

  ```go
  const chromeHeight = 3

  func (m *model) resize(width, height int) {
      m.width, m.height = max(1, width), max(1, height)
      viewportHeight := max(1, height-chromeHeight)
      if !m.ready {
          m.viewport = viewport.New(m.width, viewportHeight)
          m.ready = true
      } else {
          m.viewport.Width = m.width
          m.viewport.Height = viewportHeight
      }
      m.refreshViewport()
  }

  func (m *model) ensureCursorVisible() {
      if m.cursor < m.viewport.YOffset {
          m.viewport.YOffset = m.cursor
      }
      if m.cursor >= m.viewport.YOffset+m.viewport.Height {
          m.viewport.YOffset = m.cursor - m.viewport.Height + 1
      }
      maxOffset := max(0, len(m.visible)-m.viewport.Height)
      if m.viewport.YOffset > maxOffset {
          m.viewport.YOffset = maxOffset
      }
  }
  ```

  После каждого move/rebuild/resize вызывать `refreshViewport`, который нормализует cursor, вызывает ensure, затем `viewport.SetContent(renderRows(width))`.

- [ ] **Шаг 5: реализовать view.go**

  `renderRows` строит ровно одну строку на `visibleRow`. Source:

  ```text
  ▶ ▸ ◩ prod  1/2
  ```

  Host:

  ```text
      ├─ ☑ prod-web  deploy@10.0.0.10:22
  ```

  Использовать `▸/▾`, `☐/◩/☑`, `├─/└─`; последний host определяется по индексу в source. Реализовать `truncateWidth(text string, width int) string` через `lipgloss.Width`: уменьшать `[]rune` до ширины `max(1,width-1)` и добавлять `…`. Не использовать byte slicing для Unicode.

  `View` должен быть:

  ```go
  func (m model) View() string {
      title := styTitle.Render("sshmon: выберите серверы из SSH-конфигов")
      body := m.viewport.View()
      footer := m.footer()
      return title + "\n" + body + "\n" + footer
  }
  ```

  Footer всегда содержит `выбрано: N` и `s сохранить · space выбрать · enter открыть · q отмена`; при `saveBlocked` заменяет начало на `выберите хотя бы один сервер`.

- [ ] **Шаг 6: получить GREEN viewport**

  ```bash
  gofmt -w internal/setup/model.go internal/setup/view.go internal/setup/model_test.go
  go test -race -shuffle=on -count=1 ./internal/setup
  ```

  Ожидается PASS, включая маленький терминал, scrolling и Unicode.

- [ ] **Шаг 7: проверить размер файлов**

  ```bash
  awk '!/^[[:space:]]*$/ && !/^[[:space:]]*(\/\/)/' internal/setup/*.go | wc -l
  ```

  Затем отдельно измерить каждый production-файл. Каждый должен быть ≤250 чистых строк; при превышении перенести только rendering helpers в `view.go`, не добавляя новых абстракций.

- [ ] **Шаг 8: закоммитить viewport**

  ```bash
  git add internal/setup/model.go internal/setup/view.go internal/setup/model_test.go
  git diff --staged --check
  git commit -m "fix: прокручивать дерево SSH-хостов в маленьком терминале"
  ```

### Задача 4: интеграция выбора, группы и first-run поток

**Файлы:**
- Изменить: `internal/setup/model_test.go`
- Проверить без изменения при зелёных тестах: `cmd/sshmon/main.go`
- Проверить без изменения при зелёных тестах: `internal/config/template.go`

**Интерфейсы:**
- Потребляет: `model.selectedHosts()` и существующий `config.HostsToServers`.
- Производит: `Run(hosts []config.SSHHost) ([]config.Server, error)` с правильными группами `main`/Include.
- Сохраняет first-run контракты: missing config → `WriteWithServers`; existing empty config → `PopulateServers`; cancel → no write.

- [ ] **Шаг 1: написать интеграционный тест выбранных групп**

  В `model_test.go` создать source `main` с двумя hosts и source `prod` с двумя hosts. Выбрать весь main, раскрыть prod и выбрать один host. Затем:

  ```go
  servers := config.HostsToServers(m.selectedHosts())
  if len(servers) != 3 {
      t.Fatalf("servers=%d, want 3", len(servers))
  }
  if servers[0].Group != "main" || servers[1].Group != "main" || servers[2].Group != "prod" {
      t.Fatalf("groups=%q,%q,%q", servers[0].Group, servers[1].Group, servers[2].Group)
  }
  ```

  Добавить отдельный test cancel: q выставляет abort, `selectedHosts` может содержать выбор, но `Run`-контракт возвращает nil до преобразования/записи.

- [ ] **Шаг 2: запустить тесты интеграции**

  ```bash
  go test ./internal/setup ./internal/config -count=1
  ```

  Ожидается PASS. Если группы или порядок неверны, исправлять только `newModel`/`selectedHosts`/parser metadata; не менять YAML-схему.

- [ ] **Шаг 3: проверить firstRun компиляцией и существующими тестами**

  ```bash
  go build ./cmd/sshmon
  go test -race -shuffle=on -count=1 ./internal/config ./internal/setup
  ```

  Ожидается PASS без изменения `cmd/sshmon/main.go`. Если сигнатура `Run` сохранена, production first-run код не должен меняться.

- [ ] **Шаг 4: закоммитить интеграционные тесты**

  ```bash
  git add internal/setup/model_test.go
  git diff --staged --check
  git commit -m "test: проверить группы выбранных SSH-хостов"
  ```

### Задача 5: документация, реальный PTY-сценарий и полный quality gate

**Файлы:**
- Изменить: `README.md` в секции первого запуска и клавиш.
- Не коммитить: временную fixture под `/var/folders/q8/ql4_4q7d6yng80qqr1smz_m00000gp/T/opencode/sshmon-picker-e2e/`.

**Интерфейсы:**
- Документирует утверждённые клавиши и группировку.
- Проверяет настоящий бинарник, терминальный размер, YAML и запуск Overview.

- [ ] **Шаг 1: обновить README**

  Заменить описание плоского picker на:

  ```markdown
  При первом запуске конфиги показаны свёрнутым деревом. Хосты из
  `~/.ssh/config` относятся к группе `main`, а хосты Include-файла — к группе
  по имени файла (`prod.conf` → `prod`).

  - `enter` / `→` / `l` — раскрыть файл; `←` / `h` — свернуть или вернуться к файлу;
  - `space` — выбрать хост или весь файл; `a` — выбрать/снять всё;
  - `s` — сохранить выбранные серверы; `q` / `esc` — отменить.
  ```

- [ ] **Шаг 2: запустить модульные и статические проверки**

  ```bash
  gofmt -d cmd internal
  go build ./...
  go vet ./...
  go test -race -shuffle=on -count=1 ./...
  go build -o sshmon ./cmd/sshmon
  git diff --check
  ```

  Ожидается: пустой вывод gofmt/diff check, exit 0 всех команд, все тесты PASS.

- [ ] **Шаг 3: подготовить временную PTY-fixture**

  Создать только под разрешённым temp parent:

  ```text
  sshmon-picker-e2e/home/.ssh/config
  sshmon-picker-e2e/home/.ssh/conf.d/prod.conf
  sshmon-picker-e2e/home/.ssh/conf.d/staging.conf
  sshmon-picker-e2e/home/.config/sshmon/config.yaml
  ```

  Корневой файл должен содержать два прямых hosts и Include; `prod.conf` и `staging.conf` — не менее 15 hosts каждый, чтобы дерево гарантированно превышало терминал 40×10. Existing sshmon config должен иметь `servers: []` и нестандартный `interval: 17s`, чтобы проверить безопасное заполнение.

- [ ] **Шаг 4: выполнить настоящий expect-сценарий**

  Запустить binary с `HOME` fixture и terminal size 40×10. Expect-сценарий обязан наблюдать и выполнять:

  1. На старте видны только `▸ ☐ main`, `▸ ☐ prod`, `▸ ☐ staging`; host-строки отсутствуют.
  2. Enter раскрывает `main`, Space на одном host создаёт `◩ main`.
  3. Down/scroll до `prod`, Space на source даёт `☑ prod` без обязательного раскрытия.
  4. Раскрытие prod и многократный down прокручивает список, процесс остаётся отзывчивым.
  5. `s` печатает сообщение о созданном конфиге и запускает `Overview` без перезапуска.
  6. `q` завершает основной TUI.

  Использовать timeout 20 секунд на каждую фазу. Ожидается exit 0 expect-процесса.

- [ ] **Шаг 5: проверить итоговый YAML и регрессии headless**

  Разобрать YAML существующим `config.Load` через маленький временный Go test/script либо визуально проверить fixture только в QA-команде. Доказать:

  - выбранный root-host имеет `group: main`;
  - все выбранные prod hosts имеют `group: prod`;
  - staging hosts отсутствуют;
  - `interval: 17s` сохранён.

  Затем проверить: headless + missing config создаёт шаблон и завершает работу; headless + empty config возвращает `ErrNoServers`, не открывая picker.

- [ ] **Шаг 6: удалить временную fixture**

  ```bash
  rm -rf "/var/folders/q8/ql4_4q7d6yng80qqr1smz_m00000gp/T/opencode/sshmon-picker-e2e"
  ```

  Проверить, что `git status --short` не показывает временных файлов или бинарника.

- [ ] **Шаг 7: финальный обзор diff и документационный коммит**

  ```bash
  git status --short
  git diff --stat
  git diff --check
  git add README.md
  git diff --staged --check
  git commit -m "docs: описать дерево выбора SSH-серверов"
  ```

- [ ] **Шаг 8: проверка закоммиченного дерева**

  ```bash
  gofmt -d cmd internal
  go build ./...
  go vet ./...
  go test -race -shuffle=on -count=1 ./...
  go build -o sshmon ./cmd/sshmon
  git diff --check
  git status --short
  ```

  Ожидается: все проверки exit 0; worktree содержит только заранее известный untracked `.opencode/`; бинарник `sshmon` игнорируется `.gitignore`.

## Самопроверка плана

- Покрытие спецификации: source identity, `main`/basename groups, collapsed default, tri-state, whole-file/per-host selection, viewport, resize, cursor normalization, Unicode truncation, cancel/save, Include errors, PTY и headless-регрессии распределены по задачам 1–5.
- Границы файлов: parser, model, view и runner имеют по одной ответственности; план предотвращает рост одного setup-файла за 250 чистых строк.
- Согласованность типов: `SSHHost.SourcePath` и `SSHHost.Position` вводятся в задаче 1; `sourceNode`/`visibleRow` — в задаче 2; viewport-поля — в задаче 3; публичная сигнатура `Run` не меняется.
- Новые зависимости, v2-миграция, ProxyJump и изменения основного TUI отсутствуют.
- В плане нет незаполненных решений: клавиша сохранения — `s`, chrome height — 3, minimum viewport — 1, группы и error semantics заданы явно.

## Передача на исполнение

После утверждения плана выполнять его либо через `superpowers:subagent-driven-development` с отдельным исполнителем и review-gate на каждую задачу, либо через `superpowers:executing-plans` пакетами с контрольными точками. Перед началом создать изолированный worktree через `superpowers:using-git-worktrees`; основной worktree не использовать для реализации.
