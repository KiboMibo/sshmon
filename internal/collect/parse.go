package collect

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Один SSH-exec собирает всё сразу: /proc + df + ss.
const sampleCmd = `echo @@HOST; hostname 2>/dev/null; ` +
	`echo @@UP; cat /proc/uptime; ` +
	`echo @@LOAD; cat /proc/loadavg; ` +
	`echo @@CPU; cat /proc/stat; ` +
	`echo @@MEM; cat /proc/meminfo; ` +
	`echo @@DISK; cat /proc/diskstats; ` +
	`echo @@NET; cat /proc/net/dev; ` +
	`echo @@DF; df -kP 2>/dev/null; ` +
	`echo @@PORTS; ss -tulpn 2>/dev/null || netstat -tulpn 2>/dev/null; ` +
	// `command -v docker` и маркер, как в diagnostics.go: без них пустой ответ
	// от хоста без docker'а неотличим от «контейнеров нет». Ветка `|| echo`
	// ловит и отказ самого docker'а (нет прав, демон молчит) — там счётчиков
	// тоже нет. Заодно последняя команда сэмпла всегда выходит с нулём.
	`echo @@DOCKER; command -v docker >/dev/null 2>&1 && docker ps -a --format '{{.Status}}' 2>/dev/null || echo ` + unsupportedMarker

// sampleCmdWithOS — тот же сэмпл плюс /etc/os-release. Шлём его один раз на
// сервер: дистрибутив не меняется, а лишний cat на каждом тике не нужен — в том
// числе там, где файла нет вовсе. Обе команды — константы, подстановки в них нет.
const sampleCmdWithOS = sampleCmd + `; echo @@OS; cat /etc/os-release 2>/dev/null`

// counters — сырые счётчики одного сэмпла; скорости считаются по двум сэмплам.
type counters struct {
	at       time.Time
	cpuTotal uint64
	cpuIdle  uint64
	ncpu     int
	diskR    map[string]uint64 // секторы
	diskW    map[string]uint64
	netRx    map[string]uint64 // байты
	netTx    map[string]uint64
}

type sample struct {
	c        counters
	hostname string
	os       string
	uptime   time.Duration
	load1    float64
	load5    float64
	load15   float64
	memTotal uint64
	memAvail uint64
	swapTot  uint64
	swapFree uint64
	disks    []DiskUsage
	ports    []Port
	docker   DockerCounts
}

func sections(raw string) map[string][]string {
	out := map[string][]string{}
	var cur string
	for _, ln := range strings.Split(raw, "\n") {
		if strings.HasPrefix(ln, "@@") {
			cur = strings.TrimSpace(ln[2:])
			continue
		}
		if cur != "" {
			out[cur] = append(out[cur], ln)
		}
	}
	return out
}

