package collect

import (
	"testing"
	"time"
)

const rawFixture = `@@HOST
web1
@@UP
12345.67 23456.78
@@LOAD
0.52 0.40 0.30 1/123 4567
@@CPU
cpu  100 0 100 700 100 0 0 0 0 0
cpu0 50 0 50 350 50 0 0 0 0 0
cpu1 50 0 50 350 50 0 0 0 0 0
@@MEM
MemTotal:        8000000 kB
MemFree:         1000000 kB
MemAvailable:    4000000 kB
SwapTotal:       2000000 kB
SwapFree:        1500000 kB
@@DISK
 259       0 nvme0n1 100 0 2000 50 200 0 4000 100 0 100 150 0 0 0 0
 259       1 nvme0n1p1 100 0 2000 50 200 0 4000 100 0 100 150 0 0 0 0
   7       0 loop0 1 0 8 0 0 0 0 0 0 0 0 0 0 0 0
@@NET
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 100 1 0 0 0 0 0 0 100 1 0 0 0 0 0 0
  eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
@@DF
Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/nvme0n1p1 100000 90000 10000 90% /
tmpfs 4000 0 4000 0% /dev/shm
@@PORTS
Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
tcp   LISTEN 0      128    0.0.0.0:22        0.0.0.0:*         users:(("sshd",pid=700,fd=3))
udp   UNCONN 0      0      0.0.0.0:68        0.0.0.0:*
`

func TestParseSample(t *testing.T) {
	s := parseSample(rawFixture, time.Unix(1000, 0))
	if s.hostname != "web1" {
		t.Errorf("hostname = %q", s.hostname)
	}
	if s.uptime.Round(time.Second) != 12346*time.Second {
		t.Errorf("uptime = %s", s.uptime)
	}
	if s.load1 != 0.52 || s.load15 != 0.30 {
		t.Errorf("load = %v %v %v", s.load1, s.load5, s.load15)
	}
	if s.c.ncpu != 2 {
		t.Errorf("ncpu = %d", s.c.ncpu)
	}
	if s.c.cpuTotal != 1000 {
		t.Errorf("cpuTotal = %d", s.c.cpuTotal)
	}
	if s.c.cpuIdle != 800 {
		t.Errorf("cpuIdle = %d", s.c.cpuIdle)
	}
	if s.memTotal != 8000000 || s.memAvail != 4000000 {
		t.Errorf("mem = %d/%d", s.memAvail, s.memTotal)
	}
	if s.swapTot != 2000000 || s.swapFree != 1500000 {
		t.Errorf("swap = %d/%d", s.swapFree, s.swapTot)
	}
	if len(s.disks) != 1 || s.disks[0].Mount != "/" || s.disks[0].UsedPct != 90 {
		t.Errorf("disks = %+v", s.disks)
	}
	if _, ok := s.c.diskR["nvme0n1"]; !ok {
		t.Error("nvme0n1 отсутствует в diskstats")
	}
	if _, ok := s.c.diskR["nvme0n1p1"]; ok {
		t.Error("партиция nvme0n1p1 не должна учитываться")
	}
	if _, ok := s.c.diskR["loop0"]; ok {
		t.Error("loop0 не должен учитываться")
	}
	if s.c.netRx["eth0"] != 1000 || s.c.netTx["eth0"] != 2000 {
		t.Errorf("net eth0 = %d/%d", s.c.netRx["eth0"], s.c.netTx["eth0"])
	}
	if _, ok := s.c.netRx["lo"]; ok {
		t.Error("lo не должен учитываться")
	}
	if len(s.ports) != 2 {
		t.Fatalf("ports = %+v", s.ports)
	}
	if s.ports[0].Process != "sshd" || s.ports[0].Local != "0.0.0.0:22" {
		t.Errorf("port[0] = %+v", s.ports[0])
	}
	if s.ports[1].Proto != "udp" || s.ports[1].Local != "0.0.0.0:68" {
		t.Errorf("port[1] = %+v", s.ports[1])
	}
}

func TestRates(t *testing.T) {
	t0 := time.Unix(1000, 0)
	prev := &counters{
		at: t0, cpuTotal: 1000, cpuIdle: 800,
		diskR: map[string]uint64{"sda": 1000}, diskW: map[string]uint64{"sda": 2000},
		netRx: map[string]uint64{"eth0": 5000}, netTx: map[string]uint64{"eth0": 6000},
	}
	cur := &counters{
		at: t0.Add(10 * time.Second), cpuTotal: 2000, cpuIdle: 1300,
		diskR: map[string]uint64{"sda": 2024}, diskW: map[string]uint64{"sda": 2000},
		netRx: map[string]uint64{"eth0": 6000}, netTx: map[string]uint64{"eth0": 8000},
	}
	cpu, io, net := rates(prev, cur)
	if cpu != 50 { // (1000-500)/1000
		t.Errorf("cpu = %v", cpu)
	}
	if len(io) != 1 || io[0].ReadBps != 1024*512/10.0 || io[0].WriteBps != 0 {
		t.Errorf("io = %+v", io)
	}
	if len(net) != 1 || net[0].RxBps != 100 || net[0].TxBps != 200 {
		t.Errorf("net = %+v", net)
	}
}
