package telemetry

import (
	"runtime"
	"time"
)

type Snapshot struct {
	Architecture string    `json:"architecture"`
	OS           string    `json:"os"`
	GoVersion    string    `json:"go_version"`
	CPUs         int       `json:"cpus"`
	Goroutines   int       `json:"goroutines"`
	HeapAlloc    uint64    `json:"heap_alloc"`
	TotalAlloc   uint64    `json:"total_alloc"`
	NumGC        uint32    `json:"num_gc"`
	SystemMemory uint64    `json:"system_memory"`
	At           time.Time `json:"at"`
}

func Capture() Snapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Snapshot{Architecture: runtime.GOARCH, OS: runtime.GOOS, GoVersion: runtime.Version(), CPUs: runtime.NumCPU(), Goroutines: runtime.NumGoroutine(), HeapAlloc: m.HeapAlloc, TotalAlloc: m.TotalAlloc, NumGC: m.NumGC, SystemMemory: m.Sys, At: time.Now().UTC()}
}