var partRe = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+)\d+$|^(nvme\d+n\d+|mmcblk\d+)p\d+$`)
var skipFs = map[string]bool{"tmpfs": true, "devtmpfs": true, "udev": true, "none": true, "shm": true, "overlay": true}

// sectionParsers — по разборщику на секцию сэмпла. Диспетчер parseSample берёт
// разборщик отсюда, поэтому новая секция добавляется своей функцией и строкой в
// таблице, а не веткой в общем разборе. Секции независимы (каждая пишет свои
// поля sample), так что порядок обхода на результат не влияет.
var sectionParsers = map[string]func(*sample, []string){
	"HOST":   parseHostSection,
	"UP":     parseUptimeSection,
	"LOAD":   parseLoadSection,
	"CPU":    parseCPUSection,
	"MEM":    parseMemSection,
	"DISK":   parseDiskStatsSection,
	"NET":    parseNetSection,
	"DF":     parseDFSection,
	"OS":     parseOSSection,
	"PORTS":  parsePortsSection,
	"DOCKER": parseDockerSection,
}

func parseSample(raw string, at time.Time) *sample {
	s := &sample{}
	s.c.at = at
	s.c.diskR, s.c.diskW = map[string]uint64{}, map[string]uint64{}
	s.c.netRx, s.c.netTx = map[string]uint64{}, map[string]uint64{}
	for name, lines := range sections(raw) {
		// Пропавшая секция — это «неизвестно», и разборщик для неё не зовём:
		// нулевые поля sample уже означают ровно это.
		if parse, ok := sectionParsers[name]; ok {
			parse(s, lines)
		}
	}
	return s
}

// Всё, что приходит из вывода команд на удалённом хосте, попадает на экран как
// текст: имя хоста, дистрибутив, имена ФС, точек монтирования, дисков и
// интерфейсов. Чистим на разборе — дальше это уже данные модели.
func parseHostSection(s *sample, lines []string) {
	if len(lines) > 0 {
		s.hostname = SanitizeLine(strings.TrimSpace(lines[0]))
	}
}

func parseUptimeSection(s *sample, lines []string) {
	if len(lines) == 0 {
		return
	}
	f := strings.Fields(lines[0])
	if len(f) == 0 {
		return
	}
	if v, err := strconv.ParseFloat(f[0], 64); err == nil {
		s.uptime = time.Duration(v * float64(time.Second))
	}
}

func parseLoadSection(s *sample, lines []string) {
	if len(lines) == 0 {
		return
	}
	f := strings.Fields(lines[0])
	if len(f) < 3 {
		return
	}
	s.load1, _ = strconv.ParseFloat(f[0], 64)
	s.load5, _ = strconv.ParseFloat(f[1], 64)
	s.load15, _ = strconv.ParseFloat(f[2], 64)
}

func parseCPUSection(s *sample, lines []string) {
	for _, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] == "cpu" {
			total, idle := cpuTotals(f[1:])
			s.c.cpuTotal += total
			s.c.cpuIdle = idle
		} else {
			s.c.ncpu++
		}
	}
}

// cpuTotals складывает первые восемь колонок строки «cpu» из /proc/stat и
// отдельно выделяет простой: idle+iowait, а на ядрах без колонки iowait — один
// idle. Дальше восьмой идут guest-колонки, уже учтённые внутри user/nice.
func cpuTotals(fields []string) (total, idle uint64) {
	var vals []uint64
	for _, x := range fields {
		v, _ := strconv.ParseUint(x, 10, 64)
		vals = append(vals, v)
	}
	for i, v := range vals {
		if i < 8 {
			total += v
		}
	}
	if len(vals) > 4 {
		idle = vals[3] + vals[4]
	} else if len(vals) > 3 {
		idle = vals[3]
	}
	return
}

func parseMemSection(s *sample, lines []string) {
	for _, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			s.memTotal = v
		case "MemFree:": // старые ядра/BusyBox без MemAvailable
			if s.memAvail == 0 {
				s.memAvail = v
			}
		case "MemAvailable:":
			s.memAvail = v
		case "SwapTotal:":
			s.swapTot = v
		case "SwapFree:":
			s.swapFree = v
		}
	}
}

func parseDiskStatsSection(s *sample, lines []string) {
	for _, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 10 {
			continue
		}
		name := SanitizeLine(f[2])
		if skipBlockDev(name) {
			continue
		}
		r, _ := strconv.ParseUint(f[5], 10, 64)
		w, _ := strconv.ParseUint(f[9], 10, 64)
		s.c.diskR[name], s.c.diskW[name] = r, w
	}
}

// skipBlockDev отсеивает виртуальные устройства и разделы: раздел показывает
// тот же ввод-вывод, что и его диск, и в сумме он был бы учтён дважды.
func skipBlockDev(name string) bool {
	return strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
		strings.HasPrefix(name, "zram") || strings.HasPrefix(name, "dm-") ||
		partRe.MatchString(name)
}

func parseNetSection(s *sample, lines []string) {
	for _, ln := range lines {
		if !strings.Contains(ln, ":") {
			continue
		}
		parts := strings.SplitN(ln, ":", 2)
		iface := SanitizeLine(strings.TrimSpace(parts[0]))
		if iface == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(f[0], 10, 64)
		tx, _ := strconv.ParseUint(f[8], 10, 64)
		s.c.netRx[iface], s.c.netTx[iface] = rx, tx
	}
}

func parseDFSection(s *sample, lines []string) {
	for _, ln := range lines {
		f := strings.Fields(ln)
		if len(f) < 6 || f[0] == "Filesystem" || skipFs[f[0]] {
			continue
		}
		total, _ := strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		avail, _ := strconv.ParseUint(f[3], 10, 64)
		if total == 0 {
			continue
		}
		s.disks = append(s.disks, DiskUsage{
			Fs: SanitizeLine(f[0]), Mount: SanitizeLine(f[5]),
			TotalKB: total, UsedKB: used, AvailKB: avail,
			UsedPct: 100 * float64(used) / float64(total),
		})
	}
}

func parseOSSection(s *sample, lines []string) {
	s.os = SanitizeLine(parseOSRelease(lines))
}

func parsePortsSection(s *sample, lines []string) {
	s.ports, _ = ParsePorts(strings.Join(lines, "\n"))
}

// parseDockerSection читает секцию только целиком: маркер вместо списка — это
// «docker не спросили» (нет бинаря, нет прав, демон молчит), и счётчики тогда
// остаются неизвестными, а не нулевыми.
func parseDockerSection(s *sample, lines []string) {
	if hasUnsupportedMarker(strings.Join(lines, "\n")) {
		return
	}
	s.docker.Known = true
	for _, ln := range lines {
		if ln = strings.TrimSpace(ln); ln != "" {
			s.docker.CountContainerStatus(ln)
		}
	}
}

// parseOSRelease собирает короткое имя дистрибутива из /etc/os-release:
// «ID VERSION_ID» («debian 12»), а если версии нет — PRETTY_NAME.
func parseOSRelease(lines []string) string {
	fields := make(map[string]string, len(lines))
	for _, ln := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(ln), "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, `"'`)
	}
	id, version := fields["ID"], fields["VERSION_ID"]
	switch {
	case id != "" && version != "":
		return id + " " + version
	case fields["PRETTY_NAME"] != "":
		return fields["PRETTY_NAME"]
	default:
		return id
	}
}

