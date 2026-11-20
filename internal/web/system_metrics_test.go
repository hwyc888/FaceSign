package web

import "testing"

func TestSystemResourcesReportValidMemoryValues(t *testing.T) {
	resources := readSystemResources()
	if resources.MemoryAvailable {
		if resources.MemoryTotalBytes == 0 || resources.MemoryUsedBytes > resources.MemoryTotalBytes {
			t.Fatalf("invalid memory values: %#v", resources)
		}
		if resources.MemoryPercent < 0 || resources.MemoryPercent > 100 {
			t.Fatalf("memory percent out of range: %#v", resources)
		}
	}
	if resources.CPUAvailable && (resources.CPUPercent < 0 || resources.CPUPercent > 100) {
		t.Fatalf("CPU percent out of range: %#v", resources)
	}
}
