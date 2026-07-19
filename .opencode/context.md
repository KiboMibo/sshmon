# sshmon — контекст миссии

## Current Status
- Репо: `/Users/kibomibo/projects/sshmon`, ветка `master`, HEAD `0cd0219`.
- Задача закрепления подсказок внизу TUI завершена коммитом `0cd0219 feat: закрепить подсказки внизу TUI`.
- `composeScreen` резервирует последнюю строку контента под футер/активный control, обрезает его до одной визуальной строки и размещает overlay выше него.
- RED был засвидетельствован для Fleet, Dashboard, Processes и Help-over-Fleet; все контракты стали GREEN.
- Полный `go test -race -shuffle=on -count=1 ./...`, vet, build, gofmt и diff-check прошли. Финальный TUI race-прогон после коммита прошёл.
- Свежие текстовые captures Fleet и Help: 24×80, overflow отсутствует, границы выровнены; футер находится непосредственно над нижней границей.
- Reviewer/visual-oracle вызовы дважды упали с известной repetition anomaly; PASS не фабриковался, использованы прямые тесты и evidence.
- Рабочее дерево чисто кроме operational `?? .opencode/`. Никогда не стейджить `.opencode/` и `.superpowers/sdd/`.

## Pending Tasks
- Нет.

## Constraints
- Russian communication; autonomous witnessed RED→GREEN TDD.
- Bubble Tea/Bubbles/Lip Gloss v1; production Go files ≤250 pure LOC.
