//go:build linux

package web

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var linuxCPUSampler struct {
	sync.Mutex
	total uint64
	idle  uint64
	ready bool
	last  time.Time
}

func readSystemResources() systemResources {
	result := systemResources{}
	if cpu, ok := linuxCPUPercent(); ok {
		result.CPUPercent = cpu
		result.CPUAvailable = true
	}
	if total, available, ok := linuxMemory(); ok {
		result.MemoryTotalBytes = total
		result.MemoryUsedBytes = total - available
		result.MemoryPercent = float64(result.MemoryUsedBytes) * 100 / float64(total)
		result.MemoryAvailable = true
	}
	return result
}

func linuxCPUPercent() (float64, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, false
	}
	var total, idle uint64
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, false
		}
		if i < 8 { // guest and guest_nice are already included in user/nice
			total += value
		}
		if i == 3 || i == 4 { // idle + iowait
			idle += value
		}
	}
	linuxCPUSampler.Lock()
	defer linuxCPUSampler.Unlock()
	now := time.Now()
	if !linuxCPUSampler.ready || now.Sub(linuxCPUSampler.last) > 15*time.Second ||
		total <= linuxCPUSampler.total || idle < linuxCPUSampler.idle {
		linuxCPUSampler.total, linuxCPUSampler.idle, linuxCPUSampler.ready, linuxCPUSampler.last = total, idle, true, now
		return 0, false
	}
	totalDelta := total - linuxCPUSampler.total
	idleDelta := idle - linuxCPUSampler.idle
	linuxCPUSampler.total, linuxCPUSampler.idle, linuxCPUSampler.last = total, idle, now
	if totalDelta == 0 {
		return 0, false
	}
	busy := totalDelta - min(totalDelta, idleDelta)
	return float64(busy) * 100 / float64(totalDelta), true
}

func linuxMemory() (uint64, uint64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var totalKB, availableKB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = value
		case "MemAvailable":
			availableKB = value
		}
	}
	if totalKB == 0 || availableKB > totalKB {
		return 0, 0, false
	}
	return totalKB * 1024, availableKB * 1024, true
}
