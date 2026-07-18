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
	`echo @@PORTS; ss -tulpn 2>/dev/null || netstat -tulpn 2>/dev/null`

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

func parseSample(raw string, at time.Time) *sample {
	sec := sections(raw)
	s := &sample{}
	s.c.at = at
	s.c.diskR, s.c.diskW = map[string]uint64{}, map[string]uint64{}
	s.c.netRx, s.c.netTx = map[string]uint64{}, map[string]uint64{}

	if h := sec["HOST"]; len(h) > 0 {
		s.hostname = strings.TrimSpace(h[0])
	}
	if u := sec["UP"]; len(u) > 0 {
		if f := strings.Fields(u[0]); len(f) > 0 {
			if v, err := strconv.ParseFloat(f[0], 64); err == nil {
				s.uptime = time.Duration(v * float64(time.Second))
			}
		}
	}
	if l := sec["LOAD"]; len(l) > 0 {
		if f := strings.Fields(l[0]); len(f) >= 3 {
			s.load1, _ = strconv.ParseFloat(f[0], 64)
			s.load5, _ = strconv.ParseFloat(f[1], 64)
			s.load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}
	for _, ln := range sec["CPU"] {
		f := strings.Fields(ln)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] == "cpu" {
			var vals []uint64
			for _, x := range f[1:] {
				v, _ := strconv.ParseUint(x, 10, 64)
				vals = append(vals, v)
			}
			for i, v := range vals {
				if i < 8 {
					s.c.cpuTotal += v
				}
			}
			if len(vals) > 4 {
				s.c.cpuIdle = vals[3] + vals[4]
			} else if len(vals) > 3 {
				s.c.cpuIdle = vals[3]
			}
		} else {
			s.c.ncpu++
		}
	}
	for _, ln := range sec["MEM"] {
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
	for _, ln := range sec["DISK"] {
		f := strings.Fields(ln)
		if len(f) < 10 {
			continue
		}
		name := f[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "zram") || strings.HasPrefix(name, "dm-") ||
			partRe.MatchString(name) {
			continue
		}
		r, _ := strconv.ParseUint(f[5], 10, 64)
		w, _ := strconv.ParseUint(f[9], 10, 64)
		s.c.diskR[name], s.c.diskW[name] = r, w
	}
	for _, ln := range sec["NET"] {
		if !strings.Contains(ln, ":") {
			continue
		}
		parts := strings.SplitN(ln, ":", 2)
		iface := strings.TrimSpace(parts[0])
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
	for _, ln := range sec["DF"] {
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
			Fs: f[0], Mount: f[5],
			TotalKB: total, UsedKB: used, AvailKB: avail,
			UsedPct: 100 * float64(used) / float64(total),
		})
	}
	s.ports, _ = ParsePorts(strings.Join(sec["PORTS"], "\n"))
	return s
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
