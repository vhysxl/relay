package system

import (
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

var startTime = time.Now()

type SystemMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsedMB  float64 `json:"memory_used_mb"`
	MemoryTotalMB float64 `json:"memory_total_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	GoAllocMB     float64 `json:"go_alloc_mb"`
	GoSysMB       float64 `json:"go_sys_mb"`
	Goroutines    int     `json:"goroutines"`
	NumCPU        int     `json:"num_cpu"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

func GetMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	cpuPercents, err := cpu.Percent(0, false)
	cpuPct := 0.0
	if err == nil && len(cpuPercents) > 0 {
		cpuPct = cpuPercents[0]
	}

	vm, err := mem.VirtualMemory()
	memUsed := 0.0
	memTotal := 0.0
	memPct := 0.0
	if err == nil && vm != nil {
		memUsed = float64(vm.Used) / 1024 / 1024
		memTotal = float64(vm.Total) / 1024 / 1024
		memPct = vm.UsedPercent
	}

	return SystemMetrics{
		CPUPercent:    cpuPct,
		MemoryUsedMB:  memUsed,
		MemoryTotalMB: memTotal,
		MemoryPercent: memPct,
		GoAllocMB:     float64(m.Alloc) / 1024 / 1024,
		GoSysMB:       float64(m.Sys) / 1024 / 1024,
		Goroutines:    runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
	}
}
