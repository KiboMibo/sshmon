package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

// containerCountsCompact — «●7 ○2 ⚠1» из макета: счётчики стоят в заголовке
// плитки, поэтому нужны в самой короткой форме. Развёрнутый вариант с
// подписями остался на карточке хоста, где ширины хватает.
func containerCountsCompact(items []collect.Container) string {
	var counts collect.DockerCounts
	for _, item := range items {
		counts.CountContainerStatus(item.Status)
	}
	parts := []string{goodStyle.Render(fmt.Sprintf("●%d", counts.Running))}
	if counts.Stopped > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("○%d", counts.Stopped)))
	}
	if counts.Broken > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("⚠%d", counts.Broken)))
	}
	return strings.Join(parts, " ")
}

// containerStatus приводит сырой статус docker'а («Up 3 days», «Exited (0) 5
// minutes ago») к короткой человекочитаемой форме макета. Длительность из
// статуса выбрасывается: она живёт в своей колонке (containerUptime).
func containerStatus(status string) string {
	fields := strings.Fields(status)
	if len(fields) == 0 {
		return "—"
	}
	state := strings.ToLower(fields[0])
	switch state {
	case "exited":
		// Число в скобках — код выхода; ради него статус и читают.
		if len(fields) > 1 && strings.HasPrefix(fields[1], "(") && strings.HasSuffix(fields[1], ")") {
			return state + " " + fields[1]
		}
	case "restarting":
		// У restarting в скобках docker показывает тот же код выхода, а рядом
		// в макете стоит счётчик рестартов «×4». Читаются они одинаково, поэтому
		// скобки убраны: лучше без числа, чем выдать код за число перезапусков.
		return state
	case "up":
		if strings.Contains(strings.ToLower(status), "(unhealthy)") {
			return "up (unhealthy)"
		}
	}
	return state
}

// containerUptimeUnits — единицы относительной строки docker'а в длительности.
// Недели/месяцы/годы приблизительные: docker сам округляет до них, точнее
// исходных данных всё равно нет.
var containerUptimeUnits = map[string]time.Duration{
	"second": time.Second,
	"minute": time.Minute,
	"hour":   time.Hour,
	"day":    24 * time.Hour,
	"week":   7 * 24 * time.Hour,
	"month":  30 * 24 * time.Hour,
	"year":   365 * 24 * time.Hour,
}

// containerUptime переводит `{{.RunningFor}}` docker'а («3 weeks ago», «About
// an hour ago», «Less than a second») в компактный русский вид макета: «140д».
// Неразобранная строка даёт пустую колонку, а не сырой английский текст.
func containerUptime(runningFor string) string {
	text := strings.ToLower(strings.TrimSpace(runningFor))
	text = strings.TrimSuffix(text, " ago")
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "less than") {
		return "0с"
	}
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}
	// «About a minute», «About an hour» — количество словом, всегда единица.
	count := 1
	unit := fields[len(fields)-1]
	if value, err := strconv.Atoi(fields[0]); err == nil {
		count = value
	} else if fields[0] != "about" {
		return ""
	}
	step, ok := containerUptimeUnits[strings.TrimSuffix(unit, "s")]
	if !ok {
		return ""
	}
	return compactDuration(time.Duration(count) * step)
}

// compactDuration — «140д», «3ч», «12м», «45с»: одна значащая единица, чтобы
// колонка аптайма не прыгала по ширине. Дни не переводятся в годы намеренно:
// «730д» сравнимо с соседними строками, «2г» — уже нет.
func compactDuration(value time.Duration) string {
	switch {
	case value >= 24*time.Hour:
		return strconv.Itoa(int(value.Hours()/24)) + "д"
	case value >= time.Hour:
		return strconv.Itoa(int(value.Hours())) + "ч"
	case value >= time.Minute:
		return strconv.Itoa(int(value.Minutes())) + "м"
	case value > 0:
		return strconv.Itoa(int(value.Seconds())) + "с"
	default:
		return "0с"
	}
}

// containerMemory берёт использованную часть из `{{.MemUsage}}` docker'а
// («64MiB / 512MiB») и сжимает единицу: в колонку помещается «64M», а не
// пара абсолютных значений.
func containerMemory(usage string) string {
	used, _, _ := strings.Cut(usage, "/")
	used = strings.TrimSpace(used)
	if used == "" || strings.HasPrefix(used, "--") {
		return ""
	}
	// Порядок важен: «KiB» тоже заканчивается на «B».
	for _, unit := range []struct{ from, to string }{
		{"KiB", "K"}, {"MiB", "M"}, {"GiB", "G"}, {"TiB", "T"},
		{"kB", "K"}, {"MB", "M"}, {"GB", "G"}, {"TB", "T"}, {"B", ""},
	} {
		if suffix := strings.TrimSuffix(used, unit.from); suffix != used {
			return suffix + unit.to
		}
	}
	return used
}

type containerSort uint8

const (
	containerSortCPU containerSort = iota
	containerSortMemory
	containerSortName
)

type containerScreen struct {
	items []collect.Container
	sort  containerSort
	diagnostics
}

func sortContainers(items []collect.Container, by containerSort) []collect.Container {
	result := append([]collect.Container(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch by {
		case containerSortMemory:
			if a.MemPct != b.MemPct {
				return a.MemPct > b.MemPct
			}
		case containerSortName:
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		default:
			if a.CPUPct != b.CPUPct {
				return a.CPUPct > b.CPUPct
			}
		}
		return a.Name < b.Name
	})
	return result
}

func (s *containerScreen) apply(items []collect.Container, err error) {
	if err == nil {
		s.items = append([]collect.Container(nil), items...)
	}
	s.finish(err, len(s.items) > 0)
}

var _ screen = containerScreen{}

func (s containerScreen) view(ctx screenContext) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshmon · "+ctx.serverName+" · Docker") + "\n\n")
	b.WriteString("ИМЯ             ОБРАЗ                  СТАТУС             CPU     MEM\n")
	for _, c := range sortContainers(s.items, s.sort) {
		b.WriteString(fmt.Sprintf("%-16s %-22s %-18s %6.1f%% %6.1f%%\n", c.Name, c.Image, c.Status, c.CPUPct, c.MemPct))
	}
	b.WriteString("\n" + dimStyle.Render(diagnosticsFooter(s.status, s.err)))
	return b.String()
}
