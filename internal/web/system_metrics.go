package web

type systemResources struct {
	CPUPercent      float64 `json:"cpu_percent"`
	CPUAvailable    bool    `json:"cpu_available"`
	MemoryUsedBytes uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`
	MemoryPercent   float64 `json:"memory_percent"`
	MemoryAvailable bool    `json:"memory_available"`
}
