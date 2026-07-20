package collect

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var ErrUnsupported = errors.New("операция не поддерживается сервером")

const unsupportedMarker = "__SSHMON_UNSUPPORTED__"

var (
	ssProcessRe  = regexp.MustCompile(`\(\("?([^",]+)"?,pid=(\d+)`)
	netstatPIDRe = regexp.MustCompile(`^(\d+)/(.+)$`)
)

func ParseProcesses(raw string) ([]Process, error) {
	if strings.Contains(raw, unsupportedMarker) {
		return nil, ErrUnsupported
	}
	var out []Process
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
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
	if strings.Contains(listRaw, unsupportedMarker) || strings.Contains(statsRaw, unsupportedMarker) {
		return nil, ErrUnsupported
	}
	stats := make(map[string]Container)
	for _, line := range strings.Split(statsRaw, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			continue
		}
		stats[fields[0]] = Container{CPUPct: parsePercent(fields[1]), MemPct: parsePercent(fields[2]), MemUsage: fields[3]}
	}
	var out []Container
	for _, line := range strings.Split(listRaw, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 5 || fields[0] == "" {
			continue
		}
		container := stats[fields[0]]
		container.ID, container.Name, container.Image, container.Status, container.Ports = fields[0], fields[1], fields[2], fields[3], fields[4]
		out = append(out, container)
	}
	return out, nil
}

func ParsePorts(raw string) ([]Port, error) {
	if strings.Contains(raw, unsupportedMarker) {
		return nil, ErrUnsupported
	}
	var out []Port
	for _, line := range strings.Split(raw, "\n") {
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
		out = append(out, port)
	}
	return out, nil
}

func parsePercent(value string) float64 {
	percent, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	return percent
}

func isPortProtocol(value string) bool {
	return value == "tcp" || value == "udp" || value == "tcp6" || value == "udp6"
}
