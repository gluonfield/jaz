package acp

import "testing"

func TestAvailableMemoryUsesReclaimableMemoryAndRequiresAValidReading(t *testing.T) {
	for _, tc := range []struct {
		name string
		info string
		low  bool
	}{
		{"cache is available", "MemTotal: 10000 kB\nMemFree: 1 kB\nMemAvailable: 8000 kB\n", false},
		{"low available", "MemTotal: 10000 kB\nMemAvailable: 999 kB\n", true},
		{"threshold", "MemAvailable: 1000 kB\nMemTotal: 10000 kB\n", false},
		{"exhausted", "MemTotal: 10000 kB\nMemAvailable: 0 kB\n", true},
		{"missing available", "MemTotal: 10000 kB\nMemFree: 0 kB\n", false},
		{"missing total", "MemAvailable: 0 kB\n", false},
		{"invalid", "MemTotal: 10000 kB\nMemAvailable: unknown kB\n", false},
		{"wrong units", "MemTotal: 10000 kB\nMemAvailable: 5 MB\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lowAvailableMemory(tc.info); got != tc.low {
				t.Fatalf("memory pressure = %v, want %v", got, tc.low)
			}
		})
	}
}
