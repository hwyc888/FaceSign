//go:build windows

package web

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	metricsKernel32       = syscall.NewLazyDLL("kernel32.dll")
	metricsGetSystemTimes = metricsKernel32.NewProc("GetSystemTimes")
	metricsGlobalMemory   = metricsKernel32.NewProc("GlobalMemoryStatusEx")
)

var windowsCPUSampler struct {
	sync.Mutex
	idle   uint64
	kernel uint64
	user   uint64
	ready  bool
	last   time.Time
}

type windowsMemoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func filetimeValue(value syscall.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

func readSystemResources() systemResources {
	result := systemResources{}
	var idle, kernel, user syscall.Filetime
	ok, _, _ := metricsGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if ok != 0 {
		idleValue, kernelValue, userValue := filetimeValue(idle), filetimeValue(kernel), filetimeValue(user)
		windowsCPUSampler.Lock()
		now := time.Now()
		if windowsCPUSampler.ready && now.Sub(windowsCPUSampler.last) <= 15*time.Second &&
			kernelValue >= windowsCPUSampler.kernel &&
			userValue >= windowsCPUSampler.user &&
			idleValue >= windowsCPUSampler.idle {
			kernelDelta := kernelValue - windowsCPUSampler.kernel
			userDelta := userValue - windowsCPUSampler.user
			idleDelta := idleValue - windowsCPUSampler.idle
			totalDelta := kernelDelta + userDelta
			if totalDelta > 0 {
				busy := totalDelta - min(totalDelta, idleDelta)
				result.CPUPercent = float64(busy) * 100 / float64(totalDelta)
				result.CPUAvailable = true
			}
		}
		windowsCPUSampler.idle = idleValue
		windowsCPUSampler.kernel = kernelValue
		windowsCPUSampler.user = userValue
		windowsCPUSampler.ready = true
		windowsCPUSampler.last = now
		windowsCPUSampler.Unlock()
	}

	memory := windowsMemoryStatusEx{Length: uint32(unsafe.Sizeof(windowsMemoryStatusEx{}))}
	ok, _, _ = metricsGlobalMemory.Call(uintptr(unsafe.Pointer(&memory)))
	if ok != 0 && memory.TotalPhys > 0 && memory.AvailPhys <= memory.TotalPhys {
		result.MemoryTotalBytes = memory.TotalPhys
		result.MemoryUsedBytes = memory.TotalPhys - memory.AvailPhys
		result.MemoryPercent = float64(result.MemoryUsedBytes) * 100 / float64(memory.TotalPhys)
		result.MemoryAvailable = true
	}
	return result
}
