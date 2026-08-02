package collect

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ErrUnsupported = errors.New("операция не поддерживается сервером")

// ErrAccessDenied — утилита на хосте есть и отработала, но текущему
// пользователю не хватает прав (типичный случай — docker без группы docker).
// Отдельная ошибка, потому что для человека это не поломка сервера, а
// недостающее право, и на экране это разные формулировки.
var ErrAccessDenied = errors.New("нет доступа")

const unsupportedMarker = "__SSHMON_UNSUPPORTED__"

var (
	ssProcessRe  = regexp.MustCompile(`\(\("?([^",]+)"?,pid=(\d+)`)
	netstatPIDRe = regexp.MustCompile(`^(\d+)/(.+)$`)
)

// hasUnsupportedMarker ищет маркер отдельной строкой, а не подстрокой в любом
// месте вывода. Команду на хосте выполняет `sh -c '<вся строка>'`, и сам этот
// шелл виден в выводе `ps -eo args=` вместе со своим аргументом — включая
// `echo __SSHMON_UNSUPPORTED__` из ветки «утилиты нет». Подстрочная проверка
// принимала эту строку за ответ «не поддерживается» и объявляла `ps`
// недоступным на живом хосте, где ps есть и отработал.
func hasUnsupportedMarker(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == unsupportedMarker {
			return true
		}
	}
	return false
}

func ParseProcesses(raw string) ([]Process, error) {
	if hasUnsupportedMarker(raw) {
		return nil, ErrUnsupported
	}
	var out []Process
	for _, line := range strings.Split(raw, "\n") {
		// Командная строка чужого процесса попадает в список как есть, а её
		// содержимое задаёт тот, кто процесс запустил.
		fields := strings.Fields(SanitizeLine(line))
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		process := Process{PID: pid}
		if len(fields) >= 4 {
			cpu, cpuErr := strconv.ParseFloat(fields[1], 64)
			mem, memErr := strconv.ParseFloat(fields[2], 64)
			if cpuErr == nil && memErr == nil {
				process.CPUPct = cpu
				process.MemPct = mem
				process.Command = strings.Join(fields[3:], " ")
			} else if len(fields) >= 5 {
				process.Command = strings.Join(fields[4:], " ")
			} else {
				process.Command = fields[len(fields)-1]
			}
		} else {
			process.Command = fields[len(fields)-1]
		}
		if process.Command != "" {
			out = append(out, process)
		}
	}
	return out, nil
}

func ParseContainers(listRaw, statsRaw string) ([]Container, error) {
	if hasUnsupportedMarker(listRaw) || hasUnsupportedMarker(statsRaw) {
		return nil, ErrUnsupported
	}
	stats := make(map[string]Container)
	for _, line := range strings.Split(statsRaw, "\n") {
		// Чистим поля, а не строку целиком: колонки разделены табуляцией, а
		// SanitizeLine делает из неё пробел.
		fields := sanitizeFields(strings.Split(line, "\t"))
		if len(fields) != 4 {
			continue
		}
		stats[fields[0]] = Container{CPUPct: parsePercent(fields[1]), MemPct: parsePercent(fields[2]), MemUsage: fields[3]}
	}
	var out []Container
	for _, line := range strings.Split(listRaw, "\n") {
		fields := sanitizeFields(strings.Split(line, "\t"))
		if len(fields) < 5 || fields[0] == "" {
			continue
		}
		container := stats[fields[0]]
		container.ID, container.Name, container.Image, container.Status, container.Ports = fields[0], fields[1], fields[2], fields[3], fields[4]
		// Колонка RunningFor добавлена позже — старый вывод без неё остаётся валидным.
		if len(fields) >= 6 {
			container.RunningFor = strings.TrimSpace(fields[5])
		}
		out = append(out, container)
	}
	if len(out) == 0 {
		return nil, dockerFailure(listRaw)
	}
	return out, nil
}

// dockerFailure объясняет пустой список контейнеров. Причину отказа docker
// пишет в stderr, а он в dockerListCommand слит со stdout, поэтому непустой
// вывод без единой разобранной строки — это и есть отказ. Его текст важнее
// «Process exited with status 1» от ssh: без группы docker плитка иначе
// молчала бы ровно так же, как на хосте без контейнеров.
func dockerFailure(listRaw string) error {
	for _, line := range strings.Split(listRaw, "\n") {
		line = strings.TrimSpace(SanitizeLine(line))
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "permission denied") {
			return fmt.Errorf("%w: %s", ErrAccessDenied, line)
		}
		return errors.New(line)
	}
	return nil
}

func ParsePorts(raw string) ([]Port, error) {
	if hasUnsupportedMarker(raw) {
		return nil, ErrUnsupported
	}
	var out []Port
	for _, line := range strings.Split(raw, "\n") {
		// Имя процесса в выводе ss/netstat выбирает владелец процесса.
		line = SanitizeLine(line)
		fields := strings.Fields(line)
		if len(fields) < 5 || !isPortProtocol(fields[0]) {
			continue
		}
		port := Port{Proto: fields[0]}
		if fields[1] == "LISTEN" || fields[1] == "UNCONN" || strings.Contains(line, "users:") {
			port.Local = fields[4]
			if match := ssProcessRe.FindStringSubmatch(line); match != nil {
				port.Process = match[1]
				port.PID, _ = strconv.Atoi(match[2])
			}
		} else {
			port.Local = fields[3]
			if match := netstatPIDRe.FindStringSubmatch(fields[len(fields)-1]); match != nil {
				port.PID, _ = strconv.Atoi(match[1])
				port.Process = match[2]
			}
		}
		port.Local = normalizeListenAddress(port.Local)
		out = append(out, port)
	}
	return out, nil
}

// normalizeListenAddress приводит IPv6-адрес к единой форме `[::]:41641`.
// Формат зависит от утилиты: debian/`ss` печатает адрес в скобках, centos
// 7/`netstat` — как `:::10050` и `::1:323`. В одном списке это читается как
// разные вещи, хотя это один и тот же адрес. Нормализуем на разборе, а не на
// отрисовке: тот же список уходит наружу через MCP-сервер.
func normalizeListenAddress(address string) string {
	if strings.HasPrefix(address, "[") || strings.Count(address, ":") < 2 {
		return address
	}
	index := strings.LastIndexByte(address, ':')
	host, port := address[:index], address[index+1:]
	// Без порта («::») и без второго двоеточия в хосте это не «адрес:порт»,
	// а что-то другое — такое лучше показать как есть, чем испортить скобками.
	if port == "" || !strings.Contains(host, ":") {
		return address
	}
	return "[" + host + "]:" + port
}

func parsePercent(value string) float64 {
	percent, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	return percent
}

func isPortProtocol(value string) bool {
	return value == "tcp" || value == "udp" || value == "tcp6" || value == "udp6"
}
