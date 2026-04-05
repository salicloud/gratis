package system

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// CPUSample is a snapshot of CPU time counters from /proc/stat.
type CPUSample struct {
	total uint64
	idle  uint64
}

// SampleCPU reads the current CPU counters.
func SampleCPU() (CPUSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return CPUSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// fields: cpu user nice system idle iowait irq softirq steal guest guest_nice
		if len(fields) < 5 {
			return CPUSample{}, fmt.Errorf("unexpected /proc/stat format")
		}
		var total, idle uint64
		for i, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			total += v
			if i == 3 { // idle
				idle = v
			}
		}
		return CPUSample{total: total, idle: idle}, nil
	}
	return CPUSample{}, fmt.Errorf("cpu line not found in /proc/stat")
}

// CPUPercent computes the CPU usage percentage between two samples.
func CPUPercent(a, b CPUSample) float64 {
	totalDelta := b.total - a.total
	idleDelta := b.idle - a.idle
	if totalDelta == 0 {
		return 0
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
}

type MemInfo struct {
	Total uint64
	Used  uint64
}

// ReadMemInfo reads total and used memory from /proc/meminfo.
func ReadMemInfo() (MemInfo, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	defer f.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		values[key] = v * 1024 // /proc/meminfo is in kB
	}

	total := values["MemTotal"]
	free := values["MemFree"] + values["Buffers"] + values["Cached"] + values["SReclaimable"]
	return MemInfo{Total: total, Used: total - free}, nil
}

type LoadAvg struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

// ReadLoadAvg reads system load averages from /proc/loadavg.
func ReadLoadAvg() (LoadAvg, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadAvg{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return LoadAvg{}, fmt.Errorf("unexpected /proc/loadavg format")
	}
	l1, _ := strconv.ParseFloat(fields[0], 64)
	l5, _ := strconv.ParseFloat(fields[1], 64)
	l15, _ := strconv.ParseFloat(fields[2], 64)
	return LoadAvg{Load1: l1, Load5: l5, Load15: l15}, nil
}

type DiskInfo struct {
	Total uint64
	Used  uint64
}

// ReadDiskInfo returns disk usage for the filesystem containing path.
func ReadDiskInfo(path string) (DiskInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskInfo{}, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	return DiskInfo{Total: total, Used: total - free}, nil
}
