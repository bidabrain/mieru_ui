//go:build linux

package service

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func readCPU() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0
	}
	fields := strings.Fields(sc.Text())
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return idle, total
}

func countLines(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := strings.Count(string(raw), "\n")
	if n > 0 {
		n-- // header
	}
	return n
}

func fillPlatform(s *SystemStatus) {
	i1, t1 := readCPU()
	time.Sleep(250 * time.Millisecond)
	i2, t2 := readCPU()
	if t2 > t1 {
		s.CPUUsage = float64((t2-t1)-(i2-i1)) * 100 / float64(t2-t1)
	}

	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		mem := map[string]uint64{}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				n, _ := strconv.ParseUint(f[1], 10, 64)
				mem[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		}
		s.MemTotal = mem["MemTotal"]
		s.MemUsed = mem["MemTotal"] - mem["MemAvailable"]
		s.SwapTotal = mem["SwapTotal"]
		s.SwapUsed = mem["SwapTotal"] - mem["SwapFree"]
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err == nil {
		s.DiskTotal = st.Blocks * uint64(st.Bsize)
		s.DiskUsed = (st.Blocks - st.Bfree) * uint64(st.Bsize)
	}

	if raw, err := os.ReadFile("/proc/uptime"); err == nil {
		f := strings.Fields(string(raw))
		if len(f) > 0 {
			v, _ := strconv.ParseFloat(f[0], 64)
			s.Uptime = uint64(v)
		}
	}
	if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(raw))
		if len(f) >= 3 {
			s.Load = strings.Join(f[:3], " / ")
		}
	}

	if raw, err := os.ReadFile("/proc/net/dev"); err == nil {
		for _, line := range strings.Split(string(raw), "\n")[2:] {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
				continue
			}
			f := strings.Fields(parts[1])
			if len(f) >= 9 {
				rx, _ := strconv.ParseUint(f[0], 10, 64)
				tx, _ := strconv.ParseUint(f[8], 10, 64)
				s.NetRecv += rx
				s.NetSent += tx
			}
		}
	}
	s.TCPConns = countLines("/proc/net/tcp") + countLines("/proc/net/tcp6")
	s.UDPConns = countLines("/proc/net/udp") + countLines("/proc/net/udp6")
}