// rates считает скорости по дельтам двух сэмплов.
func rates(prev, cur *counters) (cpuPct float64, io []DiskIO, net []NetRate) {
	dt := cur.at.Sub(prev.at).Seconds()
	if dt <= 0 {
		return
	}
	if cur.cpuTotal > prev.cpuTotal && cur.cpuIdle >= prev.cpuIdle {
		dTotal := float64(cur.cpuTotal - prev.cpuTotal)
		dIdle := float64(cur.cpuIdle - prev.cpuIdle)
		cpuPct = 100 * (dTotal - dIdle) / dTotal
	}
	for dev, r := range cur.diskR {
		pr, ok := prev.diskR[dev]
		if !ok || r < pr {
			continue
		}
		w, pw := cur.diskW[dev], prev.diskW[dev]
		if w < pw {
			continue
		}
		io = append(io, DiskIO{Dev: dev, ReadBps: float64(r-pr) * 512 / dt, WriteBps: float64(w-pw) * 512 / dt})
	}
	sort.Slice(io, func(i, j int) bool { return io[i].Dev < io[j].Dev })
	for iface, rx := range cur.netRx {
		prx, ok := prev.netRx[iface]
		if !ok || rx < prx {
			continue
		}
		tx, ptx := cur.netTx[iface], prev.netTx[iface]
		if tx < ptx {
			continue
		}
		net = append(net, NetRate{Iface: iface, RxBps: float64(rx-prx) / dt, TxBps: float64(tx-ptx) / dt})
	}
	sort.Slice(net, func(i, j int) bool { return net[i].Iface < net[j].Iface })
	return
}
