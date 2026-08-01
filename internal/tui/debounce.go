package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// inputDebounce — пауза тишины перед запросом, который порождает движение
// курсора. Автоповтор удержанной стрелки даёт 20–30 событий в секунду, и без
// паузы каждое открывало бы свой SSH-канал: на широком флоте это десятки
// запросов за секунду. Отмена контекста соединение не рвёт и уже отправленную
// команду на сервере не останавливает, так что «лишние» запросы всё равно
// доигрывают до конца на чужой стороне.
const inputDebounce = 200 * time.Millisecond

type debounceKind uint8

const (
	debounceTopProcesses debounceKind = iota
	debounceLogs
)

// debounceMsg — отложенный старт запроса. Поколение то же самое, что у самого
// запроса (processes.generation / logs.generation): следующее движение курсора
// его увеличивает, и тик от предыдущего нажатия молча отбрасывается — тот же
// механизм, что отсеивает ответы устаревших запросов.
type debounceMsg struct {
	kind       debounceKind
	generation uint64
}

func debounceTick(kind debounceKind, generation uint64) tea.Cmd {
	return tea.Tick(inputDebounce, func(time.Time) tea.Msg {
		return debounceMsg{kind: kind, generation: generation}
	})
}

// applyDebounce стартует отложенный запрос, если за время паузы поколение не
// сменилось. Видимость сайдбара и прочие условия перепроверяет сам запрос: за
// 200 мс экран мог смениться.
func (m *Model) applyDebounce(msg debounceMsg) tea.Cmd {
	switch msg.kind {
	case debounceTopProcesses:
		if msg.generation != m.processes.generation {
			return nil
		}
		return m.startFleetTopProcesses()
	case debounceLogs:
		if msg.generation != m.logs.generation {
			return nil
		}
		return m.startLogsStream()
	}
	return nil
}
